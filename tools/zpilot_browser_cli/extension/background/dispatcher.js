// Runs tool calls for loopback clients and the built-in console page. Both use
// the same port-shaped interface and JSON protocol:
//
//   client -> extension  {type:"call", id, tool, args} | {type:"cancel", id} | {type:"ping", id} | {type:"list_tools", id}
//   extension -> client  {type:"hello", ...} | {type:"progress", id, message}
//                        | {type:"result", id, ok:true, result, duration_ms}
//                        | {type:"result", id, ok:false, error:{code, message, details?}, duration_ms}
//                        | {type:"pong", id} | {type:"tools", id, tools}

import { VERSION, PROTOCOL_VERSION } from "./config.js";
import { validateArgs } from "./schema.js";
import { getSettings } from "./settings.js";
import { jobStarted, jobFinished, jobQueued, jobDequeued, recordActivity } from "./state.js";
import { TOOL_MAP, describeTools } from "./tools/index.js";
import { ToolError, toErrorPayload, whenAborted, throwIfAborted } from "./util.js";
import { ToolScheduler } from "./scheduler.js";

let clientSeq = 0;

export class Dispatcher {
  constructor() {
    this.jobs = new Map();
    this.scheduler = new ToolScheduler();
  }

  attach(port, kind) {
    const client = { uid: ++clientSeq, port, kind, jobs: new Set() };
    port.onMessage.addListener((msg) => this.onMessage(client, msg));
    port.onDisconnect.addListener(() => {
      for (const key of client.jobs) {
        this.jobs.get(key)?.controller.abort(new ToolError("CANCELLED", "The client disconnected."));
      }
    });
    this.post(client, {
      type: "hello",
      name: "ZPilot Browser",
      version: VERSION,
      protocol: PROTOCOL_VERSION,
      tools: describeTools(),
    });
    return client;
  }

  post(client, msg) {
    try {
      client.port.postMessage(msg);
      return true;
    } catch {
      return false;
    }
  }

  onMessage(client, msg) {
    if (!msg || typeof msg !== "object") return;
    switch (msg.type) {
      case "call":
        this.call(client, msg);
        break;
      case "cancel":
        this.jobs.get(`${client.uid}:${msg.id}`)?.controller.abort(new ToolError("CANCELLED", "Cancelled by the client."));
        break;
      case "ping":
        this.post(client, { type: "pong", id: msg.id, version: VERSION, time: Date.now() });
        break;
      case "list_tools":
        this.post(client, { type: "tools", id: msg.id, tools: describeTools() });
        break;
      case "reload":
        // Picks up edited extension files; running calls are cancelled.
        this.post(client, { type: "reloading", id: msg.id });
        setTimeout(() => chrome.runtime.reload(), 200);
        break;
    }
  }

  async call(client, { id, tool, args }) {
    const started = Date.now();
    const key = `${client.uid}:${id}`;
    const reply = (payload) => this.post(client, { type: "result", id, ...payload, duration_ms: Date.now() - started });

    const def = TOOL_MAP.get(tool);
    if (!def) {
      reply({ ok: false, error: { code: "UNKNOWN_TOOL", message: `Unknown tool "${tool}". Available: ${[...TOOL_MAP.keys()].join(", ")}.` } });
      return;
    }
    if (this.jobs.has(key)) {
      reply({ ok: false, error: { code: "DUPLICATE_ID", message: `A call with id "${id}" is already running.` } });
      return;
    }
    if (this.jobs.size >= 128) {
      reply({ ok: false, error: { code: "QUEUE_FULL", message: "Too many pending requests. Retry later." } });
      return;
    }

    let args_;
    try {
      args_ = validateArgs(def.inputSchema, args);
    } catch (e) {
      reply({ ok: false, error: toErrorPayload(e) });
      return;
    }

    const controller = new AbortController();
    const timeoutSec = args_.timeout_sec ?? def.defaultTimeoutSec ?? 60;
    // Tools enforce timeout_sec themselves with a precise error; this is the hard stop.
    const hardStop = setTimeout(
      () => controller.abort(new ToolError("TIMEOUT", `${tool} did not finish within ${timeoutSec}s.`)),
      (timeoutSec + 5) * 1000
    );
    this.jobs.set(key, { controller });
    client.jobs.add(key);
    jobQueued();
    let queued = true;
    let running = false;

    const ctx = {
      id,
      signal: controller.signal,
      deadline: started + timeoutSec * 1000,
      progress: (message) => this.post(client, { type: "progress", id, message }),
    };

    let ok = false;
    let summary = "";
    try {
      const work = this.scheduler.schedule(tool, controller.signal, async () => {
        const settings = await getSettings();
        throwIfAborted(controller.signal);
        if (settings.disabledTools.includes(tool)) throw new ToolError("TOOL_DISABLED", `${tool} is turned off in the extension settings.`);
        queued = false;
        jobDequeued();
        running = true;
        jobStarted();
        ctx.settings = settings;
        return def.run(args_, ctx);
      });
      ctx.progress(`Waiting in the ${tool} queue…`);
      const result = await Promise.race([work, whenAborted(controller.signal)]);
      ok = true;
      summary = def.summarize?.(args_, result) || "";
      reply({ ok: true, result });
    } catch (e) {
      const error = toErrorPayload(e);
      summary = `${error.code}: ${error.message}`;
      reply({ ok: false, error });
    } finally {
      clearTimeout(hardStop);
      if (!controller.signal.aborted) controller.abort(new ToolError("CANCELLED", "Finished."));
      this.jobs.delete(key);
      client.jobs.delete(key);
      if (queued) jobDequeued();
      if (running) jobFinished();
      recordActivity({
        tool,
        ok,
        label: def.label?.(args_) || "",
        summary: summary.slice(0, 200),
        ms: Date.now() - started,
        at: Date.now(),
        via: client.kind,
      });
    }
  }
}
