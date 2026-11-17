// zagent_in_chrome: lets an agent operate Chrome like a person — navigate, read the page, click,
// type, scroll, take screenshots — in its own "ZPilot Agent" tab group that keeps working in the
// background. Input goes through CDP (trusted events); page reading goes through content scripts.

import { CONTENT } from "../config.js";
import * as cdp from "../cdp.js";
import { callContent } from "../inject.js";
import { addToGroup, closeTab, focusTab, groupTabs, navigate, openTab, tabExists, watchNavigation } from "../tabs.js";
import { downloadUrl, expectDownload, recentDownloads } from "../downloads.js";
import { ToolError, base64ToBlob, blobToBase64, normalizeUrl, pollUntil, sleep, sliceChars, truncate, throwIfAborted } from "../util.js";

const MAX_SHOT_WIDTH = 1280;
const IDLE_DETACH_MS = 10 * 60 * 1000;

const ACTIONS = [
  "tabs", "new_tab", "switch_tab", "close_tab", "attach_tab",
  "navigate", "screenshot", "zoom", "read_page", "find", "get_text",
  "click", "double_click", "triple_click", "right_click", "hover", "drag",
  "type", "key", "fill", "scroll", "scroll_to",
  "wait", "wait_for", "javascript", "console", "network", "upload", "download", "downloads", "save_page",
  "handle_dialog", "resize", "end",
];

let currentTabId = null;
let isMac = null;
const locks = new Map();
const idleTimers = new Map();

const page = (tabId, method, args = [], opts) => callContent(tabId, CONTENT.agent, "ZB_agent", method, args, opts);

export default {
  name: "zagent_in_chrome",
  title: "Agent in Chrome",
  description: [
    "Operates the user's Chrome like a person, in its own \"ZPilot Agent\" tab group that works in the background with the user's logins.",
    "Typical loop: navigate → read_page (or screenshot) → click/type/fill using refs → read_page again to check the result.",
    "Elements: read_page lists interactive elements as [ref] role \"name\" (filter=\"all\" adds headings and text); find searches them by text.",
    "Pass ref to target an element (most reliable), or coordinate [x, y] in pixels of the latest screenshot.",
    "Actions — tabs: tabs, new_tab(url?, foreground?), switch_tab(tab_id), close_tab(tab_id?), attach_tab(tab_id? — adds a user tab, default the active one).",
    "Page: navigate(url | \"back\" | \"forward\" | \"reload\"), screenshot, zoom(region [x0,y0,x1,y1]), read_page(filter?, viewport_only?, ref?, max_chars?), find(text, limit?), get_text(max_chars?, offset?).",
    "Input: click / double_click / triple_click / right_click / hover (ref | coordinate, modifiers?), drag(ref|coordinate → to_ref|end_coordinate),",
    "type(text, ref?, clear?, submit?), key(keys e.g. \"Enter\", \"ctrl+a Backspace\", repeat?), fill(ref, value — inputs, selects, checkboxes), scroll(direction?, amount?, ref|coordinate?), scroll_to(ref).",
    "Other: wait(seconds), wait_for(selector | text, seconds?), javascript(script — runs in the page, returns the last value), console(pattern?, limit?, clear?), network(pattern?, limit?, clear?),",
    "Files: upload(paths, ref? — a file input, or a button/drop zone that opens a file chooser; without ref the page's only file input),",
    "download(url | ref of a link/image/media, filename?) — downloads through Chrome with the user's session; click(..., expect_download=true) captures a download the click starts;",
    "downloads (recent downloads); save_page(format pdf | mhtml | png — full page) returns the file data.",
    "handle_dialog(accept?, text?), resize(width, height; 0,0 resets), end(close_tabs?).",
    "Alerts are accepted automatically; confirm/prompt dialogs must be answered with handle_dialog.",
  ].join(" "),
  defaultTimeoutSec: 60,
  inputSchema: {
    type: "object",
    properties: {
      action: { type: "string", enum: ACTIONS, description: "What to do." },
      tab_id: { type: "integer", description: "Tab to act on (from \"tabs\"). Defaults to the current agent tab." },
      url: { type: "string", description: "navigate / new_tab: URL to open; navigate also accepts back, forward, reload." },
      ref: { type: "string", description: "Element ref from read_page or find, e.g. \"kqm12\"." },
      coordinate: { type: "array", items: { type: "number" }, minItems: 2, maxItems: 2, description: "[x, y] in pixels of the latest screenshot." },
      to_ref: { type: "string", description: "drag: target element ref." },
      end_coordinate: { type: "array", items: { type: "number" }, minItems: 2, maxItems: 2, description: "drag: target [x, y]." },
      region: { type: "array", items: { type: "number" }, minItems: 4, maxItems: 4, description: "zoom: [x0, y0, x1, y1] in screenshot pixels." },
      text: { type: "string", description: "type: text to type. find: what to look for. wait_for: text to wait for. handle_dialog: prompt answer." },
      keys: { type: "string", description: "key: space-separated keys or combos, e.g. \"Enter\", \"ctrl+a Backspace\", \"shift+Tab\"." },
      value: { type: "string", description: "fill: value to set (option text/value for selects, true/false for checkboxes)." },
      modifiers: { type: "string", description: "click/hover: modifier keys held, e.g. \"ctrl\" or \"shift+alt\"." },
      direction: { type: "string", enum: ["up", "down", "left", "right"], default: "down", description: "scroll direction." },
      amount: { type: "integer", minimum: 1, maximum: 50, default: 5, description: "scroll: distance in steps of 100 px." },
      filter: { type: "string", enum: ["interactive", "all"], default: "interactive", description: "read_page: only interactive elements, or also headings, text and images." },
      viewport_only: { type: "boolean", default: false, description: "read_page: only elements currently visible in the viewport." },
      max_chars: { type: "integer", minimum: 100, maximum: 200000, description: "read_page / get_text: output limit (defaults 25000 / 30000)." },
      offset: { type: "integer", minimum: 0, default: 0, description: "get_text: character offset to continue from." },
      limit: { type: "integer", minimum: 1, maximum: 500, description: "find / console / network: maximum entries." },
      selector: { type: "string", description: "wait_for: CSS selector to wait for. upload: CSS selector of a (hidden) file input." },
      seconds: { type: "number", minimum: 0, maximum: 60, description: "wait: how long (default 1). wait_for: maximum wait (default 10)." },
      script: { type: "string", description: "javascript: code evaluated in the page; the value of the last expression is returned (await allowed)." },
      pattern: { type: "string", description: "console / network: regular expression (or plain text) to filter entries." },
      clear: { type: "boolean", default: false, description: "type: clear the field first. console / network: clear the buffer after reading." },
      submit: { type: "boolean", default: false, description: "type: press Enter after typing." },
      paths: { type: "array", items: { type: "string" }, minItems: 1, description: "upload: absolute file paths on this computer." },
      filename: { type: "string", description: "download: file name to save as (default: the server's name)." },
      expect_download: { type: "boolean", default: false, description: "click/double_click: wait for and return the download the click starts." },
      format: { type: "string", enum: ["pdf", "mhtml", "png"], default: "pdf", description: "save_page: PDF (printed), MHTML (complete page archive) or PNG (full-page screenshot)." },
      accept: { type: "boolean", default: true, description: "handle_dialog: accept (OK) or dismiss (Cancel)." },
      width: { type: "integer", minimum: 0, maximum: 4000, description: "resize: viewport width in CSS pixels (0 resets)." },
      height: { type: "integer", minimum: 0, maximum: 4000, description: "resize: viewport height in CSS pixels (0 resets)." },
      foreground: { type: "boolean", default: false, description: "new_tab / switch_tab: also bring the tab to the front." },
      close_tabs: { type: "boolean", default: false, description: "end: also close the agent tabs." },
      repeat: { type: "integer", minimum: 1, maximum: 50, default: 1, description: "key: press the keys this many times." },
      timeout_sec: { type: "integer", minimum: 5, maximum: 300, default: 60 },
    },
    required: ["action"],
  },

  label: (args) => `${args.action}${args.url ? " " + truncate(args.url, 60) : args.ref ? " " + args.ref : args.text ? " " + truncate(args.text, 40) : ""}`,
  summarize: (args, result) => truncate(result.message || "", 160),

  async run(args, ctx) {
    const handler = HANDLERS[args.action];
    if (args.action === "tabs" || args.action === "end" || args.action === "attach_tab" || args.action === "new_tab" || args.action === "downloads") {
      return handler(args, ctx);
    }
    const tabId = await resolveTab(args, { allowCreate: args.action === "navigate" });
    return withLock(tabId, async () => {
      throwIfAborted(ctx.signal);
      const session = cdp.getSession(tabId);
      if (session?.dialog && args.action !== "handle_dialog") {
        throw new ToolError("DIALOG_OPEN", `A ${session.dialog.type} dialog is open: "${truncate(session.dialog.message, 300)}". Answer it with handle_dialog first.`, session.dialog);
      }
      touch(tabId);
      // Navigation adds its overlay after the new document loads. Injecting into
      // the outgoing/about:blank document can race the navigation itself.
      if (!["close_tab", "handle_dialog", "navigate"].includes(args.action)) page(tabId, "showOverlay").catch(() => {});
      const result = await handler(args, ctx, tabId);
      return { action: args.action, tab_id: tabId, ...result };
    });
  },
};

