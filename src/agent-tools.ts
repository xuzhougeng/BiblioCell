// Tool calls in the assistant transcript. A run of consecutive calls folds
// into one card: a status line on top, then one row per call to expand.
// Pure string building, so the node:test suite can cover it.
import type { ToolCall } from "./mygo.ts";
import { esc } from "./model.ts";
import { I } from "./ui.ts";
import { t } from "./i18n.ts";

export interface ToolRun {
  call: ToolCall;
  /** Missing while the call is still running. */
  result?: { content: string; isError?: boolean };
}

export interface ToolCardOptions {
  /** The run is still in flight: the card stays open and the head shows what runs now. */
  running?: boolean;
  step?: number;
  /** Epoch ms the run started; the head then shows the elapsed time and keeps it ticking. */
  startedAt?: number;
}

const DISPLAY_LIMIT = 4000;
const HINT_LIMIT = 90;

export const clip = (text: string): string => (text.length > DISPLAY_LIMIT ? text.slice(0, DISPLAY_LIMIT) + t("\n…（已省略）") : text);

/** argsHint compresses a call's JSON arguments into one line: `status: "pending", limit: 50`. */
export function argsHint(args: string): string {
  let hint = args.replace(/\s+/g, " ").trim();
  try {
    const parsed: unknown = JSON.parse(args);
    if (parsed && typeof parsed === "object" && !Array.isArray(parsed)) {
      hint = Object.entries(parsed as Record<string, unknown>).map(([key, value]) => `${key}: ${compact(value)}`).join(", ");
    }
  } catch {
    // Not JSON (or truncated by the model): show the raw text.
  }
  return hint.length > HINT_LIMIT ? hint.slice(0, HINT_LIMIT - 1) + "…" : hint;
}

function compact(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value.length > 40 ? value.slice(0, 39) + "…" : value);
  if (Array.isArray(value)) return t("[{n} 项]", { n: value.length });
  if (value && typeof value === "object") return "{…}";
  return String(value);
}

/** breakdown counts calls per tool in order of first appearance: `list_papers ×9 · update_paper ×19`. */
export function breakdown(runs: ToolRun[]): string {
  const counts = new Map<string, number>();
  for (const { call } of runs) counts.set(call.name, (counts.get(call.name) ?? 0) + 1);
  return [...counts].map(([name, n]) => `${name} ×${n}`).join(" · ");
}

export const formatElapsed = (ms: number): string => {
  const s = Math.max(0, Math.round(ms / 1000));
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m ${s % 60}s`;
};

const chevron = `<span class="agent-tools-chevron" aria-hidden="true">${I("chevron-right", 14)}</span>`;

function rowState(run: ToolRun): string {
  if (!run.result) return `<span class="agent-tool-state running">${t("运行中…")}</span>`;
  if (run.result.isError) return `<span class="agent-tool-state failed">${t("失败")}</span>`;
  return "";
}

function body(run: ToolRun): string {
  const output = run.result ? `<span class="agent-tool-caption">${t("结果")}</span><pre>${esc(clip(run.result.content))}</pre>` : "";
  return `<div class="agent-tool-body"><span class="agent-tool-caption">${t("参数")}</span><pre>${esc(clip(run.call.arguments))}</pre>${output}</div>`;
}

function row(run: ToolRun): string {
  const hint = argsHint(run.call.arguments);
  return `<li><details class="agent-tool-item" data-key="${esc(run.call.id)}"><summary><code>${esc(run.call.name)}</code><span class="agent-tool-hint">${esc(hint)}</span>${rowState(run)}${chevron}</summary>${body(run)}</details></li>`;
}

/** toolCardHTML renders one run of consecutive tool calls as a card. */
export function toolCardHTML(runs: ToolRun[], options: ToolCardOptions = {}): string {
  if (!runs.length) return "";
  const first = runs[0]!;
  const pending = runs.filter((r) => !r.result);
  const failed = runs.filter((r) => r.result?.isError).length;
  const running = Boolean(options.running) && pending.length > 0;
  const elapsed = options.startedAt
    ? `<span class="agent-tools-elapsed" data-started="${options.startedAt}">${formatElapsed(Date.now() - options.startedAt)}</span>`
    : "";
  const meta = [options.running && options.step ? t("第 {n} 步", { n: options.step }) : "", elapsed].filter(Boolean).join(" · ");
  const metaHTML = meta ? `<span class="agent-tools-meta">${meta}</span>` : "";

  let icon: string;
  let label: string;
  let sub = "";
  if (running) {
    const now = pending[pending.length - 1]!.call;
    icon = `<span class="agent-tools-icon running">${I("clock", 15)}</span>`;
    label = t("正在调用 {name}", { name: `<code>${esc(now.name)}</code>` });
    sub = runs.length > 1 ? t("已发起 {n} 次 · {breakdown}", { n: runs.length, breakdown: esc(breakdown(runs)) }) : esc(argsHint(now.arguments));
  } else if (runs.length === 1) {
    icon = failed ? `<span class="agent-tools-icon failed">${I("alert", 15)}</span>` : `<span class="agent-tools-icon done">${I("check-circle", 15)}</span>`;
    const name = `<code>${esc(first.call.name)}</code>`;
    label = failed ? t("调用了 {name}，失败", { name }) : t("调用了 {name}", { name });
    sub = esc(argsHint(first.call.arguments));
  } else {
    icon = failed ? `<span class="agent-tools-icon failed">${I("alert", 15)}</span>` : `<span class="agent-tools-icon done">${I("check-circle", 15)}</span>`;
    label = failed ? t("调用了 {n} 次工具，{failed} 次失败", { n: runs.length, failed }) : t("调用了 {n} 次工具", { n: runs.length });
    sub = esc(breakdown(runs));
  }

  const head = `<summary class="agent-tools-head">${icon}<span class="agent-tools-label">${label}</span><span class="agent-tools-sub">${sub}</span>${metaHTML}${chevron}</summary>`;
  // One finished call needs no list: its details sit right under the head.
  const content = runs.length === 1 && !running ? body(first) : `<ol class="agent-tools-list">${runs.map(row).join("")}</ol>`;
  return `<div class="agent-row block"><details class="agent-tools" data-key="${esc(first.call.id)}"${running ? " open" : ""}>${head}${content}</details></div>`;
}
