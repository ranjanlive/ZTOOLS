const response = await chrome.runtime.sendMessage({ type: "zpilot.reconnect" });
document.getElementById("status").textContent = response?.ok ? "This profile is ready for your tools." : "Open the extension popup to reconnect.";
