// AI calibration: how often the model's screening suggestions matched the
// person's decisions, per model and per run, with the papers they disagree
// on. Every suggestion is recorded on the Go side with the model profile that
// gave it; the decision compared against is always the one on record now.
import type { App } from "./app.ts";
import { Library, type CalibrationFilter, type CalibrationPair, type CalibrationReport, type ScreeningSuggestion } from "./mygo.ts";
import { SCREEN, dateText, esc } from "./model.ts";
import { openBatch } from "./screen-batch.ts";
import type { AIResult, ScreenState } from "./screen.ts";
import { $, I, badge, empty, errorMessage } from "./ui.ts";
import { t } from "./i18n.ts";

export interface CalibrationState {
  filter: CalibrationFilter;
  report: CalibrationReport | null;
  loading: boolean;
  error: string;
}

export const emptyCalibration = (): CalibrationState => ({ filter: { profileId: "", model: "", runId: "", from: "", to: "" }, report: null, loading: false, error: "" });

const VERDICT_COLOR: Record<string, string> = { included: "green", excluded: "gray", uncertain: "amber" };
const KIND: Record<string, string> = { screen: t("逐篇筛选"), batch: t("批量筛选"), rerun: t("重跑对照") };
export const percent = (rate: number): string => `${Math.round(rate * 100)}%`;
export const confidenceText = (confidence: number | null): string => (confidence == null ? "" : t("置信度 {n}", { n: percent(confidence) }));

/** openCalibration shows the comparison tab, optionally narrowed to one run. */
export function openCalibration(app: App, filter?: Partial<CalibrationFilter>): void {
  app.calibrationView = true;
  app.filters.review = "";
  app.filters.starred = false;
  if (filter) Object.assign(app.calibration.filter, filter);
  app.navigate("papers");
  void loadCalibration(app);
}

/** loadCalibration fetches the report for the current filter; a filter changed meanwhile wins. */
export async function loadCalibration(app: App): Promise<void> {
  const state = app.calibration;
  if (!app.env.real || !app.env.desktop) return;
  const filter = { ...state.filter };
  const current = () => JSON.stringify(filter) === JSON.stringify(state.filter);
  state.loading = true;
  state.error = "";
  updatePanel(app);
  try {
    const report = await Library.calibrationReport(filter);
    if (!current()) return;
    state.report = report;
  } catch (error) {
    if (!current()) return;
    state.error = errorMessage(error, t("读取对照数据失败，请重试。"));
  } finally {
    if (current()) state.loading = false;
  }
  updatePanel(app);
}

/** Only the panel is rebuilt, so the tabs and toolbar around it keep their state. */
function updatePanel(app: App): void {
  const panel = $("#calibration-panel");
  if (panel && app.calibrationView) panel.outerHTML = renderCalibration(app);
}

const verdictBadge = (status: string): string => badge(SCREEN[status] ?? status, VERDICT_COLOR[status] ?? "gray");
const brief = (text: string, limit = 90): string => (text.length > limit ? `${text.slice(0, limit)}…` : text);

