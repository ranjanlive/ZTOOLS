import { throwIfAborted } from "./util.js";

// One FIFO per tool, shared across every client. A slow image request never
// blocks web_search, but a second image request waits for the first to settle.
export class ToolScheduler {
  constructor() { this.queues = new Map(); }

  schedule(tool, signal, run) {
    const previous = this.queues.get(tool) || Promise.resolve();
    const job = previous.catch(() => {}).then(() => {
      throwIfAborted(signal);
      return run();
    });
    const tail = job.catch(() => {});
    this.queues.set(tool, tail);
    tail.then(() => { if (this.queues.get(tool) === tail) this.queues.delete(tool); });
    return job;
  }
}
