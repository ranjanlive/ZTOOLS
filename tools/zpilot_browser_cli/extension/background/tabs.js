// Tab and tab-group management: every tab ZPilot Browser opens lives in one of its own groups,
// and navigations are tracked through webNavigation so load failures are reported precisely.

import { TAB_GROUPS } from "./config.js";
import { ToolError, abortReason } from "./util.js";

const groupLocks = new Map();

export async function findGroup(kind) {
  const { title } = TAB_GROUPS[kind];
  const groups = await chrome.tabGroups.query({});
  return groups.find((g) => g.title === title) || null;
}

export async function groupTabs(kind) {
  const { title } = TAB_GROUPS[kind];
  const groups = (await chrome.tabGroups.query({})).filter((g) => g.title === title);
  const tabs = [];
  for (const g of groups) tabs.push(...(await chrome.tabs.query({ groupId: g.id })));
  return tabs;
}

async function targetWindowId() {
  try {
    const win = await chrome.windows.getLastFocused({ windowTypes: ["normal"] });
    if (win?.type === "normal") return win.id;
  } catch {}
  const wins = await chrome.windows.getAll({ windowTypes: ["normal"] });
  if (wins.length) return wins[0].id;
  const win = await chrome.windows.create({ focused: false });
  return win.id;
}

// Adds a tab to the ZPilot Browser group of the given kind in the tab's window, creating the group if needed.
// Serialized per kind so parallel jobs do not create duplicate groups.
export function addToGroup(tabId, kind) {
  const previous = groupLocks.get(kind) || Promise.resolve();
  const next = previous
    .catch(() => {})
    .then(async () => {
      const { title, color } = TAB_GROUPS[kind];
      const tab = await chrome.tabs.get(tabId);
      const existing = (await chrome.tabGroups.query({ windowId: tab.windowId })).find((g) => g.title === title);
      if (existing) {
        await chrome.tabs.group({ tabIds: [tabId], groupId: existing.id });
        return existing.id;
      }
      const groupId = await chrome.tabs.group({ tabIds: [tabId], createProperties: { windowId: tab.windowId } });
      await chrome.tabGroups.update(groupId, { title, color, collapsed: false });
      return groupId;
    });
  groupLocks.set(kind, next);
  return next;
}

// Opens a blank tab inside a ZPilot Browser group (in the window that already holds the group, if any).
// Callers navigate it themselves so they can close it again when loading fails.
export async function openTab({ kind = "tools", active = false } = {}) {
  const group = await findGroup(kind);
  const windowId = group ? group.windowId : await targetWindowId();
  const tab = await chrome.tabs.create({ windowId, url: "about:blank", active });
  chrome.tabs.update(tab.id, { autoDiscardable: false }).catch(() => {});
  await addToGroup(tab.id, kind).catch(() => {});
  return tab.id;
}

export async function closeTab(tabId) {
  if (tabId == null) return;
  try {
    await chrome.tabs.remove(tabId);
  } catch {}
}

export async function focusTab(tabId) {
  const tab = await chrome.tabs.update(tabId, { active: true });
  try {
    await chrome.windows.update(tab.windowId, { focused: true });
  } catch {}
}

export async function tabExists(tabId) {
  if (tabId == null) return null;
  try {
    return await chrome.tabs.get(tabId);
  } catch {
    return null;
  }
}

