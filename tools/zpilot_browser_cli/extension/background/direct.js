// Loopback transport served by any tool binary. Connection IDs are secret,
// per-profile credentials; Chrome's runtime.id is only the extension's address.
import { setLinkStatus } from "./state.js";

const BASE = "http://127.0.0.1:19280";
const ID_KEY = "connectionId";

function newID() {
  const secret = crypto.getRandomValues(new Uint8Array(32));
  return `${chrome.runtime.id}.${Array.from(secret, (b) => b.toString(16).padStart(2, "0")).join("")}`;
}

function event() {
  const listeners = new Set();
  return { addListener: (fn) => listeners.add(fn), emit: (...args) => { for (const fn of listeners) fn(...args); } };
}

export class DirectLink {
  constructor(dispatcher) {
    this.dispatcher = dispatcher;
    this.id = null;
    this.loadingID = null;
    this.connection = null;
    this.timer = null;
    this.backoff = 1500;
  }

  async identity() {
    if (!this.loadingID) this.loadingID = (async () => {
      const saved = await chrome.storage.local.get(ID_KEY);
      this.id = saved[ID_KEY] || newID();
      if (!saved[ID_KEY]) await chrome.storage.local.set({ [ID_KEY]: this.id });
      return this.id;
    })();
    return this.loadingID;
  }

  async connect() {
    if (this.connection) return;
    clearTimeout(this.timer);
    const id = await this.identity();
    if (this.connection) return;
    const connection = {
      id, instance: crypto.randomUUID(), controller: new AbortController(),
      onMessage: event(), onDisconnect: event(), attached: false, closed: false,
      replies: Promise.resolve(),
    };
    connection.postMessage = (message) => {
      if (connection.closed) throw new Error("Connection closed");
      // Preserve progress/result order and never retry a result as a new call.
      connection.replies = connection.replies.then(() => this.request(connection, "/reply", { instance: connection.instance, message }));
      connection.replies.catch((e) => this.failed(connection, e));
    };
    this.connection = connection;
    this.loop(connection).catch((e) => this.failed(connection, e));
  }

  async request(connection, path, body) {
    const response = await fetch(`${BASE}${path}`, {
      method: "POST", headers: { "Content-Type": "application/json", Authorization: `Bearer ${connection.id}` },
      body: JSON.stringify(body), signal: connection.controller.signal, cache: "no-store",
      credentials: "omit", redirect: "error",
    });
    if (!response.ok) {
      const error = new Error((await response.text()).slice(0, 500));
      error.status = response.status;
      throw error;
    }
    return response.json();
  }

  async loop(connection) {
    while (!connection.closed) {
      const data = await this.request(connection, "/poll", { instance: connection.instance });
      if (connection.closed) return;
      this.backoff = 1500;
      if (!connection.attached) {
        connection.attached = true;
        this.dispatcher.attach(connection, "direct");
      }
      // A poll completes every ten seconds. This extension API call also keeps
      // the worker alive during a live connection; alarms wake an idle worker.
      await setLinkStatus({ state: "connected", since: Date.now() });
      for (const message of data.messages || []) connection.onMessage.emit(message);
    }
  }

  stop(connection) {
    if (!connection || connection.closed) return;
    connection.closed = true;
    connection.controller.abort();
    connection.onDisconnect.emit();
    if (this.connection === connection) this.connection = null;
  }

  failed(connection, error) {
    if (connection.closed || this.connection !== connection) return;
    this.stop(connection);
    const duplicate = error?.status === 409;
    setLinkStatus({ state: duplicate ? "duplicate" : "ready", error: duplicate ? error.message : undefined });
    clearTimeout(this.timer);
    const delay = duplicate ? 30000 : this.backoff;
    this.backoff = Math.min(this.backoff * 2, 30000);
    this.timer = setTimeout(() => this.connect(), delay);
  }

  async reconnect() {
    const connection = this.connection;
    clearTimeout(this.timer);
    // Keep the same instance on a reconnect to avoid stealing another profile's
    // identity. Explicitly disconnect the old instance before creating a new one.
    if (connection) {
      try {
        await fetch(`${BASE}/disconnect`, {
          method: "POST", headers: { "Content-Type": "application/json", Authorization: `Bearer ${connection.id}` },
          body: JSON.stringify({ instance: connection.instance }), signal: AbortSignal.timeout(2000),
          credentials: "omit", redirect: "error",
        });
      } catch {}
      this.stop(connection);
    }
    await this.connect();
  }

  async reset() {
    clearTimeout(this.timer);
    const connection = this.connection;
    // Stop accepting old calls immediately. The broker fails its outstanding
    // callers, and disconnect aborts every queued/running extension job.
    this.stop(connection);
    if (connection) {
      try {
        await fetch(`${BASE}/disconnect`, {
          method: "POST", headers: { "Content-Type": "application/json", Authorization: `Bearer ${connection.id}` },
          body: JSON.stringify({ instance: connection.instance }), signal: AbortSignal.timeout(2000),
          credentials: "omit", redirect: "error",
        });
      } catch {}
    }
    this.id = newID();
    this.loadingID = Promise.resolve(this.id);
    await chrome.storage.local.set({ [ID_KEY]: this.id });
    await setLinkStatus({ state: "ready" });
    await this.connect();
    return this.id;
  }
}
