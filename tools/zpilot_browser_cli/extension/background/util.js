// Small helpers shared by the service worker modules.

// An error a tool reports to the agent. `code` is a stable machine-readable identifier.
export class ToolError extends Error {
  constructor(code, message, details) {
    super(message);
    this.name = "ToolError";
    this.code = code;
    if (details !== undefined) this.details = details;
  }
}

export function toErrorPayload(err) {
  if (err instanceof ToolError) {
    const payload = { code: err.code, message: err.message };
    if (err.details !== undefined) payload.details = err.details;
    return payload;
  }
  return { code: "INTERNAL", message: String(err?.message || err) };
}

export function abortReason(signal) {
  const reason = signal?.reason;
  if (reason instanceof ToolError) return reason;
  return new ToolError("CANCELLED", typeof reason === "string" ? reason : "The call was cancelled.");
}

export function throwIfAborted(signal) {
  if (signal?.aborted) throw abortReason(signal);
}

export function sleep(ms, signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) return reject(abortReason(signal));
    const timer = setTimeout(() => {
      signal?.removeEventListener("abort", onAbort);
      resolve();
    }, ms);
    function onAbort() {
      clearTimeout(timer);
      reject(abortReason(signal));
    }
    signal?.addEventListener("abort", onAbort, { once: true });
  });
}

// Calls `fn` until it returns a truthy value or the time runs out (then returns null).
// Errors thrown by `fn` propagate, so a check can fail fast.
export async function pollUntil(fn, { timeoutMs, intervalMs = 500, signal } = {}) {
  const deadline = Date.now() + timeoutMs;
  for (;;) {
    throwIfAborted(signal);
    const value = await fn();
    if (value) return value;
    if (Date.now() + intervalMs > deadline) return null;
    await sleep(intervalMs, signal);
  }
}

export function withTimeout(promise, ms, makeError) {
  let timer;
  const timeout = new Promise((_, reject) => {
    timer = setTimeout(() => reject(makeError()), ms);
  });
  return Promise.race([promise, timeout]).finally(() => clearTimeout(timer));
}

// Resolves when the signal aborts; used to race work that cannot be cancelled itself.
export function whenAborted(signal) {
  return new Promise((_, reject) => {
    if (signal.aborted) return reject(abortReason(signal));
    signal.addEventListener("abort", () => reject(abortReason(signal)), { once: true });
  });
}

export function truncate(text, max) {
  if (typeof text !== "string" || text.length <= max) return text;
  return text.slice(0, max - 1) + "…";
}

// Adds https:// when missing and accepts only http(s) URLs with a real host.
export function normalizeUrl(raw) {
  let s = String(raw || "").trim();
  if (!s) throw new ToolError("INVALID_ARGS", "A URL is required.");
  if (!/^[a-z][a-z0-9+.-]*:/i.test(s)) s = "https://" + s;
  let url;
  try {
    url = new URL(s);
  } catch {
    throw new ToolError("INVALID_URL", `"${raw}" is not a valid URL.`);
  }
  if (url.protocol !== "http:" && url.protocol !== "https:") {
    throw new ToolError("INVALID_URL", `Only http and https URLs are supported (got "${url.protocol}").`);
  }
  const host = url.hostname;
  if (!host || (host !== "localhost" && !host.includes(".") && !host.startsWith("["))) {
    throw new ToolError("INVALID_URL", `"${raw}" does not contain a valid host name.`);
  }
  return url.href;
}

// Slices by Unicode code points so a chunk never splits a character.
export function sliceChars(text, offset, maxChars) {
  const chars = Array.from(text);
  const total = chars.length;
  const start = Math.max(0, offset | 0);
  if (start > 0 && start >= total) {
    throw new ToolError("OFFSET_OUT_OF_RANGE", `offset ${offset} is past the end of the content (${total} characters).`);
  }
  const end = maxChars > 0 ? Math.min(total, start + maxChars) : total;
  return { text: chars.slice(start, end).join(""), start, end, total, hasMore: end < total };
}

export function blobToBase64(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result).split(",", 2)[1] || "");
    reader.onerror = () => reject(reader.error);
    reader.readAsDataURL(blob);
  });
}

export function base64ToBlob(data, type) {
  const bin = atob(data);
  const bytes = new Uint8Array(bin.length);
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
  return new Blob([bytes], { type });
}
