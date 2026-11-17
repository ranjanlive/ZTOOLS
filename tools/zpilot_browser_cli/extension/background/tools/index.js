// Registry of the tools ZPilot Browser exposes. The schemas here are the single source of truth:
// they are sent to clients in the "hello" message.

import zagentInChrome from "./zagent_in_chrome.js";
import generateImage from "./generate_image.js";
import generateMusic from "./generate_music.js";
import webSearch from "./web_search.js";
import webpageRead from "./webpage_read.js";

export const TOOLS = [zagentInChrome, generateImage, generateMusic, webSearch, webpageRead];
export const TOOL_MAP = new Map(TOOLS.map((t) => [t.name, t]));

export function describeTools() {
  return TOOLS.map(({ name, title, description, inputSchema }) => ({ name, title, description, inputSchema }));
}
