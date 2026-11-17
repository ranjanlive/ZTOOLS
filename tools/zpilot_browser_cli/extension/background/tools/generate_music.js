// generate_music: drives Gemini's "Create music" tool in a ZPilot tab and returns the finished
// track: the MP3 (and optionally the MP4 with cover art) downloaded through Chrome, plus the cover
// art as an image. Calls run one at a time because they share one Gemini tab and one account.
// Continuing the same conversation (conversation_url) keeps a series consistent.

import { CONTENT } from "../config.js";
import { attach, insertText, detach, pressCombo } from "../cdp.js";
import { allowAutomaticDownloads, expectDownload } from "../downloads.js";
import { callContent } from "../inject.js";
import { addToGroup, closeTab, focusTab, navigate, openTab, tabExists, withTabVisible } from "../tabs.js";
import { ToolError, pollUntil, sleep, throwIfAborted, truncate } from "../util.js";

const HOME = "https://gemini.google.com/app";
const VOCALS = { on: "Vocals on", vocals: "Vocals on", "vocals on": "Vocals on", instrumental: "Instrumental", none: "Instrumental", off: "Instrumental", custom: "Custom" };

const LIMIT = [/reached (your|the)[\w\s]{0,40}limit/i, /limit (for|on) (music|songs|tracks)/i, /try again (later|tomorrow|in \d)/i, /too many requests/i, /come back (later|tomorrow)/i];
const REFUSAL = [/\b(can['’]?t|cannot|unable to|not able to)\b[^.]{0,80}\b(create|generate|make|compose|produce)\b/i, /\bpolic(y|ies)\b/i, /\bnot (allowed|permitted)\b/i, /\bI['’]m (sorry|unable)\b/i];
const NOT_AVAILABLE = [/music (generation|creation) (is )?(not|isn['’]t) available/i, /not available in your (country|region)/i];

let queue = Promise.resolve();
let pending = 0;
let musicTabId = null;

const page = (tabId, method, args = [], opts) => callContent(tabId, CONTENT.gemini, "ZB_gemini", method, args, opts);
const match = (patterns, text) => patterns.find((re) => re.test(text || "")) && text;

export default {
  name: "generate_music",
  title: "Generate music with Gemini",
  description:
    "Generates a music track with Gemini's Create music tool in the user's signed-in Chrome. Returns the downloaded MP3 " +
    "as file paths in Chrome's download folder, the cover art image, the chat title, Gemini's reply (description or lyrics) and the duration. " +
    "Options: length (short | standard), vocals (on | instrumental | custom), genre (pop, rock, lo-fi, cinematic, classical, …). " +
    "Pass conversation_url to make the next track in the same chat (keeps a series consistent). Calls run one at a time; a track takes about 1-4 minutes.",
  defaultTimeoutSec: 600,
  inputSchema: {
    type: "object",
    properties: {
      prompt: { type: "string", description: "Description of the track: mood, instruments, tempo, use, lyrics or theme." },
      conversation_url: { type: "string", description: "An existing https://gemini.google.com/app/... chat to continue." },
      length: { type: "string", description: "short or standard (Gemini's Length option). Empty keeps Gemini's default." },
      vocals: { type: "string", description: "on, instrumental or custom (Gemini's Vocals option). Empty keeps the default." },
      genre: { type: "string", description: "Genre as named in Gemini's Genre menu (pop, hip-hop and rap, rock, k-pop, latin, electronic, r&b, country, afrobeats, reggae, jazz and blues, classical, folk, lo-fi, acoustic, cinematic, ambient, …) or custom." },
      cover: { type: "boolean", default: true, description: "Return the cover art as a PNG image." },
      background: { type: "boolean", default: true, description: "Work in a background tab without taking focus." },
      timeout_sec: { type: "integer", minimum: 60, maximum: 1500, default: 600, description: "Overall time limit including time spent queued." },
    },
    required: ["prompt"],
  },

  label: (args) => truncate(args.prompt, 80),
  summarize: (args, r) => `"${r.chat_title}" ${r.duration_sec}s`,

  run(args, ctx) {
    const ahead = pending++;
    if (ahead > 0) ctx.progress(`Queued behind ${ahead} other music job(s)…`);
    const job = queue.then(() => {
      throwIfAborted(ctx.signal);
      return generate(args, ctx);
    });
    queue = job.catch(() => {}).finally(() => pending--);
    return job;
  },
};

async function generate(args, ctx) {
  const { signal } = ctx;
  const started = Date.now();
  const deadline = ctx.deadline - 3000;
  if (args.conversation_url && !/^https:\/\/gemini\.google\.com\/app\/[\w-]+/.test(args.conversation_url)) {
    throw new ToolError("INVALID_ARGS", "conversation_url must be a https://gemini.google.com/app/... chat URL.");
  }

  ctx.progress("Opening Gemini…");
  const tabId = await prepareTab(args, ctx);
  const state = () => page(tabId, "state", [], { timeoutMs: 15000 });

  const ready = await pollUntil(
    async () => {
      const s = await state();
      if (s.loginRequired) throw new ToolError("LOGIN_REQUIRED", "Gemini is not signed in. Sign in at gemini.google.com in this Chrome profile and retry.");
      return s.composer && !s.busy ? s : null;
    },
    { timeoutMs: Math.min(45000, deadline - Date.now()), intervalMs: 700, signal }
  );
  if (!ready) throw new ToolError("GEMINI_NOT_READY", "Gemini's prompt box did not become available (is a previous reply still running?).");

  // Gemini's editor and menus ignore input while the tab is in the background: show it while
  // setting up and sending, then return to the previous tab.
  const base = await withTabVisible(tabId, () => setUp(tabId, args, ctx, ready, state));
  const baseline = { tracks: base.tracks.length, responses: base.responses };
  ctx.progress("Submitting the prompt…");
  await withTabVisible(tabId, () => submit(tabId, args.prompt, baseline, state, signal));
  return finish(tabId, args, ctx, baseline, state, deadline, started);
}

async function setUp(tabId, args, ctx, ready, state) {
  if (!ready.musicMode) {
    ctx.progress("Turning on Create music…");
    const on = await page(tabId, "openMusicMode", [], { timeoutMs: 20000 });
    if (!on.ok) throw new ToolError("MUSIC_UNAVAILABLE", `Could not turn on Gemini's Create music tool: ${on.reason}.`);
  }
  const chosen = {};
  for (const [kind, raw] of [["Length", args.length], ["Vocals", args.vocals], ["Genre", args.genre]]) {
    if (!raw) continue;
    const value = kind === "Vocals" ? VOCALS[String(raw).trim().toLowerCase()] || raw : raw;
    const res = await page(tabId, "setOption", [kind, value], { timeoutMs: 15000 });
    if (!res.ok && !res.options?.length && /was not found/.test(res.reason || "")) {
      // Follow-up prompts in a music chat may not offer the option chips; the chat keeps its settings.
      (ctx.warnings ||= []).push(`${kind} was not set: ${res.reason}`);
      continue;
    }
    if (!res.ok) {
      throw new ToolError("INVALID_ARGS", `${kind}: ${res.reason}.${res.options?.length ? ` Options: ${res.options.join(", ")}.` : ""}`);
    }
    chosen[kind.toLowerCase()] = res.chosen;
  }
  ctx.chosen = chosen;
  return state();
}

async function finish(tabId, args, ctx, baseline, state, deadline, started) {
  const { signal } = ctx;
  const chosen = ctx.chosen || {};
  const verdict = await watch(tabId, baseline, ctx, state, deadline);
  const s = await state();
  const conversationUrl = s.url;
  const details = { reply_text: truncate(verdict.text || "", 2000), conversation_url: conversationUrl };
  switch (verdict.type) {
    case "track":
      break;
    case "limit":
      throw new ToolError("RATE_LIMITED", `Gemini reports a usage limit: "${truncate(verdict.text, 300)}"`, details);
    case "refused":
      throw new ToolError("MUSIC_REFUSED", `Gemini declined to make this track: "${truncate(verdict.text, 400)}". Rephrase the prompt (for example without real artists or copyrighted lyrics).`, details);
    case "unavailable":
      throw new ToolError("MUSIC_UNAVAILABLE", `Gemini says music generation is not available: "${truncate(verdict.text, 300)}"`, details);
    case "text":
      throw new ToolError("TEXT_INSTEAD_OF_MUSIC", `Gemini answered with text instead of a track: "${truncate(verdict.text, 400)}"`, details);
    default:
      throw new ToolError("TIMEOUT", `No finished track appeared before the time limit (last state: ${verdict.detail}).`, details);
  }

  const index = verdict.index;
  const track = s.tracks[index] || {};
  const result = {
    conversation_url: conversationUrl,
    chat_title: s.title,
    reply_text: truncate(verdict.text || "", 8000),
    duration_sec: track.duration || 0,
    track_index: index,
    options: chosen,
    files: {},
  };

  if (args.cover) {
    ctx.progress("Capturing the cover art…");
    const cover = await withTabVisible(tabId, async () => {
      const shot = await page(tabId, "captureCover", [index], { timeoutMs: 25000 }).catch((e) => ({ ok: false, reason: e.message }));
      await pollUntil(async () => (await state()).tracks[index]?.ready, { timeoutMs: 5000, intervalMs: 500, signal });
      return shot;
    });
    if (cover.ok) result.cover = { data: cover.data, mime: cover.mime, width: cover.width, height: cover.height, bytes: cover.bytes };
    else result.cover_warning = cover.reason;
  }

  ctx.progress("Downloading the MP3…");
  await allowAutomaticDownloads("https://gemini.google.com/*");
  for (let attempt = 1; !result.files.audio; attempt++) {
    await backToChat(tabId, conversationUrl, index, state, signal);
    try {
      result.files.audio = await withTabVisible(tabId, () =>
        expectDownload(
          async () => {
            const res = await page(tabId, "clickDownload", [index, "audio"], { timeoutMs: 15000 });
            if (!res.ok) throw new ToolError("DOWNLOAD_FAILED", `Could not start the MP3 download: ${res.reason}.`, details);
          },
          { signal, startTimeoutMs: 60000, timeoutMs: Math.max(30000, deadline - Date.now()) }
        )
      );
    } catch (e) {
      if (attempt >= 2 || signal?.aborted || !(e instanceof ToolError) || !["NO_DOWNLOAD", "DOWNLOAD_FAILED"].includes(e.code)) throw e;
      ctx.progress("Retrying the MP3 download…");
    }
  }
  const after = await state();
  if (after.tracks[index]?.duration) result.duration_sec = after.tracks[index].duration;
  if (ctx.warnings?.length) result.warnings = ctx.warnings;
  result.duration_total_sec = Math.round((Date.now() - started) / 100) / 10;
  return result;
}

// Makes sure the tab still shows the chat with the finished track (Gemini sometimes returns to a
// new chat after a download), reopening it if needed.
async function backToChat(tabId, url, index, state, signal) {
  const s = await state();
  if (sameChat(s.url, url) && s.tracks.length > index) return;
  await navigate(tabId, url, { timeoutMs: 45000, signal });
  const ok = await pollUntil(async () => (await state()).tracks.length > index, { timeoutMs: 30000, intervalMs: 700, signal });
  if (!ok) throw new ToolError("DOWNLOAD_FAILED", "The chat with the track could not be reopened.", { conversation_url: url });
}

async function prepareTab(args, ctx) {
  const { signal } = ctx;
  const target = args.conversation_url || HOME;
  let tab = await tabExists(musicTabId);
  if (!tab || !tab.url?.startsWith("https://gemini.google.com")) {
    const { musicTabId: stored } = await chrome.storage.session.get("musicTabId");
    tab = await tabExists(stored);
    if (!tab || !tab.url?.startsWith("https://gemini.google.com")) tab = null;
  }
  if (!tab) {
    const tabId = await openTab({ kind: "tools", active: !args.background });
    try {
      await navigate(tabId, target, { timeoutMs: 45000, signal });
    } catch (e) {
      await closeTab(tabId);
      throw e;
    }
    remember(tabId);
    return tabId;
  }
  remember(tab.id);
  addToGroup(tab.id, "tools").catch(() => {});
  const same = args.conversation_url && sameChat(tab.url, args.conversation_url);
  if (!same) await navigate(tab.id, target, { timeoutMs: 45000, signal });
  if (!args.background) await focusTab(tab.id);
  return tab.id;
}

function remember(tabId) {
  musicTabId = tabId;
  chrome.storage.session.set({ musicTabId: tabId }).catch(() => {});
}

function sameChat(a, b) {
  try {
    const norm = (u) => {
      const x = new URL(u);
      return x.origin + x.pathname.replace(/\/+$/, "");
    };
    return norm(a) === norm(b);
  } catch {
    return false;
  }
}

async function submit(tabId, prompt, baseline, state, signal) {
  let res = await page(tabId, "submit", [prompt]);
  if (!res.entered) {
    const focused = await page(tabId, "focusComposer", []);
    if (!focused) throw new ToolError("SUBMIT_FAILED", "Could not find Gemini's prompt box.");
    await attach(tabId);
    try {
      await insertText(tabId, prompt);
    } finally {
      await detach(tabId);
    }
    await sleep(300, signal);
    res = await page(tabId, "clickSend", []);
  }
  if (!res.sent) throw new ToolError("SUBMIT_FAILED", `Could not send the prompt (${res.reason || "unknown reason"}).`);
  let last = null;
  const accepted = async () => {
    const s = (last = await state());
    return s.busy || s.responses > baseline.responses || (!s.composerText && s.composer);
  };
  let sent = await pollUntil(accepted, { timeoutMs: 8000, intervalMs: 500, signal });
  // On a freshly opened page Gemini sometimes ignores the first click on Send: send again.
  for (let i = 0; !sent && i < 2 && last?.composerText; i++) {
    if (i === 0) await page(tabId, "clickSend", []);
    else {
      // Last try: a trusted Enter key press in the prompt box.
      await page(tabId, "focusComposer", []);
      await attach(tabId);
      try {
        await pressCombo(tabId, "End");
        await pressCombo(tabId, "Enter");
      } finally {
        await detach(tabId);
      }
    }
    sent = await pollUntil(accepted, { timeoutMs: 8000, intervalMs: 500, signal });
  }
  if (!sent) throw new ToolError("SUBMIT_FAILED", `Gemini did not accept the prompt (the send button may be disabled). Send: ${JSON.stringify(res)}; page: ${JSON.stringify({ composer: last?.composer, text: last?.composerText?.slice(0, 80), busy: last?.busy, responses: last?.responses, baseline: baseline.responses, url: last?.url })}`);
}

async function watch(tabId, baseline, ctx, state, deadline) {
  const { signal } = ctx;
  let replyText = null;
  let replySince = 0;
  let lastProgress = 0;
  let detail = "waiting for the reply";
  const started = Date.now();
  while (Date.now() < deadline) {
    const s = await state();
    if (s.loginRequired) return { type: "error", text: "Signed out of Gemini during generation." };
    for (const alert of s.alerts) {
      if (match(LIMIT, alert)) return { type: "limit", text: alert };
    }
    const fresh = s.tracks.slice(baseline.tracks);
    const newReply = s.responses > baseline.responses ? s.lastReply : null;
    if (fresh.length) {
      const t = fresh[fresh.length - 1];
      detail = `track found, ready=${t.ready}, busy=${s.busy}`;
      if ((t.ready || t.hasDownload) && !s.busy) return { type: "track", index: t.index, text: newReply?.text || "" };
    } else if (newReply && !s.busy && !newReply.generating) {
      if (newReply.text !== replyText) {
        replyText = newReply.text;
        replySince = Date.now();
      }
      const settled = Date.now() - replySince;
      detail = `text reply settled ${Math.round(settled / 1000)}s, no track`;
      if (settled >= 8000) {
        if (match(LIMIT, replyText)) return { type: "limit", text: replyText };
        if (match(NOT_AVAILABLE, replyText)) return { type: "unavailable", text: replyText };
        if (match(REFUSAL, replyText)) return { type: "refused", text: replyText };
        if (settled >= 20000) return { type: "text", text: replyText };
      }
    } else {
      detail = s.busy || newReply?.generating ? "Gemini is generating the track" : "waiting for the reply";
    }
    if (Date.now() - lastProgress >= 10000) {
      lastProgress = Date.now();
      ctx.progress(`Gemini is composing… ${Math.round((Date.now() - started) / 1000)}s (${detail})`);
    }
    await sleep(2000, signal);
  }
  return { type: "timeout", detail };
}
