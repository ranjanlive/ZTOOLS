// web_search: runs a query on a real search engine in a background ZPilot Browser tab and returns
// the organic results. Supports Google (default), Bing, DuckDuckGo, Brave and Yahoo.

import { CONTENT } from "../config.js";
import { callContent } from "../inject.js";
import { closeTab, focusTab, navigate, openTab, watchNavigation } from "../tabs.js";
import { ToolError, pollUntil, sleep, truncate } from "../util.js";

const MAX_PAGES = 5;

const TIME = {
  google: { day: "qdr:d", week: "qdr:w", month: "qdr:m", year: "qdr:y" },
  bing: { day: 'ex1:"ez1"', week: 'ex1:"ez2"', month: 'ex1:"ez3"' },
  duckduckgo: { day: "d", week: "w", month: "m", year: "y" },
  brave: { day: "pd", week: "pw", month: "pm", year: "py" },
  yahoo: { day: "d", week: "w", month: "m" },
};

const ENGINES = {
  google: {
    label: "Google",
    url({ q, page, time, language, region }) {
      const u = new URL("https://www.google.com/search");
      u.searchParams.set("q", q);
      if (language) u.searchParams.set("hl", language);
      if (region) u.searchParams.set("gl", region);
      if (time) u.searchParams.set("tbs", time);
      if (page) u.searchParams.set("start", String(page * 10));
      return u.href;
    },
  },
  bing: {
    label: "Bing",
    clickPaging: true, // Bing often ignores &first= on direct loads; its Next link works
    url({ q, page, time, language, region }) {
      const u = new URL("https://www.bing.com/search");
      u.searchParams.set("q", q);
      if (language) u.searchParams.set("setlang", language);
      if (region) u.searchParams.set("cc", region);
      if (time) u.searchParams.set("filters", time);
      if (page) u.searchParams.set("first", String(page * 10 + 1));
      return u.href;
    },
  },
  duckduckgo: {
    label: "DuckDuckGo",
    clickPaging: true, // the HTML endpoint pages through a form POST
    url({ q, time, region }) {
      const u = new URL("https://html.duckduckgo.com/html/");
      u.searchParams.set("q", q);
      if (time) u.searchParams.set("df", time);
      if (region) u.searchParams.set("kl", region);
      return u.href;
    },
  },
  brave: {
    label: "Brave Search",
    url({ q, page, time, region }) {
      const u = new URL("https://search.brave.com/search");
      u.searchParams.set("q", q);
      u.searchParams.set("source", "web");
      if (time) u.searchParams.set("tf", time);
      if (region) u.searchParams.set("country", region);
      if (page) u.searchParams.set("offset", String(page));
      return u.href;
    },
  },
  yahoo: {
    label: "Yahoo",
    url({ q, page, time }) {
      const u = new URL("https://search.yahoo.com/search");
      u.searchParams.set("p", q);
      if (time) u.searchParams.set("btf", time);
      if (page) u.searchParams.set("b", String(page * 10 + 1));
      return u.href;
    },
  },
};

const ALIASES = { ddg: "duckduckgo", duck: "duckduckgo", "duck duck go": "duckduckgo", brave_search: "brave", msn: "bing" };

