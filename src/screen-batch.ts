import type { App } from "./app.ts";
import type { Paper } from "./mygo.ts";
import { Library } from "./mygo.ts";
import { configured, modelFor, modelName, models } from "./models.ts";
import { esc, filterPapers, SCREEN } from "./model.ts";
import { $, I } from "./ui.ts";
import { t } from "./i18n.ts";
import { lastRequirement, rememberRequirement } from "./screen.ts";
import { ScreeningBatch } from "./screen-batch-queue.ts";
import { confidenceText, openCalibration } from "./calibration.ts";

export type BatchKind = "batch" | "rerun";

export interface BatchDraft {
  current: string[];
  selected: Set<string>;
  scope: string;
  search: string;
  requirement: string;
  modelId: string;
  concurrency: number;
  /** A rerun regenerates suggestions for decided papers, to compare with the decisions; it never changes them. */
  kind: BatchKind;
}

const decided = (p: Paper): boolean => p.screening !== "pending";

function candidates(app: App): Paper[] {
  const draft = app.batchDraft!;
  const current = new Set(draft.current);
  const query = draft.search.trim().toLowerCase();
  const inScope = (p: Paper): boolean => draft.scope === "all" || (draft.scope === "pending" ? p.screening === "pending" : draft.scope === "decided" ? decided(p) : current.has(p.id));
  return app.state.papers.filter(p => inScope(p) && (!query || `${p.title} ${p.authors} ${p.doi} ${p.year ?? ""}`.toLowerCase().includes(query)));
}

const batchTitle = (kind: BatchKind): string => (kind === "rerun" ? t("重跑 AI 对照") : t("批量 AI 筛选"));

export function openBatch(app: App, kind: BatchKind = "batch"): void {
  if (app.batch) { showBatchProgress(app); return; }
  // Each entry from the library takes a fresh snapshot of its candidates: the
  // current filtered results for a batch, every decided paper for a rerun.
  // Returning from configuration uses showCandidates directly, retaining manual choices.
  const current = kind === "rerun" ? app.state.papers.filter(decided).map(p => p.id) : filterPapers(app.state.papers, app.filters).map(p => p.id);
  const previous = app.batchDraft;
  app.batchDraft = {
    current, selected: new Set(current), scope: kind === "rerun" ? "decided" : "current", search: "",
    requirement: previous?.requirement ?? lastRequirement(),
    modelId: previous?.modelId ?? modelFor(app.env.settings, "screen")?.id ?? "",
    concurrency: previous?.concurrency ?? (app.env.settings.batchScreenConcurrency || 3),
    kind,
  };
  showCandidates(app);
}

function steps(second: boolean): string {
  return `<div class="batch-steps"><span class="${second ? "" : "active"}">1 · ${t("选择候选文献")}</span>${I("chevron-right", 16)}<span class="${second ? "active" : ""}">2 · ${t("设置 AI 筛选")}</span></div>`;
}

function showCandidates(app: App): void {
  const draft = app.batchDraft!;
  // Papers can have been deleted while this draft was closed.
  draft.selected = new Set([...draft.selected].filter(id => app.paperById(id)));
  const intro = draft.kind === "rerun"
    ? t("勾选已做出收录判断的文献，用指定模型重新生成 AI 建议。人工判定不会被改动；结果作为新一批建议进入“AI 对照”，用来比较换模型或改要求后的一致率。")
    : t("从当前筛选结果开始，勾选需要 AI 阅读的文献。可以切换范围或搜索后继续选择。");
  app.showModal(batchTitle(draft.kind), `${steps(false)}<p class="muted">${intro}</p>
    <div class="batch-controls"><select class="input" id="batch-scope" aria-label="${t("候选范围")}">
    ${[["current", t("当前列表筛选结果")], ["pending", t("所有待筛选文献")], ["decided", t("已人工判定的文献")], ["all", t("全部文献")]].map(([value, label]) => `<option value="${value}" ${draft.scope === value ? "selected" : ""}>${label}</option>`).join("")}</select>
    <input class="input" id="batch-search" placeholder="${t("搜索题目、作者或 DOI")}" aria-label="${t("搜索候选文献")}" value="${esc(draft.search)}"></div>
    <div class="batch-selection"><span id="batch-selected" role="status"></span><span class="section-actions"><button class="text-link" data-action="batch-select-visible">${t("全选搜索结果")}</button><button class="text-link" data-action="batch-clear">${t("清空选择")}</button></span></div><div id="batch-candidates" class="batch-candidates"></div>`,
    `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-primary" id="batch-next" data-action="batch-config">${t("下一步：设置模型")}${I("arrow-right", 16)}</button>`, true);
  updateCandidates(app);
}

