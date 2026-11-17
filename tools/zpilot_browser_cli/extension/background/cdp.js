// Chrome DevTools Protocol access through chrome.debugger: trusted input events, screenshots,
// JavaScript evaluation, dialogs, console and network capture. Works on background tabs.

import { ToolError, withTimeout } from "./util.js";

const CONSOLE_LIMIT = 300;
const NETWORK_LIMIT = 400;
const sessions = new Map();

export function getSession(tabId) {
  return sessions.get(tabId) || null;
}

export function attachedTabIds() {
  return [...sessions.keys()];
}

export async function attach(tabId) {
  const existing = sessions.get(tabId);
  if (existing) return existing;
  try {
    await chrome.debugger.attach({ tabId }, "1.3");
  } catch (e) {
    const msg = String(e?.message || e);
    if (!/already attached/i.test(msg)) {
      throw new ToolError("DEBUGGER_UNAVAILABLE", `Could not take control of the tab: ${msg}`);
    }
    // Either our own earlier attachment (service worker restarted) or another debugger.
    try {
      await chrome.debugger.sendCommand({ tabId }, "Runtime.evaluate", { expression: "1" });
    } catch {
      throw new ToolError("DEBUGGER_UNAVAILABLE", "Another debugger (DevTools or another extension) is attached to this tab. Close it and retry.");
    }
  }
  const session = {
    tabId,
    console: [],
    consoleEnabled: false,
    network: new Map(),
    dialog: null,
    dialogWaiters: new Set(),
    shot: null, // { cssPerPx } of the latest screenshot, used to map coordinates
    eventWaiters: new Set(), // { method, resolve } for waitForEvent
  };
  sessions.set(tabId, session);
  // Runtime.enable is deliberately not called here: some bot checks detect it. Console capture
  // is switched on by the first "console" action instead.
  await Promise.allSettled([
    send(tabId, "Page.enable"),
    send(tabId, "Network.enable", { maxPostDataSize: 0 }),
    send(tabId, "Emulation.setFocusEmulationEnabled", { enabled: true }),
  ]);
  return session;
}

export async function detach(tabId) {
  sessions.delete(tabId);
  try {
    await chrome.debugger.detach({ tabId });
  } catch {}
}

export async function detachAll() {
  await Promise.all([...sessions.keys()].map(detach));
}

export async function send(tabId, method, params = {}, timeoutMs = 15000) {
  try {
    return await withTimeout(
      chrome.debugger.sendCommand({ tabId }, method, params),
      timeoutMs,
      () => new ToolError("CDP_TIMEOUT", `${method} did not respond within ${Math.round(timeoutMs / 1000)}s.`)
    );
  } catch (e) {
    if (e instanceof ToolError) throw e;
    const msg = String(e?.message || e);
    if (/not attached|No tab with given id|Detached while handling/i.test(msg)) {
      sessions.delete(tabId);
      throw new ToolError("DEBUGGER_DETACHED", "Control of the tab was lost (it was closed, or the debugging banner was cancelled). Retry the action.");
    }
    throw new ToolError("CDP_ERROR", `${method} failed: ${msg}`);
  }
}

export async function enableConsole(tabId) {
  const s = await attach(tabId);
  if (!s.consoleEnabled) {
    await send(tabId, "Runtime.enable");
    s.consoleEnabled = true;
  }
  return s;
}

// Races `promise` against a JavaScript dialog opening in the tab. A dialog blocks the page, so an
// input event that triggers confirm() would otherwise not return until the dialog is closed.
export function raceDialog(tabId, promise) {
  const s = sessions.get(tabId);
  const work = promise.then((value) => ({ value }));
  if (!s) return work;
  let waiter;
  const opened = new Promise((resolve) => {
    waiter = resolve;
    s.dialogWaiters.add(resolve);
  });
  return Promise.race([work, opened.then((dialog) => ({ dialog }))]).finally(() => s.dialogWaiters.delete(waiter));
}