// Runs `trigger` (which starts a navigation) and waits until the new main-frame document has loaded.
// Resolves { url, timedOut } — a slow page is still usable, so a timeout is not an error.
// Rejects with NAVIGATION_FAILED when the load itself fails (DNS, TLS, refused...).
export function watchNavigation(tabId, trigger, { timeoutMs = 30000, signal, label = "" } = {}) {
  if (signal?.aborted) return Promise.reject(abortReason(signal));
  return new Promise((resolve, reject) => {
    let committed = false;
    let done = false;
    let abortTimer = null;
    let bfcacheTimer = null;

    const finish = (err, value) => {
      if (done) return;
      done = true;
      clearTimeout(timer);
      clearTimeout(abortTimer);
      clearTimeout(bfcacheTimer);
      chrome.webNavigation.onCommitted.removeListener(onCommitted);
      chrome.webNavigation.onCompleted.removeListener(onCompleted);
      chrome.webNavigation.onErrorOccurred.removeListener(onError);
      chrome.tabs.onRemoved.removeListener(onRemoved);
      signal?.removeEventListener("abort", onAbort);
      if (err) reject(err);
      else resolve(value);
    };

    // A new tab's own about:blank load can report after the real navigation started; ignore it.
    const isBlank = (d) => d.url === "about:blank";
    const onCommitted = (d) => {
      if (d.tabId !== tabId || d.frameId !== 0 || isBlank(d)) return;
      committed = true;
      clearTimeout(abortTimer);
      // A back/forward restore from the bfcache may not fire onCompleted.
      if (d.transitionQualifiers?.includes("forward_back")) {
        clearTimeout(bfcacheTimer);
        bfcacheTimer = setTimeout(() => finish(null, { url: d.url, timedOut: false }), 1500);
      }
    };
    const onCompleted = (d) => {
      if (d.tabId === tabId && d.frameId === 0 && committed && !isBlank(d)) finish(null, { url: d.url, timedOut: false });
    };
    const onError = (d) => {
      if (d.tabId !== tabId || d.frameId !== 0 || isBlank(d)) return;
      if (d.error === "net::ERR_ABORTED") {
        // Either replaced by another navigation (fine) or the URL was a download.
        clearTimeout(abortTimer);
        abortTimer = setTimeout(() => {
          if (!committed) {
            finish(new ToolError("NAVIGATION_ABORTED", `Loading was aborted${label ? ` for ${label}` : ""} (the URL may be a file download).`));
          }
        }, 2500);
        return;
      }
      const code = d.error === "net::ERR_HTTP_RESPONSE_CODE_FAILURE" ? "HTTP_ERROR" : "NAVIGATION_FAILED";
      finish(new ToolError(code, describeNetError(d.error, label || d.url), { error: d.error }));
    };
    const onRemoved = (id) => {
      if (id === tabId) finish(new ToolError("TAB_CLOSED", "The tab was closed while loading."));
    };
    const onAbort = () => finish(abortReason(signal));

    const timer = setTimeout(() => finish(null, { url: label, timedOut: true }), timeoutMs);
    chrome.webNavigation.onCommitted.addListener(onCommitted);
    chrome.webNavigation.onCompleted.addListener(onCompleted);
    chrome.webNavigation.onErrorOccurred.addListener(onError);
    chrome.tabs.onRemoved.addListener(onRemoved);
    signal?.addEventListener("abort", onAbort, { once: true });

    Promise.resolve()
      .then(() => {
        if (signal?.aborted) throw abortReason(signal);
        return trigger();
      })
      .catch((e) => finish(new ToolError("TAB_CLOSED", `Could not navigate the tab: ${e?.message || e}`)));
  });
}

export function navigate(tabId, url, options = {}) {
  return watchNavigation(tabId, () => chrome.tabs.update(tabId, { url }), { label: url, ...options });
}

export function describeNetError(code, url) {
  const reasons = {
    "net::ERR_NAME_NOT_RESOLVED": "domain not found (DNS lookup failed)",
    "net::ERR_NAME_RESOLUTION_FAILED": "domain not found (DNS lookup failed)",
    "net::ERR_ADDRESS_UNREACHABLE": "host unreachable",
    "net::ERR_CONNECTION_REFUSED": "connection refused",
    "net::ERR_CONNECTION_TIMED_OUT": "connection timed out",
    "net::ERR_TIMED_OUT": "connection timed out",
    "net::ERR_CONNECTION_RESET": "connection reset by the server",
    "net::ERR_CONNECTION_CLOSED": "connection closed by the server",
    "net::ERR_EMPTY_RESPONSE": "the server sent an empty response",
    "net::ERR_INTERNET_DISCONNECTED": "no internet connection",
    "net::ERR_SSL_PROTOCOL_ERROR": "TLS/SSL error",
    "net::ERR_TOO_MANY_REDIRECTS": "too many redirects",
    "net::ERR_BLOCKED_BY_CLIENT": "blocked by the browser or another extension",
    "net::ERR_BLOCKED_BY_ADMINISTRATOR": "blocked by a browser policy",
    "net::ERR_HTTP_RESPONSE_CODE_FAILURE": "the server answered with an HTTP error status and an empty page (often 404, 403 or 429)",
  };
  let reason = reasons[code];
  if (!reason && code.startsWith("net::ERR_CERT_")) reason = "invalid TLS certificate";
  return `Could not load ${url}: ${reason || "navigation failed"} (${code}).`;
}

// Runs fn with tabId as the active tab of its window (pages that rely on animation frames, such as
// Gemini's editor and menus, ignore input in background tabs), then restores the previous tab.
export async function withTabVisible(tabId, fn) {
  let previous = null;
  try {
    const tab = await chrome.tabs.get(tabId);
    if (!tab.active) {
      [previous] = await chrome.tabs.query({ active: true, windowId: tab.windowId });
      await chrome.tabs.update(tabId, { active: true });
      await new Promise((r) => setTimeout(r, 300));
    }
  } catch {}
  try {
    return await fn();
  } finally {
    if (previous && previous.id !== tabId) chrome.tabs.update(previous.id, { active: true }).catch(() => {});
  }
}