function updateSelected(app: App): void {
  const count = app.batchDraft!.selected.size;
  const label = $("#batch-selected");
  if (label) label.textContent = t("已选择 {n} 篇", { n: count });
  const next = $<HTMLButtonElement>("#batch-next");
  if (next) next.disabled = !count;
}

function updateCandidates(app: App): void {
  const list = $("#batch-candidates");
  if (!list) return;
  list.innerHTML = candidates(app).map(p => `<label class="batch-candidate"><input type="checkbox" data-batch-paper="${esc(p.id)}" ${app.batchDraft!.selected.has(p.id) ? "checked" : ""}><span><strong>${esc(p.title)}</strong><small class="muted">${esc([p.year, p.journal, SCREEN[p.screening]].filter(Boolean).join(" · "))}</small></span></label>`).join("") || `<p class="empty-state">${t("没有可筛选的文献")}</p>`;
  updateSelected(app);
}

function showConfig(app: App): void {
  const draft = app.batchDraft!;
  if (!draft.selected.size) return;
  const profiles = models(app.env.settings).filter(configured);
  if (!profiles.some(p => p.id === draft.modelId)) draft.modelId = profiles[0]?.id ?? "";
  const ready = app.env.real && app.env.desktop && profiles.length > 0;
  app.showModal(batchTitle(draft.kind), `${steps(true)}<form id="batch-screen-form">
    <p class="muted">${t("已选择 {n} 篇", { n: draft.selected.size })} · ${draft.kind === "rerun" ? t("只生成新一批建议，已有的人工判定保持不变。") : t("只生成建议，收录判断由你逐篇确认。")}</p>
    <div class="settings-grid"><label class="form-field"><span>${t("筛选模型")}</span><select class="input" name="modelId" required ${profiles.length ? "" : "disabled"}>${profiles.map(p => `<option value="${esc(p.id)}" ${draft.modelId === p.id ? "selected" : ""}>${esc(p.name)} · ${esc(modelName(p.config))}</option>`).join("")}</select></label>
    <label class="form-field"><span>${t("并发文献数")}</span><select class="input" name="concurrency">${[1, 2, 3, 4, 5, 6, 7, 8].map(n => `<option ${n === draft.concurrency ? "selected" : ""}>${n}</option>`).join("")}</select></label></div>
    ${!ready ? `<p class="notice-box">${!app.env.real || !app.env.desktop ? t("请在桌面端的真实数据库模式中运行 AI 筛选。") : t("请先在设置中添加可用的 AI 模型。")}</p>` : ""}
    <button class="text-link" type="button" data-action="batch-settings">${t("管理模型")}${I("arrow-right", 14)}</button>
    <label class="form-field"><span>${t("筛选要求")}</span><textarea class="textarea" name="requirement" rows="4" maxlength="4000" required placeholder="${t("例如：只保留对植物根组织做了单细胞或单核 RNA 测序、并公开了原始数据的研究")}">${esc(draft.requirement)}</textarea></label>
    <p class="form-hint">${t("优先读取全文，没有全文时使用摘要；两者都没有则跳过。并发越高，对模型服务的请求越密集。")}</p><p id="batch-error" class="form-error" role="alert"></p></form>`,
    `<button class="btn btn-secondary" data-action="batch-back">${t("返回选择")}</button><button class="btn btn-primary" type="submit" form="batch-screen-form" ${ready ? "" : "disabled"}>${I("sparkle", 16)}${t("开始筛选 {n} 篇", { n: draft.selected.size })}</button>`, true);
}

function captureConfig(app: App): void {
  const form = $<HTMLFormElement>("#batch-screen-form");
  if (!form || !app.batchDraft) return;
  const data = new FormData(form);
  Object.assign(app.batchDraft, { modelId: String(data.get("modelId") ?? ""), concurrency: Number(data.get("concurrency")), requirement: String(data.get("requirement") ?? "") });
}

