import test from "node:test";
import assert from "node:assert/strict";

function event() {
  const listeners = new Set();
  return { addListener: (fn) => listeners.add(fn), removeListener: (fn) => listeners.delete(fn),
    emit: (value) => { for (const fn of listeners) fn(value); }, get size() { return listeners.size; } };
}
globalThis.chrome = {
  runtime: { getManifest: () => ({ version: "2.0.0" }) },
  webNavigation: { onCommitted: event(), onCompleted: event(), onErrorOccurred: event() },
  tabs: { onRemoved: event() },
};
const { watchNavigation } = await import("../background/tabs.js");

test("pre-cancelled navigation does not trigger a browser action", async () => {
  const controller = new AbortController();
  controller.abort();
  let triggered = false;
  await assert.rejects(watchNavigation(1, () => { triggered = true; }, { signal: controller.signal }), { code: "CANCELLED" });
  assert.equal(triggered, false);
});

test("cancellation before the trigger microtask also prevents navigation", async () => {
  const controller = new AbortController();
  let triggered = false;
  const job = watchNavigation(1, () => { triggered = true; }, { signal: controller.signal });
  controller.abort();
  await assert.rejects(job, { code: "CANCELLED" });
  assert.equal(triggered, false);
  assert.equal(chrome.webNavigation.onCommitted.size, 0);
});

test("a late blank-document error does not fail the real navigation", async () => {
  const value = await watchNavigation(1, () => {
    chrome.webNavigation.onErrorOccurred.emit({ tabId: 1, frameId: 0, url: "about:blank", error: "net::ERR_FAILED" });
    const details = { tabId: 1, frameId: 0, url: "https://example.test/" };
    chrome.webNavigation.onCommitted.emit(details);
    chrome.webNavigation.onCompleted.emit(details);
  });
  assert.equal(value.url, "https://example.test/");
  assert.equal(value.timedOut, false);
  assert.equal(chrome.webNavigation.onErrorOccurred.size, 0);
});