// Resolves with the params of the next `method` event in the tab (null on timeout).
export function waitForEvent(tabId, method, timeoutMs = 10000) {
  const s = sessions.get(tabId);
  if (!s) return Promise.resolve(null);
  return new Promise((resolve) => {
    const w = { method, resolve: (p) => (clearTimeout(timer), s.eventWaiters.delete(w), resolve(p)) };
    const timer = setTimeout(() => w.resolve(null), timeoutMs);
    s.eventWaiters.add(w);
  });
}

chrome.debugger.onEvent.addListener((source, method, params) => {
  const s = sessions.get(source.tabId);
  if (!s) return;
  for (const w of [...s.eventWaiters]) if (w.method === method) w.resolve(params);
  switch (method) {
    case "Runtime.consoleAPICalled":
      pushConsole(s, {
        level: params.type,
        text: (params.args || []).map(formatRemote).join(" "),
        source: params.stackTrace?.callFrames?.[0]?.url || "",
      });
      break;
    case "Runtime.exceptionThrown": {
      const d = params.exceptionDetails || {};
      pushConsole(s, { level: "exception", text: d.exception?.description || d.text || "Uncaught exception", source: d.url || "" });
      break;
    }
    case "Network.requestWillBeSent":
      if (s.network.size >= NETWORK_LIMIT) s.network.delete(s.network.keys().next().value);
      s.network.set(params.requestId, {
        method: params.request.method,
        url: params.request.url,
        type: params.type || "",
        status: null,
        time: Date.now(),
      });
      break;
    case "Network.responseReceived": {
      const r = s.network.get(params.requestId);
      if (r) {
        r.status = params.response.status;
        r.mimeType = params.response.mimeType;
      }
      break;
    }
    case "Network.loadingFailed": {
      const r = s.network.get(params.requestId);
      if (r) r.failed = params.errorText || "failed";
      break;
    }
    case "Page.javascriptDialogOpening": {
      const dialog = { type: params.type, message: params.message, defaultPrompt: params.defaultPrompt || "", url: params.url };
      if (params.type === "alert" || params.type === "beforeunload") {
        dialog.handled = "accepted automatically";
        send(source.tabId, "Page.handleJavaScriptDialog", { accept: true }).catch(() => {});
      } else {
        s.dialog = dialog;
      }
      for (const w of s.dialogWaiters) w(dialog);
      break;
    }
    case "Page.javascriptDialogClosed":
      s.dialog = null;
      break;
  }
});

chrome.debugger.onDetach.addListener((source) => {
  if (source.tabId != null) sessions.delete(source.tabId);
});

chrome.tabs.onRemoved.addListener((tabId) => sessions.delete(tabId));

function pushConsole(s, entry) {
  s.console.push({ ...entry, time: Date.now() });
  if (s.console.length > CONSOLE_LIMIT) s.console.splice(0, s.console.length - CONSOLE_LIMIT);
}

function formatRemote(arg) {
  if (!arg) return "";
  if ("value" in arg) return typeof arg.value === "string" ? arg.value : JSON.stringify(arg.value);
  if (arg.unserializableValue) return arg.unserializableValue;
  return arg.description || arg.type || "";
}

// ---------- Input ----------

const MODIFIER_BITS = { alt: 1, option: 1, ctrl: 2, control: 2, meta: 4, cmd: 4, command: 4, win: 4, shift: 8 };
const BUTTON_MASK = { left: 1, right: 2, middle: 4 };

export function parseModifiers(text) {
  let bits = 0;
  for (const part of String(text || "").toLowerCase().split(/[+\s,]+/).filter(Boolean)) {
    if (!(part in MODIFIER_BITS)) throw new ToolError("INVALID_ARGS", `Unknown modifier "${part}". Use ctrl, shift, alt or meta.`);
    bits |= MODIFIER_BITS[part];
  }
  return bits;
}

export function mouseMove(tabId, x, y, modifiers = 0, buttons = 0) {
  return send(tabId, "Input.dispatchMouseEvent", { type: "mouseMoved", x, y, modifiers, buttons, button: buttons ? "left" : "none" });
}

