// ZPilot Browser popup: connection status, tool switches, settings and recent activity.

const TOOLS = [
  { name: "zagent_in_chrome", icon: "🖱️", title: "ZAgent in Chrome", desc: "Browse, click, type, upload, download, see pages" },
  { name: "generate_image", icon: "🎨", title: "Generate image", desc: "ChatGPT image series with references" },
  { name: "generate_music", icon: "🎵", title: "Generate music", desc: "Gemini Create music: MP3, video, cover" },
  { name: "web_search", icon: "🔍", title: "Web search", desc: "Google, Bing, DuckDuckGo, Brave, Yahoo" },
  { name: "webpage_read", icon: "📄", title: "Webpage read", desc: "Markdown, text or HTML of any page" },
];

const LINK_STATES = {
  connected: { pill: "Connected", cls: "ok", detail: "Tools can use this browser profile." },
  ready: { pill: "Ready", cls: "ok", detail: "Run any tool with this Connection ID. It connects automatically." },
  duplicate: { pill: "ID in use", cls: "bad", detail: "Another profile is using this Connection ID. Reset it in this profile." },
  idle: { pill: "Ready", cls: "ok", detail: "Copy this Connection ID into your tool configs." },
};

const $ = (id) => document.getElementById(id);

function ago(ts) {
  const s = Math.round((Date.now() - ts) / 1000);
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  return `${Math.round(s / 3600)}h ago`;
}

function renderLink(link = { state: "idle" }) {
  const info = LINK_STATES[link.state] || LINK_STATES.idle;
  const pill = $("linkPill");
  pill.textContent = info.pill;
  pill.className = `pill ${info.cls}`;
  let detail = info.detail;
  if (link.error && link.state !== "connected") detail += ` (${link.error})`;
  $("linkDetail").textContent = detail;
}

function renderTools(settings, running, queued = 0) {
  const list = $("tools");
  list.replaceChildren();
  for (const tool of TOOLS) {
    const li = document.createElement("li");
    const icon = document.createElement("span");
    icon.className = "icon";
    icon.textContent = tool.icon;
    const meta = document.createElement("div");
    meta.className = "meta";
    const name = document.createElement("div");
    name.className = "tname";
    name.textContent = tool.title;
    const desc = document.createElement("div");
    desc.className = "tdesc";
    desc.textContent = tool.desc;
    meta.append(name, desc);
    const sw = document.createElement("label");
    sw.className = "switch";
    sw.title = `Enable ${tool.title}`;
    const input = document.createElement("input");
    input.type = "checkbox";
    input.checked = !settings.disabledTools.includes(tool.name);
    input.setAttribute("aria-label", `Enable ${tool.title}`);
    input.addEventListener("change", () => {
      const disabled = new Set(settings.disabledTools);
      input.checked ? disabled.delete(tool.name) : disabled.add(tool.name);
      settings.disabledTools = [...disabled];
      saveSettings({ disabledTools: settings.disabledTools });
    });
    sw.append(input, document.createElement("span"));
    li.append(icon, meta, sw);
    list.append(li);
  }
  $("running").textContent = running || queued ? `${running} running · ${queued} queued` : "";
}

function renderActivity(activity = []) {
  const list = $("activity");
  list.replaceChildren();
  if (!activity.length) {
    const li = document.createElement("li");
    li.className = "muted empty";
    li.textContent = "No tool calls yet.";
    list.append(li);
    return;
  }
  for (const a of activity.slice(0, 15)) {
    const li = document.createElement("li");
    const dot = document.createElement("span");
    dot.className = `dot${a.ok ? "" : " fail"}`;
    const what = document.createElement("div");
    what.className = "what";
    const head = document.createElement("b");
    head.textContent = `${a.tool}${a.label ? " · " + a.label : ""}`;
    const sub = document.createElement("div");
    sub.textContent = `${a.summary || ""} · ${(a.ms / 1000).toFixed(1)}s${a.via === "console" ? " · console" : ""}`;
    what.append(head, sub);
    const when = document.createElement("span");
    when.className = "when";
    when.textContent = ago(a.at);
    li.append(dot, what, when);
    list.append(li);
  }
}

async function loadSettings() {
  const defaults = { disabledTools: [], defaultEngine: "google", showCursor: true, keepTabsOnError: false };
  const { settings } = await chrome.storage.local.get("settings");
  return { ...defaults, ...(settings || {}) };
}

function saveSettings(patch) {
  return chrome.runtime.sendMessage({ type: "zpilot.settings", patch });
}

async function init() {
  $("version").textContent = `Version ${chrome.runtime.getManifest().version} · browser tools for AI agents`;
  const identity = await chrome.runtime.sendMessage({ type: "zpilot.identity" });
  $("extId").textContent = identity.id;

  const settings = await loadSettings();
  const session = await chrome.storage.session.get(["link", "running", "queued", "activity"]);
  renderLink(session.link);
  renderTools(settings, session.running || 0, session.queued || 0);
  renderActivity(session.activity);

  $("engine").value = settings.defaultEngine;
  $("engine").addEventListener("change", (e) => saveSettings({ defaultEngine: e.target.value }));
  $("cursor").checked = settings.showCursor;
  $("cursor").addEventListener("change", (e) => saveSettings({ showCursor: e.target.checked }));

  $("reconnect").addEventListener("click", () => chrome.runtime.sendMessage({ type: "zpilot.reconnect" }));
  $("copyId").addEventListener("click", async () => {
    await navigator.clipboard.writeText($("extId").textContent);
    $("copyId").textContent = "Copied";
    setTimeout(() => ($("copyId").textContent = "Copy"), 1200);
  });
  $("resetId").addEventListener("click", async () => {
    $("resetId").disabled = true;
    try {
      const result = await chrome.runtime.sendMessage({ type: "zpilot.resetId" });
      $("extId").textContent = result.id;
      $("resetNote").textContent = "New ID created. Copy it into your tool configs; the old ID no longer connects.";
    } finally { $("resetId").disabled = false; }
  });
  chrome.storage.local.onChanged.addListener((changes) => {
    if (changes.connectionId) $("extId").textContent = changes.connectionId.newValue;
  });
  $("releaseAgent").addEventListener("click", async () => {
    $("releaseAgent").disabled = true;
    await chrome.runtime.sendMessage({ type: "zpilot.endAgent", closeTabs: false });
    $("releaseAgent").textContent = "Released";
  });
  $("openConsole").addEventListener("click", () => chrome.runtime.openOptionsPage());

  chrome.storage.session.onChanged.addListener((changes) => {
    if (changes.link) renderLink(changes.link.newValue);
    if (changes.activity) renderActivity(changes.activity.newValue);
    if (changes.running) session.running = changes.running.newValue;
    if (changes.queued) session.queued = changes.queued.newValue;
    if (changes.running || changes.queued) $("running").textContent = `${session.running || 0} running · ${session.queued || 0} queued`;
  });
}

init();