// ---------- Tabs ----------

async function setCurrent(tabId) {
  currentTabId = tabId;
  await chrome.storage.session.set({ agentTab: tabId }).catch(() => {});
}

async function resolveTab(args, { allowCreate = false } = {}) {
  const tabs = await groupTabs("agent");
  const ids = new Set(tabs.map((t) => t.id));
  if (args.tab_id != null) {
    if (!ids.has(args.tab_id)) {
      throw new ToolError("TAB_NOT_IN_GROUP", `Tab ${args.tab_id} is not in the ZPilot Agent group. Use "tabs" to list agent tabs or "attach_tab" to add a tab.`);
    }
    await setCurrent(args.tab_id);
    return args.tab_id;
  }
  if (currentTabId == null) currentTabId = (await chrome.storage.session.get("agentTab")).agentTab ?? null;
  if (currentTabId != null && ids.has(currentTabId)) return currentTabId;
  if (tabs.length) {
    const pick = tabs.find((t) => t.active) || tabs[tabs.length - 1];
    await setCurrent(pick.id);
    return pick.id;
  }
  if (allowCreate) {
    const id = await openTab({ kind: "agent", active: false });
    await setCurrent(id);
    return id;
  }
  throw new ToolError("NO_AGENT_TAB", 'No ZPilot Agent tab is open. Start with "navigate" (with a url) or "new_tab".');
}

function withLock(tabId, fn) {
  const previous = locks.get(tabId) || Promise.resolve();
  const next = previous.catch(() => {}).then(fn);
  locks.set(tabId, next.catch(() => {}));
  return next;
}

