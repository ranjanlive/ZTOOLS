// ZPilot Browser console: uses the same queued dispatcher as independent tool binaries.

const PRESETS = {
  generate_image: {
    "Simple": { prompt: "A watercolor fox reading a book under a lamp", format: "png" },
    "Temporary chat": { prompt: "Isometric illustration of a tiny coffee shop", temporary_chat: true },
    "Refusal test": { prompt: "A photorealistic photo of a famous living politician" },
  },
  generate_music: {
    "Instrumental, short": { prompt: "Calm lo-fi intro for a tech podcast, soft piano and vinyl crackle", length: "short", vocals: "instrumental" },
    "Song with vocals": { prompt: "Upbeat pop song about launching a startup", vocals: "on", genre: "pop", downloads: ["audio", "video"] },
  },
  web_search: {
    "Google": { query: "MV3 native messaging service worker lifetime", max_results: 5 },
    "Bing": { query: "best open source vector databases", engine: "bing", max_results: 8 },
    "DuckDuckGo, past week": { query: "chrome release notes", engine: "duckduckgo", time_range: "week" },
    "Site search": { query: "tab groups api", site: "developer.chrome.com", max_results: 5 },
  },
  webpage_read: {
    "Markdown": { url: "https://en.wikipedia.org/wiki/Web_browser", max_chars: 8000 },
    "No links/media": { url: "https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging", include_links: false, include_images: false, include_media: false },
    "Plain text": { url: "https://example.com", format: "text" },
    "HTML source": { url: "https://example.com", format: "html" },
  },
  zagent_in_chrome: {
    "Navigate": { action: "navigate", url: "https://www.wikipedia.org" },
    "Read page": { action: "read_page" },
    "Screenshot": { action: "screenshot" },
    "Find": { action: "find", text: "search" },
    "Click ref": { action: "click", ref: "" },
    "Type": { action: "type", ref: "", text: "Chrome extensions", submit: true },
    "Tabs": { action: "tabs" },
    "End": { action: "end" },
  },
};

const $ = (id) => document.getElementById(id);
let port = null;
let tools = [];
let current = null;
let runningId = null;
let lastResult = null;
const drafts = {};

function connect() {
  port = chrome.runtime.connect({ name: "zpilot-console" });
  port.onMessage.addListener(onMessage);
  port.onDisconnect.addListener(() => {
    setConn("Reconnecting…", "warn");
    if (runningId) finish(false, { code: "DISCONNECTED", message: "The extension restarted while the call was running." });
    setTimeout(connect, 800);
  });
}

function setConn(text, cls) {
  $("conn").textContent = text;
  $("conn").className = `pill ${cls}`;
}

function onMessage(msg) {
  if (msg.type === "hello") {
    setConn(`ZPilot Browser ${msg.version}`, "ok");
    tools = msg.tools;
    renderTabs();
    return;
  }
  if (msg.id !== runningId) return;
  if (msg.type === "progress") log(msg.message);
  if (msg.type === "result") finish(msg.ok, msg.ok ? msg.result : msg.error, msg.duration_ms);
}

function renderTabs() {
  const nav = $("toolTabs");
  nav.replaceChildren();
  const saved = localStorageGet("zpilot.console.tool");
  for (const t of tools) {
    const b = document.createElement("button");
    b.type = "button";
    b.role = "tab";
    b.textContent = t.title;
    b.dataset.name = t.name;
    b.addEventListener("click", () => selectTool(t.name));
    nav.append(b);
  }
  selectTool(current || (tools.some((t) => t.name === saved) ? saved : tools[0]?.name));
}

function selectTool(name) {
  if (current) drafts[current] = $("args").value;
  current = name;
  localStorageSet("zpilot.console.tool", name);
  const tool = tools.find((t) => t.name === name);
  for (const b of $("toolTabs").children) b.setAttribute("aria-selected", String(b.dataset.name === name));
  $("toolDesc").textContent = tool.description;
  $("schema").textContent = JSON.stringify(tool.inputSchema, null, 2);
  const presets = PRESETS[name] || {};
  $("args").value = drafts[name] ?? JSON.stringify(Object.values(presets)[0] || {}, null, 2);
  $("argsError").textContent = "";
  const box = $("presets");
  box.replaceChildren();
  for (const [label, args] of Object.entries(presets)) {
    const b = document.createElement("button");
    b.type = "button";
    b.textContent = label;
    b.addEventListener("click", () => ($("args").value = JSON.stringify(args, null, 2)));
    box.append(b);
  }
}

