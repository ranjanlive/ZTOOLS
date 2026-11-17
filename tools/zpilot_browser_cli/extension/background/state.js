// Runtime state shown in the popup: link status, running jobs and recent activity.
// Kept in chrome.storage.session so it survives service-worker restarts but not browser restarts.

const MAX_ACTIVITY = 30;
let running = 0;
let queued = 0;

export function jobQueued() { queued++; publishRunning(); }
export function jobDequeued() { queued = Math.max(0, queued - 1); publishRunning(); }

export function jobStarted() {
  running++;
  publishRunning();
}

export function jobFinished() {
  running = Math.max(0, running - 1);
  publishRunning();
}

function publishRunning() {
  chrome.action.setBadgeText({ text: running + queued > 0 ? String(running + queued) : "" }).catch(() => {});
  chrome.action.setBadgeBackgroundColor({ color: "#7c3aed" }).catch(() => {});
  chrome.storage.session.set({ running, queued }).catch(() => {});
}

// Serialized so concurrent jobs do not overwrite each other's entries.
let activityWrite = Promise.resolve();

export function recordActivity(entry) {
  activityWrite = activityWrite
    .then(async () => {
      const { activity = [] } = await chrome.storage.session.get("activity");
      activity.unshift(entry);
      await chrome.storage.session.set({ activity: activity.slice(0, MAX_ACTIVITY) });
    })
    .catch(() => {});
  return activityWrite;
}

export function setLinkStatus(status) {
  return chrome.storage.session.set({ link: status }).catch(() => {});
}
