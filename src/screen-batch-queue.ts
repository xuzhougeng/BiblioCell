import type { ScreeningSuggestion } from "./mygo.ts";
import type { AIResult } from "./screen.ts";

/** A bounded pool. Stopping drains in-flight calls, leaving untouched papers queued. */
export class ScreeningBatch {
  readonly ids: string[];
  readonly results = new Map<string, AIResult>();
  running = false;
  stopping = false;
  readonly requirement: string;
  readonly modelId: string;
  readonly concurrency: number;
  /** Every answer of this run is recorded under runId, as a batch or a calibration rerun. */
  readonly runId: string;
  readonly kind: "batch" | "rerun";

  constructor(ids: string[], requirement: string, modelId: string, concurrency: number, runId = "", kind: "batch" | "rerun" = "batch") {
    if (!Number.isInteger(concurrency) || concurrency < 1 || concurrency > 8) throw new Error("Invalid concurrency");
    this.ids = [...new Set(ids)];
    this.requirement = requirement;
    this.modelId = modelId;
    this.concurrency = concurrency;
    this.runId = runId;
    this.kind = kind;
  }

  stop(): void { this.stopping = true; }

  async run(request: (id: string) => Promise<ScreeningSuggestion>, changed: (id?: string) => void): Promise<void> {
    if (this.running) return;
    this.running = true;
    this.stopping = false;
    const pending = this.ids.filter(id => this.results.get(id)?.state !== "done");
    let cursor = 0;
    const worker = async () => {
      while (!this.stopping && cursor < pending.length) {
        const id = pending[cursor++]!;
        this.results.set(id, { state: "loading" });
        changed(id);
        try {
          const judgement = await request(id);
          this.results.set(id, { state: "done", judgement });
        } catch (error) {
          this.results.set(id, { state: "error", message: error instanceof Error ? error.message : String(error) });
        }
        changed(id);
      }
    };
    try { await Promise.all(Array.from({ length: Math.min(this.concurrency, pending.length) }, worker)); }
    finally { this.running = false; changed(); }
  }
}