export async function mouseClick(tabId, x, y, { button = "left", clickCount = 1, modifiers = 0 } = {}) {
  await mouseMove(tabId, x, y, modifiers);
  for (let i = 1; i <= clickCount; i++) {
    await send(tabId, "Input.dispatchMouseEvent", { type: "mousePressed", x, y, button, buttons: BUTTON_MASK[button], clickCount: i, modifiers });
    await send(tabId, "Input.dispatchMouseEvent", { type: "mouseReleased", x, y, button, buttons: 0, clickCount: i, modifiers });
  }
}

export async function mouseDrag(tabId, from, to, steps = 12) {
  await mouseMove(tabId, from.x, from.y);
  await send(tabId, "Input.dispatchMouseEvent", { type: "mousePressed", x: from.x, y: from.y, button: "left", buttons: 1, clickCount: 1 });
  for (let i = 1; i <= steps; i++) {
    const x = from.x + ((to.x - from.x) * i) / steps;
    const y = from.y + ((to.y - from.y) * i) / steps;
    await mouseMove(tabId, x, y, 0, 1);
  }
  await send(tabId, "Input.dispatchMouseEvent", { type: "mouseReleased", x: to.x, y: to.y, button: "left", buttons: 0, clickCount: 1 });
}

export function mouseWheel(tabId, x, y, deltaX, deltaY) {
  return send(tabId, "Input.dispatchMouseEvent", { type: "mouseWheel", x, y, deltaX, deltaY });
}

export function insertText(tabId, text) {
  return send(tabId, "Input.insertText", { text });
}

const NAMED_KEYS = {
  enter: { key: "Enter", code: "Enter", keyCode: 13, text: "\r" },
  return: { key: "Enter", code: "Enter", keyCode: 13, text: "\r" },
  tab: { key: "Tab", code: "Tab", keyCode: 9 },
  escape: { key: "Escape", code: "Escape", keyCode: 27 },
  esc: { key: "Escape", code: "Escape", keyCode: 27 },
  backspace: { key: "Backspace", code: "Backspace", keyCode: 8 },
  delete: { key: "Delete", code: "Delete", keyCode: 46 },
  del: { key: "Delete", code: "Delete", keyCode: 46 },
  insert: { key: "Insert", code: "Insert", keyCode: 45 },
  space: { key: " ", code: "Space", keyCode: 32, text: " " },
  arrowup: { key: "ArrowUp", code: "ArrowUp", keyCode: 38 },
  up: { key: "ArrowUp", code: "ArrowUp", keyCode: 38 },
  arrowdown: { key: "ArrowDown", code: "ArrowDown", keyCode: 40 },
  down: { key: "ArrowDown", code: "ArrowDown", keyCode: 40 },
  arrowleft: { key: "ArrowLeft", code: "ArrowLeft", keyCode: 37 },
  left: { key: "ArrowLeft", code: "ArrowLeft", keyCode: 37 },
  arrowright: { key: "ArrowRight", code: "ArrowRight", keyCode: 39 },
  right: { key: "ArrowRight", code: "ArrowRight", keyCode: 39 },
  home: { key: "Home", code: "Home", keyCode: 36 },
  end: { key: "End", code: "End", keyCode: 35 },
  pageup: { key: "PageUp", code: "PageUp", keyCode: 33 },
  pagedown: { key: "PageDown", code: "PageDown", keyCode: 34 },
  ctrl: { key: "Control", code: "ControlLeft", keyCode: 17 },
  control: { key: "Control", code: "ControlLeft", keyCode: 17 },
  shift: { key: "Shift", code: "ShiftLeft", keyCode: 16 },
  alt: { key: "Alt", code: "AltLeft", keyCode: 18 },
  meta: { key: "Meta", code: "MetaLeft", keyCode: 91 },
  cmd: { key: "Meta", code: "MetaLeft", keyCode: 91 },
};
for (let i = 1; i <= 12; i++) NAMED_KEYS[`f${i}`] = { key: `F${i}`, code: `F${i}`, keyCode: 111 + i };