export function startBatch(app: App, _form: HTMLFormElement): void {
  if (app.batch?.running || !app.batchDraft) return;
  captureConfig(app);
  const draft = app.batchDraft;
  const ids = [...draft.selected].filter(id => app.paperById(id));
  let error = "";
  if (!draft.requirement.trim()) error = t("请填写筛选要求。");
  else if (!ids.length) error = t("没有可筛选的文献");
  else if (!app.env.real || !app.env.desktop || !configured(models(app.env.settings).find(p => p.id === draft.modelId))) error = t("请先在设置中添加可用的 AI 模型。");
  else if (!Number.isInteger(draft.concurrency) || draft.concurrency < 1 || draft.concurrency > 8) error = t("并发文献数应为 1 到 8。");
  if (error) { const el = $("#batch-error"); if (el) el.textContent = error; return; }
  rememberRequirement(draft.requirement.trim());
  const runId = `R-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
  app.batch = new ScreeningBatch(ids, draft.requirement.trim(), draft.modelId, draft.concurrency, runId, draft.kind);
  showBatchProgress(app);
  runBatch(app);
}

function runBatch(app: App): void {
  const batch = app.batch!;
  void batch.run(id => Library.screenPaperInRun(id, batch.requirement, batch.modelId, batch.runId, batch.kind), id => updateBatch(app, id)).then(() => {
    app.toast(t("批量筛选已结束，可查看结果并逐篇核对。"));
  });
}

function rowResult(app: App, batch: ScreeningBatch, id: string): string {
  const result = batch.results.get(id);
  if (!result) return `<span class="muted">${t("等待筛查")}</span>`;
  if (result.state === "loading") return `<span class="batch-reading"><span class="ai-spinner"></span>${t("正在阅读与判断…")}</span>`;
  if (result.state === "error") return `<span class="form-error">${t("筛查失败")} · ${esc(result.message)}</span>`;
  if (result.judgement.skipped) return `<span class="muted">${t("没有全文也没有摘要，已跳过 AI 判断。")}</span>`;
  const j = result.judgement;
  const paper = batch.kind === "rerun" ? app.paperById(id) : null;
  const decision = paper && paper.screening !== "pending" ? ` · <span class="batch-decision ${paper.screening === j.verdict ? "agree" : "disagree"}">${paper.screening === j.verdict ? t("与人工判定一致") : t("人工判定：{label}", { label: SCREEN[paper.screening] ?? paper.screening })}</span>` : "";
  return `<span class="batch-verdict ${esc(j.verdict)}">${t("建议：{label}", { label: SCREEN[j.verdict] ?? j.verdict })}</span><span class="muted small"> · ${j.basis === "fulltext" ? t("依据全文") : t("依据摘要")}${j.confidence == null ? "" : ` · ${esc(confidenceText(j.confidence))}`}${decision}</span><p>${esc(j.reason)}</p>`;
}

function showBatchProgress(app: App): void {
  const batch = app.batch!;
  app.showModal(batchTitle(batch.kind), `<div id="batch-progress"><div class="batch-visual" aria-hidden="true"><div class="batch-paper"><i></i><i></i><i></i><i></i><div class="batch-scan"></div></div><div class="batch-spark">${I("sparkle", 28)}</div><div class="batch-output">${I("check-circle", 30)}</div></div>
    <div class="batch-progress-heading"><h3 id="batch-status"></h3><strong id="batch-fraction"></strong></div><progress id="batch-meter" max="${batch.ids.length}" value="0" aria-label="${t("筛查进度")}"></progress>
    <p id="batch-counts" class="muted" role="status" aria-live="polite"></p><p class="form-hint">${t("关闭此窗口后任务继续；退出应用会结束任务。每条建议都会连同所用模型记录到“AI 对照”，采纳与否由你决定。")}</p>
    <details class="batch-requirement"><summary>${t("本次筛选设置")}</summary><p>${esc(models(app.env.settings).find(p => p.id === batch.modelId)?.name ?? batch.modelId)} · ${t("并发文献数")}: ${batch.concurrency}</p><p>${esc(batch.requirement)}</p></details>
    <div class="batch-results">${batch.ids.map((id, i) => `<article class="batch-result" data-batch-row="${i}"><span class="batch-row-number">${i + 1}</span><div><strong>${esc(app.paperById(id)?.title ?? id)}</strong><div class="batch-row-result">${rowResult(app, batch, id)}</div></div></article>`).join("")}</div></div>`,
    `<button class="btn btn-secondary" data-action="close-modal">${t("收起")}</button><button class="btn btn-secondary" id="batch-stop" data-action="batch-stop">${t("停止后续任务")}</button><button class="btn btn-secondary" id="batch-retry" data-action="batch-retry">${t("重试失败及未开始项")}</button><button class="btn btn-secondary" id="batch-new" data-action="batch-new">${t("新一轮筛选")}</button><button class="btn ${batch.kind === "rerun" ? "btn-secondary" : "btn-primary"}" id="batch-review" data-action="batch-review">${t("逐篇核对建议")}${I("arrow-right", 16)}</button>${batch.kind === "rerun" ? `<button class="btn btn-primary" id="batch-calibration" data-action="batch-calibration">${t("查看对照")}${I("arrow-right", 16)}</button>` : ""}`, true);
  updateBatch(app);
}

function updateBatch(app: App, id?: string): void {
  const batch = app.batch;
  if (!batch) return;
  const results = [...batch.results.values()];
  const done = results.filter(r => r.state === "done").length;
  const failed = results.filter(r => r.state === "error").length;
  const active = results.filter(r => r.state === "loading").length;
  const skipped = results.filter(r => r.state === "done" && r.judgement.skipped).length;
  const progress = $("#batch-progress");
  // Only patch the changed row and counters, preserving animations, focus and scroll.
  if (progress) {
    progress.classList.toggle("is-running", batch.running);
    $("#batch-status")!.textContent = batch.running ? (batch.stopping ? t("正在等待运行中的文献完成…") : t("AI 正在筛查文献")) : (done + failed === batch.ids.length ? t("本轮筛查完成") : t("筛查已停止"));
    $("#batch-fraction")!.textContent = `${done + failed} / ${batch.ids.length}`;
    $<HTMLProgressElement>("#batch-meter")!.value = done + failed;
    $("#batch-counts")!.textContent = t("完成 {done} · 阅读中 {active} · 跳过 {skipped} · 失败 {failed} · 等待 {waiting}", { done: done - skipped, active, skipped, failed, waiting: batch.ids.length - done - failed - active });
    const stop = $<HTMLButtonElement>("#batch-stop")!;
    stop.hidden = !batch.running;
    stop.disabled = batch.stopping;
    $("#batch-retry")!.hidden = batch.running || done === batch.ids.length;
    $("#batch-new")!.hidden = batch.running;
    $<HTMLButtonElement>("#batch-review")!.disabled = batch.running || done === skipped;
    const calibration = $<HTMLButtonElement>("#batch-calibration");
    if (calibration) calibration.disabled = batch.running;
    if (id) {
      const row = $(`[data-batch-row="${batch.ids.indexOf(id)}"]`, progress);
      row?.classList.toggle("is-reading", batch.results.get(id)?.state === "loading");
      const region = row?.querySelector(".batch-row-result");
      if (region) region.innerHTML = rowResult(app, batch, id);
    }
  }
  const label = $("#batch-entry-label");
  if (label) label.textContent = batch.running ? t("AI 筛查中 {done}/{total}", { done: done + failed, total: batch.ids.length }) : t("批量筛选结果");
}

export function batchChange(app: App, target: HTMLElement): boolean {
  if (!app.batchDraft) return false;
  if (target instanceof HTMLInputElement && target.dataset.batchPaper) {
    if (target.checked) app.batchDraft.selected.add(target.dataset.batchPaper); else app.batchDraft.selected.delete(target.dataset.batchPaper);
    updateSelected(app); return true;
  }
  if (target instanceof HTMLSelectElement && target.id === "batch-scope") { app.batchDraft.scope = target.value; updateCandidates(app); return true; }
  return false;
}

export function batchSearch(app: App, value: string): void {
  if (!app.batchDraft) return;
  app.batchDraft.search = value; updateCandidates(app);
}

export const batchActions = (app: App): Record<string, () => void> => ({
  "batch-open": () => openBatch(app),
  "batch-config": () => showConfig(app),
  "batch-back": () => { captureConfig(app); showCandidates(app); },
  "batch-settings": () => { captureConfig(app); app.closeModal(); app.navigate("settings"); },
  "batch-select-visible": () => { for (const p of candidates(app)) app.batchDraft!.selected.add(p.id); updateCandidates(app); },
  "batch-clear": () => { app.batchDraft!.selected.clear(); updateCandidates(app); },
  "batch-stop": () => { app.batch?.stop(); updateBatch(app); },
  "batch-retry": () => { if (app.batch && !app.batch.running) runBatch(app); },
  "batch-new": () => { if (!app.batch?.running) { const kind = app.batch?.kind ?? "batch"; app.batch = null; app.batchDraft = null; openBatch(app, kind); } },
  "batch-calibration": () => {
    const batch = app.batch;
    if (!batch || batch.running) return;
    app.closeModal();
    openCalibration(app, { runId: batch.runId, profileId: "", model: "" });
  },
  "batch-review": () => {
    const batch = app.batch;
    if (!batch || batch.running) return;
    const ids = batch.ids.filter(id => { const r = batch.results.get(id); return app.paperById(id) && r?.state === "done" && !r.judgement.skipped; });
    if (!ids.length) return;
    app.closeModal();
    app.navigate("papers");
    app.screen = { ids, index: 0, ai: { requirement: batch.requirement, results: new Map(batch.results) } };
    app.render();
  },
});