// Detaches the debugger (and its "is debugging this browser" banner) after a quiet period.
function touch(tabId) {
  clearTimeout(idleTimers.get(tabId));
  idleTimers.set(
    tabId,
    setTimeout(() => {
      idleTimers.delete(tabId);
      page(tabId, "removeOverlay").catch(() => {});
      cdp.detach(tabId);
    }, IDLE_DETACH_MS)
  );
}

async function tabSummary(tabId) {
  const tab = await tabExists(tabId);
  return tab ? { url: tab.url || tab.pendingUrl || "", title: tab.title || "" } : { closed: true };
}

export async function endAgentSession({ closeTabs = false } = {}) {
  const tabs = await groupTabs("agent");
  for (const t of tabs) {
    clearTimeout(idleTimers.get(t.id));
    idleTimers.delete(t.id);
    await page(t.id, "removeOverlay").catch(() => {});
    await cdp.detach(t.id);
    if (closeTabs) await closeTab(t.id);
  }
  currentTabId = null;
  await chrome.storage.session.remove("agentTab").catch(() => {});
  return tabs.length;
}

async function platformIsMac() {
  if (isMac == null) isMac = (await chrome.runtime.getPlatformInfo()).os === "mac";
  return isMac;
}

// ---------- Helpers ----------

async function showCursor(ctx, tabId, x, y, click) {
  if (!ctx.settings.showCursor) return;
  try {
    await page(tabId, click ? "cursorClick" : "cursorMove", [x, y], { timeoutMs: 3000 });
    await sleep(170, ctx.signal);
  } catch {}
}

// Turns ref / coordinate arguments into a point in CSS pixels.
async function pointFrom(tabId, args, refKey = "ref", coordKey = "coordinate") {
  if (args[refKey]) {
    const loc = await page(tabId, "locate", [args[refKey]]);
    return { x: loc.x, y: loc.y, target: loc.description, occludedBy: loc.occludedBy };
  }
  if (args[coordKey]) {
    const k = cdp.getSession(tabId)?.shot?.cssPerPx || 1;
    const x = args[coordKey][0] * k;
    const y = args[coordKey][1] * k;
    const at = await page(tabId, "elementAt", [x, y]).catch(() => null);
    if (at?.outside) {
      throw new ToolError("INVALID_ARGS", `${coordKey} [${args[coordKey].join(", ")}] is outside the visible page (${Math.round(at.width / k)}x${Math.round(at.height / k)} in screenshot pixels). Scroll first or take a new screenshot.`);
    }
    return { x, y, target: at?.description || "", targetRef: at?.ref };
  }
  throw new ToolError("INVALID_ARGS", `This action needs "${refKey}" or "${coordKey}".`);
}

// Watches for a navigation or a new tab caused by an input action and reports it.
async function afterInput(tabId, beforeUrl, popups, signal) {
  await sleep(250, signal);
  let tab = await tabExists(tabId);
  if (!tab) return { note: "The tab closed." };
  if (tab.status === "loading") {
    await pollUntil(async () => (await tabExists(tabId))?.status !== "loading", { timeoutMs: 10000, intervalMs: 250, signal });
    tab = (await tabExists(tabId)) || tab;
  }
  const notes = [];
  if (tab.url !== beforeUrl) notes.push(`Page navigated to ${tab.url}`);
  for (const id of popups) {
    const t = await tabExists(id);
    if (!t) continue;
    await addToGroup(id, "agent").catch(() => {});
    await setCurrent(id);
    notes.push(`A new tab ${id} opened (${t.pendingUrl || t.url}) and is now the current agent tab`);
  }
  return { note: notes.join(". "), url: tab.url, title: tab.title };
}

function watchPopups(tabId) {
  const created = [];
  const listener = (tab) => {
    if (tab.openerTabId === tabId) created.push(tab.id);
  };
  chrome.tabs.onCreated.addListener(listener);
  return { created, stop: () => chrome.tabs.onCreated.removeListener(listener) };
}

function dialogNote(dialog) {
  if (!dialog) return "";
  return dialog.handled
    ? ` A ${dialog.type} dialog appeared ("${truncate(dialog.message, 200)}") and was ${dialog.handled}.`
    : ` A ${dialog.type} dialog is now open ("${truncate(dialog.message, 200)}"); answer it with handle_dialog.`;
}

async function capture(tabId) {
  const info = await page(tabId, "pageInfo");
  await page(tabId, "overlayVisible", [false], { timeoutMs: 3000 }).catch(() => {});
  let shot;
  try {
    shot = await cdp.send(tabId, "Page.captureScreenshot", { format: "png", fromSurface: true }, 5000).catch(async (e) => {
      if (e.code !== "CDP_TIMEOUT") throw e;
      // Background tabs sometimes produce no frame; show the tab briefly, then restore the user's tab.
      const tab = await chrome.tabs.get(tabId);
      const [previous] = await chrome.tabs.query({ active: true, windowId: tab.windowId });
      await chrome.tabs.update(tabId, { active: true });
      await sleep(250);
      try {
        return await cdp.send(tabId, "Page.captureScreenshot", { format: "png", fromSurface: true }, 10000);
      } catch {
        throw new ToolError("SCREENSHOT_FAILED", "Chrome did not render the tab (is the window minimized?). Restore the browser window and retry.");
      } finally {
        if (previous && previous.id !== tabId) chrome.tabs.update(previous.id, { active: true }).catch(() => {});
      }
    });
  } finally {
    page(tabId, "overlayVisible", [true], { timeoutMs: 3000 }).catch(() => {});
  }
  const bitmap = await createImageBitmap(base64ToBlob(shot.data, "image/png"));
  return { bitmap, info };
}