const PUNCT_CODES = {
  "-": ["Minus", 189], "=": ["Equal", 187], "[": ["BracketLeft", 219], "]": ["BracketRight", 221],
  "\\": ["Backslash", 220], ";": ["Semicolon", 186], "'": ["Quote", 222], ",": ["Comma", 188],
  ".": ["Period", 190], "/": ["Slash", 191], "`": ["Backquote", 192],
};

function keyDef(name) {
  const lower = name.toLowerCase();
  if (NAMED_KEYS[lower]) return NAMED_KEYS[lower];
  if ([...name].length === 1) {
    const ch = name;
    if (/[a-z]/i.test(ch)) return { key: ch, code: `Key${ch.toUpperCase()}`, keyCode: ch.toUpperCase().charCodeAt(0), text: ch };
    if (/[0-9]/.test(ch)) return { key: ch, code: `Digit${ch}`, keyCode: ch.charCodeAt(0), text: ch };
    const p = PUNCT_CODES[ch];
    return { key: ch, code: p ? p[0] : "", keyCode: p ? p[1] : 0, text: ch };
  }
  throw new ToolError("INVALID_ARGS", `Unknown key "${name}". Use names like Enter, Tab, Escape, ArrowDown, PageDown, F5 or single characters, combined with + (e.g. ctrl+a).`);
}

const MAC_COMMANDS = { a: "selectAll", c: "copy", v: "paste", x: "cut", z: "undo", y: "redo" };

// Presses one combination such as "Enter", "ctrl+a" or "shift+Tab".
export async function pressCombo(tabId, combo, { isMac = false } = {}) {
  const parts = combo === "+" ? ["+"] : combo.split("+").filter((p) => p !== "");
  if (combo.length > 1 && combo.endsWith("++")) parts.push("+");
  if (!parts.length) throw new ToolError("INVALID_ARGS", `Empty key combination "${combo}".`);
  const mainName = parts.pop();
  let modifiers = 0;
  const held = [];
  for (const m of parts) {
    const bit = MODIFIER_BITS[m.toLowerCase()];
    if (!bit) throw new ToolError("INVALID_ARGS", `"${m}" is not a modifier in "${combo}".`);
    modifiers |= bit;
    const def = keyDef(m);
    held.push(def);
    await send(tabId, "Input.dispatchKeyEvent", { type: "rawKeyDown", key: def.key, code: def.code, windowsVirtualKeyCode: def.keyCode, modifiers });
  }
  const def = keyDef(mainName);
  const shifted = modifiers & 8;
  let text = def.text;
  if (text && shifted && /[a-z]/.test(text)) text = text.toUpperCase();
  const typing = text && !(modifiers & (2 | 4));
  const down = {
    type: typing ? "keyDown" : "rawKeyDown",
    key: typing && shifted && def.key.length === 1 ? def.key.toUpperCase() : def.key,
    code: def.code,
    windowsVirtualKeyCode: def.keyCode,
    modifiers,
  };
  if (typing) {
    down.text = text;
    down.unmodifiedText = def.text;
  }
  if (isMac && modifiers & 4 && MAC_COMMANDS[mainName.toLowerCase()]) down.commands = [MAC_COMMANDS[mainName.toLowerCase()]];
  await send(tabId, "Input.dispatchKeyEvent", down);
  await send(tabId, "Input.dispatchKeyEvent", { type: "keyUp", key: down.key, code: def.code, windowsVirtualKeyCode: def.keyCode, modifiers });
  for (const h of held.reverse()) {
    modifiers &= ~(MODIFIER_BITS[h.key.toLowerCase()] || 0);
    await send(tabId, "Input.dispatchKeyEvent", { type: "keyUp", key: h.key, code: h.code, windowsVirtualKeyCode: h.keyCode, modifiers });
  }
}
