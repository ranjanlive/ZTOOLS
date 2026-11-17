// ZPilot Browser content module for gemini.google.com (music generation). Stateless: the
// service worker drives the timing and calls state() repeatedly. All page-specific selectors and
// texts live in SEL / TEXT so a Gemini redesign only needs edits here.

(() => {
  if (globalThis.ZB_gemini) return;

  const SEL = {
    composer: ["rich-textarea .ql-editor[contenteditable='true']", ".ql-editor[contenteditable='true']", "div[contenteditable='true'][role='textbox']"],
    send: ["button[aria-label='Send message']", "button.send-button", "button[aria-label*='Send']"],
    stop: ["button[aria-label='Stop response']", "button[aria-label*='Stop']"],
    tools: ["button[aria-label='Upload and tools']", "button[aria-label*='tools']", "button[aria-label*='Upload']"],
    response: "model-response",
    track: "generated-music",
    menuItem: ".cdk-overlay-pane [role='menuitem'], .cdk-overlay-pane [role='menuitemradio'], .cdk-overlay-pane button, .cdk-overlay-pane [role='option']",
    login: ["a[href*='ServiceLogin']", "a[href*='accounts.google.com/']", "button[aria-label='Sign in']"],
  };
  const TEXT = {
    musicSelected: /^deselect music$/i,
    moreTools: /^more tools$/i,
    createMusic: /^create music$/i,
    generating: /generating your track|creating your (song|track|music)|composing/i,
  };

  const clean = (s) => (s || "").replace(/\s+/g, " ").trim();
  const visible = (el) => !!el && el.isConnected && (typeof el.checkVisibility !== "function" || el.checkVisibility());
  const wait = (ms) => new Promise((r) => setTimeout(r, ms));

  function first(list, root = document) {
    for (const sel of list) for (const el of root.querySelectorAll(sel)) if (visible(el)) return el;
    return null;
  }

  function buttonByText(re, root = document) {
    for (const b of root.querySelectorAll("button, [role='button']")) {
      const label = clean(b.getAttribute("aria-label") || "") || clean(b.innerText);
      if (re.test(label) && visible(b)) return b;
    }
    return null;
  }

  function menuItems() {
    return [...document.querySelectorAll(SEL.menuItem)].filter(visible);
  }

  async function waitFor(fn, ms = 4000) {
    const end = Date.now() + ms;
    for (;;) {
      const v = fn();
      if (v) return v;
      if (Date.now() > end) return null;
      await wait(120);
    }
  }

  const composer = () => first(SEL.composer);
  const composerText = (box = composer()) => (box ? clean(box.innerText) : "");
  const musicMode = () => !!buttonByText(TEXT.musicSelected);

  function tracks() {
    return [...document.querySelectorAll(SEL.track)].map((el, index) => {
      const v = el.querySelector("video, audio");
      return {
        index,
        ready: !!v && v.readyState >= 1 && Number.isFinite(v.duration) && v.duration > 0,
        duration: v && Number.isFinite(v.duration) ? Math.round(v.duration * 10) / 10 : 0,
        width: v?.videoWidth || 0,
        height: v?.videoHeight || 0,
        hasDownload: !!el.querySelector("button[aria-label*='Download']"),
      };
    });
  }

  function state() {
    const responses = document.querySelectorAll(SEL.response);
    const last = responses[responses.length - 1] || null;
    const lastText = last ? clean(last.innerText).slice(0, 6000) : "";
    const alerts = [...document.querySelectorAll("[role='alert'], snack-bar-container, .mat-mdc-snack-bar-label, simple-snack-bar")]
      .filter(visible)
      .map((e) => clean(e.innerText))
      .filter(Boolean)
      .slice(0, 5);
    return {
      url: location.href,
      title: document.title.replace(/\s*-\s*Google Gemini\s*$/i, ""),
      composer: !!composer(),
      composerText: composerText(),
      busy: !!first(SEL.stop),
      musicMode: musicMode(),
      loginRequired: !composer() && !!first(SEL.login),
      responses: responses.length,
      tracks: tracks(),
      lastReply: last ? { text: lastText, generating: TEXT.generating.test(lastText.slice(0, 300)), hasTrack: !!last.querySelector(SEL.track) } : null,
      alerts,
    };
  }

  // Turns on the "Create music" tool: Upload and tools → More tools → Create music.
  async function openMusicMode() {
    if (musicMode()) return { ok: true, already: true };
    const tools = first(SEL.tools);
    if (!tools) return { ok: false, reason: "the Upload and tools button was not found" };
    tools.click();
    const more = await waitFor(() => menuItems().find((b) => TEXT.moreTools.test(clean(b.innerText))));
    if (more) {
      more.click();
      await wait(250);
    }
    const create = await waitFor(() => menuItems().find((b) => TEXT.createMusic.test(clean(b.innerText).split(/\n/)[0])) || buttonByText(TEXT.createMusic));
    if (!create) {
      document.body.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      return { ok: false, reason: "Create music is not offered in the tools menu (the account or region may not have it)" };
    }
    create.click();
    const on = await waitFor(musicMode, 5000);
    return on ? { ok: true } : { ok: false, reason: "music mode did not turn on" };
  }

  // Chooses an option in one of the music chips (Length, Vocals, Genre).
  async function setOption(kind, value) {
    // The chips render a moment after music mode turns on.
    const chip = await waitFor(
      () => buttonByText(new RegExp(`^${kind}$`, "i")) || [...document.querySelectorAll("button")].find((b) => visible(b) && clean(b.innerText).toLowerCase().startsWith(kind.toLowerCase())),
      8000
    );
    if (!chip) return { ok: false, reason: `the ${kind} button was not found`, options: [] };
    chip.click();
    const items = await waitFor(() => {
      const list = menuItems().filter((b) => b.getAttribute("role") === "menuitemradio" || b.getAttribute("role") === "menuitem");
      return list.length ? list : null;
    });
    if (!items) return { ok: false, reason: `the ${kind} menu did not open`, options: [] };
    const options = items.map((b) => clean(b.innerText).split(/ (?=Based on)/)[0]);
    const want = String(value).trim().toLowerCase();
    const pick = items.find((b, i) => options[i].toLowerCase() === want) || items.find((b, i) => options[i].toLowerCase().startsWith(want)) || items.find((b, i) => options[i].toLowerCase().includes(want));
    if (!pick) {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      chip.click();
      return { ok: false, reason: `no ${kind} option matches "${value}"`, options };
    }
    const chosen = options[items.indexOf(pick)];
    pick.click();
    await wait(200);
    if (menuItems().length) document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    return { ok: true, chosen, options };
  }

  function setText(box, text) {
    box.focus();
    const sel = window.getSelection();
    const range = document.createRange();
    range.selectNodeContents(box);
    sel.removeAllRanges();
    sel.addRange(range);
    if (!document.execCommand("insertText", false, text) || !composerText(box)) {
      const p = document.createElement("p");
      p.textContent = text;
      box.replaceChildren(p);
      box.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText", data: text }));
    }
  }

  async function submit(prompt) {
    const box = composer();
    if (!box) return { entered: false, sent: false, reason: "composer_missing" };
    setText(box, prompt);
    await wait(250);
    if (!composerText()) return { entered: false, sent: false, reason: "text_not_entered" };
    return clickSend();
  }

  async function clickSend() {
    const btn = await waitFor(() => {
      const b = first(SEL.send);
      return b && !b.disabled && b.getAttribute("aria-disabled") !== "true" ? b : null;
    }, 6000);
    if (btn) {
      btn.click();
      return { entered: true, sent: true };
    }
    const box = composer();
    if (!box) return { entered: true, sent: false, reason: "composer_missing" };
    box.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", code: "Enter", keyCode: 13, which: 13, bubbles: true, cancelable: true }));
    return { entered: true, sent: true, method: "enter" };
  }

  function focusComposer() {
    const box = composer();
    if (!box) return false;
    box.focus();
    const range = document.createRange();
    range.selectNodeContents(box);
    const sel = window.getSelection();
    sel.removeAllRanges();
    sel.addRange(range);
    return true;
  }

  // Opens the Download menu of track `index` and clicks the entry whose text matches `which`
  // ("audio" → "Audio only / MP3 track", "video" → "Video / Audio with cover art").
  async function clickDownload(index, which) {
    const el = document.querySelectorAll(SEL.track)[index];
    if (!el) return { ok: false, reason: "track not found" };
    el.scrollIntoView({ block: "center", behavior: "instant" });
    el.dispatchEvent(new MouseEvent("mouseover", { bubbles: true }));
    el.dispatchEvent(new MouseEvent("mouseenter", { bubbles: true }));
    const btn = el.querySelector("button[aria-label*='Download']");
    if (!btn) return { ok: false, reason: "the track has no Download button" };
    const re = which === "video" ? /^video|cover art/i : /audio only|mp3/i;
    const find = () => menuItems().find((b) => re.test(clean(b.innerText)));
    // The button toggles the menu: only click it when the menu is not already open.
    if (!find()) btn.click();
    let item = await waitFor(find, 4000);
    if (!item) {
      btn.click();
      item = await waitFor(find, 4000);
    }
    if (!item) {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
      return { ok: false, reason: `the ${which} download option was not found` };
    }
    item.click();
    return { ok: true, option: clean(item.innerText) };
  }

  function blobToBase64(blob) {
    return new Promise((resolve, reject) => {
      const reader = new FileReader();
      reader.onload = () => resolve(String(reader.result).split(",", 2)[1] || "");
      reader.onerror = () => reject(reader.error);
      reader.readAsDataURL(blob);
    });
  }

  // Captures the track's cover art: one frame of its visual (the video is CORS-enabled).
  async function captureCover(index, atSec = 1.5) {
    const el = document.querySelectorAll(SEL.track)[index];
    // Gemini loads a track's media only when it is on screen.
    el?.scrollIntoView({ block: "center", behavior: "instant" });
    const v = await waitFor(() => {
      const x = el?.querySelector("video");
      return x && x.videoWidth ? x : null;
    }, 10000);
    if (!v) return { ok: false, reason: "the track has no visual" };
    const t = Math.min(atSec, Math.max(0, (v.duration || 2) - 0.1));
    const was = v.currentTime;
    if (Math.abs(v.currentTime - t) > 0.05) {
      await new Promise((resolve) => {
        const done = () => (v.removeEventListener("seeked", done), resolve());
        v.addEventListener("seeked", done);
        v.currentTime = t;
        setTimeout(done, 3000);
      });
    }
    try {
      const canvas = new OffscreenCanvas(v.videoWidth, v.videoHeight);
      canvas.getContext("2d").drawImage(v, 0, 0);
      const blob = await canvas.convertToBlob({ type: "image/png" });
      return { ok: true, data: await blobToBase64(blob), mime: "image/png", width: v.videoWidth, height: v.videoHeight, bytes: blob.size };
    } catch (e) {
      return { ok: false, reason: `cover capture failed: ${e?.message || e}` };
    } finally {
      try {
        v.currentTime = was;
      } catch {}
    }
  }

  globalThis.ZB_gemini = { state, openMusicMode, setOption, submit, clickSend, focusComposer, clickDownload, captureCover };
})();