async function encode(canvas) {
  const blob = await canvas.convertToBlob({ type: "image/jpeg", quality: 0.8 });
  return blobToBase64(blob);
}

// ---------- Action handlers ----------

const HANDLERS = {
  async tabs() {
    const tabs = await groupTabs("agent");
    if (currentTabId == null) currentTabId = (await chrome.storage.session.get("agentTab")).agentTab ?? null;
    const list = tabs.map((t) => ({ tab_id: t.id, title: t.title, url: t.url || t.pendingUrl, current: t.id === currentTabId }));
    return {
      action: "tabs",
      tabs: list,
      message: list.length ? list.map((t) => `${t.current ? "* " : "  "}${t.tab_id}: ${truncate(t.title || "", 60)} — ${t.url}`).join("\n") : "No agent tabs are open.",
    };
  },

  async new_tab(args, ctx) {
    const id = await openTab({ kind: "agent", active: args.foreground });
    await setCurrent(id);
    let note = "";
    if (args.url) {
      const nav = await navigate(id, normalizeUrl(args.url), { timeoutMs: 30000, signal: ctx.signal });
      if (nav.timedOut) note = " (still loading)";
    }
    if (args.foreground) await focusTab(id);
    const s = await tabSummary(id);
    return { action: "new_tab", tab_id: id, ...s, message: `Opened tab ${id}${args.url ? `: ${s.title || s.url}${note}` : ""}. It is now the current agent tab.` };
  },

  async switch_tab(args, ctx, tabId) {
    if (args.foreground) await focusTab(tabId);
    const s = await tabSummary(tabId);
    return { ...s, message: `Current agent tab is now ${tabId}: ${s.title} — ${s.url}` };
  },

  async close_tab(args, ctx, tabId) {
    await cdp.detach(tabId);
    await closeTab(tabId);
    currentTabId = null;
    const rest = await groupTabs("agent");
    if (rest.length) await setCurrent(rest[rest.length - 1].id);
    return { message: `Closed tab ${tabId}.${rest.length ? ` Current agent tab is now ${currentTabId}.` : " No agent tabs remain."}` };
  },

  async attach_tab(args) {
    let tab;
    if (args.tab_id != null) {
      tab = await tabExists(args.tab_id);
      if (!tab) throw new ToolError("TAB_NOT_FOUND", `Tab ${args.tab_id} does not exist.`);
    } else {
      [tab] = await chrome.tabs.query({ active: true, lastFocusedWindow: true });
      if (!tab) throw new ToolError("TAB_NOT_FOUND", "There is no active tab to attach.");
    }
    if (/^(chrome|edge|about|chrome-extension|devtools):/i.test(tab.url || "")) {
      throw new ToolError("PAGE_NOT_SCRIPTABLE", `Browser-internal pages (${tab.url}) cannot be controlled.`);
    }
    await addToGroup(tab.id, "agent");
    await setCurrent(tab.id);
    return { action: "attach_tab", tab_id: tab.id, url: tab.url, title: tab.title, message: `Tab ${tab.id} (${tab.title}) was added to the ZPilot Agent group and is now the current agent tab.` };
  },

  async navigate(args, ctx, tabId) {
    if (!args.url) throw new ToolError("INVALID_ARGS", 'navigate needs "url" (or "back", "forward", "reload").');
    const how = args.url.trim().toLowerCase();
    const opts = { timeoutMs: Math.min(30000, (args.timeout_sec - 5) * 1000), signal: ctx.signal };
    let nav;
    if (how === "back") nav = await watchNavigation(tabId, () => chrome.tabs.goBack(tabId), { ...opts, timeoutMs: 10000 });
    else if (how === "forward") nav = await watchNavigation(tabId, () => chrome.tabs.goForward(tabId), { ...opts, timeoutMs: 10000 });
    else if (how === "reload") nav = await watchNavigation(tabId, () => chrome.tabs.reload(tabId), opts);
    else nav = await navigate(tabId, normalizeUrl(args.url), opts);
    page(tabId, "showOverlay").catch(() => {});
    const s = await tabSummary(tabId);
    return { ...s, message: `${how === "back" || how === "forward" || how === "reload" ? `Went ${how}` : "Loaded"}: ${s.title || "(no title)"} — ${s.url}${nav.timedOut ? " (page still loading)" : ""}` };
  },

  async screenshot(args, ctx, tabId) {
    await cdp.attach(tabId);
    const { bitmap, info } = await capture(tabId);
    const cssW = info.viewport.width;
    const cssH = info.viewport.height;
    const outW = Math.min(cssW, MAX_SHOT_WIDTH);
    const outH = Math.round((cssH * outW) / cssW);
    const canvas = new OffscreenCanvas(outW, outH);
    canvas.getContext("2d").drawImage(bitmap, 0, 0, bitmap.width, bitmap.height, 0, 0, outW, outH);
    bitmap.close();
    cdp.getSession(tabId).shot = { cssPerPx: cssW / outW };
    return {
      url: info.url,
      title: info.title,
      image: { data: await encode(canvas), mime: "image/jpeg", width: outW, height: outH },
      message: `Screenshot of ${info.title || info.url} (${outW}x${outH}; scrolled to ${info.scroll.y} of ${info.scroll.height}px). Use these pixel coordinates for coordinate arguments.`,
    };
  },

  async zoom(args, ctx, tabId) {
    if (!args.region) throw new ToolError("INVALID_ARGS", 'zoom needs "region": [x0, y0, x1, y1].');
    await cdp.attach(tabId);
    const { bitmap, info } = await capture(tabId);
    const k = cdp.getSession(tabId).shot?.cssPerPx || 1;
    const devPerCss = bitmap.width / info.viewport.width;
    let [x0, y0, x1, y1] = args.region.map((v) => v * k * devPerCss);
    if (x1 < x0) [x0, x1] = [x1, x0];
    if (y1 < y0) [y0, y1] = [y1, y0];
    x0 = Math.max(0, x0);
    y0 = Math.max(0, y0);
    x1 = Math.min(bitmap.width, x1);
    y1 = Math.min(bitmap.height, y1);
    if (x1 - x0 < 4 || y1 - y0 < 4) {
      bitmap.close();
      throw new ToolError("INVALID_ARGS", "The zoom region is empty or outside the viewport.");
    }
    const scale = Math.min(1, MAX_SHOT_WIDTH / (x1 - x0));
    const outW = Math.round((x1 - x0) * scale);
    const outH = Math.round((y1 - y0) * scale);
    const canvas = new OffscreenCanvas(outW, outH);
    canvas.getContext("2d").drawImage(bitmap, x0, y0, x1 - x0, y1 - y0, 0, 0, outW, outH);
    bitmap.close();
    return {
      image: { data: await encode(canvas), mime: "image/jpeg", width: outW, height: outH },
      message: `Zoomed view of region [${args.region.join(", ")}] at ${outW}x${outH}. Coordinates for actions still refer to the full screenshot.`,
    };
  },

  async read_page(args, ctx, tabId) {
    const snap = await page(tabId, "snapshot", [{ filter: args.filter, maxChars: args.max_chars || 25000, viewportOnly: args.viewport_only, ref: args.ref }]);
    return { ...(await tabSummary(tabId)), message: snap.text, interactive_count: snap.interactive, truncated: snap.truncated };
  },

  async find(args, ctx, tabId) {
    if (!args.text) throw new ToolError("INVALID_ARGS", 'find needs "text".');
    const res = await page(tabId, "find", [args.text, { limit: args.limit || 20 }]);
    return { message: res.text, count: res.count };
  },

  async get_text(args, ctx, tabId) {
    const res = await callContent(tabId, CONTENT.reader, "ZB_reader", "extract", [{ format: "markdown", mainContentOnly: true }]);
    const chunk = sliceChars(res.content, args.offset, args.max_chars || 30000);
    return {
      url: res.url,
      title: res.title,
      message: chunk.text,
      total_chars: chunk.total,
      has_more: chunk.hasMore,
      ...(chunk.hasMore ? { next_offset: chunk.end } : {}),
    };
  },

  click: (args, ctx, tabId) => clickMaybeDownload(args, ctx, tabId, { button: "left", clickCount: 1, verb: "Clicked" }),
  double_click: (args, ctx, tabId) => clickMaybeDownload(args, ctx, tabId, { button: "left", clickCount: 2, verb: "Double-clicked" }),
  triple_click: (args, ctx, tabId) => clickAction(args, ctx, tabId, { button: "left", clickCount: 3, verb: "Triple-clicked" }),
  right_click: (args, ctx, tabId) => clickAction(args, ctx, tabId, { button: "right", clickCount: 1, verb: "Right-clicked" }),

  async hover(args, ctx, tabId) {
    await cdp.attach(tabId);
    const pt = await pointFrom(tabId, args);
    await showCursor(ctx, tabId, pt.x, pt.y, false);
    await cdp.mouseMove(tabId, pt.x, pt.y, cdp.parseModifiers(args.modifiers));
    await sleep(300, ctx.signal);
    return { message: `Hovering over ${pt.target || "the page"} at (${Math.round(pt.x)}, ${Math.round(pt.y)}).` };
  },

  async drag(args, ctx, tabId) {
    await cdp.attach(tabId);
    const from = await pointFrom(tabId, args, "ref", "coordinate");
    const to = await pointFrom(tabId, args, "to_ref", "end_coordinate");
    await showCursor(ctx, tabId, from.x, from.y, false);
    await cdp.mouseDrag(tabId, from, to);
    await showCursor(ctx, tabId, to.x, to.y, false);
    await sleep(300, ctx.signal);
    return { message: `Dragged from ${from.target || "point"} (${Math.round(from.x)}, ${Math.round(from.y)}) to ${to.target || "point"} (${Math.round(to.x)}, ${Math.round(to.y)}).` };
  },

  async type(args, ctx, tabId) {
    if (args.text == null) throw new ToolError("INVALID_ARGS", 'type needs "text".');
    await cdp.attach(tabId);
    let target = "the focused element";
    if (args.ref || args.coordinate) {
      const pt = await pointFrom(tabId, args);
      await showCursor(ctx, tabId, pt.x, pt.y, true);
      await cdp.mouseClick(tabId, pt.x, pt.y);
      target = pt.target || target;
      await sleep(80, ctx.signal);
    }
    if (args.clear) {
      await cdp.pressCombo(tabId, (await platformIsMac()) ? "meta+a" : "ctrl+a", { isMac: await platformIsMac() });
      await cdp.pressCombo(tabId, "Backspace");
    }
    const before = (await tabExists(tabId))?.url;
    const popups = watchPopups(tabId);
    let dialog = null;
    try {
      if (args.text) dialog = (await cdp.raceDialog(tabId, cdp.insertText(tabId, args.text))).dialog;
      if (args.submit && !dialog) dialog = (await cdp.raceDialog(tabId, cdp.pressCombo(tabId, "Enter"))).dialog;
      const after = await afterInput(tabId, before, popups.created, ctx.signal);
      return {
        url: after.url,
        title: after.title,
        message: `Typed ${JSON.stringify(truncate(args.text, 200))} into ${target}${args.submit ? " and pressed Enter" : ""}.${after.note ? " " + after.note + "." : ""}${dialogNote(dialog)}`,
      };
    } finally {
      popups.stop();
    }
  },

  async key(args, ctx, tabId) {
    if (!args.keys) throw new ToolError("INVALID_ARGS", 'key needs "keys", e.g. "Enter" or "ctrl+a".');
    await cdp.attach(tabId);
    const mac = await platformIsMac();
    const combos = args.keys.trim().split(/\s+/);
    const before = (await tabExists(tabId))?.url;
    const popups = watchPopups(tabId);
    let dialog = null;
    try {
      for (let i = 0; i < args.repeat && !dialog; i++) {
        for (const combo of combos) {
          dialog = (await cdp.raceDialog(tabId, cdp.pressCombo(tabId, combo, { isMac: mac }))).dialog;
          if (dialog) break;
        }
      }
      const after = await afterInput(tabId, before, popups.created, ctx.signal);
      return {
        url: after.url,
        title: after.title,
        message: `Pressed ${args.keys}${args.repeat > 1 ? ` ×${args.repeat}` : ""}.${after.note ? " " + after.note + "." : ""}${dialogNote(dialog)}`,
      };
    } finally {
      popups.stop();
    }
  },

  async fill(args, ctx, tabId) {
    if (!args.ref) throw new ToolError("INVALID_ARGS", 'fill needs "ref".');
    if (args.value == null) throw new ToolError("INVALID_ARGS", 'fill needs "value".');
    const res = await page(tabId, "fill", [args.ref, args.value]);
    if (res.kind === "done") return { message: res.message };
    await cdp.attach(tabId);
    if (args.value === "") await cdp.pressCombo(tabId, "Delete");
    else await cdp.insertText(tabId, args.value);
    const now = await page(tabId, "readValue", [args.ref]).catch(() => null);
    return { message: `Filled ${res.description} with ${JSON.stringify(truncate(args.value, 200))}${now != null && now !== args.value ? ` (the field now shows ${JSON.stringify(truncate(now, 200))})` : ""}.` };
  },

  async scroll(args, ctx, tabId) {
    await cdp.attach(tabId);
    const info = await page(tabId, "pageInfo");
    const px = args.amount * 100;
    const dx = args.direction === "left" ? -px : args.direction === "right" ? px : 0;
    const dy = args.direction === "up" ? -px : args.direction === "down" ? px : 0;
    let pt = { x: info.viewport.width / 2, y: info.viewport.height / 2 };
    if (args.ref || args.coordinate) pt = await pointFrom(tabId, args);
    await showCursor(ctx, tabId, pt.x, pt.y, false);
    await cdp.mouseWheel(tabId, pt.x, pt.y, dx, dy);
    await sleep(400, ctx.signal);
    let after = (await page(tabId, "pageInfo")).scroll;
    const moved = after.x !== info.scroll.x || after.y !== info.scroll.y;
    if (!moved && !args.ref && !args.coordinate) after = await page(tabId, "scrollWindow", [dx, dy]);
    return { message: `Scrolled ${args.direction} ${px}px. Page scroll is now ${after.x},${after.y} of ${after.width}x${after.height}.` };
  },

  async scroll_to(args, ctx, tabId) {
    if (!args.ref) throw new ToolError("INVALID_ARGS", 'scroll_to needs "ref".');
    const loc = await page(tabId, "locate", [args.ref]);
    return { message: `Scrolled ${loc.description} into view (now at ${Math.round(loc.x)}, ${Math.round(loc.y)}).` };
  },

  async wait(args, ctx) {
    const seconds = args.seconds ?? 1;
    await sleep(Math.min(seconds, 60) * 1000, ctx.signal);
    return { message: `Waited ${seconds}s.` };
  },

  async wait_for(args, ctx, tabId) {
    if (!args.selector && !args.text) throw new ToolError("INVALID_ARGS", 'wait_for needs "selector" or "text".');
    const seconds = args.seconds ?? 10;
    const found = await pollUntil(() => page(tabId, "exists", [{ selector: args.selector, text: args.text }]).catch(() => false), {
      timeoutMs: seconds * 1000,
      intervalMs: 400,
      signal: ctx.signal,
    });
    const what = args.selector ? `selector ${args.selector}` : `text ${JSON.stringify(args.text)}`;
    if (!found) throw new ToolError("WAIT_TIMEOUT", `${what} did not appear within ${seconds}s.`);
    return { message: `Found ${what}.` };
  },

  async javascript(args, ctx, tabId) {
    if (!args.script) throw new ToolError("INVALID_ARGS", 'javascript needs "script".');
    await cdp.attach(tabId);
    const res = await cdp.send(
      tabId,
      "Runtime.evaluate",
      { expression: args.script, awaitPromise: true, returnByValue: true, userGesture: true, replMode: true, timeout: 20000 },
      25000
    );
    if (res.exceptionDetails) {
      const d = res.exceptionDetails;
      throw new ToolError("JS_ERROR", truncate(d.exception?.description || d.text || "Script threw an exception.", 2000));
    }
    const r = res.result || {};
    let text;
    if (r.type === "undefined") text = "undefined";
    else if ("value" in r) text = typeof r.value === "string" ? r.value : JSON.stringify(r.value, null, 2);
    else text = r.unserializableValue || r.description || r.type;
    return { message: truncate(String(text), 20000) };
  },

  async console(args, ctx, tabId) {
    const wasEnabled = !!cdp.getSession(tabId)?.consoleEnabled;
    const s = await cdp.enableConsole(tabId);
    const re = toMatcher(args.pattern);
    const entries = s.console.filter((e) => re(`${e.level} ${e.text} ${e.source}`)).slice(-(args.limit || 50));
    if (args.clear) s.console.length = 0;
    const lines = entries.map((e) => `[${e.level}] ${truncate(e.text, 500)}${e.source ? `  (${truncate(e.source, 80)})` : ""}`);
    const intro = wasEnabled ? "" : "Console capture started now; only messages logged from now on are recorded.\n";
    return { message: intro + (lines.length ? lines.join("\n") : "No console messages."), count: lines.length };
  },

  async network(args, ctx, tabId) {
    const s = await cdp.attach(tabId);
    const re = toMatcher(args.pattern);
    const entries = [...s.network.values()].filter((r) => re(`${r.method} ${r.url} ${r.type}`)).slice(-(args.limit || 50));
    if (args.clear) s.network.clear();
    const lines = entries.map((r) => `${r.method} ${r.failed ? `FAILED(${r.failed})` : r.status ?? "…"} ${r.type} ${truncate(r.url, 200)}`);
    return { message: lines.length ? lines.join("\n") : "No requests recorded (capture starts when the agent first controls the tab).", count: lines.length };
  },

  async upload(args, ctx, tabId) {
    if (!args.paths?.length) throw new ToolError("INVALID_ARGS", 'upload needs "paths" (absolute file paths).');
    await cdp.attach(tabId);
    const token = Math.random().toString(36).slice(2);
    const target = await page(tabId, "fileTarget", [args.ref || null, token, args.selector || null]);
    if (target.kind === "none" && target.reason) throw new ToolError("NO_FILE_INPUT", `The page has ${target.reason}.`);
    if (target.kind === "none") {
      throw new ToolError(
        "NO_FILE_INPUT",
        target.count
          ? `The page has ${target.count} file inputs; pass the ref of the one to use (or of its upload button).`
          : "The page has no file input. Pass the ref of the upload button or drop zone."
      );
    }
    if (target.kind === "input") {
      if (!target.multiple && args.paths.length > 1) throw new ToolError("INVALID_ARGS", `${target.description} accepts one file; upload them one at a time.`);
      try {
        const { result } = await cdp.send(tabId, "Runtime.evaluate", { expression: `document.querySelector('[data-zpilot-target="${token}"]')` });
        if (!result?.objectId) throw new ToolError("NOT_INTERACTABLE", "The file input could not be reached (it may be inside a frame).");
        await cdp.send(tabId, "DOM.setFileInputFiles", { files: args.paths, objectId: result.objectId });
      } finally {
        page(tabId, "unmark", [token]).catch(() => {});
      }
      await sleep(400, ctx.signal);
      return { message: `Attached ${args.paths.length} file(s) to ${target.description}${target.accept ? ` (accepts ${target.accept})` : ""}.` };
    }
    // A button or drop zone: intercept the file chooser it opens and answer it with the files.
    await cdp.send(tabId, "Page.setInterceptFileChooserDialog", { enabled: true });
    try {
      const opened = cdp.waitForEvent(tabId, "Page.fileChooserOpened", 10000);
      const pt = await pointFrom(tabId, args);
      await showCursor(ctx, tabId, pt.x, pt.y, true);
      await cdp.mouseClick(tabId, pt.x, pt.y);
      const ev = await opened;
      if (!ev) {
        throw new ToolError("NO_FILE_CHOOSER", `Clicking ${pt.target || "the element"} did not open a file chooser. Find the file input with read_page filter=all, or the real upload button.`);
      }
      if (ev.mode === "selectSingle" && args.paths.length > 1) throw new ToolError("INVALID_ARGS", "This upload accepts one file; upload them one at a time.");
      await cdp.send(tabId, "DOM.setFileInputFiles", { files: args.paths, backendNodeId: ev.backendNodeId });
    } finally {
      await cdp.send(tabId, "Page.setInterceptFileChooserDialog", { enabled: false }).catch(() => {});
    }
    await sleep(400, ctx.signal);
    return { message: `Chose ${args.paths.length} file(s) in the file chooser opened by ${target.description}.` };
  },

  async download(args, ctx, tabId) {
    let url = args.url;
    let name = args.filename;
    if (!url && args.ref) {
      const t = await page(tabId, "linkTarget", [args.ref]);
      if (!t.url) throw new ToolError("NO_URL", `${t.description} has no link or media URL. Click it with expect_download=true instead.`);
      url = t.url;
      name ||= t.download_name;
    }
    if (!url) throw new ToolError("INVALID_ARGS", 'download needs "url" or "ref".');
    if (/^blob:/i.test(url)) throw new ToolError("NO_URL", "That is a page-internal blob URL; click the element with expect_download=true instead.");
    if (!/^data:/i.test(url)) url = normalizeUrl(url);
    ctx.progress(`Downloading ${truncate(url, 100)}…`);
    const file = await downloadUrl(url, { filename: name, signal: ctx.signal, timeoutMs: Math.max(10000, ctx.deadline - Date.now() - 2000) });
    return { file, message: `Downloaded ${file.filename} (${file.bytes} bytes${file.mime ? ", " + file.mime : ""}).` };
  },

  async downloads(args) {
    const list = await recentDownloads(args.limit || 20);
    const name = (p) => p.split(/[\\/]/).pop();
    return {
      action: "downloads",
      downloads: list,
      message: list.length ? list.map((d) => `${d.id}: ${d.state} ${name(d.filename)} (${d.bytes} bytes) from ${truncate(d.url, 100)}`).join("\n") : "No downloads.",
    };
  },

  async save_page(args, ctx, tabId) {
    await cdp.attach(tabId);
    const info = await page(tabId, "pageInfo");
    const format = args.format || "pdf";
    let data;
    let mime;
    if (format === "pdf") {
      const r = await cdp.send(tabId, "Page.printToPDF", { printBackground: true, preferCSSPageSize: true }, 60000);
      data = r.data;
      mime = "application/pdf";
    } else if (format === "mhtml") {
      const r = await cdp.send(tabId, "Page.captureSnapshot", { format: "mhtml" }, 60000);
      data = await blobToBase64(new Blob([r.data], { type: "multipart/related" }));
      mime = "multipart/related";
    } else {
      const m = await cdp.send(tabId, "Page.getLayoutMetrics");
      const size = m.cssContentSize || m.contentSize;
      const width = Math.min(Math.ceil(size.width), 4096);
      const height = Math.min(Math.ceil(size.height), 16384);
      await page(tabId, "overlayVisible", [false], { timeoutMs: 3000 }).catch(() => {});
      try {
        const r = await cdp.send(tabId, "Page.captureScreenshot", { format: "png", captureBeyondViewport: true, clip: { x: 0, y: 0, width, height, scale: 1 } }, 60000);
        data = r.data;
      } finally {
        page(tabId, "overlayVisible", [true], { timeoutMs: 3000 }).catch(() => {});
      }
      mime = "image/png";
    }
    return {
      url: info.url,
      title: info.title,
      page_file: { data, mime, format },
      message: `Saved ${info.title || info.url} as ${format.toUpperCase()} (${Math.round((data.length * 3) / 4 / 1024)} KB).`,
    };
  },

  async handle_dialog(args, ctx, tabId) {
    const s = cdp.getSession(tabId);
    if (!s?.dialog) throw new ToolError("NO_DIALOG", "No dialog is open in this tab.");
    const dialog = s.dialog;
    await cdp.send(tabId, "Page.handleJavaScriptDialog", { accept: args.accept, ...(args.text != null ? { promptText: args.text } : {}) });
    s.dialog = null;
    return { message: `${args.accept ? "Accepted" : "Dismissed"} the ${dialog.type} dialog "${truncate(dialog.message, 200)}".` };
  },

  async resize(args, ctx, tabId) {
    await cdp.attach(tabId);
    if (!args.width && !args.height) {
      await cdp.send(tabId, "Emulation.clearDeviceMetricsOverride");
      return { message: "Viewport size reset to the window size." };
    }
    if (!args.width || !args.height) throw new ToolError("INVALID_ARGS", 'resize needs both "width" and "height" (or 0, 0 to reset).');
    await cdp.send(tabId, "Emulation.setDeviceMetricsOverride", { width: args.width, height: args.height, deviceScaleFactor: 0, mobile: false });
    return { message: `Viewport set to ${args.width}x${args.height}.` };
  },

  async end(args) {
    const count = await endAgentSession({ closeTabs: args.close_tabs });
    return { action: "end", message: `Released ${count} agent tab(s)${args.close_tabs ? " and closed them" : ""}.` };
  },
};