function filterBar(app: App): string {
  const { filter, report } = app.calibration;
  const models = report?.models ?? [];
  const runs = report?.runs ?? [];
  const modelIndex = models.findIndex(m => m.profileId === filter.profileId && m.model === filter.model);
  const modelOptions = [`<option value="" ${modelIndex < 0 ? "selected" : ""}>${t("全部模型")}</option>`]
    .concat(models.map((m, i) => `<option value="${i}" ${modelIndex === i ? "selected" : ""}>${esc(m.profileName || m.profileId)} · ${esc(m.model)} (${m.compared})</option>`)).join("");
  const runOptions = [`<option value="" ${filter.runId ? "" : "selected"}>${t("全部建议（每篇文献取各模型最新一条）")}</option>`]
    .concat(runs.map(r => `<option value="${esc(r.runId)}" ${filter.runId === r.runId ? "selected" : ""}>${esc(dateText(r.startedAt))} · ${esc(KIND[r.kind] ?? r.kind)} · ${esc(r.profileName || r.profileId)} · ${t("{n} 篇", { n: r.total })}</option>`)).join("");
  const active = filter.profileId || filter.model || filter.runId || filter.from || filter.to;
  return `<div class="toolbar calibration-toolbar"><label class="form-field compact"><span>${t("模型配置")}</span><select class="input" data-calibration="model" aria-label="${t("模型配置")}">${modelOptions}</select></label>
    <label class="form-field compact"><span>${t("批次")}</span><select class="input" data-calibration="run" aria-label="${t("批次")}">${runOptions}</select></label>
    <label class="form-field compact"><span>${t("从")}</span><input class="input" type="date" data-calibration="from" value="${esc(filter.from)}" aria-label="${t("开始日期")}"></label>
    <label class="form-field compact"><span>${t("到")}</span><input class="input" type="date" data-calibration="to" value="${esc(filter.to)}" aria-label="${t("结束日期")}"></label>
    ${active ? `<button class="text-link" data-action="calibration-clear">${I("x", 13)} ${t("清除筛选")}</button>` : ""}
    <span class="section-actions"><button class="btn btn-secondary" data-action="calibration-refresh">${t("刷新")}</button><button class="btn btn-primary" data-action="calibration-rerun">${I("sparkle", 16)} ${t("重跑对照")}</button></span></div>`;
}

function matrixTable(report: CalibrationReport): string {
  const head = `<tr><th>${t("AI 建议 ↓ · 人工判定 →")}</th>${report.verdicts.map(v => `<th>${esc(SCREEN[v] ?? v)}</th>`).join("")}<th>${t("小计")}</th></tr>`;
  const rows = report.verdicts.map(verdict => {
    const cells = report.verdicts.map(decision => {
      const count = report.matrix.find(c => c.verdict === verdict && c.decision === decision)?.count ?? 0;
      return `<td class="${verdict === decision ? "agree" : count ? "disagree" : ""}">${count}</td>`;
    }).join("");
    const subtotal = report.matrix.filter(c => c.verdict === verdict).reduce((sum, c) => sum + c.count, 0);
    return `<tr><th>${esc(SCREEN[verdict] ?? verdict)}</th>${cells}<td class="muted">${subtotal}</td></tr>`;
  }).join("");
  return `<table class="calibration-matrix" aria-label="${t("混淆矩阵")}"><thead>${head}</thead><tbody>${rows}</tbody></table>`;
}

function modelTable(report: CalibrationReport): string {
  if (!report.byModel.length) return "";
  return `<table class="calibration-table"><thead><tr><th>${t("模型配置")}</th><th>${t("模型")}</th><th>${t("已对照")}</th><th>${t("一致")}</th><th>${t("一致率")}</th></tr></thead><tbody>${report.byModel.map(m =>
    `<tr><td>${esc(m.profileName || m.profileId || t("未记录"))}</td><td><code>${esc(m.model)}</code></td><td>${m.compared}</td><td>${m.agreed}</td><td><strong>${m.compared ? percent(m.rate) : "–"}</strong></td></tr>`).join("")}</tbody></table>`;
}

function runTable(app: App, report: CalibrationReport): string {
  if (!report.runs.length) return "";
  const current = app.calibration.filter.runId;
  return `<table class="calibration-table"><thead><tr><th>${t("时间")}</th><th>${t("类型")}</th><th>${t("模型配置")}</th><th>${t("筛选要求")}</th><th>${t("篇数")}</th><th>${t("已对照")}</th><th>${t("一致率")}</th><th></th></tr></thead><tbody>${report.runs.map(r =>
    `<tr class="${current === r.runId ? "is-current" : ""}"><td>${esc(dateText(r.startedAt))}</td><td>${esc(KIND[r.kind] ?? r.kind)}</td><td>${esc(r.profileName || r.profileId)} · <code>${esc(r.model)}</code></td><td title="${esc(r.requirement)}">${esc(brief(r.requirement, 60))}</td><td>${r.total}</td><td>${r.compared}</td><td><strong>${r.compared ? percent(r.rate) : "–"}</strong></td><td>${current === r.runId ? `<button class="text-link" data-action="calibration-run" data-run="">${t("取消只看")}</button>` : `<button class="text-link" data-action="calibration-run" data-run="${esc(r.runId)}">${t("只看此批")}</button>`}</td></tr>`).join("")}</tbody></table>`;
}

