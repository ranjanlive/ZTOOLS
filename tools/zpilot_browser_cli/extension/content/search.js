// ZPilot Browser content module for search result pages. One extractor per engine plus a generic
// fallback that works on most result layouts when an engine changes its markup.

(() => {
  if (globalThis.ZB_search) return;

  const clean = (s) => (s || "").replace(/\s+/g, " ").trim();
  const text = (el) => clean(el?.innerText || el?.textContent);
  const textOf = (root, selectors) => {
    for (const sel of selectors) {
      const t = text(root.querySelector(sel));
      if (t) return t;
    }
    return "";
  };
  const hostOf = (u) => {
    try {
      return new URL(u).hostname.replace(/^www\./, "");
    } catch {
      return "";
    }
  };
  const isHttp = (u) => /^https?:\/\//i.test(u || "");

  function decodeGoogle(u) {
    try {
      const url = new URL(u);
      if (/(^|\.)google\.[a-z.]+$/.test(url.hostname) && url.pathname === "/url") return url.searchParams.get("q") || url.searchParams.get("url") || u;
    } catch {}
    return u;
  }

  function decodeBing(u) {
    try {
      const url = new URL(u);
      if (url.hostname.endsWith("bing.com") && url.pathname.startsWith("/ck/")) {
        let p = url.searchParams.get("u");
        if (p && p.startsWith("a1")) {
          p = p.slice(2).replace(/-/g, "+").replace(/_/g, "/");
          p += "=".repeat((4 - (p.length % 4)) % 4);
          const decoded = atob(p);
          if (isHttp(decoded)) return decoded;
        }
      }
    } catch {}
    return u;
  }

  function decodeDuck(u) {
    try {
      const url = new URL(u, location.href);
      return url.searchParams.get("uddg") || url.href;
    } catch {
      return u;
    }
  }

  function decodeYahoo(u) {
    const m = (u || "").match(/\/RU=([^/]+)\/R[KS]=/);
    if (m) {
      try {
        return decodeURIComponent(m[1]);
      } catch {}
    }
    return u;
  }

  function google() {
    const results = [];
    for (const h3 of document.querySelectorAll("#search a h3, #rso a h3, #botstuff a h3")) {
      const a = h3.closest("a");
      if (!a || a.closest("#tads, #tadsb, #bottomads, [data-text-ad], [aria-label='Ads']")) continue;
      const url = decodeGoogle(a.href);
      if (!isHttp(url)) continue;
      if (/(^|\.)google\.[a-z.]+$/.test(hostOf(url)) && /\/search\b/.test(url)) continue;
      const block = a.closest("div.MjjYud, div.g, div[data-hveid], div[data-sokoban-container]") || a.parentElement?.parentElement || a;
      let snippet = textOf(block, [".VwiC3b", "[data-sncf]", "div[style*='-webkit-line-clamp']", ".ITZIwc", ".lEBKkf", ".s3v9rd"]);
      if (!snippet) {
        const title = text(h3);
        snippet = clean(text(block).replace(title, "")).slice(0, 320);
      }
      results.push({ title: text(h3), url, snippet, source: textOf(block, [".VuuXrf"]) || hostOf(url) });
    }
    const answer = textOf(document, [
      ".hgKElc",
      '[data-attrid="wa:/description"]',
      ".IZ6rdc",
      ".Z0LcW",
      '[data-attrid*="answer"] .LGOjbe',
      ".kno-rdesc span",
      "[data-tts='answers']",
    ]);
    const related = [];
    for (const el of document.querySelectorAll("div[data-q], .related-question-pair")) {
      const q = clean(el.getAttribute("data-q") || text(el.querySelector("span")));
      if (q && !related.includes(q)) related.push(q);
    }
    return {
      results,
      answer,
      related: related.slice(0, 8),
      hasNext: !!document.querySelector("#pnnext, a[aria-label='Next page'], a[id='pnnext']"),
      noResults: /did not match any documents|no results found for/i.test(text(document.querySelector("#topstuff, #res"))),
    };
  }

  function bing() {
    const results = [];
    for (const li of document.querySelectorAll("#b_results > li.b_algo")) {
      const a = li.querySelector("h2 a");
      if (!a) continue;
      const url = decodeBing(a.href);
      if (!isHttp(url)) continue;
      results.push({
        title: text(a),
        url,
        snippet: textOf(li, [".b_caption p", ".b_lineclamp2", ".b_lineclamp3", ".b_lineclamp4", ".b_paractl", "p"]),
        source: textOf(li, [".tptt"]) || hostOf(url),
      });
    }
    return {
      results,
      answer: textOf(document, [".b_focusTextLarge", ".b_focusTextMedium", ".b_ans .b_paractl", ".b_entityTP .b_snippet"]),
      related: [...document.querySelectorAll(".b_rs a, #brsv3 a")].map(text).filter(Boolean).slice(0, 8),
      hasNext: !!document.querySelector("a.sb_pagN, a[title='Next page']"),
      noResults: !!document.querySelector(".b_no"),
    };
  }

  function duckduckgo() {
    const results = [];
    for (const r of document.querySelectorAll("#links .result, .results .result, .serp__results .result")) {
      if (r.classList.contains("result--ad") || r.querySelector(".badge--ad")) continue;
      const a = r.querySelector("a.result__a");
      if (!a) continue;
      const url = decodeDuck(a.getAttribute("href"));
      if (!isHttp(url)) continue;
      results.push({ title: text(a), url, snippet: textOf(r, [".result__snippet"]), source: textOf(r, [".result__url"]) || hostOf(url) });
    }
    return {
      results,
      answer: textOf(document, [".zci__result", "#zero_click_abstract"]),
      related: [],
      hasNext: !!duckNextForm(),
      noResults: !!document.querySelector(".no-results"),
    };
  }

  function duckNextForm() {
    for (const input of document.querySelectorAll("form input[type='submit']")) {
      if (/next/i.test(input.value || "")) return input.form;
    }
    return null;
  }

  function brave() {
    const results = [];
    for (const r of document.querySelectorAll(".snippet[data-type='web'], #results [data-type='web']")) {
      const a = r.querySelector("a[href^='http']");
      if (!a || r.closest(".serp-columns-side")) continue;
      const url = a.href;
      const title = textOf(r, [".search-snippet-title", ".snippet-title", ".title"]) || text(a);
      let snippet = textOf(r, [".snippet-description", ".generic-snippet .content", "[class*='description']"]);
      if (!snippet) {
        // The description has no stable class: take the block text minus the link (site + title).
        snippet = clean(text(r).replace(text(a), "").replace(title, "")).slice(0, 320);
      }
      results.push({ title, url, snippet, source: textOf(r, [".site-name-wrapper .t-secondary", ".netloc"]) || hostOf(url) });
    }
    return {
      results,
      answer: textOf(document, ["#rh-answer .answer", ".infobox-description"]),
      related: [],
      hasNext: results.length > 0,
      noResults: /no results found/i.test(text(document.querySelector("#results, #mixed-main"))),
    };
  }

  function yahoo() {
    const results = [];
    for (const r of document.querySelectorAll("#web .algo, #web li .dd.algo")) {
      const a = r.querySelector("h3 a, .compTitle a");
      if (!a) continue;
      const url = decodeYahoo(a.href);
      if (!isHttp(url)) continue;
      const title = clean(a.getAttribute("aria-label")) || textOf(r, ["h3 .title", "h3"]) || text(a);
      results.push({ title, url, snippet: textOf(r, [".compText", ".fc-falcon"]), source: hostOf(url) });
    }
    return {
      results,
      answer: "",
      related: [...document.querySelectorAll(".AlsoTry a, .compList a")].map(text).filter(Boolean).slice(0, 8),
      hasNext: !!document.querySelector("a.next"),
      noResults: /we did not find results/i.test(text(document.querySelector("#web"))),
    };
  }

  // Fallback for unknown or changed layouts: result titles are headings inside links.
  function generic() {
    const engineHost = location.hostname.replace(/^www\./, "");
    const results = [];
    for (const h of document.querySelectorAll("a h2, a h3, h2 a, h3 a")) {
      const a = h.tagName === "A" ? h : h.closest("a");
      if (!a || !isHttp(a.href)) continue;
      const url = decodeBing(decodeDuck(decodeYahoo(decodeGoogle(a.href))));
      const host = hostOf(url);
      if (!host || host.endsWith(engineHost) || engineHost.endsWith(host)) continue;
      const block = a.closest("li, article, [data-hveid]") || a.parentElement?.parentElement || a;
      const title = text(h);
      results.push({ title, url, snippet: clean(text(block).replace(title, "")).slice(0, 320), source: host });
    }
    return { results, answer: "", related: [], hasNext: results.length > 0, noResults: false };
  }

  function detectBlock() {
    const href = location.href;
    if (/\/sorry\/|\/captcha/i.test(href)) return "captcha";
    if (document.querySelector("#captcha-form, form[action*='sorry'], iframe[src*='recaptcha'], iframe[src*='hcaptcha'], #challenge-form, .cf-turnstile, iframe[src*='challenges.cloudflare'], #b_captcha")) return "captcha";
    const t = (document.body?.innerText || "").slice(0, 4000);
    if (/^captcha|captcha -/i.test(document.title)) return "captcha";
    if (/unusual traffic|are you a robot|verify you are (a )?human|verifying you'?re not a bot|i'?m not a robot|bots use duckduckgo too|quick check before you continue/i.test(t)) return "captcha";
    if (/(^|\.)consent\.(google|yahoo)\.|guce\.yahoo\./.test(location.hostname)) return "consent";
    return null;
  }

  const EXTRACTORS = { google, bing, duckduckgo, brave, yahoo };

  function extract(engine) {
    const block = detectBlock();
    if (block) return { block, results: [], answer: "", related: [], hasNext: false, noResults: false, url: location.href };
    let data = (EXTRACTORS[engine] || generic)();
    let usedFallback = false;
    if (!data.results.length && !data.noResults && EXTRACTORS[engine]) {
      const fallback = generic();
      if (fallback.results.length) {
        data = { ...data, results: fallback.results };
        usedFallback = true;
      }
    }
    const seen = new Set();
    data.results = data.results.filter((r) => r.title && r.url && !seen.has(r.url) && seen.add(r.url));
    return { ...data, block: null, usedFallback, url: location.href };
  }

  function nextPage(engine) {
    if (engine === "duckduckgo") {
      const form = duckNextForm();
      if (!form) return false;
      // Deferred so this call returns before the page starts unloading.
      setTimeout(() => form.submit(), 0);
      return true;
    }
    const link = document.querySelector("#pnnext, a.sb_pagN, a.next, a[aria-label='Next page']");
    if (!link) return false;
    setTimeout(() => link.click(), 0);
    return true;
  }

  globalThis.ZB_search = { extract, nextPage };
})();
