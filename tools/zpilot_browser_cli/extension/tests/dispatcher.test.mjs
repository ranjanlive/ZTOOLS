import test from "node:test";
import assert from "node:assert/strict";

const noopEvent = { addListener() {}, removeListener() {} };
const storage = { async get() { return {}; }, async set() {} };
globalThis.chrome = {
  runtime: { getManifest: () => ({ version: "2.0.0" }) },
  storage: { local: storage, session: storage },
  action: { async setBadgeText() {}, async setBadgeBackgroundColor() {} },
  debugger: { onEvent: noopEvent, onDetach: noopEvent },
  tabs: { onRemoved: noopEvent },
  downloads: { onDeterminingFilename: noopEvent },
};
const { Dispatcher } = await import("../background/dispatcher.js");
const { TOOL_MAP } = await import("../background/tools/index.js");
const { sleep } = await import("../background/util.js");

function port() {
  const messages = [];
  const disconnect = [];
  return { messages, onMessage: noopEvent, onDisconnect: { addListener: (fn) => disconnect.push(fn) },
    postMessage: (msg) => messages.push(msg), disconnect: () => disconnect.forEach((fn) => fn()) };
}
function tool(name, run) { TOOL_MAP.set(name, { name, inputSchema: { type: "object", properties: {} }, run }); }
function gate() { let release; return { promise: new Promise((r) => { release = r; }), release: () => release() }; }
async function until(condition) {
  const limit = Date.now() + 2000;
  while (!condition()) { assert.ok(Date.now() < limit, "condition timed out"); await sleep(1); }
}

test("same-tool requests are FIFO across clients; different tools run in parallel", async () => {
  const first = gate();
  const started = [];
  let number = 0;
  tool("test_search", async () => { const n = ++number; started.push(`search${n}`); if (n === 1) await first.promise; return { n }; });
  tool("test_read", async () => { started.push("read"); return { read: true }; });
  const dispatcher = new Dispatcher();
  const p1 = port(), p2 = port();
  const c1 = dispatcher.attach(p1, "test"), c2 = dispatcher.attach(p2, "test");
  const a = dispatcher.call(c1, { id: 1, tool: "test_search", args: {} });
  await until(() => started.includes("search1"));
  const b = dispatcher.call(c2, { id: 1, tool: "test_search", args: {} });
  await dispatcher.call(c2, { id: 2, tool: "test_read", args: {} });
  assert.deepEqual(started, ["search1", "read"]);
  first.release();
  await Promise.all([a, b]);
  assert.deepEqual(started, ["search1", "read", "search2"]);
  assert.equal(dispatcher.jobs.size, 0);
});

test("cancelled queued calls never execute and failures do not poison the queue", async () => {
  const first = gate();
  let runs = 0;
  tool("test_cancel", async () => { if (++runs === 1) { await first.promise; throw new Error("first failed"); } return {}; });
  const dispatcher = new Dispatcher(), p = port();
  const client = dispatcher.attach(p, "test");
  const a = dispatcher.call(client, { id: "a", tool: "test_cancel", args: {} });
  await until(() => runs === 1);
  const b = dispatcher.call(client, { id: "b", tool: "test_cancel", args: {} });
  dispatcher.onMessage(client, { type: "cancel", id: "b" });
  await b;
  const c = dispatcher.call(client, { id: "c", tool: "test_cancel", args: {} });
  first.release();
  await Promise.all([a, c]);
  assert.equal(runs, 2);
  assert.equal(p.messages.find((m) => m.type === "result" && m.id === "b").error.code, "CANCELLED");
  assert.equal(p.messages.find((m) => m.type === "result" && m.id === "c").ok, true);
});

test("disconnect cancels running and queued calls; duplicate IDs are rejected", async () => {
  let runs = 0;
  tool("test_disconnect", async (_, ctx) => { runs++; await sleep(5000, ctx.signal); return {}; });
  const dispatcher = new Dispatcher(), p = port();
  const client = dispatcher.attach(p, "test");
  const a = dispatcher.call(client, { id: "a", tool: "test_disconnect", args: {} });
  const b = dispatcher.call(client, { id: "b", tool: "test_disconnect", args: {} });
  await until(() => runs === 1);
  await dispatcher.call(client, { id: "b", tool: "test_disconnect", args: {} });
  assert.equal(p.messages.find((m) => m.type === "result" && m.id === "b").error.code, "DUPLICATE_ID");
  p.disconnect();
  await Promise.all([a, b]);
  assert.equal(runs, 1);
  assert.equal(dispatcher.jobs.size, 0);
});
