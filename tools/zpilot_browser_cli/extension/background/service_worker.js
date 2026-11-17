// ZPilot Browser service worker: connects to independent tool binaries.

import { Dispatcher } from "./dispatcher.js";
import { DirectLink } from "./direct.js";
import { updateSettings } from "./settings.js";
import { endAgentSession } from "./tools/zagent_in_chrome.js";

const dispatcher = new Dispatcher();
const link = new DirectLink(dispatcher);

// The console page uses the same dispatcher and queues as independent tools.
chrome.runtime.onConnect.addListener((port) => {
  if (port.sender?.id !== chrome.runtime.id || port.name !== "zpilot-console") {
    port.disconnect();
    return;
  }
  dispatcher.attach(port, "console");
});

chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (sender.id !== chrome.runtime.id || !msg || typeof msg.type !== "string") return;
  switch (msg.type) {
    case "zpilot.reconnect":
      link.reconnect().then(() => sendResponse({ ok: true }));
      return true;
    case "zpilot.identity":
      link.connect();
      link.identity().then((id) => sendResponse({ ok: true, id }));
      return true;
    case "zpilot.resetId":
      link.reset().then((id) => sendResponse({ ok: true, id }));
      return true;
    case "zpilot.settings":
      updateSettings(msg.patch || {}).then((settings) => sendResponse({ ok: true, settings }));
      return true;
    case "zpilot.endAgent":
      endAgentSession({ closeTabs: !!msg.closeTabs }).then(
        () => sendResponse({ ok: true }),
        (e) => sendResponse({ ok: false, error: String(e?.message || e) })
      );
      return true;
  }
});

// Idle service workers can sleep; waking takes at most 30s, or open the popup.
chrome.alarms.create("zpilot-heartbeat", { periodInMinutes: 0.5 });
chrome.alarms.onAlarm.addListener((alarm) => {
  if (alarm.name === "zpilot-heartbeat") link.connect();
});

chrome.runtime.onStartup.addListener(() => link.connect());
chrome.runtime.onInstalled.addListener(() => link.connect());

chrome.action.setBadgeText({ text: "" }).catch(() => {});
link.connect();
