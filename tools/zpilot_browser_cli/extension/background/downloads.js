// Downloads through Chrome itself, so they use the profile's logins, cookies and redirects exactly
// like a person's download. Files land in Chrome's download folder under "ZPilot/"; the agent-side
// tool then moves them to their final place in the workspace.
//
//   downloadUrl(url)            starts a download of a URL and waits until it is complete
//   expectDownload(trigger)     runs trigger() (e.g. a click on a page's Download button) and
//                               captures the download it starts

import { ToolError, abortReason } from "./util.js";

const SUBFOLDER = "ZPilot";
let armed = null; // { since, take(item) } while expectDownload waits for a page-started download
let queue = Promise.resolve();

// Serialize expectDownload calls: each one owns "the next download".
function serialized(fn) {
  const run = queue.catch(() => {}).then(fn);
  queue = run.catch(() => {});
  return run;
}

function safeName(name) {
  const base = String(name || "download").split(/[\\/]/).pop();
  return base.replace(/[<>:"|?*\u0000-\u001f]/g, "_").replace(/^\.+/, "").slice(0, 150) || "download";
}

// Puts downloads started while armed into the ZPilot subfolder; other downloads are untouched.
chrome.downloads.onDeterminingFilename.addListener((item, suggest) => {
  if (armed && !armed.item && new Date(item.startTime).getTime() >= armed.since - 1000) {
    armed.item = item;
    suggest({ filename: `${SUBFOLDER}/${safeName(item.filename)}`, conflictAction: "uniquify" });
    armed.take(item);
    return;
  }
  suggest();
});

// Waits until download `id` is complete (or fails) and returns its details.
function waitComplete(id, { signal, timeoutMs }) {
  return new Promise((resolve, reject) => {
    let timer = null;
    const finish = (err, value) => {
      clearTimeout(timer);
      chrome.downloads.onChanged.removeListener(onChanged);
      signal?.removeEventListener("abort", onAbort);
      if (err) reject(err);
      else resolve(value);
    };
    const check = async () => {
      const [d] = await chrome.downloads.search({ id });
      if (!d) return finish(new ToolError("DOWNLOAD_FAILED", "The download disappeared from Chrome's download list."));
      if (d.state === "complete") {
        return finish(null, { path: d.filename, url: d.finalUrl || d.url, mime: d.mime || "", bytes: d.fileSize || d.bytesReceived || 0, filename: d.filename.split(/[\\/]/).pop(), id });
      }
      if (d.state === "interrupted") {
        return finish(new ToolError("DOWNLOAD_FAILED", `The download was interrupted (${d.error || "unknown reason"}).`, { url: d.url }));
      }
      if (d.danger && !["safe", "accepted"].includes(d.danger)) {
        return finish(new ToolError("DOWNLOAD_BLOCKED", `Chrome flagged the download as ${d.danger}; keep it manually in chrome://downloads if you trust it.`, { url: d.url }));
      }
    };
    const onChanged = (delta) => {
      if (delta.id === id) check().catch((e) => finish(e));
    };
    const onAbort = () => {
      chrome.downloads.cancel(id).catch(() => {});
      finish(abortReason(signal));
    };
    chrome.downloads.onChanged.addListener(onChanged);
    signal?.addEventListener("abort", onAbort, { once: true });
    timer = setTimeout(() => {
      chrome.downloads.cancel(id).catch(() => {});
      finish(new ToolError("TIMEOUT", `The download did not finish within ${Math.round(timeoutMs / 1000)}s.`));
    }, timeoutMs);
    check().catch((e) => finish(e));
  });
}

export async function downloadUrl(url, { filename, signal, timeoutMs = 300000 } = {}) {
  let id;
  try {
    id = await chrome.downloads.download({
      url,
      saveAs: false,
      conflictAction: "uniquify",
      ...(filename ? { filename: `${SUBFOLDER}/${safeName(filename)}` } : {}),
    });
  } catch (e) {
    throw new ToolError("DOWNLOAD_FAILED", `Chrome refused to download ${url}: ${e?.message || e}`);
  }
  return waitComplete(id, { signal, timeoutMs });
}

// Arms a watcher, runs trigger(), and returns the first download the page starts.
// Chrome blocks a site's second and later downloads ("multiple automatic downloads") unless the
// site may download automatically. Allows it for one site (e.g. "https://gemini.google.com/*").
export async function allowAutomaticDownloads(pattern) {
  try {
    const url = pattern.replace(/\*$/, "");
    const { setting } = await chrome.contentSettings.automaticDownloads.get({ primaryUrl: url });
    if (setting !== "allow") await chrome.contentSettings.automaticDownloads.set({ primaryPattern: pattern, setting: "allow" });
    return true;
  } catch {
    return false;
  }
}

export function expectDownload(trigger, { signal, startTimeoutMs = 20000, timeoutMs = 300000 } = {}) {
  return serialized(async () => {
    let take;
    const started = new Promise((resolve) => (take = resolve));
    armed = { since: Date.now(), item: null, take };
    try {
      await trigger();
      const item = await Promise.race([
        started,
        new Promise((_, reject) =>
          setTimeout(() => reject(new ToolError("NO_DOWNLOAD", `No download started within ${Math.round(startTimeoutMs / 1000)}s after the action.`)), startTimeoutMs)
        ),
        new Promise((_, reject) => signal?.addEventListener("abort", () => reject(abortReason(signal)), { once: true })),
      ]);
      armed = null;
      return await waitComplete(item.id, { signal, timeoutMs });
    } finally {
      armed = null;
    }
  });
}

// Recent downloads (for the agent's "downloads" action).
export async function recentDownloads(limit = 20) {
  const items = await chrome.downloads.search({ orderBy: ["-startTime"], limit });
  return items.map((d) => ({
    id: d.id,
    filename: d.filename,
    url: d.finalUrl || d.url,
    state: d.state,
    bytes: d.fileSize || d.bytesReceived || 0,
    mime: d.mime || "",
    started: d.startTime,
    ...(d.error ? { error: d.error } : {}),
  }));
}
