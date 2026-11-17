// webpage_read: loads a URL in a background ZPilot Browser tab with the user's session and returns the
// rendered page as Markdown (default), plain text or HTML, in chunks for long pages.

import { CONTENT } from "../config.js";
import { callContent } from "../inject.js";
import { closeTab, navigate, openTab } from "../tabs.js";
import { ToolError, normalizeUrl, pollUntil, sleep, sliceChars, truncate } from "../util.js";

export default {
  name: "webpage_read",
  title: "Read web page",
  description:
    "Opens a URL in the user's Chrome (with their logins, after JavaScript has run) and returns its content. " +
    "format=markdown (default) gives clean Markdown of the main content with links, images and tables; text gives plain text; html gives the page source (rendered DOM). " +
    "Set include_links/include_images/include_media to false to drop links, images or attachments (videos, embeds, downloadable files). " +
    "Long pages are returned in chunks: when has_more is true, call again with offset=next_offset.",
  defaultTimeoutSec: 45,
  inputSchema: {
    type: "object",
    properties: {
      url: { type: "string", description: "Page to fetch (http or https; https:// is added when missing)." },
      format: { type: "string", enum: ["markdown", "text", "html"], default: "markdown", description: "Output format." },
      main_content_only: { type: "boolean", description: "Strip navigation, sidebars, footers, ads and cookie banners. Default: true for markdown/text, false for html (full source)." },
      include_links: { type: "boolean", default: true, description: "Keep hyperlinks as [text](url). When false only the link text remains." },
      include_images: { type: "boolean", default: true, description: "Keep images as ![alt](url)." },
      include_media: { type: "boolean", default: true, description: "Keep videos, audio, embeds and downloadable files, and list them in attachments." },
      max_chars: { type: "integer", minimum: 0, maximum: 2000000, default: 50000, description: "Maximum characters returned per call (0 = no limit)." },
      offset: { type: "integer", minimum: 0, default: 0, description: "Character offset to continue from (use next_offset from the previous call)." },
      wait_for: { type: "string", description: "CSS selector to wait for before reading (for pages that load content late)." },
      wait_ms: { type: "integer", minimum: 0, maximum: 30000, default: 0, description: "Extra time to wait after the page has loaded." },
      background: { type: "boolean", default: true, description: "Load in a background tab without taking focus." },
      keep_tab: { type: "boolean", default: false, description: "Leave the tab open afterwards." },
      timeout_sec: { type: "integer", minimum: 5, maximum: 300, default: 45 },
    },
    required: ["url"],
  },

  label: (args) => truncate(args.url, 90),
  summarize: (args, result) => `${result.returned_chars} of ${result.total_chars} chars (${result.format})`,

  async run(args, ctx) {
    const { signal } = ctx;
    const url = normalizeUrl(args.url);
    const deadline = ctx.deadline - 1500;
    const warnings = [];

    ctx.progress(`Loading ${truncate(url, 100)}…`);
    const tabId = await openTab({ kind: "tools", active: !args.background });
    try {
      const nav = await navigate(tabId, url, { timeoutMs: Math.max(3000, Math.min(30000, deadline - Date.now() - 4000)), signal });
      if (nav.timedOut) warnings.push("The page was still loading when it was read; some content may be missing.");

      if (args.wait_for) {
        const found = await pollUntil(() => callContent(tabId, CONTENT.reader, "ZB_reader", "exists", [{ selector: args.wait_for }]), {
          timeoutMs: Math.max(1000, deadline - Date.now() - 3000),
          intervalMs: 500,
          signal,
        });
        if (!found) warnings.push(`"${args.wait_for}" did not appear before the time limit.`);
      }
      await callContent(tabId, CONTENT.reader, "ZB_reader", "settle", [{ quietMs: 500, maxMs: Math.min(4000, Math.max(0, deadline - Date.now() - 3000)) }]);
      if (args.wait_ms) await sleep(Math.min(args.wait_ms, Math.max(0, deadline - Date.now() - 2000)), signal);

      ctx.progress("Reading the page…");
      const page = await callContent(
        tabId,
        CONTENT.reader,
        "ZB_reader",
        "extract",
        [
          {
            format: args.format,
            mainContentOnly: args.main_content_only,
            includeLinks: args.include_links,
            includeImages: args.include_images,
            includeMedia: args.include_media,
          },
        ],
        { timeoutMs: 30000 }
      );

      const chunk = sliceChars(page.content, args.offset, args.max_chars);
      if (page.http_status >= 400) warnings.push(`The server answered with HTTP ${page.http_status}.`);
      if (args.format !== "html" && page.word_count < 30 && !chunk.start) warnings.push("Very little text was found; the page may need a login, a wait_for selector or more wait_ms.");

      return {
        url,
        final_url: page.url,
        title: page.title,
        ...(page.description ? { description: page.description } : {}),
        ...(page.author ? { author: page.author } : {}),
        ...(page.published ? { published: page.published } : {}),
        ...(page.site_name ? { site_name: page.site_name } : {}),
        ...(page.language ? { language: page.language } : {}),
        ...(page.http_status ? { http_status: page.http_status } : {}),
        content_type: page.content_type,
        format: args.format,
        content: chunk.text,
        total_chars: chunk.total,
        offset: chunk.start,
        returned_chars: chunk.end - chunk.start,
        has_more: chunk.hasMore,
        ...(chunk.hasMore ? { next_offset: chunk.end } : {}),
        word_count: page.word_count,
        ...(page.attachments?.length ? { attachments: page.attachments } : {}),
        ...(warnings.length ? { warnings } : {}),
      };
    } catch (e) {
      if (e instanceof ToolError && e.code === "PAGE_NOT_SCRIPTABLE") {
        throw new ToolError("PAGE_NOT_SCRIPTABLE", `${url} cannot be read by extensions (browser-internal or store page).`);
      }
      throw e;
    } finally {
      if (!args.keep_tab) closeTab(tabId);
    }
  },
};
