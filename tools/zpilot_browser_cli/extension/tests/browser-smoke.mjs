// Run with PLAYWRIGHT_MODULE pointing at Playwright's index.mjs when it is not
// installed locally, and CHROMIUM_PATH pointing at an extension-capable Chromium.
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import { createServer } from "node:http";
import { mkdtemp, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const { chromium } = await import(process.env.PLAYWRIGHT_MODULE ? pathToFileURL(process.env.PLAYWRIGHT_MODULE).href : "playwright");
const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../..");
const platform = `${process.platform === "win32" ? "windows" : process.platform}-${process.arch === "x64" ? "amd64" : "arm64"}`;
const executable = (tool) => path.join(root, tool, "binaries", platform, tool + (process.platform === "win32" ? ".exe" : ""));
const temporary = await mkdtemp(path.join(tmpdir(), "zpilot-smoke-"));
const browsers = [];
const site = createServer((req, res) => {
  res.setHeader("Content-Type", "text/html");
  res.end("<!doctype html><title>ZPilot fixture</title><main><h1>Independent connection works</h1><p>Local smoke test content.</p><input aria-label='Message'><button>Save</button></main>");
});
await new Promise((resolve) => site.listen(0, "127.0.0.1", resolve));
const url = `http://127.0.0.1:${site.address().port}/`;

function command(tool, id, extra = []) {
  return new Promise((resolve, reject) => {
    const child = spawn(executable(tool), ["--extension_id", id, "--auto_launch=false", "--json", ...extra], { windowsHide: true });
    let stdout = "", stderr = "";
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    const timeout = setTimeout(() => { child.kill(); reject(new Error(`${tool} timed out: ${stderr}`)); }, 65000);
    child.on("error", reject);
    child.on("close", (code) => {
      clearTimeout(timeout);
      if (code !== 0) reject(new Error(`${tool} exited ${code}: ${stderr}`));
      else { try { resolve(JSON.parse(stdout)); } catch (error) { reject(error); } }
    });
  });
}

async function profile(name) {
  const extension = path.join(root, "extension");
  const context = await chromium.launchPersistentContext(path.join(temporary, name), {
    headless: true, channel: "chromium", executablePath: process.env.CHROMIUM_PATH || undefined,
    args: [`--disable-extensions-except=${extension}`, `--load-extension=${extension}`],
  });
  browsers.push(context);
  const worker = context.serviceWorkers()[0] || await context.waitForEvent("serviceworker");
  const nativeID = new URL(worker.url()).hostname;
  const popup = await context.newPage();
  await popup.goto(`chrome-extension://${nativeID}/ui/popup.html`);
  await popup.waitForFunction(() => document.querySelector("#extId").textContent.length > 90);
  const id = await popup.locator("#extId").textContent();
  return { context, worker, popup, id };
}

try {
  // No broker is preinstalled or started: the first final tool must start it.
  const first = await profile("one");
  const second = await profile("two");
  assert.notEqual(first.id, second.id, "profiles must have different Connection IDs");
  const read = await command("webpage_read", first.id, ["--url", url]);
  assert.match(JSON.stringify(read), /Independent connection works/);
  console.log("PASS: standalone binary starts embedded bridge and reads a real browser page");
  await command("zagent_in_chrome", first.id, ["--action", "navigate", "--url", url]);
  const tabs1 = await command("zagent_in_chrome", first.id, ["--action", "tabs"]);
  const tabs2 = await command("zagent_in_chrome", second.id, ["--action", "tabs"]);
  assert.match(JSON.stringify(tabs1), /ZPilot fixture/);
  assert.doesNotMatch(JSON.stringify(tabs2), /ZPilot fixture/);
  console.log("PASS: two profiles with the same Chrome extension ID are routed independently");

  // Longer reads provide enough overlap to observe the queue and another tool.
  const slow = command("webpage_read", first.id, ["--url", url, "--wait_ms", "2000"]);
  await first.popup.waitForFunction(async () => (await chrome.storage.session.get("running")).running === 1);
  const queued = command("webpage_read", first.id, ["--url", url]);
  await first.popup.waitForFunction(async () => (await chrome.storage.session.get("queued")).queued >= 1);
  const independent = await command("zagent_in_chrome", first.id, ["--action", "tabs"]);
  assert.match(JSON.stringify(independent), /ZPilot fixture/);
  await Promise.all([slow, queued]);
  console.log("PASS: same-tool requests queue while another tool completes in parallel");

  // Reset during an active request must fail the caller and replace this profile's credential.
  const pending = command("webpage_read", first.id, ["--url", url, "--wait_ms", "20000"]).then(() => "success", () => "cancelled");
  await first.popup.waitForFunction(async () => (await chrome.storage.session.get("running")).running === 1);
  await first.popup.locator("#resetId").click();
  await first.popup.waitForFunction((old) => document.querySelector("#extId").textContent !== old, first.id);
  assert.equal(await pending, "cancelled");
  const updated = await first.popup.locator("#extId").textContent();
  const newRead = await command("webpage_read", updated, ["--url", url]);
  assert.match(JSON.stringify(newRead), /Independent connection works/);
  const oldConfig = path.join(temporary, "old-id.json");
  const { writeFile } = await import("node:fs/promises");
  await writeFile(oldConfig, JSON.stringify({ extension_id: first.id, auto_launch: false, connect_timeout_sec: 1 }));
  await assert.rejects(command("webpage_read", first.id, ["--config", oldConfig, "--url", url]), /extension did not connect/);
  console.log("PASS: Reset ID cancels active work; new ID reconnects successfully");
} finally {
  for (const browser of browsers) await browser.close();
  site.closeAllConnections();
  await new Promise((resolve) => site.close(resolve));
  // Temporary directories are created by this test under the OS temp directory.
  assert.equal(path.dirname(path.resolve(temporary)), path.resolve(tmpdir()));
  assert.ok(path.basename(temporary).startsWith("zpilot-smoke-"));
  await rm(temporary, { recursive: true, force: true });
}
