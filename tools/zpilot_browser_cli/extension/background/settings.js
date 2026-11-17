// User settings, edited from the popup and stored in chrome.storage.local.

export const DEFAULT_SETTINGS = {
  disabledTools: [],
  defaultEngine: "google",
  showCursor: true,
  keepTabsOnError: false,
};

export async function getSettings() {
  const { settings } = await chrome.storage.local.get("settings");
  return { ...DEFAULT_SETTINGS, ...(settings || {}) };
}

export async function updateSettings(patch) {
  const next = { ...(await getSettings()), ...patch };
  await chrome.storage.local.set({ settings: next });
  return next;
}