function log(text, cls) {
  const li = document.createElement("li");
  li.textContent = `${new Date().toLocaleTimeString()}  ${text}`;
  if (cls) li.className = cls;
  $("log").append(li);
  $("log").scrollTop = $("log").scrollHeight;
}

function clearOutput() {
  $("log").replaceChildren();
  $("images").replaceChildren();
  $("contentBox").hidden = true;
  $("rawBox").hidden = true;
  $("copy").disabled = true;
  lastResult = null;
}

function run() {
  if (runningId || !current) return;
  let args;
  try {
    args = JSON.parse($("args").value || "{}");
  } catch (e) {
    $("argsError").textContent = `Invalid JSON: ${e.message}`;
    return;
  }
  $("argsError").textContent = "";
  clearOutput();
  runningId = `console-${Date.now()}`;
  $("run").disabled = true;
  $("cancel").disabled = false;
  $("status").textContent = `Running ${current}…`;
  log(`→ ${current} ${JSON.stringify(args)}`);
  port.postMessage({ type: "call", id: runningId, tool: current, args });
}

function finish(ok, payload, ms) {
  runningId = null;
  $("run").disabled = false;
  $("cancel").disabled = true;
  const secs = ms != null ? ` in ${(ms / 1000).toFixed(1)}s` : "";
  $("status").textContent = ok ? `Done${secs}` : `Failed${secs}`;
  if (!ok) {
    log(`✖ ${payload.code}: ${payload.message}`, "bad");
    lastResult = { ok: false, error: payload };
  } else {
    log(`✔ Finished${secs}`, "ok");
    lastResult = { ok: true, result: payload };
    showResult(payload);
  }
  showRaw(lastResult);
}

function showResult(result) {
  const imgs = [...(result.images || []), ...(result.image ? [result.image] : [])];
  for (const [i, img] of imgs.entries()) {
    const fig = document.createElement("figure");
    const el = document.createElement("img");
    el.src = `data:${img.mime};base64,${img.data}`;
    el.alt = `Result image ${i + 1}`;
    const cap = document.createElement("figcaption");
    cap.textContent = `${img.width}×${img.height} · ${img.mime}${img.bytes ? ` · ${(img.bytes / 1024).toFixed(0)} KB` : ""}`;
    fig.append(el, cap);
    $("images").append(fig);
  }
  const text = result.content ?? result.message ?? (result.results ? formatResults(result) : null);
  if (text) {
    $("contentLabel").textContent = result.content != null ? `Content (${result.format}, ${result.returned_chars} of ${result.total_chars} chars)` : "Output";
    $("content").textContent = text;
    $("contentBox").hidden = false;
  }
}

function formatResults(r) {
  const lines = r.results.map((x) => `${x.position}. ${x.title}\n   ${x.url}\n   ${x.snippet || ""}`);
  if (r.answer) lines.unshift(`Answer: ${r.answer}\n`);
  return lines.join("\n\n");
}

// Large base64 payloads are shortened in the JSON view.
function showRaw(obj) {
  const view = JSON.stringify(obj, (k, v) => (k === "data" && typeof v === "string" && v.length > 120 ? `${v.slice(0, 60)}… (${v.length} chars)` : v), 2);
  $("raw").textContent = view;
  $("rawBox").hidden = false;
  $("copy").disabled = false;
}

function localStorageGet(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function localStorageSet(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {}
}

$("run").addEventListener("click", run);
$("cancel").addEventListener("click", () => runningId && port.postMessage({ type: "cancel", id: runningId }));
$("schemaBtn").addEventListener("click", () => ($("schema").hidden = !$("schema").hidden));
$("copy").addEventListener("click", () => navigator.clipboard.writeText(JSON.stringify(lastResult, null, 2)));
$("args").addEventListener("keydown", (e) => {
  if ((e.ctrlKey || e.metaKey) && e.key === "Enter") {
    e.preventDefault();
    run();
  }
});

connect();
