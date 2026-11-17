// generate_image: drives ChatGPT in a ZPilot tab and returns the finished image(s).
// Jobs are queued because they share one ChatGPT tab and one account's limits. Continuing the
// same conversation (conversation_url) keeps a series consistent; reference_images are uploaded
// with the prompt (used when a series moves to a new chat, or for user-provided references).

import { CONTENT } from "../config.js";
import { attach, detach, insertText, send } from "../cdp.js";
import { callContent } from "../inject.js";
import { addToGroup, closeTab, focusTab, navigate, openTab, tabExists } from "../tabs.js";
import { ToolError, pollUntil, sleep, throwIfAborted, truncate } from "../util.js";

const HOME = "https://chatgpt.com/";
const TEMPORARY = "https://chatgpt.com/?temporary-chat=true";
const MIME = { png: "image/png", jpeg: "image/jpeg", webp: "image/webp" };

// A sharp image must keep the same src this long before it is accepted as final.
const STABLE_MS = 2500;
// Used when the Stop button never disappears (or the image never reports "loaded").
const STABLE_SLOW_MS = 8000;
// A finished text reply is classified after it stopped changing for this long.
const REPLY_SETTLE_MS = 5000;
const TEXT_ONLY_SETTLE_MS = 15000;

const LIMIT_PATTERNS = [
  /rate.?limit/i,
  /reached (your|the)[\w\s]{0,40}limit/i,
  /hit (your|the)[\w\s]{0,40}limit/i,
  /(image|generation|usage) limit/i,
  /too many requests/i,
  /try again (later|in \d|after)/i,
  /come back (later|in \d|after)/i,
  /upgrade to (plus|pro|go|team)/i,
];
const LOGIN_PATTERNS = [/log ?in to (create|generate|edit) images?/i, /sign ?in to (create|generate)/i];
const REFUSAL_PATTERNS = [
  /\b(can['’]?t|cannot|can not|unable to|not able to|won['’]?t be able to|am not able to)\b[^.]{0,100}\b(create|generate|make|produce|draw|depict|render|help with)\b/i,
  /\b(content|usage) polic(y|ies)\b/i,
  /\bviolat(e|es|ing) (our|the|openai['’]?s?) (policies|guidelines|terms)\b/i,
  /\bnot (allowed|permitted) to (create|generate|make)\b/i,
  /\b(likeness of|real (person|people)|public figures?)\b/i,
  /\bI['’]m (sorry|unable)\b/i,
];
const ERROR_PATTERNS = [/something went wrong/i, /network error/i, /an error occurred/i, /error generating/i];

const IMAGE_WORDS = /\b(image|picture|photo|photograph|illustration|drawing|draw|paint|painting|render|logo|icon|poster|wallpaper|sketch|art|artwork|portrait|banner|thumbnail|sticker|diagram)\b/i;

let queue = Promise.resolve();
let pending = 0;
let chatTabId = null;

const match = (patterns, text) => {
  if (!text) return null;
  for (const re of patterns) {
    const m = text.match(re);
    if (m) return m[0];
  }
  return null;
};

export default {
  name: "generate_image",
  title: "Generate image with ChatGPT",
  description:
    "Generates an image with ChatGPT in the user's signed-in Chrome and returns it base64-encoded. " +
    "reference_images (absolute file paths) are uploaded with the prompt; aspect adds an aspect-ratio instruction. " +
    "Fails with a clear error code when ChatGPT refuses (IMAGE_REFUSED), answers with text instead of an image (TEXT_INSTEAD_OF_IMAGE), " +
    "hits a usage limit (RATE_LIMITED) or is not signed in (LOGIN_REQUIRED); ChatGPT's reply is included in error.details.reply_text. " +
    "To edit an image, pass the conversation_url returned by the previous call with a follow-up prompt. Calls run one at a time.",
  defaultTimeoutSec: 300,
  inputSchema: {
    type: "object",
    properties: {
      prompt: { type: "string", description: "Description of the image to create, or the edit to make in a continued conversation." },
      reference_images: { type: "array", items: { type: "string" }, maxItems: 10, description: "Absolute paths of images to upload with the prompt (style or character references)." },
      aspect: { type: "string", description: "Aspect ratio or orientation, e.g. 1:1, 16:9, 9:16, 4:5, square, portrait, landscape." },
      format: { type: "string", enum: ["png", "jpeg", "webp"], default: "png", description: "Encoding of the returned image." },
      new_chat: { type: "boolean", default: true, description: "Start a fresh conversation. Ignored when conversation_url is given." },
      conversation_url: { type: "string", description: "An existing https://chatgpt.com/c/... conversation to continue (for edits and variations)." },
      temporary_chat: { type: "boolean", default: false, description: "Use a ChatGPT temporary chat that is not saved to the history." },
      ensure_image_intent: { type: "boolean", default: true, description: "Prefix the prompt with \"Generate an image:\" when it does not mention an image." },
      max_images: { type: "integer", minimum: 1, maximum: 4, default: 1, description: "Maximum number of images to return when ChatGPT produces several." },
      background: { type: "boolean", default: true, description: "Work in a background tab without taking focus." },
      timeout_sec: { type: "integer", minimum: 30, maximum: 900, default: 300, description: "Overall time limit including time spent queued." },
    },
    required: ["prompt"],
  },

  label: (args) => truncate(args.prompt, 80),
  summarize: (args, result) => `${result.images.length} image(s), ${result.images[0]?.width}×${result.images[0]?.height}`,

  run(args, ctx) {
    const ahead = pending++;
    if (ahead > 0) ctx.progress(`Queued behind ${ahead} other image job(s)…`);
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
  const deadline = ctx.deadline - 2000;
  if (args.conversation_url && !/^https:\/\/chatgpt\.com\//.test(args.conversation_url)) {
    throw new ToolError("INVALID_ARGS", "conversation_url must be a https://chatgpt.com/ conversation URL.");
  }

  ctx.progress("Opening ChatGPT…");
  const tabId = await prepareTab(args, ctx);
  const state = () => callContent(tabId, CONTENT.chatgpt, "ZB_chatgpt", "state", [], { timeoutMs: 15000 });

  const ready = await pollUntil(
    async () => {
      const s = await state();
      if (s.blocked) throw new ToolError("CHALLENGE", "ChatGPT is showing a browser verification page. Open the ZPilot Browser tab group, complete it, then retry.");
      if (s.loginRequired) throw new ToolError("LOGIN_REQUIRED", "ChatGPT is not signed in. Sign in at chatgpt.com in this Chrome profile and retry.");
      return s.composer && !s.busy ? s : null;
    },
    { timeoutMs: Math.min(45000, deadline - Date.now()), intervalMs: 700, signal }
  );
  if (!ready) throw new ToolError("CHATGPT_NOT_READY", "The ChatGPT message box did not become available (is a previous reply still running?).");

  const baseline = { turns: ready.turns, assistantCount: ready.assistantCount, srcs: new Set(ready.images.map((i) => i.src)) };
  let prompt = args.ensure_image_intent && !IMAGE_WORDS.test(args.prompt) ? `Generate an image: ${args.prompt}` : args.prompt;
  if (args.aspect) prompt += `

Aspect ratio: ${aspectText(args.aspect)}.`;
  let uploadWarning = "";
  if (args.reference_images?.length) {
    ctx.progress(`Uploading ${args.reference_images.length} reference image(s)…`);
    uploadWarning = await uploadReferences(tabId, args.reference_images, ctx);
  }

  ctx.progress("Submitting the prompt…");
  await submit(tabId, prompt, baseline, state, signal);

  const verdict = await watch(tabId, baseline, args, ctx, state, deadline);
  const conversationUrl = (await tabExists(tabId))?.url || "";
  const details = { reply_text: verdict.text ? truncate(verdict.text, 2000) : undefined, conversation_url: conversationUrl };

  switch (verdict.type) {
    case "images":
      break;
    case "refused":
      throw new ToolError("IMAGE_REFUSED", `ChatGPT refused to create this image: "${truncate(verdict.text, 400)}". Rephrase the prompt (for example remove real people, brands or logos) instead of retrying it unchanged.`, details);
    case "text":
      throw new ToolError("TEXT_INSTEAD_OF_IMAGE", `ChatGPT answered with text instead of an image: "${truncate(verdict.text, 400)}"`, details);
    case "limit":
      throw new ToolError("RATE_LIMITED", `ChatGPT reports a usage limit: "${truncate(verdict.text, 300)}"`, details);
    case "login":
      throw new ToolError("LOGIN_REQUIRED", `ChatGPT requires sign-in to create images: "${truncate(verdict.text, 200)}"`, details);
    case "error":
      throw new ToolError("CHATGPT_ERROR", `ChatGPT reported an error: "${truncate(verdict.text, 300)}"`, details);
    default:
      throw new ToolError("TIMEOUT", `No finished image appeared before the time limit (last state: ${verdict.detail}).`, details);
  }

  ctx.progress(`Image ready — downloading ${verdict.images.length} file(s)…`);
  const images = [];
  for (const img of verdict.images) {
    const file = await callContent(tabId, CONTENT.chatgpt, "ZB_chatgpt", "fetchImage", [img.src, MIME[args.format]], { timeoutMs: 60000 });
    if (!file.ok) throw new ToolError("DOWNLOAD_FAILED", `The image was generated but could not be downloaded: ${file.reason}.`, details);
    images.push({
      data: file.data,
      mime: file.mime,
      width: file.width,
      height: file.height,
      bytes: file.bytes,
      ...(file.warning ? { warning: file.warning } : {}),
    });
  }

  return {
    images,
    prompt_sent: prompt,
    ...(args.reference_images?.length ? { references_uploaded: args.reference_images.length } : {}),
    ...(uploadWarning ? { warning: uploadWarning } : {}),
    conversation_url: conversationUrl,
    reply_text: verdict.text ? truncate(verdict.text, 1000) : "",
    duration_sec: Math.round((Date.now() - started) / 100) / 10,
  };
}

async function prepareTab(args, ctx) {
  const { signal } = ctx;
  const navTimeout = 45000;

  if (args.conversation_url) {
    const same = (await chrome.tabs.query({ url: "https://chatgpt.com/*" })).find((t) => sameChat(t.url, args.conversation_url));
    if (same) {
      chatTabId = same.id;
      if (!args.background) await focusTab(same.id);
      return same.id;
    }
  }

  let tab = await tabExists(chatTabId);
  if (!tab || !tab.url?.startsWith("https://chatgpt.com")) {
    // Recover the tab after a service-worker restart.
    const { chatTabId: stored } = await chrome.storage.session.get("chatTabId");
    tab = await tabExists(stored);
    if (!tab || !tab.url?.startsWith("https://chatgpt.com")) tab = null;
  }

  const target = args.conversation_url || (args.temporary_chat ? TEMPORARY : args.new_chat ? HOME : null);
  if (!tab) {
    const tabId = await openTab({ kind: "tools", active: !args.background });
    try {
      await navigate(tabId, target || HOME, { timeoutMs: navTimeout, signal });
    } catch (e) {
      await closeTab(tabId);
      throw e;
    }
    remember(tabId);
    return tabId;
  }

  remember(tab.id);
  addToGroup(tab.id, "tools").catch(() => {});
  if (target) await navigate(tab.id, target, { timeoutMs: navTimeout, signal });
  if (!args.background) await focusTab(tab.id);
  return tab.id;
}

function remember(tabId) {
  chatTabId = tabId;
  chrome.storage.session.set({ chatTabId: tabId }).catch(() => {});
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
  let res = await callContent(tabId, CONTENT.chatgpt, "ZB_chatgpt", "submit", [prompt]);
  if (!res.entered) {
    // The page ignored synthetic editing; type with trusted input events instead.
    const focused = await callContent(tabId, CONTENT.chatgpt, "ZB_chatgpt", "focusComposer", []);
    if (!focused) throw new ToolError("SUBMIT_FAILED", "Could not find the ChatGPT message box.");
    await attach(tabId);
    try {
      await insertText(tabId, prompt);
    } finally {
      await detach(tabId);
    }
    await sleep(300, signal);
    res = await callContent(tabId, CONTENT.chatgpt, "ZB_chatgpt", "clickSend", []);
  }
  if (!res.sent) throw new ToolError("SUBMIT_FAILED", `Could not send the prompt (${res.reason || "unknown reason"}).`);

  const sent = await pollUntil(
    async () => {
      const s = await state();
      return s.busy || s.turns > baseline.turns || s.assistantCount > baseline.assistantCount || (!s.composerText && s.composer);
    },
    { timeoutMs: 15000, intervalMs: 500, signal }
  );
  if (!sent) throw new ToolError("SUBMIT_FAILED", "ChatGPT did not accept the prompt (the send button may be disabled).");
}

async function watch(tabId, baseline, args, ctx, state, deadline) {
  const { signal } = ctx;
  let candidateKey = "";
  let stableSince = 0;
  let replyText = null;
  let replySince = 0;
  let lastProgress = 0;
  let detail = "waiting for the reply";
  const started = Date.now();

  while (Date.now() < deadline) {
    const s = await state();

    if (s.loginRequired) return { type: "login", text: "Signed out of ChatGPT during generation." };
    for (const alert of s.alerts) {
      if (match(LIMIT_PATTERNS, alert)) return { type: "limit", text: alert };
      if (match(LOGIN_PATTERNS, alert)) return { type: "login", text: alert };
      if (match(ERROR_PATTERNS, alert) && !s.busy) return { type: "error", text: alert };
    }

    const fresh = s.images.filter((i) => !baseline.srcs.has(i.src) && (i.turn < 0 || i.turn >= baseline.turns));
    const newReply = s.assistantCount > baseline.assistantCount ? s.lastReply : null;

    if (fresh.length) {
      // Only finished images: not blurred, and marked final when the page marks generated images.
      const sharp = fresh.filter((i) => !i.blurred && i.final !== false);
      const unique = [...new Map(sharp.map((i) => [i.src, i])).values()];
      const key = unique.map((i) => i.src).join("|");
      if (!unique.length || key !== candidateKey) {
        candidateKey = key;
        stableSince = Date.now();
      }
      const allLoaded = unique.every((i) => i.loaded);
      // While ChatGPT still works, previews may change: wait for it to finish (or a long stable spell).
      const need = s.busy ? STABLE_SLOW_MS * 3 : allLoaded ? STABLE_MS : STABLE_SLOW_MS;
      const stableMs = Date.now() - stableSince;
      if (unique.length && stableMs >= need) {
        return { type: "images", images: unique.slice(-args.max_images), text: newReply?.text || "" };
      }
      detail = `${fresh.length} new image(s), ${unique.length} sharp, loaded=${allLoaded}, busy=${s.busy}, stable ${Math.round(stableMs / 1000)}s`;
    } else if (newReply && !s.busy) {
      const text = newReply.text;
      if (text !== replyText) {
        replyText = text;
        replySince = Date.now();
      }
      const settled = Date.now() - replySince;
      detail = `text reply settled ${Math.round(settled / 1000)}s, no image`;
      if (text && !newReply.imageInProgress) {
        if (settled >= REPLY_SETTLE_MS) {
          if (match(LIMIT_PATTERNS, text)) return { type: "limit", text };
          if (match(LOGIN_PATTERNS, text)) return { type: "login", text };
          if (match(REFUSAL_PATTERNS, text)) return { type: "refused", text };
        }
        if (settled >= TEXT_ONLY_SETTLE_MS) {
          if (match(ERROR_PATTERNS, text)) return { type: "error", text };
          return { type: "text", text };
        }
      }
    } else {
      detail = s.busy ? "ChatGPT is still generating" : "waiting for the reply";
    }

    if (Date.now() - lastProgress >= 5000) {
      lastProgress = Date.now();
      ctx.progress(`ChatGPT is working… ${Math.round((Date.now() - started) / 1000)}s (${detail})`);
    }
    await sleep(1000, signal);
  }
  return { type: "timeout", detail };
}

const ASPECTS = { square: "1:1 (square)", portrait: "portrait (2:3)", landscape: "landscape (3:2)", wide: "16:9 (wide)", tall: "9:16 (tall)", story: "9:16 (vertical story)" };
function aspectText(a) {
  const k = String(a).trim().toLowerCase();
  return ASPECTS[k] || k;
}

// Uploads reference images through the page's own image file input (CDP DOM.setFileInputFiles)
// and waits until their previews appear. Returns a warning text when the upload looks incomplete.
async function uploadReferences(tabId, paths, ctx) {
  const chat = (method, args = []) => callContent(tabId, CONTENT.chatgpt, "ZB_chatgpt", method, args, { timeoutMs: 15000 });
  const before = await chat("attachments");
  const token = Math.random().toString(36).slice(2);
  if (!(await chat("markImageInput", [token]))) throw new ToolError("UPLOAD_FAILED", "ChatGPT's file upload input was not found.");
  await attach(tabId);
  try {
    const { result } = await send(tabId, "Runtime.evaluate", { expression: `document.querySelector('[data-zpilot-upload="${token}"]')` });
    if (!result?.objectId) throw new ToolError("UPLOAD_FAILED", "ChatGPT's file upload input could not be reached.");
    await send(tabId, "DOM.setFileInputFiles", { files: paths, objectId: result.objectId });
  } catch (e) {
    if (e instanceof ToolError && e.code !== "CDP_ERROR") throw e;
    throw new ToolError("UPLOAD_FAILED", `Uploading the reference images failed: ${e.message}. Check that the paths exist on this computer.`);
  } finally {
    await detach(tabId);
  }
  const done = await pollUntil(
    async () => {
      const a = await chat("attachments");
      return a.previews >= before.previews + paths.length && !a.uploading ? a : null;
    },
    { timeoutMs: 90000, intervalMs: 700, signal: ctx.signal }
  );
  if (!done) {
    const a = await chat("attachments").catch(() => ({}));
    throw new ToolError("UPLOAD_FAILED", `The reference images did not finish uploading within 90s (${a.previews ?? "?"} of ${before.previews + paths.length} attached${a.uploading ? ", still uploading" : ""}). Nothing was sent.`);
  }
  return "";
}