export default {
  name: "web_search",
  title: "Web search",
  description:
    "Searches the web with a real search engine in the user's Chrome (no API keys, no bot blocks) and returns organic results " +
    "with title, URL and snippet, plus the engine's direct answer and related questions when shown. " +
    "Engines: google (default), bing, duckduckgo, brave, yahoo. Read a result in full with webpage_read.",
  defaultTimeoutSec: 45,
  inputSchema: {
    type: "object",
    properties: {
      query: { type: "string", description: "Search query. Engine operators such as quotes, -exclude or filetype: are passed through." },
      engine: { type: "string", description: "Search engine: google, bing, duckduckgo, brave or yahoo. Defaults to the engine chosen in the extension (Google)." },
      max_results: { type: "integer", minimum: 1, maximum: 50, default: 10, description: "Number of results to return; more than one page is fetched when needed." },
      site: { type: "string", description: "Restrict results to one site, e.g. \"github.com\" (adds site:)." },
      time_range: { type: "string", enum: ["any", "day", "week", "month", "year"], default: "any", description: "Only results from this period (bing and yahoo have no \"year\")." },
      language: { type: "string", description: "Interface/result language code such as \"en\" or \"de\" (google, bing)." },
      region: { type: "string", description: "Country code such as \"us\" or \"in\" (google, bing, brave; duckduckgo uses codes like \"us-en\")." },
      background: { type: "boolean", default: true, description: "Search in a background tab without taking focus." },
      keep_tab: { type: "boolean", default: false, description: "Leave the results tab open afterwards." },
      fallback: { type: "boolean", default: true, description: "When the engine shows a CAPTCHA, a consent page or an unreadable layout, retry once with another engine (bing, then duckduckgo)." },
      timeout_sec: { type: "integer", minimum: 10, maximum: 180, default: 45 },
    },
    required: ["query"],
  },

  label: (args) => `${args.engine || "default"}: ${truncate(args.query, 70)}`,
  summarize: (args, result) => `${result.results.length} result(s) from ${result.engine}`,

  async run(args, ctx) {
    const first = resolveEngine(args.engine || ctx.settings.defaultEngine);
    const order = [first, ...["bing", "duckduckgo", "google"].filter((e) => e !== first)].slice(0, args.fallback ? 3 : 1);
    const skipped = [];
    for (const engine of order) {
      try {
        const result = await this.runOnce({ ...args, engine, keep_tab: args.keep_tab && skipped.length === 0 }, ctx);
        if (skipped.length) result.notes = [...(result.notes || []), `Fell back to ${engine}: ${skipped.join("; ")}.`];
        return result;
      } catch (e) {
        const retryable = e instanceof ToolError && ["CAPTCHA", "CONSENT_REQUIRED", "NO_RESULTS_PARSED"].includes(e.code);
        if (!retryable || engine === order[order.length - 1] || Date.now() > ctx.deadline - 15000) throw e;
        skipped.push(`${engine} ${e.code}`);
        ctx.progress(`${engine} is blocked (${e.code}); trying another engine…`);
        if (e.details?.tab_id) closeTab(e.details.tab_id);
      }
    }
  },

  async runOnce(args, ctx) {
    const { signal } = ctx;
    const engineName = resolveEngine(args.engine || ctx.settings.defaultEngine);
    const engine = ENGINES[engineName];
    const q = args.site ? `${args.query} site:${args.site.replace(/^https?:\/\//, "").replace(/\/.*$/, "")}` : args.query;
    const notes = [];
    let time = "";
    if (args.time_range !== "any") {
      time = TIME[engineName][args.time_range] || "";
      if (!time) notes.push(`${engine.label} has no "${args.time_range}" filter; results are not time-limited.`);
    }
    const params = { q, time, language: args.language, region: args.region };
    const deadline = ctx.deadline - 1500;

    ctx.progress(`Searching ${engine.label} for "${truncate(args.query, 80)}"…`);
    const tabId = await openTab({ kind: "tools", active: !args.background });
    let keepTab = args.keep_tab;
    try {
      await navigate(tabId, engine.url({ ...params, page: 0 }), { timeoutMs: Math.min(20000, deadline - Date.now()), signal });

      const results = [];
      const seen = new Set();
      let answer = "";
      let related = [];
      let usedFallback = false;

      for (let page = 0; page < MAX_PAGES && results.length < args.max_results; page++) {
        if (page > 0) {
          if (Date.now() > deadline - 6000) {
            notes.push("Stopped paging early to stay within the time limit.");
            break;
          }
          ctx.progress(`Fetching results page ${page + 1}…`);
          if (engine.clickPaging) {
            const moved = await watchNavigation(tabId, () => callContent(tabId, CONTENT.search, "ZB_search", "nextPage", [engineName]), {
              timeoutMs: 15000,
              signal,
            }).catch(() => null);
            if (!moved) break;
          } else {
            await navigate(tabId, engine.url({ ...params, page }), { timeoutMs: 15000, signal });
          }
        }

        const data = await readResults(tabId, engineName, page === 0 ? 12000 : 8000, signal);
        if (!data) {
          if (page === 0) throw new ToolError("NO_RESULTS_PARSED", `Could not read ${engine.label}'s result page (its layout may have changed). Try another engine.`);
          break;
        }
        if (data.block === "captcha") {
          keepTab = true;
          await focusTab(tabId).catch(() => {});
          throw new ToolError("CAPTCHA", `${engine.label} is asking for a CAPTCHA / human check. Solve it in the opened "ZPilot Browser" tab, then retry — or use another engine.`, { tab_id: tabId, engine: engineName });
        }
        if (data.block === "consent") {
          keepTab = true;
          await focusTab(tabId).catch(() => {});
          throw new ToolError("CONSENT_REQUIRED", `${engine.label} shows a cookie-consent page. Accept or reject it once in the opened tab, then retry.`, { tab_id: tabId, engine: engineName });
        }
        if (page === 0) {
          answer = data.answer;
          related = data.related;
        }
        usedFallback ||= data.usedFallback;

        let added = 0;
        for (const r of data.results) {
          if (seen.has(r.url)) continue;
          seen.add(r.url);
          results.push({ position: results.length + 1, title: r.title, url: r.url, snippet: r.snippet, source: r.source });
          added++;
        }
        if (!added || !data.hasNext) break;
      }

      const finalResults = results.slice(0, args.max_results);
      await Promise.all(
        finalResults.map(async (r) => {
          if (GOOGLE_REDIRECT.test(r.url)) r.url = await resolveRedirect(r.url);
        })
      );

      if (usedFallback) notes.push(`${engine.label}'s usual result layout was not found; results were read with the generic parser.`);
      return {
        engine: engineName,
        query: args.query,
        results: finalResults,
        ...(answer ? { answer } : {}),
        ...(related.length ? { related_questions: related } : {}),
        ...(notes.length ? { notes } : {}),
      };
    } finally {
      if (!keepTab) closeTab(tabId);
    }
  },
};