function mismatchList(report: CalibrationReport): string {
  if (!report.mismatches.length) {
    return report.compared ? `<p class="empty-state">${t("已对照的建议都与人工判定一致。")}</p>` : `<p class="empty-state">${t("还没有可对照的文献：先对有 AI 建议的文献做出收录判断，或用“重跑对照”为已判定的文献生成建议。")}</p>`;
  }
  return `<div class="calibration-mismatches">${report.mismatches.map(m => `<article class="calibration-mismatch"><button class="calibration-title" data-action="calibration-screen" data-id="${esc(m.suggestionId)}"><strong>${esc(m.title)}</strong>${I("arrow-right", 14)}</button>
    <div class="calibration-verdicts"><span>${t("AI 建议")} ${verdictBadge(m.verdict)}</span><span>${t("人工判定")} ${verdictBadge(m.decision)}</span><span class="muted small">${esc(m.profileName || m.profileId)} · <code>${esc(m.model)}</code>${m.confidence == null ? "" : ` · ${esc(confidenceText(m.confidence))}`} · ${esc(dateText(m.createdAt))}</span></div>
    ${m.reason ? `<p class="muted small">${t("AI 理由：{reason}", { reason: esc(brief(m.reason, 200)) })}</p>` : ""}${m.decisionReason ? `<p class="muted small">${t("人工依据：{reason}", { reason: esc(brief(m.decisionReason, 200)) })}</p>` : ""}</article>`).join("")}</div>`;
}

/** renderCalibration draws the comparison panel below the papers tabs. */
export function renderCalibration(app: App): string {
  if (!app.env.real || !app.env.desktop) {
    return `<section class="panel" id="calibration-panel">${empty(t("AI 对照需要真实数据库"), t("在桌面端的真实数据库模式中，每条 AI 筛选建议都会连同模型配置记录下来，与你的收录判断对照。"))}</section>`;
  }
  const { report, loading, error } = app.calibration;
  const body = !report
    ? `<p class="empty-state">${loading ? t("正在读取对照数据…") : esc(error || t("尚未读取对照数据。"))}</p>`
    : `<div class="stat-grid calibration-stats"><div class="stat-card"><div class="stat-label">${t("一致率")}</div><div class="stat-value">${report.compared ? percent(report.rate) : "–"}</div><div class="stat-meta">${t("{agreed} / {compared} 篇与人工判定一致", { agreed: report.agreed, compared: report.compared })}</div></div>
      <div class="stat-card"><div class="stat-label">${t("已对照")}</div><div class="stat-value">${report.compared}</div><div class="stat-meta">${t("有 AI 建议且已人工判定")}</div></div>
      <div class="stat-card"><div class="stat-label">${t("待人工判定")}</div><div class="stat-value">${report.pending}</div><div class="stat-meta">${t("有 AI 建议但尚未判定")}</div></div>
      <div class="stat-card"><div class="stat-label">${t("不一致")}</div><div class="stat-value">${report.mismatches.length}</div><div class="stat-meta">${t("点开可在筛选页复核")}</div></div></div>
      <div class="calibration-grid"><section class="detail-section"><h3>${t("混淆矩阵")}</h3>${matrixTable(report)}</section><section class="detail-section"><h3>${t("按模型配置")}</h3>${modelTable(report) || `<p class="muted small">${t("尚无记录")}</p>`}</section></div>
      ${report.runs.length ? `<section class="detail-section"><h3>${t("批次")} <span class="count-pill">${report.runs.length}</span></h3>${runTable(app, report)}</section>` : ""}
      <section class="detail-section"><h3>${t("不一致样本")} <span class="count-pill">${report.mismatches.length}</span></h3>${mismatchList(report)}</section>`;
  return `<section class="panel calibration-panel${loading ? " is-loading" : ""}" id="calibration-panel" aria-busy="${loading}"><p class="muted calibration-intro">${t("AI 只提议，人来拍板：这里把每条记录在案的 AI 建议和你最终的收录判断放在一起，看它判得准不准。换模型或改筛选要求后，用“重跑对照”对同一批已判定文献再生成一次建议，比较一致率的变化；人工判定不会被改动。")}</p>${filterBar(app)}${error && report ? `<p class="form-error" role="alert">${esc(error)}</p>` : ""}${body}</section>`;
}

