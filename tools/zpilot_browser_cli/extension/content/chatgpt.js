// ZPilot Browser content module for chatgpt.com (image generation). Stateless: the service worker drives the timing and
// calls state() repeatedly, so background-tab timer throttling cannot slow detection down.
// All page-specific selectors live in SEL so a ChatGPT redesign only needs edits here.

(() => {
  if (globalThis.ZB_chatgpt) return;

  const SEL = {
    composer: [
      "div.ProseMirror[contenteditable='true']",
      "[contenteditable='true'][aria-label='Ask ChatGPT']",
      "#prompt-textarea",
      'div[contenteditable="true"][data-virtualkeyboard]',
      'form div[contenteditable="true"]',
      'textarea[name="prompt-textarea"]',
      "main form textarea",
    ],
    send: [
      'button[aria-label="Send"]',
      'button[data-testid="send-button"]',
      "#composer-submit-button",
      'button[aria-label="Send prompt"]',
      'button[aria-label*="Send"]',
      'form button[type="submit"]',
    ],
    stop: ['button[data-testid="stop-button"]', 'button[aria-label*="Stop streaming"]', 'button[aria-label*="Stop generating"]', 'button[aria-label^="Stop"]'],
    // 2026 layout: one [data-turn-key] per exchange (user bubble + reply); older layouts use
    // conversation-turn articles and message-author-role attributes.
    turn: '[data-turn-key], article[data-testid^="conversation-turn"], [data-testid^="conversation-turn-"]',
    assistant: '[data-message-author-role="assistant"]',
    user: '[data-user-message-bubble="true"], [data-message-author-role="user"]',
    generated: '[data-testid="generated-image-preview"] img, [data-testid="generated-image-gallery"] img',
    login: ['button[data-testid="login-button"]', '[data-testid="login-button"]', 'a[href*="/auth/login"]'],
    alerts: '[role="alert"], [data-testid*="toast"], .toast-root',
  };

  // Text shown while an image is still being produced (no <img> may exist yet).
  const IMAGE_IN_PROGRESS = /creating image|generating image|image creation|getting started|making (your|the) image|almost (done|there)|adding (the )?(final|details)/i;

  const visible = (el) => !!el && el.isConnected && (typeof el.checkVisibility !== "function" || el.checkVisibility());

  function first(list) {
    for (const sel of list) {
      for (const el of document.querySelectorAll(sel)) if (visible(el)) return el;
    }
    return null;
  }

  // The signed-out page has no stable test ids, so also look for visible "Log in" / "Sign up" buttons.
  const SIGNED_OUT_TEXT = /^(log ?in|sign ?up|sign up for free|log in or create account|log in or sign up)$/i;
  function signedOut() {
    if (first(SEL.login)) return true;
    for (const el of document.querySelectorAll("button, a[href]")) {
      if (SIGNED_OUT_TEXT.test((el.innerText || "").trim()) && visible(el)) return true;
    }
    return false;
  }

  const composer = () => first(SEL.composer);
  const sendButton = () => first(SEL.send);
  const stopButton = () => first(SEL.stop);

  function composerText(box = composer()) {
    if (!box) return "";
    if (box.tagName === "TEXTAREA") return (box.value || "").trim();
    return (box.innerText || box.textContent || "").trim();
  }

  function isBlurred(img) {
    let el = img;
    for (let depth = 0; el && depth < 5; depth++, el = el.parentElement) {
      const m = (getComputedStyle(el).filter || "").match(/blur\(([\d.]+)px\)/);
      if (m && parseFloat(m[1]) > 0) return true;
    }
    return false;
  }

  // Generated images. The 2026 layout marks them (generated-image-preview, alt "Generated image N");
  // other large images in a reply are a fallback for older layouts.
  function images(turns) {
    const out = [];
    const seen = new Set();
    const marked = new Set(document.querySelectorAll(SEL.generated));
    for (const img of document.querySelectorAll("img")) {
      const src = img.currentSrc || img.src;
      if (!src || src.startsWith("data:image/svg")) continue;
      if (img.closest(`nav, aside, header, form, ${SEL.user}`)) continue;
      const generated = marked.has(img) || /^generated image/i.test(img.alt || "");
      const rect = img.getBoundingClientRect();
      const size = Math.max(img.naturalWidth, img.naturalHeight, rect.width, rect.height);
      if (!generated && size > 0 && size < 160) continue; // avatars, icons, file chips
      if (marked.size && !generated) continue; // the page marks generated images: trust only those
      const turnEl = img.closest(SEL.turn);
      const key = src + "|" + isBlurred(img);
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({
        src,
        turn: turnEl ? turns.indexOf(turnEl) : -1,
        blurred: isBlurred(img),
        loaded: img.complete && img.naturalWidth > 0,
        final: generated && img.complete && img.naturalWidth >= 256,
        width: img.naturalWidth,
        height: img.naturalHeight,
      });
    }
    return out;
  }

  // The reply part of a turn: its text without the user's bubble.
  function replyOf(turn) {
    if (!turn) return null;
    const user = turn.querySelector(SEL.user);
    let text = (turn.innerText || "").trim();
    if (user) text = text.replace((user.innerText || "").trim(), "");
    text = text.replace(/^\s*(you said:|chatgpt said:)\s*/gim, "").trim();
    const hasReply = !!turn.querySelector(SEL.generated) || !!turn.querySelector(SEL.assistant) || text.length > 0;
    return hasReply ? text : null;
  }

  // A turn still streaming: an attribute ending in "turn-state" that is not "complete".
  function turnStreaming(turn) {
    if (!turn) return false;
    for (const el of [turn, ...turn.querySelectorAll("*")]) {
      for (const a of el.attributes) if (/turn-state$/.test(a.name) && a.value && a.value !== "complete") return true;
    }
    return false;
  }

  function state() {
    const turns = [...document.querySelectorAll(SEL.turn)].filter((t) => !t.parentElement?.closest(SEL.turn));
    const lastTurn = turns[turns.length - 1] || null;
    const replyText = replyOf(lastTurn);
    const assistants = document.querySelectorAll(SEL.assistant).length || turns.filter((t) => replyOf(t) !== null).length;
    const box = composer();
    return {
      url: location.href,
      title: document.title,
      composer: !!box,
      composerText: composerText(box),
      busy: !!stopButton() || turnStreaming(lastTurn),
      loginRequired: signedOut(),
      blocked: /just a moment|attention required|verify you are human/i.test(document.title),
      turns: turns.length,
      assistantCount: assistants,
      lastReply:
        replyText !== null
          ? { text: replyText.slice(0, 4000), imageInProgress: IMAGE_IN_PROGRESS.test(replyText.slice(0, 500)) || /\b\d{1,3}%/.test(replyText.slice(0, 200)) }
          : null,
      images: images(turns),
      alerts: [...document.querySelectorAll(SEL.alerts)]
        .filter(visible)
        .map((el) => (el.innerText || "").trim())
        .filter((t) => t && !/don.t have access to this conversation/i.test(t))
        .slice(0, 5),
    };
  }

  function setText(box, text) {
    box.focus();
    if (box.tagName === "TEXTAREA") {
      const setter = Object.getOwnPropertyDescriptor(HTMLTextAreaElement.prototype, "value")?.set;
      setter ? setter.call(box, text) : (box.value = text);
      box.dispatchEvent(new Event("input", { bubbles: true }));
      return;
    }
    const sel = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(box);
    sel.removeAllRanges();
    sel.addRange(range);
    const ok = document.execCommand("insertText", false, text);
    if (!ok || !composerText(box)) {
      const p = document.createElement("p");
      p.textContent = text;
      box.replaceChildren(p);
      box.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText", data: text }));
    }
  }

  function sendReady() {
    const btn = sendButton();
    return btn && !btn.disabled && btn.getAttribute("aria-disabled") !== "true" ? btn : null;
  }

  const wait = (ms) => new Promise((r) => setTimeout(r, ms));

  // Types the prompt and presses send. Returns {entered, sent, method}.
  async function submit(prompt) {
    const box = composer();
    if (!box) return { entered: false, sent: false, reason: "composer_missing" };
    setText(box, prompt);
    await wait(250);
    if (!composerText()) return { entered: false, sent: false, reason: "text_not_entered" };
    return clickSend();
  }

  async function clickSend() {
    const deadline = Date.now() + 6000;
    while (Date.now() < deadline) {
      const btn = sendReady();
      if (btn) {
        btn.click();
        return { entered: true, sent: true, method: "click" };
      }
      await wait(150);
    }
    const box = composer();
    if (!box) return { entered: true, sent: false, reason: "composer_missing" };
    box.focus();
    box.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", keyCode: 13, which: 13, bubbles: true, cancelable: true }));
    return { entered: true, sent: true, method: "enter" };
  }

  // Focuses the composer and selects its content so trusted typing replaces it.
  function focusComposer() {
    const box = composer();
    if (!box) return false;
    box.focus();
    if (box.tagName === "TEXTAREA") box.select();
    else {
      const range = document.createRange();
      range.selectNodeContents(box);
      const sel = window.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    }
    return true;
  }

  function blobToBase64(blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result).split(",", 2)[1] || "");
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(blob);
    });
  }

  // Downloads an image with the page's credentials and re-encodes it to `mime` when needed.
  async function fetchImage(src, mime) {
    let blob = null;
    let fetchError = "";
    try {
      const resp = await fetch(src, { credentials: "include" });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      blob = await resp.blob();
    } catch (e) {
      fetchError = String(e?.message || e);
    }
    if (!blob) {
      const img = [...document.images].find((i) => (i.currentSrc || i.src) === src && i.naturalWidth > 0 && i.complete);
      if (!img) return { ok: false, reason: `download failed (${fetchError})` };
      try {
        const canvas = new OffscreenCanvas(img.naturalWidth, img.naturalHeight);
        canvas.getContext("2d").drawImage(img, 0, 0);
        blob = await canvas.convertToBlob({ type: "image/png" });
      } catch (e) {
        return { ok: false, reason: `download failed (${fetchError}) and canvas copy failed (${e?.message || e})` };
      }
    }
    let bitmap;
    try {
      bitmap = await createImageBitmap(blob);
    } catch {
      return { ok: false, reason: `the downloaded file is not an image (${blob.type || "unknown type"})` };
    }
    const { width, height } = bitmap;
    let warning = "";
    if (mime && blob.type !== mime) {
      try {
        const canvas = new OffscreenCanvas(width, height);
        const ctx = canvas.getContext("2d");
        if (mime === "image/jpeg") {
          ctx.fillStyle = "#ffffff";
          ctx.fillRect(0, 0, width, height);
        }
        ctx.drawImage(bitmap, 0, 0);
        blob = await canvas.convertToBlob({ type: mime, quality: 0.95 });
      } catch (e) {
        warning = `could not convert to ${mime}: ${e?.message || e}`;
      }
    }
    bitmap.close();
    return { ok: true, data: await blobToBase64(blob), mime: blob.type, width, height, bytes: blob.size, warning };
  }

  // ---------- reference images (uploaded with CDP DOM.setFileInputFiles) ----------

  // Marks the file input that accepts images so the service worker can reach it through CDP.
  function markImageInput(token) {
    const inputs = [...document.querySelectorAll("input[type='file']")];
    const pick =
      inputs.find((i) => (i.accept || "").trim() === "image/*") ||
      inputs.find((i) => /image/.test(i.accept || "") && !/video/.test(i.accept || "")) ||
      inputs.find((i) => /image/.test(i.accept || "")) ||
      inputs.find((i) => !i.accept);
    if (!pick) return false;
    pick.setAttribute("data-zpilot-upload", token);
    return true;
  }

  // Attachments shown in the composer and whether any is still uploading. The 2026 layout keeps
  // them in [data-composer-attachments] (one role=button per file, a role=progressbar while uploading).
  function attachments() {
    const area = document.querySelector("[data-composer-attachments]") || composer()?.closest("form");
    if (!area) return { previews: 0, uploading: false };
    let previews = area.matches("[data-composer-attachments]") ? area.querySelectorAll("[role='button'][aria-label]").length : 0;
    if (!previews) {
      previews = [...area.querySelectorAll("img")].filter((img) => {
        const r = img.getBoundingClientRect();
        return r.width >= 24 && r.width <= 400;
      }).length;
    }
    const uploading = !!area.querySelector("[role='progressbar'], [class*='animate-spin'], circle-progress");
    return { previews, uploading };
  }

  globalThis.ZB_chatgpt = { state, submit, clickSend, focusComposer, fetchImage, markImageInput, attachments };
})();