// With expect_download, the click must start a download, which is returned when complete.
async function clickMaybeDownload(args, ctx, tabId, how) {
  if (!args.expect_download) return clickAction(args, ctx, tabId, how);
  let clicked;
  const file = await expectDownload(
    async () => {
      clicked = await clickAction(args, ctx, tabId, how);
    },
    { signal: ctx.signal, startTimeoutMs: 30000, timeoutMs: Math.max(10000, ctx.deadline - Date.now() - 2000) }
  );
  return { ...clicked, file, message: `${clicked.message} Downloaded ${file.filename} (${file.bytes} bytes).` };
}

async function clickAction(args, ctx, tabId, { button, clickCount, verb }) {
  await cdp.attach(tabId);
  const pt = await pointFrom(tabId, args);
  await showCursor(ctx, tabId, pt.x, pt.y, true);
  const before = (await tabExists(tabId))?.url;
  const popups = watchPopups(tabId);
  try {
    const { dialog } = await cdp.raceDialog(tabId, cdp.mouseClick(tabId, pt.x, pt.y, { button, clickCount, modifiers: cdp.parseModifiers(args.modifiers) }));
    const after = await afterInput(tabId, before, popups.created, ctx.signal);
    const where = `(${Math.round(pt.x)}, ${Math.round(pt.y)})`;
    const warn = pt.occludedBy ? ` Warning: ${pt.occludedBy} was covering it, so the click may have hit that instead.` : "";
    return {
      url: after.url,
      title: after.title,
      message: `${verb} ${pt.target || "the page"} at ${where}.${warn}${after.note ? " " + after.note + "." : ""}${dialogNote(dialog)}`,
    };
  } finally {
    popups.stop();
  }
}

function toMatcher(pattern) {
  if (!pattern) return () => true;
  try {
    const re = new RegExp(pattern, "i");
    return (s) => re.test(s);
  } catch {
    const p = pattern.toLowerCase();
    return (s) => s.toLowerCase().includes(p);
  }
}