/** calibrationChange applies a filter control; returns whether the event was one. */
export function calibrationChange(app: App, target: HTMLElement): boolean {
  const key = target.dataset.calibration;
  if (!key || !(target instanceof HTMLSelectElement || target instanceof HTMLInputElement)) return false;
  const filter = app.calibration.filter;
  if (key === "model") {
    const model = app.calibration.report?.models[Number(target.value)];
    filter.profileId = target.value === "" || !model ? "" : model.profileId;
    filter.model = target.value === "" || !model ? "" : model.model;
  } else if (key === "run") filter.runId = target.value;
  else if (key === "from") filter.from = target.value;
  else if (key === "to") filter.to = target.value;
  else return false;
  void loadCalibration(app);
  return true;
}

/** suggestionOf rebuilds the recorded suggestion a mismatch came from, for the screening page's AI box. */
export const suggestionOf = (pair: CalibrationPair): ScreeningSuggestion => ({
  id: pair.suggestionId, paperId: pair.paperId, requirement: pair.requirement, profileId: pair.profileId, profileName: pair.profileName,
  model: pair.model, protocol: "", verdict: pair.verdict, reason: pair.reason, basis: pair.basis, excerpt: pair.excerpt, page: pair.page,
  confidence: pair.confidence, runId: pair.runId, kind: pair.kind, inputTokens: 0, outputTokens: 0, skipped: false, createdAt: pair.createdAt,
});

/** screeningOfMismatches opens the mismatched papers one at a time, starting at the clicked one,
 * with each paper's recorded suggestion shown instead of asking the model again. */
export function screeningOfMismatches(pairs: CalibrationPair[], suggestionId: string): ScreenState | null {
  const clicked = pairs.find(p => p.suggestionId === suggestionId);
  if (!clicked) return null;
  const ids: string[] = [];
  const results = new Map<string, AIResult>();
  for (const pair of pairs) {
    if (results.has(pair.paperId)) continue;
    ids.push(pair.paperId);
    results.set(pair.paperId, { state: "done", judgement: suggestionOf(pair) });
  }
  return { ids, index: Math.max(ids.indexOf(clicked.paperId), 0), ai: { requirement: clicked.requirement, results }, returnTo: "calibration" };
}

export const calibrationActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "calibration-open": () => openCalibration(app),
  "calibration-refresh": () => { void loadCalibration(app); },
  "calibration-clear": () => { app.calibration.filter = emptyCalibration().filter; void loadCalibration(app); },
  "calibration-run": (el) => { app.calibration.filter.runId = el.dataset.run ?? ""; void loadCalibration(app); },
  "calibration-rerun": () => openBatch(app, "rerun"),
  "calibration-screen": (_el, id) => {
    const screen = screeningOfMismatches(app.calibration.report?.mismatches ?? [], id);
    if (!screen) return;
    app.calibrationView = false;
    app.screen = screen;
    app.closeDrawer();
    app.render();
    window.scrollTo({ top: 0 });
  },
});