const GOOGLE_REDIRECT = /^https:\/\/www\.google\.[a-z.]+\/(goto|url)\?/;

// Google sometimes links results through an opaque /goto?url=… redirect. Following it only until
// the response headers arrive reveals the destination without downloading the page.
async function resolveRedirect(url, timeoutMs = 5000) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const res = await fetch(url, { credentials: "include", redirect: "follow", signal: controller.signal });
    controller.abort();
    return res.redirected && /^https?:/.test(res.url) ? res.url : url;
  } catch {
    return url;
  } finally {
    clearTimeout(timer);
  }
}

function resolveEngine(name) {
  const key = String(name || "google").trim().toLowerCase();
  const resolved = ALIASES[key] || key;
  if (!ENGINES[resolved]) {
    throw new ToolError("INVALID_ARGS", `Unknown search engine "${name}". Use one of: ${Object.keys(ENGINES).join(", ")}.`);
  }
  return resolved;
}

// Polls the page until results (or a block / no-results page) are visible, then reads them once
// more after a short pause so late-rendering snippets are included.
async function readResults(tabId, engineName, timeoutMs, signal) {
  const extract = () => callContent(tabId, CONTENT.search, "ZB_search", "extract", [engineName], { timeoutMs: 10000 });
  const first = await pollUntil(
    async () => {
      const d = await extract();
      return d.block || d.noResults || d.results.length ? d : null;
    },
    { timeoutMs, intervalMs: 400, signal }
  );
  if (!first || first.block || !first.results.length) return first;
  await sleep(350, signal);
  const second = await extract().catch(() => null);
  return second && second.results.length >= first.results.length ? second : first;
}
