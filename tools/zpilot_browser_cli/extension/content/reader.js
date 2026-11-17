// ZPilot Browser content module: converts the rendered page to Markdown, plain text or HTML.
// Walks the live DOM (not a clone), so hidden elements, shadow DOM and same-origin iframes
// are handled the way the user sees them. Code blocks are protected from all whitespace cleanup.

(() => {
  if (globalThis.ZB_reader) return;

  const SKIP = new Set([
    "script", "style", "noscript", "template", "head", "meta", "link", "svg", "canvas", "map", "area",
    "input", "select", "textarea", "button", "option", "datalist", "object", "embed", "param", "source",
    "track", "zpilot-overlay",
  ]);

  const NOISE = [
    "nav", "aside", "footer", "[role='navigation']", "[role='banner']", "[role='contentinfo']", "[role='complementary']",
    "[role='search']", "[role='menu']", "[role='menubar']", "[role='toolbar']",
    ".sidebar", "#sidebar", ".widget-area", ".breadcrumb", ".breadcrumbs",
    ".share", ".sharing", ".social-share", ".share-buttons", ".social-links",
    ".related", ".related-posts", ".recommended", ".newsletter", ".newsletter-signup",
    ".cookie", ".cookies", ".cookie-banner", "#cookie-banner", "#cookie-notice", ".consent-banner", ".gdpr",
    ".ad", ".ads", ".advert", ".advertisement", ".ad-container", "[id^='google_ads']", "[id^='div-gpt-ad']",
    ".comments", "#comments", ".comment-list", "#disqus_thread",
    ".mw-editsection", ".navbox", ".vertical-navbox", ".catlinks", ".printfooter", "#toc", ".toc",
    "sup.reference", ".reflist", "ol.references", ".skip-link", ".sr-only-focusable",
  ].join(",");

  const FILE_EXT = /\.(pdf|zip|tar|gz|tgz|7z|rar|csv|tsv|xlsx?|docx?|pptx?|odt|ods|epub|mp3|wav|flac|mp4|mov|webm|mkv|json|xml|txt|apk|exe|msi|dmg|iso)$/i;
  const MARK = "\u0000ZT";

  const clean = (s) => (s || "").replace(/\s+/g, " ").trim();
  const abs = (u) => {
    if (!u) return "";
    try {
      return new URL(u, document.baseURI).href;
    } catch {
      return "";
    }
  };
  const escUrl = (u) => u.replace(/ /g, "%20").replace(/\(/g, "%28").replace(/\)/g, "%29");
  const isFile = (u) => {
    try {
      return FILE_EXT.test(new URL(u).pathname);
    } catch {
      return false;
    }
  };

  function shown(el, ctx) {
    if (ctx.inDetails || typeof el.checkVisibility !== "function") return true;
    if (el.checkVisibility({ checkVisibilityCSS: true, visibilityProperty: true })) return true;
    // display:contents has no box but its children render.
    return getComputedStyle(el).display === "contents";
  }

  function noise(el, ctx) {
    if (!ctx.mainOnly || el === ctx.root) return false;
    if (el.getAttribute("aria-hidden") === "true") return true;
    if (el.localName === "header" && !el.closest("article")) return true;
    return el.matches(NOISE);
  }

  function childNodesOf(el) {
    if (el.shadowRoot) return [...el.shadowRoot.childNodes];
    if (el.localName === "slot") {
      const assigned = el.assignedNodes({ flatten: true });
      return assigned.length ? assigned : [...el.childNodes];
    }
    return [...el.childNodes];
  }

  // Joins converted parts, dropping whitespace at line starts and doubled spaces between parts.
  function join(parts) {
    let out = "";
    for (let p of parts) {
      if (!p) continue;
      if (out === "" || out.endsWith("\n")) p = p.replace(/^[ \t]+/, "");
      else if (out.endsWith(" ") && p.startsWith(" ")) p = p.slice(1);
      out += p;
    }
    return out;
  }

  const children = (el, ctx, skip) => join(childNodesOf(el).map((n) => (skip && skip(n) ? "" : convert(n, ctx))));
  const inline = (el, ctx) => children(el, ctx).replace(/\s*\n+\s*/g, " ").trim();
  const block = (s) => s.replace(/[ \t]+\n/g, "\n").replace(/\n{3,}/g, "\n\n").trim();

  function wrap(content, mark) {
    const m = content.match(/^(\s*)([\s\S]*?)(\s*)$/);
    if (!m[2]) return content;
    return `${m[1]}${mark}${m[2]}${mark}${m[3]}`;
  }

  function code(ctx, text) {
    ctx.blocks.push(text);
    return `\n\n${MARK}${ctx.blocks.length - 1}\u0000\n\n`;
  }

  // Indents every line but the first, re-indenting protected code blocks too.
  function indentRest(text, pad, ctx) {
    return text
      .split("\n")
      .map((line, i) => {
        if (i === 0 || !line) return line;
        const m = line.match(/^\u0000ZT(\d+)\u0000$/);
        if (m) ctx.blocks[+m[1]] = ctx.blocks[+m[1]].split("\n").map((l, k) => (k && l ? pad + l : l)).join("\n");
        return pad + line;
      })
      .join("\n");
  }

  function addMedia(ctx, type, url, title) {
    if (!ctx.includeMedia || !url || ctx.mediaSeen.has(url)) return;
    ctx.mediaSeen.add(url);
    if (ctx.media.length < 150) ctx.media.push({ type, url, title: clean(title).slice(0, 200) });
  }

  function imageSource(img) {
    const srcset = img.getAttribute("srcset") || img.getAttribute("data-srcset") || "";
    return abs(
      img.currentSrc ||
        img.getAttribute("src") ||
        img.getAttribute("data-src") ||
        img.getAttribute("data-lazy-src") ||
        img.getAttribute("data-original") ||
        srcset.split(",")[0]?.trim().split(/\s+/)[0]
    );
  }

  function convert(node, ctx) {
    if (node.nodeType === Node.TEXT_NODE) return ctx.pre ? node.data : node.data.replace(/\s+/g, " ");
    if (node.nodeType !== Node.ELEMENT_NODE) return "";
    const el = node;
    const tag = el.localName;
    if (SKIP.has(tag)) return "";
    if (!shown(el, ctx) || noise(el, ctx)) return "";

    switch (tag) {
      case "h1": case "h2": case "h3": case "h4": case "h5": case "h6": {
        const t = inline(el, ctx);
        if (!t) return "";
        return ctx.plain ? `\n\n${t}\n\n` : `\n\n${"#".repeat(+tag[1])} ${t}\n\n`;
      }
      case "p": {
        const t = block(children(el, ctx).replace(/ *\n */g, "\n"));
        return t ? `\n\n${t}\n\n` : "";
      }
      case "br":
        return "\n";
      case "hr":
        return ctx.plain ? "\n\n" : "\n\n---\n\n";
      case "blockquote": {
        const t = block(children(el, ctx));
        if (!t) return "";
        return `\n\n${t.split("\n").map((l) => (l ? `> ${l}` : ">")).join("\n")}\n\n`;
      }
      case "pre": {
        const text = (el.innerText ?? el.textContent).replace(/\n+$/, "");
        if (!text.trim()) return "";
        if (ctx.plain) return code(ctx, text);
        const runs = text.match(/`{3,}/g) || [];
        const fence = "`".repeat(Math.max(3, ...runs.map((r) => r.length + 1)));
        return code(ctx, `${fence}${language(el)}\n${text}\n${fence}`);
      }
      case "code": case "kbd": case "samp": {
        const t = clean(el.textContent);
        if (!t || ctx.plain) return t;
        const longest = Math.max(0, ...(t.match(/`+/g) || []).map((r) => r.length));
        const ticks = "`".repeat(longest + 1);
        const pad = t.startsWith("`") || t.endsWith("`") ? " " : "";
        return `${ticks}${pad}${t}${pad}${ticks}`;
      }
      case "strong": case "b":
        return ctx.plain ? children(el, ctx) : wrap(children(el, ctx), "**");
      case "em": case "i": case "cite": case "dfn":
        return ctx.plain ? children(el, ctx) : wrap(children(el, ctx), "*");
      case "del": case "s": case "strike":
        return ctx.plain ? children(el, ctx) : wrap(children(el, ctx), "~~");
      case "q":
        return `"${children(el, ctx).trim()}"`;
      case "a":
        return link(el, ctx);
      case "img":
        return image(el, ctx);
      case "picture":
        return children(el, ctx);
      case "figure": {
        const caption = el.querySelector(":scope > figcaption");
        const body = block(children(el, ctx, (n) => n === caption));
        const cap = caption && shown(caption, ctx) ? inline(caption, ctx) : "";
        if (!body && !cap) return "";
        return `\n\n${body}${cap ? `\n${ctx.plain ? cap : `*${cap}*`}` : ""}\n\n`;
      }
      case "video": case "audio": {
        const src = abs(el.currentSrc || el.getAttribute("src") || el.querySelector("source[src]")?.getAttribute("src"));
        const title = el.getAttribute("title") || el.getAttribute("aria-label") || tag;
        if (!ctx.includeMedia || !src || src.startsWith("blob:")) return "";
        addMedia(ctx, tag, src, title);
        const label = tag === "video" ? "Video" : "Audio";
        return ctx.plain ? `\n\n[${label}: ${title}] ${src}\n\n` : `\n\n[${label}: ${clean(title)}](${escUrl(src)})\n\n`;
      }
      case "iframe": {
        try {
          const body = el.contentDocument?.body;
          if (body) return `\n\n${children(body, ctx)}\n\n`;
        } catch {}
        const src = abs(el.getAttribute("src"));
        if (!ctx.includeMedia || !/^https?:/.test(src) || /doubleclick|googlesyndication|googleads|adservice/.test(src)) return "";
        const title = el.getAttribute("title") || "Embedded content";
        addMedia(ctx, "embed", src, title);
        return ctx.plain ? `\n\n[Embed: ${title}] ${src}\n\n` : `\n\n[Embed: ${clean(title)}](${escUrl(src)})\n\n`;
      }
      case "ul":
        return list(el, ctx, false);
      case "ol":
        return list(el, ctx, true);
      case "dl":
        return `\n\n${block(children(el, ctx))}\n\n`;
      case "dt": {
        const t = inline(el, ctx);
        return t ? `\n\n${ctx.plain ? t : `**${t}**`}\n` : "";
      }
      case "dd": {
        const t = block(children(el, ctx));
        return t ? `\n${ctx.plain ? t : `: ${indentRest(t, "  ", ctx)}`}\n` : "";
      }
      case "details": {
        const saved = ctx.inDetails;
        ctx.inDetails = true;
        const summary = el.querySelector(":scope > summary");
        const head = summary ? inline(summary, ctx) : "";
        const body = block(children(el, ctx, (n) => n === summary));
        ctx.inDetails = saved;
        return `\n\n${head ? (ctx.plain ? head : `**${head}**`) + "\n\n" : ""}${body}\n\n`;
      }
      case "table":
        return table(el, ctx);
      default:
        return children(el, ctx);
    }
  }

  function language(pre) {
    const candidates = [pre, pre.querySelector("code"), pre.parentElement].filter(Boolean);
    for (const el of candidates) {
      const data = el.getAttribute("data-lang") || el.getAttribute("data-language");
      if (data) return data.toLowerCase();
      const m = (el.getAttribute("class") || "").match(/(?:language|lang|highlight-source|brush:)[-\s:]?([a-z0-9+#.-]+)/i);
      if (m) return m[1].toLowerCase();
    }
    return "";
  }

  function link(el, ctx) {
    const content = children(el, ctx);
    const label = clean(content);
    if (!ctx.includeLinks) return content;
    const href = el.getAttribute("href") || el.getAttribute("xlink:href");
    if (!href || href.startsWith("#") || /^javascript:/i.test(href)) return content;
    const url = abs(href);
    if (!/^(https?|mailto|tel|ftp):/i.test(url)) return content;
    if (isFile(url)) addMedia(ctx, "file", url, label || url.split("/").pop());
    if (!label) return "";
    const lead = /^\s/.test(content) ? " " : "";
    const trail = /\s$/.test(content) ? " " : "";
    if (ctx.plain) return `${lead}${label === url ? url : `${label} (${url})`}${trail}`;
    return `${lead}[${label}](${escUrl(url)})${trail}`;
  }

  function image(el, ctx) {
    if (!ctx.includeImages) return "";
    const src = imageSource(el);
    if (!src || src.startsWith("data:")) return "";
    const w = el.naturalWidth || +el.getAttribute("width") || 0;
    const h = el.naturalHeight || +el.getAttribute("height") || 0;
    if ((w && w < 40) || (h && h < 40)) return "";
    const alt = clean(el.getAttribute("alt") || el.getAttribute("title"));
    if (!alt && /(^|[/_.-])(icon|logo|sprite|spacer|pixel|badge|avatar|emoji)/i.test(src)) return "";
    ctx.imageCount++;
    if (ctx.plain) return alt ? ` [Image: ${alt}] ` : "";
    return `![${alt.replace(/[[\]]/g, "")}](${escUrl(src)})`;
  }

  function list(el, ctx, ordered) {
    let n = ordered ? parseInt(el.getAttribute("start") || "1", 10) || 1 : 0;
    const items = [];
    for (const child of el.children) {
      const tag = child.localName;
      if (tag === "ul" || tag === "ol") {
        const nested = block(convert(child, ctx));
        if (!nested) continue;
        if (items.length) items[items.length - 1] += "\n" + "  " + indentRest(nested, "  ", ctx);
        else items.push(nested);
        continue;
      }
      if (tag !== "li" || !shown(child, ctx) || noise(child, ctx)) continue;
      const body = block(children(child, ctx));
      if (!body) continue;
      const marker = ordered ? `${n++}.` : "-";
      let prefix = `${marker} `;
      const box = child.querySelector(":scope > input[type='checkbox'], :scope > label > input[type='checkbox'], :scope > p > input[type='checkbox']");
      if (box && !ctx.plain) prefix += box.checked ? "[x] " : "[ ] ";
      items.push(prefix + indentRest(body, " ".repeat(marker.length + 1), ctx));
    }
    return items.length ? `\n\n${items.join("\n")}\n\n` : "";
  }

  function cellText(cell, ctx) {
    return block(children(cell, ctx))
      .replace(/\u0000ZT(\d+)\u0000/g, (_, i) => {
        const raw = ctx.blocks[+i].replace(/^`{3,}.*\n|\n`{3,}$/g, "");
        return ctx.plain ? clean(raw) : "`" + clean(raw) + "`";
      })
      .replace(/\n{2,}/g, "<br>")
      .replace(/\n/g, " ")
      .replace(/\|/g, "\\|")
      .trim();
  }

  function table(el, ctx) {
    const layout = el.getAttribute("role") === "presentation" || el.querySelector(":scope > tbody > tr > td > table, :scope > tr > td > table");
    const rows = [...el.rows].filter((tr) => shown(tr, ctx));
    if (!rows.length) return "";
    const maxCells = Math.max(...rows.map((r) => r.cells.length));
    if (layout || maxCells <= 1) {
      return `\n\n${rows.map((tr) => [...tr.cells].map((c) => block(children(c, ctx))).filter(Boolean).join("\n\n")).filter(Boolean).join("\n\n")}\n\n`;
    }
    const grid = [];
    for (const tr of rows) {
      const cells = [];
      for (const cell of tr.cells) {
        cells.push(cellText(cell, ctx));
        for (let i = 1; i < Math.min(cell.colSpan || 1, 20); i++) cells.push("");
      }
      if (cells.some(Boolean)) grid.push(cells);
    }
    if (!grid.length) return "";
    const cols = Math.max(...grid.map((r) => r.length));
    const caption = el.caption ? inline(el.caption, ctx) : "";
    if (ctx.plain) {
      return `\n\n${caption ? caption + "\n" : ""}${grid.map((r) => r.join("\t")).join("\n")}\n\n`;
    }
    const line = (r) => `| ${Array.from({ length: cols }, (_, i) => r[i] || "").join(" | ")} |`;
    const out = [line(grid[0]), `| ${Array(cols).fill("---").join(" | ")} |`, ...grid.slice(1).map(line)];
    return `\n\n${caption ? `*${caption}*\n\n` : ""}${out.join("\n")}\n\n`;
  }

  // ---------- Main-content detection ----------

  // innerText is empty for display:contents elements, so fall back to textContent there.
  const textLength = (el) => (el.innerText || "").length || (getComputedStyle(el).display === "contents" ? (el.textContent || "").length : 0);

  function visibleElement(el) {
    if (typeof el.checkVisibility !== "function" || el.checkVisibility()) return true;
    // checkVisibility() is false for display:contents wrappers (e.g. MDN's <main>), whose content renders.
    return getComputedStyle(el).display === "contents";
  }

  function commonAncestor(nodes) {
    let anc = nodes[0];
    while (anc && !nodes.every((n) => anc.contains(n))) anc = anc.parentElement;
    return anc;
  }

  function pickRoot() {
    const body = document.body;
    if (!body) return document.documentElement;
    const bodyLen = textLength(body) || 1;

    const articles = [...document.querySelectorAll("article")].filter(
      (a) => visibleElement(a) && !a.parentElement?.closest("article") && textLength(a) >= 200
    );
    if (articles.length === 1 && textLength(articles[0]) >= bodyLen * 0.2) return articles[0];
    if (articles.length > 1) {
      const anc = commonAncestor(articles);
      if (anc && anc !== body && anc !== document.documentElement) return anc;
    }
    const main = [...document.querySelectorAll("main, [role='main']")].find((m) => visibleElement(m) && textLength(m) >= 200);
    if (main) return main;
    for (const sel of ["#content", "#main-content", "#main", ".post-content", ".article-content", ".entry-content", ".markdown-body", "#bodyContent", ".content"]) {
      const el = document.querySelector(sel);
      if (el && visibleElement(el) && textLength(el) >= bodyLen * 0.3) return el;
    }
    return bestScored() || body;
  }

  // Readability-style scoring: paragraphs vote for their parent and grandparent.
  function bestScored() {
    const scores = new Map();
    for (const p of document.querySelectorAll("p, pre, blockquote, td, li")) {
      const t = (p.textContent || "").trim();
      if (t.length < 40) continue;
      const s = 1 + Math.min(3, t.length / 100) + (t.match(/[,.;:]/g) || []).length * 0.05;
      let el = p.parentElement;
      let factor = 1;
      for (let i = 0; i < 3 && el && el !== document.body; i++, el = el.parentElement, factor /= 2) {
        scores.set(el, (scores.get(el) || 0) + s * factor);
      }
    }
    let best = null;
    let bestScore = 0;
    const top = [...scores.entries()].sort((a, b) => b[1] - a[1]).slice(0, 10);
    for (const [el, score] of top) {
      if (!visibleElement(el)) continue;
      const total = (el.textContent || "").length || 1;
      const linked = [...el.querySelectorAll("a")].reduce((n, a) => n + (a.textContent || "").length, 0);
      const adjusted = score * (1 - Math.min(0.9, linked / total));
      if (adjusted > bestScore) {
        best = el;
        bestScore = adjusted;
      }
    }
    return best;
  }

  // ---------- Metadata ----------

  function jsonLd() {
    for (const s of document.querySelectorAll("script[type='application/ld+json']")) {
      try {
        const data = JSON.parse(s.textContent);
        const items = [].concat(data["@graph"] || data);
        for (const item of items) {
          if (/Article|BlogPosting|NewsArticle|Report|WebPage/i.test([].concat(item?.["@type"] || []).join(" "))) {
            const author = [].concat(item.author || []).map((a) => (typeof a === "string" ? a : a?.name)).filter(Boolean).join(", ");
            return { author, published: item.datePublished || "" };
          }
        }
      } catch {}
    }
    return {};
  }

  function metadata() {
    const m = (sel) => document.querySelector(sel)?.getAttribute("content")?.trim() || "";
    const ld = jsonLd();
    const nav = performance.getEntriesByType("navigation")[0];
    return {
      url: location.href,
      title: document.title.trim() || m("meta[property='og:title']"),
      description: m("meta[name='description']") || m("meta[property='og:description']"),
      author: m("meta[name='author']") || m("meta[property='article:author']") || ld.author || clean(document.querySelector("[rel='author']")?.innerText),
      published: m("meta[property='article:published_time']") || ld.published || document.querySelector("time[datetime]")?.getAttribute("datetime") || "",
      site_name: m("meta[property='og:site_name']"),
      language: document.documentElement.lang || "",
      http_status: nav?.responseStatus || undefined,
    };
  }

  // ---------- Public API ----------

  function extract(opts = {}) {
    const format = opts.format || "markdown";
    const mainOnly = opts.mainContentOnly ?? format !== "html";
    const includeLinks = opts.includeLinks !== false;
    const includeImages = opts.includeImages !== false;
    const includeMedia = opts.includeMedia !== false;
    const meta = metadata();
    const type = document.contentType || "text/html";

    if (!/html/i.test(type)) {
      if (/pdf/i.test(type)) {
        const err = new Error("This URL is a PDF document, which webpage_read cannot read yet.");
        err.code = "UNSUPPORTED_CONTENT";
        throw err;
      }
      let content = document.body ? document.body.innerText : document.documentElement.textContent || "";
      if (/json/i.test(type) && format === "markdown") {
        try {
          content = "```json\n" + JSON.stringify(JSON.parse(content), null, 2) + "\n```";
        } catch {}
      }
      return { ...meta, content_type: type, content, word_count: content.split(/\s+/).filter(Boolean).length, attachments: [] };
    }

    if (format === "html") {
      let html;
      if (mainOnly) {
        const copy = pickRoot().cloneNode(true);
        copy.querySelectorAll("script, style, noscript, template, zpilot-overlay").forEach((e) => e.remove());
        if (!includeImages) copy.querySelectorAll("img, picture").forEach((e) => e.remove());
        if (!includeMedia) copy.querySelectorAll("video, audio, iframe, embed, object").forEach((e) => e.remove());
        html = copy.outerHTML;
      } else {
        html = "<!DOCTYPE html>\n" + document.documentElement.outerHTML;
      }
      return { ...meta, content_type: type, content: html, word_count: 0, attachments: [] };
    }

    const root = mainOnly ? pickRoot() : document.body || document.documentElement;
    const ctx = {
      root,
      mainOnly,
      plain: format === "text",
      includeLinks,
      includeImages,
      includeMedia,
      blocks: [],
      media: [],
      mediaSeen: new Set(),
      imageCount: 0,
      inDetails: false,
      pre: false,
    };
    let content = block(convert(root, ctx)).replace(/\u0000ZT(\d+)\u0000/g, (_, i) => ctx.blocks[+i]);

    // File attachments linked from the content are listed even when link output is off.
    if (includeMedia) {
      for (const a of root.querySelectorAll("a[href]")) {
        const url = abs(a.getAttribute("href"));
        if (isFile(url)) addMedia(ctx, "file", url, clean(a.innerText) || url.split("/").pop());
      }
    }

    return {
      ...meta,
      content_type: type,
      content,
      word_count: content.split(/\s+/).filter(Boolean).length,
      image_count: ctx.imageCount,
      main_content_only: mainOnly,
      attachments: includeMedia ? ctx.media : [],
    };
  }

  // Resolves once the DOM has stopped changing for quietMs (or after maxMs).
  function settle({ quietMs = 600, maxMs = 4000 } = {}) {
    return new Promise((resolve) => {
      const start = performance.now();
      let last = start;
      const observer = new MutationObserver(() => {
        last = performance.now();
      });
      observer.observe(document.documentElement, { subtree: true, childList: true, characterData: true });
      const tick = () => {
        const now = performance.now();
        if (now - last >= quietMs || now - start >= maxMs) {
          observer.disconnect();
          resolve({ waited_ms: Math.round(now - start) });
        } else setTimeout(tick, 100);
      };
      setTimeout(tick, 100);
    });
  }

  function exists({ selector, text } = {}) {
    if (selector) {
      const el = document.querySelector(selector);
      return !!el && visibleElement(el);
    }
    if (text) return (document.body?.innerText || "").toLowerCase().includes(String(text).toLowerCase());
    return false;
  }

  globalThis.ZB_reader = { extract, settle, exists, metadata };
})();
