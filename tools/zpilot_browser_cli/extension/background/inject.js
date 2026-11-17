// Calls into ZPilot Browser content modules. Modules are injected on demand: a call first tries the
// already-loaded module and injects its files only when the page does not have it yet.
// Each module registers one global object (e.g. globalThis.ZB_reader) in the isolated world.

import { ToolError, withTimeout } from "./util.js";

export async function callContent(tabId, files, ns, method, args = [], { frameId, timeoutMs = 30000 } = {}) {
  const target = { tabId, ...(frameId !== undefined ? { frameIds: [frameId] } : {}) };
  const invoke = async () => {
    const results = await chrome.scripting.executeScript({ target, func: invokeInPage, args: [ns, method, args] });
    return results?.[0]?.result;
  };
  const timeoutError = () =>
    new ToolError("PAGE_UNRESPONSIVE", `The page did not answer within ${Math.round(timeoutMs / 1000)}s (it may be frozen or showing a dialog).`);

  let reply = await withTimeout(guard(invoke), timeoutMs, timeoutError);
  if (reply?.__zt === "missing") {
    await guard(() => chrome.scripting.executeScript({ target, files }));
    reply = await withTimeout(guard(invoke), timeoutMs, timeoutError);
  }
  if (!reply || typeof reply !== "object" || !("__zt" in reply)) {
    throw new ToolError("PAGE_CHANGED", "The page navigated or reloaded while it was being read. Try again.");
  }
  if (reply.__zt === "missing") throw new ToolError("SCRIPT_ERROR", `Content module ${ns} could not be loaded into the page.`);
  if (reply.__zt === "error") throw new ToolError(reply.code || "PAGE_ERROR", reply.message || "Page script failed.");
  return reply.value;
}

// Runs in the page (isolated world). Must stay self-contained.
function invokeInPage(ns, method, args) {
  const api = globalThis[ns];
  if (!api || typeof api[method] !== "function") return { __zt: "missing" };
  const fail = (e) => ({ __zt: "error", code: e && e.code, message: String((e && e.message) || e) });
  try {
    return Promise.resolve(api[method](...args)).then((value) => ({ __zt: "ok", value }), fail);
  } catch (e) {
    return fail(e);
  }
}

async function guard(fn) {
  try {
    return await fn();
  } catch (e) {
    throw mapScriptingError(e);
  }
}

function mapScriptingError(e) {
  if (e instanceof ToolError) return e;
  const msg = String(e?.message || e);
  if (/Cannot access|cannot be scripted|chrome:\/\/|chrome-extension:\/\/|extensions gallery|Missing host permission|The extensions gallery/i.test(msg)) {
    return new ToolError("PAGE_NOT_SCRIPTABLE", `Extensions are not allowed to access this page (${msg}).`);
  }
  if (/No tab with id|tab was closed|Frame with ID \d+ was removed|No frame with id/i.test(msg)) {
    return new ToolError("TAB_CLOSED", "The tab was closed or its page was replaced.");
  }
  if (/error page/i.test(msg)) {
    return new ToolError("NAVIGATION_FAILED", "The tab is showing a browser error page.");
  }
  return new ToolError("SCRIPT_ERROR", msg);
}
