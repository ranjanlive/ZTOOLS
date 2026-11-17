// ZPilot Browser content module for zagent_in_chrome: accessibility-style page snapshots with element
// refs, element lookup, form filling and the on-page cursor / "controlled by ZPilot Browser" overlay.
// Clicks and typing are NOT done here: the service worker sends trusted CDP input events.

(() => {
  if (globalThis.ZB_agent) return;

  // Refs embed a per-document tag so a ref from a previous page can never hit a new element.
  const DOC_TAG = Array.from({ length: 3 }, () => "abcdefghijkmnpqrstuvwxyz"[Math.floor(Math.random() * 24)]).join("");
  const refs = new Map();
  const refOf = new WeakMap();
  let seq = 0;

  function refFor(el) {
    let id = refOf.get(el);
    if (!id) {
      id = `${DOC_TAG}${++seq}`;
      refOf.set(el, id);
      refs.set(id, new WeakRef(el));
    }
    return id;
  }

  function byRef(id) {
    const el = refs.get(String(id || "").trim())?.deref();
    if (el && el.isConnected) return el;
    const err = new Error(
      String(id || "").startsWith(DOC_TAG)
        ? `Element ${id} is no longer on the page. Run read_page or find again.`
        : `Unknown ref "${id}" — it belongs to an earlier page. Run read_page or find to get current refs.`
    );
    err.code = "REF_NOT_FOUND";
    throw err;
  }

  const clean = (s) => (s || "").replace(/\s+/g, " ").trim();
  const cut = (s, n) => (s.length > n ? s.slice(0, n - 1) + "…" : s);
  const quote = (s) => JSON.stringify(s);

  // ---------- Roles and names ----------

  const INTERACTIVE = new Set([
    "button", "link", "textbox", "searchbox", "checkbox", "radio", "combobox", "listbox", "option", "menuitem",
    "menuitemcheckbox", "menuitemradio", "tab", "switch", "slider", "spinbutton", "treeitem",
  ]);
  const LANDMARKS = new Set(["navigation", "main", "banner", "contentinfo", "complementary", "form", "search", "dialog", "alertdialog", "region"]);
  const INPUT_ROLES = {
    checkbox: "checkbox", radio: "radio", range: "slider", number: "spinbutton", search: "searchbox",
    submit: "button", button: "button", reset: "button", image: "button", file: "button", color: "button",
  };

  function roleOf(el) {
    const explicit = el.getAttribute("role");
    if (explicit) return explicit.split(/\s+/)[0];
    const tag = el.localName;
    switch (tag) {
      case "a":
        return el.hasAttribute("href") ? "link" : null;
      case "button":
      case "summary":
        return "button";
      case "input": {
        const type = (el.getAttribute("type") || "text").toLowerCase();
        if (type === "hidden") return null;
        return INPUT_ROLES[type] || "textbox";
      }
      case "select":
        return el.multiple || el.size > 1 ? "listbox" : "combobox";
      case "textarea":
        return "textbox";
      case "option":
        return "option";
      case "img":
        return el.getAttribute("alt") ? "img" : null;
      case "h1": case "h2": case "h3": case "h4": case "h5": case "h6":
        return "heading";
      case "nav":
        return "navigation";
      case "main":
        return "main";
      case "form":
        return "form";
      case "dialog":
        return "dialog";
      case "table":
        return "table";
      case "iframe":
        return "iframe";
      case "video":
      case "audio":
        return tag;
      case "label":
        return "label";
    }
    if (el.isContentEditable && !el.parentElement?.isContentEditable) return "textbox";
    return null;
  }

  function textOfIds(ids) {
    return ids
      .split(/\s+/)
      .map((id) => document.getElementById(id))
      .filter(Boolean)
      .map((n) => clean(n.innerText || n.textContent))
      .join(" ");
  }

  function nameOf(el, role) {
    const labelledby = el.getAttribute("aria-labelledby");
    if (labelledby) {
      const t = textOfIds(labelledby);
      if (t) return t;
    }
    const aria = el.getAttribute("aria-label");
    if (aria && aria.trim()) return clean(aria);
    const tag = el.localName;
    if (tag === "input" || tag === "textarea" || tag === "select") {
      const label = el.labels?.[0];
      if (label) {
        const t = clean(label.innerText || label.textContent);
        if (t) return t;
      }
      if (tag === "input" && /^(submit|button|reset)$/i.test(el.type)) return clean(el.value);
      if (tag === "input" && el.type === "image") return clean(el.alt);
      return clean(el.getAttribute("placeholder") || el.getAttribute("title") || el.getAttribute("name") || "");
    }
    if (tag === "img") return clean(el.getAttribute("alt") || el.getAttribute("title"));
    if (tag === "iframe") return clean(el.getAttribute("title") || el.getAttribute("name") || "");
    if (role && (INTERACTIVE.has(role) || role === "heading" || role === "dialog" || role === "label")) {
      let t = clean(el.innerText || el.textContent);
      if (!t) {
        const img = el.querySelector("img[alt], svg[aria-label], [aria-label]");
        t = clean(img?.getAttribute("alt") || img?.getAttribute("aria-label") || "");
      }
      if (t) return t;
    }
    return clean(el.getAttribute("title") || "");
  }

  function hasPointer(el) {
    if (!el.parentElement) return false;
    const style = getComputedStyle(el);
    return style.cursor === "pointer" && getComputedStyle(el.parentElement).cursor !== "pointer";
  }

  function isInteractive(el, role) {
    if (role && INTERACTIVE.has(role)) return true;
    if (el.hasAttribute("onclick") || el.getAttribute("tabindex") === "0") return true;
    if (el.isContentEditable && !el.parentElement?.isContentEditable) return true;
    const tag = el.localName;
    if (tag === "div" || tag === "span" || tag === "li" || tag === "img" || tag === "i" || tag === "svg") return hasPointer(el);
    return false;
  }

  function visible(el) {
    if (typeof el.checkVisibility === "function" && !el.checkVisibility({ checkVisibilityCSS: true, visibilityProperty: true })) {
      return getComputedStyle(el).display === "contents";
    }
    return true;
  }

  // Offset of an element's document relative to the top window (same-origin iframes only).
  function frameOffset(el) {
    let x = 0;
    let y = 0;
    let win = el.ownerDocument.defaultView;
    while (win && win !== window && win.frameElement) {
      const r = win.frameElement.getBoundingClientRect();
      x += r.left + win.frameElement.clientLeft;
      y += r.top + win.frameElement.clientTop;
      win = win.parent;
    }
    return { x, y };
  }

  function rectOf(el) {
    let r = el.getBoundingClientRect();
    if (!r.width || !r.height) {
      const first = el.getClientRects()[0];
      if (first) r = first;
    }
    const o = frameOffset(el);
    return { x: r.left + o.x, y: r.top + o.y, w: r.width, h: r.height };
  }

  function inViewport(r) {
    return r.x + r.w > 0 && r.y + r.h > 0 && r.x < innerWidth && r.y < innerHeight;
  }

  function stateAttrs(el, role) {
    const parts = [];
    const tag = el.localName;
    if (role === "link") {
      const href = el.getAttribute("href") || "";
      if (href && !href.startsWith("javascript:")) {
        let shown = href;
        try {
          const u = new URL(href, document.baseURI);
          shown = u.origin === location.origin ? u.pathname + u.search + u.hash : u.href;
        } catch {}
        parts.push(`href=${quote(cut(shown, 90))}`);
      }
    }
    if (tag === "input" || tag === "textarea") {
      const type = (el.getAttribute("type") || "").toLowerCase();
      if (type && type !== "text" && (!INPUT_ROLES[type] || type === "file")) parts.push(`type=${type}`);
      if (type === "checkbox" || type === "radio") {
        if (el.checked) parts.push("checked");
      } else if (el.value) {
        parts.push(`value=${quote(type === "password" ? "••••" : cut(el.value, 60))}`);
      } else if (el.placeholder) {
        parts.push(`placeholder=${quote(cut(el.placeholder, 50))}`);
      }
    }
    if (tag === "select") {
      const opt = el.selectedOptions?.[0];
      if (opt) parts.push(`value=${quote(cut(clean(opt.text), 50))}`);
      parts.push(`options=${el.options.length}`);
    }
    if (el.isContentEditable && role === "textbox") {
      const t = clean(el.innerText);
      if (t) parts.push(`value=${quote(cut(t, 60))}`);
    }
    for (const attr of ["aria-checked", "aria-selected", "aria-expanded", "aria-pressed", "aria-current"]) {
      const v = el.getAttribute(attr);
      if (v && v !== "false") parts.push(`${attr.slice(5)}=${v}`);
    }
    if (role === "heading") parts.push(`level=${el.localName.match(/^h(\d)$/)?.[1] || el.getAttribute("aria-level") || "?"}`);
    if (el.disabled || el.getAttribute("aria-disabled") === "true") parts.push("disabled");
    if (el.required) parts.push("required");
    return parts;
  }

  function describe(el) {
    const role = roleOf(el) || el.localName;
    const name = nameOf(el, role);
    return `${role}${name ? " " + quote(cut(name, 60)) : ""}`;
  }

  // ---------- Snapshot ----------

  function snapshot({ filter = "interactive", maxChars = 25000, viewportOnly = false, ref } = {}) {
    const root = ref ? byRef(ref) : document.body || document.documentElement;
    const lines = [];
    let length = 0;
    let truncated = false;
    let interactiveCount = 0;
    const all = filter === "all";

    const emit = (depth, text) => {
      if (truncated) return;
      const line = "  ".repeat(Math.min(depth, 12)) + text;
      if (length + line.length + 1 > maxChars) {
        truncated = true;
        return;
      }
      lines.push(line);
      length += line.length + 1;
    };

    const walk = (node, depth) => {
      if (truncated) return;
      const kids = node.shadowRoot ? node.shadowRoot.children : node.children;
      for (const el of kids) {
        if (truncated) return;
        const tag = el.localName;
        if (tag === "script" || tag === "style" || tag === "noscript" || tag === "template" || tag === "zpilot-overlay" || tag === "head") continue;
        if (!visible(el)) continue;

        const role = roleOf(el);
        let nextDepth = depth;

        if (tag === "iframe") {
          let doc = null;
          try {
            doc = el.contentDocument;
          } catch {}
          const r = rectOf(el);
          if (viewportOnly && !inViewport(r)) continue;
          if (doc?.body) {
            emit(depth, `[${refFor(el)}] iframe ${quote(cut(nameOf(el, "iframe") || el.src || "", 60))}`);
            walk(doc.body, depth + 1);
          } else {
            emit(depth, `[${refFor(el)}] iframe ${quote(cut(nameOf(el, "iframe") || el.src || "", 60))} (cross-origin: use screenshot + coordinates inside it)`);
          }
          continue;
        }

        const interactive = isInteractive(el, role);
        if (interactive) {
          const r = rectOf(el);
          if (r.w > 0 && r.h > 0 && (!viewportOnly || inViewport(r))) {
            const name = nameOf(el, role);
            const attrs = stateAttrs(el, role);
            if (!inViewport(r)) attrs.push("offscreen");
            emit(depth, `[${refFor(el)}] ${role || "clickable"}${name ? " " + quote(cut(name, 80)) : ""}${attrs.length ? " " + attrs.join(" ") : ""}`);
            interactiveCount++;
          }
          // Do not descend into simple controls; their text is already in the name.
          if (role === "button" || role === "link" || role === "textbox" || role === "combobox" || role === "option" || tag === "select") {
            if (!el.querySelector("input, select, textarea, button, a[href]")) continue;
          }
          nextDepth = depth + 1;
        } else if (all) {
          const r = rectOf(el);
          const show = !viewportOnly || inViewport(r);
          if (role === "heading" && show) {
            emit(depth, `heading ${quote(cut(nameOf(el, role), 100))} ${stateAttrs(el, role).join(" ")}`);
            continue;
          }
          if (role === "img" && show) {
            emit(depth, `img ${quote(cut(nameOf(el, role), 80))}`);
            continue;
          }
          if (role && LANDMARKS.has(role) && show) {
            const name = nameOf(el, role);
            emit(depth, `${role}${name ? " " + quote(cut(name, 60)) : ""}`);
            nextDepth = depth + 1;
          } else if (show) {
            const own = ownText(el);
            if (own.length > 1) emit(depth, `text ${quote(cut(own, 160))}`);
          }
        }
        walk(el, nextDepth);
      }
    };

    walk(root, 0);
    const doc = document.scrollingElement || document.documentElement;
    const header = [
      `Page: ${quote(document.title)} ${location.href}`,
      `Viewport: ${innerWidth}x${innerHeight}, scroll ${Math.round(scrollX)},${Math.round(scrollY)} of ${doc.scrollWidth}x${doc.scrollHeight}`,
    ];
    if (truncated) lines.push(`… output truncated at ${maxChars} characters; use find, viewport_only or a ref to narrow it.`);
    return { text: header.join("\n") + "\n\n" + (lines.join("\n") || "(no matching elements)"), interactive: interactiveCount, truncated };
  }

  // Text directly inside an element (not inside its child elements), for the "all" filter.
  function ownText(el) {
    let t = "";
    for (const n of el.childNodes) {
      if (n.nodeType === Node.TEXT_NODE) t += n.data;
      else if (n.nodeType === Node.ELEMENT_NODE && /^(b|strong|i|em|span|code|small|mark|sup|sub|abbr|time|label)$/.test(n.localName) && !isInteractive(n, roleOf(n))) {
        t += n.textContent;
      }
    }
    return clean(t);
  }

  // ---------- Find ----------

  function find(query, { limit = 20 } = {}) {
    const q = clean(query).toLowerCase();
    if (!q) return { text: "Empty query.", count: 0 };
    const terms = q.split(" ");
    const hits = [];
    const visit = (root) => {
      for (const el of root.querySelectorAll("*")) {
        if (el.shadowRoot) visit(el.shadowRoot);
        const tag = el.localName;
        if (tag === "script" || tag === "style" || tag === "zpilot-overlay") continue;
        const role = roleOf(el);
        const interactive = isInteractive(el, role);
        if (!interactive && role !== "heading" && role !== "img" && role !== "label" && !(ownText(el).length > 2)) continue;
        const name = interactive || role === "heading" || role === "img" || role === "label" ? nameOf(el, role) : ownText(el);
        const hay = `${name} ${role || ""} ${el.getAttribute("placeholder") || ""} ${el.getAttribute("title") || ""} ${el.getAttribute("name") || ""} ${el.id || ""}`.toLowerCase();
        let score = 0;
        if (hay.includes(q)) score += 3;
        const matched = terms.filter((t) => hay.includes(t)).length;
        if (!score && matched < Math.ceil(terms.length * 0.6)) continue;
        score += matched / terms.length + (interactive ? 1 : 0);
        if (name.toLowerCase() === q) score += 2;
        if (!visible(el)) continue;
        const r = rectOf(el);
        if (!r.w || !r.h) continue;
        hits.push({ el, role, name, score, r, interactive });
      }
    };
    visit(document);
    hits.sort((a, b) => b.score - a.score);
    // Drop ancestors whose match comes only from a matching descendant.
    const picked = [];
    for (const h of hits) {
      if (picked.length >= limit) break;
      if (picked.some((p) => p.el.contains(h.el) || h.el.contains(p.el))) continue;
      picked.push(h);
    }
    const lines = picked.map((h) => {
      const attrs = stateAttrs(h.el, h.role);
      if (!inViewport(h.r)) attrs.push("offscreen");
      return `[${refFor(h.el)}] ${h.role || (h.interactive ? "clickable" : "text")} ${quote(cut(h.name, 100))}${attrs.length ? " " + attrs.join(" ") : ""}`;
    });
    return { text: lines.length ? lines.join("\n") : `No element matches "${query}".`, count: lines.length };
  }

  // ---------- Locate / act ----------

  // Scrolls the element into view when needed and returns its center in top-window CSS pixels.
  function locate(ref, { cursor = false, scroll = true } = {}) {
    const el = byRef(ref);
    let r = rectOf(el);
    if (scroll && !(r.y >= 0 && r.y + r.h <= innerHeight && r.x >= 0 && r.x + r.w <= innerWidth)) {
      el.scrollIntoView({ block: "center", inline: "center", behavior: "instant" });
      r = rectOf(el);
    }
    if (!r.w && !r.h) {
      const err = new Error(`Element ${ref} (${describe(el)}) has no size on the page and cannot be clicked.`);
      err.code = "NOT_INTERACTABLE";
      throw err;
    }
    const x = Math.min(Math.max(r.x + r.w / 2, 1), innerWidth - 1);
    const y = Math.min(Math.max(r.y + r.h / 2, 1), innerHeight - 1);
    let occludedBy = null;
    if (el.ownerDocument === document) {
      const hit = document.elementFromPoint(x, y);
      if (hit && hit !== el && !el.contains(hit) && !hit.contains(el)) occludedBy = describe(hit);
    }
    if (cursor) overlay.move(x, y);
    return { x, y, description: describe(el), occludedBy };
  }

  function elementAt(x, y) {
    if (x < 0 || y < 0 || x >= innerWidth || y >= innerHeight) return { outside: true, width: innerWidth, height: innerHeight };
    const hit = document.elementFromPoint(x, y);
    if (!hit) return null;
    const target = hit.closest("a[href], button, input, select, textarea, [role], [onclick], label, summary") || hit;
    return { description: describe(target), ref: refFor(target) };
  }

  function focus(ref) {
    const el = byRef(ref);
    el.focus({ preventScroll: false });
    return { description: describe(el), focused: el.ownerDocument.activeElement === el };
  }

  // Prepares a field for filling. Text fields are focused and their content selected so the
  // service worker can replace it with trusted typing; other controls are set directly.
  function fill(ref, value) {
    const el = byRef(ref);
    const tag = el.localName;
    const type = (el.getAttribute("type") || "").toLowerCase();
    const role = roleOf(el);

    if (tag === "select") {
      const want = String(value).trim().toLowerCase();
      const options = [...el.options];
      const opt =
        options.find((o) => o.value === String(value)) ||
        options.find((o) => clean(o.text).toLowerCase() === want) ||
        options.find((o) => clean(o.text).toLowerCase().includes(want));
      if (!opt) {
        const err = new Error(`No option matches "${value}". Options: ${options.map((o) => quote(clean(o.text))).slice(0, 30).join(", ")}`);
        err.code = "OPTION_NOT_FOUND";
        throw err;
      }
      el.value = opt.value;
      opt.selected = true;
      el.dispatchEvent(new Event("input", { bubbles: true }));
      el.dispatchEvent(new Event("change", { bubbles: true }));
      return { kind: "done", message: `Selected ${quote(clean(opt.text))} in ${describe(el)}.` };
    }

    if ((tag === "input" && (type === "checkbox" || type === "radio")) || role === "checkbox" || role === "switch" || role === "radio") {
      const want = !/^(false|0|no|off|unchecked)$/i.test(String(value).trim());
      const current = tag === "input" ? el.checked : el.getAttribute("aria-checked") === "true";
      if (current !== want) el.click();
      const now = tag === "input" ? el.checked : el.getAttribute("aria-checked") === "true";
      return { kind: "done", message: `${describe(el)} is now ${now ? "checked" : "unchecked"}.` };
    }

    if (tag === "input" && /^(date|time|datetime-local|month|week|color|range)$/.test(type)) {
      const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value").set;
      setter.call(el, String(value));
      el.dispatchEvent(new Event("input", { bubbles: true }));
      el.dispatchEvent(new Event("change", { bubbles: true }));
      return { kind: "done", message: `Set ${describe(el)} to ${quote(el.value)}.` };
    }

    if (tag === "input" || tag === "textarea") {
      el.scrollIntoView({ block: "center", behavior: "instant" });
      el.focus();
      el.select();
      return { kind: "type", description: describe(el) };
    }
    if (el.isContentEditable) {
      el.scrollIntoView({ block: "center", behavior: "instant" });
      el.focus();
      const range = el.ownerDocument.createRange();
      range.selectNodeContents(el);
      const sel = el.ownerDocument.getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
      return { kind: "type", description: describe(el) };
    }
    const err = new Error(`${describe(el)} is not a form field. Use click and type instead.`);
    err.code = "NOT_FILLABLE";
    throw err;
  }

  function readValue(ref) {
    const el = byRef(ref);
    if (el.localName === "input" || el.localName === "textarea") return el.type === "password" ? "••••" : el.value;
    return clean(el.innerText);
  }

  // Marks an element so a CDP Runtime.evaluate in the main world can find it (e.g. file inputs).
  function mark(ref, token) {
    const el = byRef(ref);
    el.setAttribute("data-zpilot-target", token);
    return describe(el);
  }

  // Finds where files go for an upload. With a ref: the file input itself, a label or wrapper
  // holding exactly one file input, or "other" (a button or drop zone that opens a file chooser).
  // Without a ref: the page's only file input.
  function fileTarget(ref, token, selector) {
    const isFile = (el) => el && el.localName === "input" && (el.getAttribute("type") || "").toLowerCase() === "file";
    let input = null;
    let el = null;
    if (selector) {
      const found = document.querySelector(selector);
      if (!isFile(found)) return { kind: "none", count: 0, reason: `no file input matches ${selector}` };
      input = found;
    } else if (ref) {
      el = byRef(ref);
      if (isFile(el)) input = el;
      else if (el.localName === "label" && isFile(el.control)) input = el.control;
      else {
        const inner = el.querySelectorAll("input[type='file']");
        if (inner.length === 1) input = inner[0];
      }
    } else {
      const all = [...document.querySelectorAll("input[type='file']")];
      if (all.length !== 1) return { kind: "none", count: all.length };
      input = all[0];
    }
    if (!input) return { kind: "other", description: describe(el) };
    input.setAttribute("data-zpilot-target", token);
    return { kind: "input", multiple: input.multiple, accept: input.accept || "", description: describe(el || input) };
  }

  // The URL behind a link, image, video, audio or source element (for downloads).
  function linkTarget(ref) {
    const el = byRef(ref);
    const pick = (e) => e && (e.href || e.currentSrc || e.src || e.getAttribute?.("data-src") || "");
    let url = pick(el.closest("a[href]")) || pick(el);
    if (!url) url = pick(el.querySelector("a[href], img, video, audio, source"));
    try {
      url = url ? new URL(url, document.baseURI).href : "";
    } catch {}
    const name = el.closest("a[href]")?.getAttribute("download") || "";
    return { url, description: describe(el), download_name: name };
  }

  function unmark(token) {
    document.querySelectorAll(`[data-zpilot-target="${CSS.escape(token)}"]`).forEach((el) => el.removeAttribute("data-zpilot-target"));
    return true;
  }

  function pageInfo() {
    const doc = document.scrollingElement || document.documentElement;
    return {
      url: location.href,
      title: document.title,
      viewport: { width: innerWidth, height: innerHeight },
      scroll: { x: Math.round(scrollX), y: Math.round(scrollY), width: doc.scrollWidth, height: doc.scrollHeight },
      dpr: devicePixelRatio,
      ready: document.readyState,
    };
  }

  function scrollWindow(dx, dy) {
    window.scrollBy({ left: dx, top: dy, behavior: "instant" });
    return pageInfo().scroll;
  }

  function exists({ selector, text } = {}) {
    if (selector) {
      const el = document.querySelector(selector);
      return !!el && visible(el);
    }
    if (text) return (document.body?.innerText || "").toLowerCase().includes(String(text).toLowerCase());
    return false;
  }

  // ---------- Overlay: animated cursor and "controlled by ZPilot Browser" frame ----------

  const overlay = {
    host: null,
    cursor: null,
    ripple: null,
    shown: true,

    ensure() {
      if (this.host?.isConnected) return;
      const host = document.createElement("zpilot-overlay");
      host.style.cssText = "all: initial; position: fixed; inset: 0; pointer-events: none; z-index: 2147483647;";
      const root = host.attachShadow({ mode: "closed" });
      const style = document.createElement("style");
      style.textContent = `
        .frame { position: fixed; inset: 0; border: 2px solid rgba(124, 58, 237, .85); box-shadow: inset 0 0 18px rgba(124, 58, 237, .35); border-radius: 2px; animation: pulse 2.4s ease-in-out infinite; }
        @keyframes pulse { 50% { box-shadow: inset 0 0 30px rgba(124, 58, 237, .55); } }
        .badge { position: fixed; left: 50%; bottom: 14px; transform: translateX(-50%); display: flex; align-items: center; gap: 8px;
          padding: 6px 14px; border-radius: 999px; background: rgba(17, 12, 34, .88); color: #fff; font: 600 12px/1.2 system-ui, sans-serif;
          letter-spacing: .2px; box-shadow: 0 6px 24px rgba(0, 0, 0, .3); }
        .dot { width: 8px; height: 8px; border-radius: 50%; background: #a78bfa; box-shadow: 0 0 0 0 rgba(167, 139, 250, .7); animation: ping 1.6s infinite; }
        @keyframes ping { 70% { box-shadow: 0 0 0 8px rgba(167, 139, 250, 0); } 100% { box-shadow: 0 0 0 0 rgba(167, 139, 250, 0); } }
        .cursor { position: fixed; left: 0; top: 0; width: 24px; height: 24px; transform: translate(-100px, -100px);
          transition: transform .16s cubic-bezier(.2, .8, .2, 1); filter: drop-shadow(0 2px 3px rgba(0, 0, 0, .35)); }
        .ripple { position: fixed; left: 0; top: 0; width: 30px; height: 30px; margin: -15px 0 0 -15px; border-radius: 50%;
          border: 2px solid rgba(124, 58, 237, .9); opacity: 0; pointer-events: none; }
      `;
      const frame = document.createElement("div");
      frame.className = "frame";
      const badge = document.createElement("div");
      badge.className = "badge";
      const dot = document.createElement("span");
      dot.className = "dot";
      badge.append(dot, document.createTextNode("ZPilot Browser is controlling this tab"));
      const cursor = document.createElement("div");
      cursor.className = "cursor";
      const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
      svg.setAttribute("viewBox", "0 0 24 24");
      svg.setAttribute("width", "24");
      svg.setAttribute("height", "24");
      const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
      path.setAttribute("d", "M3 2l7.5 19 2.6-7.4L20.5 11z");
      path.setAttribute("fill", "#7c3aed");
      path.setAttribute("stroke", "#ffffff");
      path.setAttribute("stroke-width", "1.6");
      path.setAttribute("stroke-linejoin", "round");
      svg.append(path);
      cursor.append(svg);
      const ripple = document.createElement("div");
      ripple.className = "ripple";
      root.append(style, frame, badge, ripple, cursor);
      document.documentElement.append(host);
      this.host = host;
      this.cursor = cursor;
      this.ripple = ripple;
      this.setVisible(this.shown);
    },

    move(x, y) {
      this.ensure();
      // The arrow tip sits at (3, 2) inside the 24px icon.
      this.cursor.style.transform = `translate(${x - 3}px, ${y - 2}px)`;
      return true;
    },

    click(x, y) {
      this.move(x, y);
      this.ripple.style.transform = `translate(${x}px, ${y}px)`;
      this.ripple.animate(
        [
          { opacity: 0.9, transform: `translate(${x}px, ${y}px) scale(.4)` },
          { opacity: 0, transform: `translate(${x}px, ${y}px) scale(1.6)` },
        ],
        { duration: 450, easing: "ease-out" }
      );
      return true;
    },

    setVisible(v) {
      this.shown = v;
      if (this.host) this.host.style.display = v ? "" : "none";
      return true;
    },

    remove() {
      this.host?.remove();
      this.host = null;
      return true;
    },
  };

  globalThis.ZB_agent = {
    snapshot,
    find,
    locate,
    elementAt,
    focus,
    fill,
    readValue,
    mark,
    unmark,
    fileTarget,
    linkTarget,
    pageInfo,
    scrollWindow,
    exists,
    showOverlay: () => (overlay.ensure(), true),
    cursorMove: (x, y) => overlay.move(x, y),
    cursorClick: (x, y) => overlay.click(x, y),
    overlayVisible: (v) => overlay.setVisible(v),
    removeOverlay: () => overlay.remove(),
  };
})();
