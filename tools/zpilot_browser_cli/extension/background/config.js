// Shared constants for the ZPilot Browser service worker.

export const VERSION = chrome.runtime.getManifest().version;

// Version of the message protocol spoken over the native port. Bump on breaking changes.
export const PROTOCOL_VERSION = 2;

// Tab groups ZPilot Browser works in. "tools" holds job tabs (search, page reading, ChatGPT, Gemini);
// "agent" holds the tabs that zagent_in_chrome drives.
export const TAB_GROUPS = {
  tools: { title: "ZPilot", color: "purple" },
  agent: { title: "ZPilot Agent", color: "cyan" },
};

// Content modules, injected on demand (never into every page).
export const CONTENT = {
  chatgpt: ["content/chatgpt.js"],
  search: ["content/search.js"],
  reader: ["content/reader.js"],
  agent: ["content/agent.js"],
  gemini: ["content/gemini.js"],
};
