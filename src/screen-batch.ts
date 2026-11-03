import type { App } from "./app.ts";
import type { JudgeTask, Paper } from "./mygo.ts";
import { Library } from "./mygo.ts";
import { configured, modelFor, modelName, models } from "./models.ts";
import { esc, filterPapers, SCREEN } from "./model.ts";
import { $, I } from "./ui.ts";
import { t, tv } from "./i18n.ts";
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
  /** What the model decides: screening against the requirement, a built-in question, or a custom one. */
  taskId: "screening" | "custom" | string;
  /** The custom task's question and the tag an adopted yes adds. */
  question: string;
  tag: string;
  /** Built-in questions, fetched once per draft. */
  presets: JudgeTask[];
}

export const TASK_KEY = "bibliocell-screen-ai-task";
const YES_NO: Record<string, string> = { included: t("是"), excluded: t("否"), uncertain: t("不确定") };
/** verdictLabel names a verdict for a run: a screening decision, or yes/no for a question. */
export const verdictLabel = (verdict: string, task: JudgeTask | null): string => (task ? YES_NO[verdict] : SCREEN[verdict]) ?? verdict;
/** targetText says where an adopted yes lands. */
export function targetText(task: JudgeTask): string {
  if (task.target.kind === "articleType") return t("采纳后文章类型加上“{value}”", { value: tv(ARTICLE_TYPES[task.target.value] ?? task.target.value) });
  return t("采纳后打上标签“{value}”", { value: task.target.value });
}
const ARTICLE_TYPES: Record<string, string> = { original: "原创研究", method: "方法研究", review: "综述", other: "相关资料" }; // i18n-ignore: database labels, translated with tv()

/** draftTask builds the task the draft describes; null means screening. */
export function draftTask(draft: BatchDraft): JudgeTask | null {
  if (draft.taskId === "screening" || draft.kind === "rerun") return null;
  if (draft.taskId === "custom") return { id: "custom", name: "", question: draft.question.trim(), basis: "abstract", target: { kind: "tag", value: draft.tag.trim() } };
  return draft.presets.find(p => p.id === draft.taskId) ?? null;
}

const decided = (p: Paper): boolean => p.screening !== "pending";

function candidates(app: App): Paper[] {
  const draft = app.batchDraft!;
  const current = new Set(draft.current);
  const query = draft.search.trim().toLowerCase();
  const inScope = (p: Paper): boolean => draft.scope === "all" || (draft.scope === "pending" ? p.screening === "pending" : draft.scope === "decided" ? decided(p) : current.has(p.id));
  return app.state.papers.filter(p => inScope(p) && (!query || `${p.title} ${p.authors} ${p.doi} ${p.year ?? ""}`.toLowerCase().includes(query)));
}

const batchTitle = (kind: BatchKind, task: JudgeTask | null = null): string => (kind === "rerun" ? t("重跑 AI 对照") : task ? t("批量 AI 判断") : t("批量 AI 筛选"));

const read = (key: string): string => { try { return localStorage.getItem(key) ?? ""; } catch { return ""; } };
const write = (key: string, value: string): void => { try { localStorage.setItem(key, value); } catch { /* The choice then lasts for this session only. */ } };

export function openBatch(app: App, kind: BatchKind = "batch", ids?: string[]): void {
  if (app.batch) { showBatchProgress(app); return; }
  // Each entry from the library takes a fresh snapshot of its candidates: the
  // current filtered results for a batch, every decided paper for a rerun, or
  // the papers ticked in the table. Returning from configuration uses
  // showCandidates directly, retaining manual choices.
  const current = ids?.length ? ids.filter(id => app.paperById(id)) : kind === "rerun" ? app.state.papers.filter(decided).map(p => p.id) : filterPapers(app.state.papers, app.filters).map(p => p.id);
  const previous = app.batchDraft;
  app.batchDraft = {
    current, selected: new Set(current), scope: kind === "rerun" ? "decided" : "current", search: "",
    requirement: previous?.requirement ?? lastRequirement(),
    modelId: previous?.modelId ?? modelFor(app.env.settings, "screen")?.id ?? "",
    concurrency: previous?.concurrency ?? (app.env.settings.batchScreenConcurrency || 3),
    kind,
    taskId: previous?.taskId ?? (read(TASK_KEY) || "screening"),
    question: previous?.question ?? "",
    tag: previous?.tag ?? "",
    presets: previous?.presets ?? [],
  };
  showCandidates(app);
  if (!app.batchDraft.presets.length && app.env.real) {
    const draft = app.batchDraft;
    Library.judgePresets().then(presets => { if (app.batchDraft === draft) draft.presets = presets; }, () => { /* Screening and custom questions still work. */ });
  }
}

function steps(second: boolean): string {
  return `<div class="batch-steps"><span class="${second ? "" : "active"}">1 · ${t("选择候选文献")}</span>${I("chevron-right", 16)}<span class="${second ? "active" : ""}">2 · ${t("设置 AI 任务")}</span></div>`;
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

/** taskFields are the inputs that depend on the chosen task: the requirement, a preset's question, or a custom question and tag. */
function taskFields(draft: BatchDraft): string {
  const task = draftTask(draft);
  if (draft.kind === "rerun" || draft.taskId === "screening") {
    return `<label class="form-field"><span>${t("筛选要求")}</span><textarea class="textarea" name="requirement" rows="4" maxlength="4000" required placeholder="${t("例如：只保留对植物根组织做了单细胞或单核 RNA 测序、并公开了原始数据的研究")}">${esc(draft.requirement)}</textarea></label>
    <p class="form-hint">${t("优先读取全文，没有全文时使用摘要；两者都没有则跳过。并发越高，对模型服务的请求越密集。")}</p>`;
  }
  if (draft.taskId === "custom") {
    return `<label class="form-field"><span>${t("判断问题")}</span><textarea class="textarea" name="question" rows="3" maxlength="2000" required placeholder="${t("例如：这篇文献是否使用了 10x Genomics 平台？")}">${esc(draft.question)}</textarea></label>
    <label class="form-field"><span>${t("采纳后打上标签")}</span><input class="input" name="tag" maxlength="60" required placeholder="${t("例如：10x")}" value="${esc(draft.tag)}"></label>
    <p class="form-hint">${t("只读题目、元数据和摘要，对几百篇文献也很省。模型只回答是、否或不确定；答“是”的文献由你决定是否采纳。")}</p>`;
  }
  if (!task) return `<p class="notice-box">${t("预置问题尚未加载，请稍后再试或改用自定义问题。")}</p>`;
  return `<div class="form-field"><span>${t("判断问题")}</span><p class="batch-question">${esc(task.question)}</p></div>
    <p class="form-hint">${esc(targetText(task))} · ${task.basis === "fulltext" ? t("优先读取全文，没有全文时使用摘要；两者都没有则跳过。并发越高，对模型服务的请求越密集。") : t("只读题目、元数据和摘要，对几百篇文献也很省。模型只回答是、否或不确定；答“是”的文献由你决定是否采纳。")}</p>`;
}

function showConfig(app: App): void {
  const draft = app.batchDraft!;
  if (!draft.selected.size) return;
  const profiles = models(app.env.settings).filter(configured);
  if (!profiles.some(p => p.id === draft.modelId)) draft.modelId = profiles[0]?.id ?? "";
  const ready = app.env.real && app.env.desktop && profiles.length > 0;
  const task = draftTask(draft);
  const taskOptions = draft.kind === "rerun" ? "" : `<label class="form-field"><span>${t("任务")}</span><select class="input" id="batch-task" name="taskId">
    <option value="screening" ${draft.taskId === "screening" ? "selected" : ""}>${t("收录筛选：是否符合筛选要求")}</option>
    ${draft.presets.map(p => `<option value="${esc(p.id)}" ${draft.taskId === p.id ? "selected" : ""}>${esc(tv(p.name))}</option>`).join("")}
    <option value="custom" ${draft.taskId === "custom" ? "selected" : ""}>${t("自定义问题…")}</option></select></label>`;
  const submit = draft.kind === "rerun" ? t("开始筛选 {n} 篇", { n: draft.selected.size }) : task ? t("开始判断 {n} 篇", { n: draft.selected.size }) : t("开始筛选 {n} 篇", { n: draft.selected.size });
  app.showModal(batchTitle(draft.kind, task), `${steps(true)}<form id="batch-screen-form">
    <p class="muted">${t("已选择 {n} 篇", { n: draft.selected.size })} · ${draft.kind === "rerun" ? t("只生成新一批建议，已有的人工判定保持不变。") : t("只生成建议，收录判断由你逐篇确认。")}</p>
    ${taskOptions}
    <div class="settings-grid"><label class="form-field"><span>${t("筛选模型")}</span><select class="input" name="modelId" required ${profiles.length ? "" : "disabled"}>${profiles.map(p => `<option value="${esc(p.id)}" ${draft.modelId === p.id ? "selected" : ""}>${esc(p.name)} · ${esc(modelName(p.config))}</option>`).join("")}</select></label>
    <label class="form-field"><span>${t("并发文献数")}</span><select class="input" name="concurrency">${[1, 2, 3, 4, 5, 6, 7, 8].map(n => `<option ${n === draft.concurrency ? "selected" : ""}>${n}</option>`).join("")}</select></label></div>
    ${!ready ? `<p class="notice-box">${!app.env.real || !app.env.desktop ? t("请在桌面端的真实数据库模式中运行 AI 筛选。") : t("请先在设置中添加可用的 AI 模型。")}</p>` : ""}
    <button class="text-link" type="button" data-action="batch-settings">${t("管理模型")}${I("arrow-right", 14)}</button>
    ${taskFields(draft)}<p id="batch-error" class="form-error" role="alert"></p></form>`,
    `<button class="btn btn-secondary" data-action="batch-back">${t("返回选择")}</button><button class="btn btn-primary" type="submit" form="batch-screen-form" ${ready ? "" : "disabled"}>${I("sparkle", 16)}${submit}</button>`, true);
}

function captureConfig(app: App): void {
  const form = $<HTMLFormElement>("#batch-screen-form");
  if (!form || !app.batchDraft) return;
  const data = new FormData(form);
  const text = (name: string, fallback: string): string => (data.has(name) ? String(data.get(name) ?? "") : fallback);
  const draft = app.batchDraft;
  Object.assign(draft, {
    modelId: String(data.get("modelId") ?? ""), concurrency: Number(data.get("concurrency")),
    taskId: text("taskId", draft.taskId), requirement: text("requirement", draft.requirement), question: text("question", draft.question), tag: text("tag", draft.tag),
  });
}

export function startBatch(app: App, _form: HTMLFormElement): void {
  if (app.batch?.running || !app.batchDraft) return;
  captureConfig(app);
  const draft = app.batchDraft;
  const ids = [...draft.selected].filter(id => app.paperById(id));
  const task = draftTask(draft);
  let error = "";
  if (!task && draft.taskId !== "screening" && draft.kind !== "rerun") error = t("预置问题尚未加载，请稍后再试或改用自定义问题。");
  else if (!task && !draft.requirement.trim()) error = t("请填写筛选要求。");
  else if (task && !task.question) error = t("请填写判断问题。");
  else if (task && task.target.kind === "tag" && !task.target.value) error = t("请填写采纳后要打的标签。");
  else if (!ids.length) error = t("没有可筛选的文献");
  else if (!app.env.real || !app.env.desktop || !configured(models(app.env.settings).find(p => p.id === draft.modelId))) error = t("请先在设置中添加可用的 AI 模型。");
  else if (!Number.isInteger(draft.concurrency) || draft.concurrency < 1 || draft.concurrency > 8) error = t("并发文献数应为 1 到 8。");
  if (error) { const el = $("#batch-error"); if (el) el.textContent = error; return; }
  if (task) write(TASK_KEY, draft.taskId); else { write(TASK_KEY, "screening"); rememberRequirement(draft.requirement.trim()); }
  const runId = `R-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
  app.batch = new ScreeningBatch(ids, task ? task.question : draft.requirement.trim(), draft.modelId, draft.concurrency, runId, draft.kind, task);
  showBatchProgress(app);
  runBatch(app);
}

function runBatch(app: App): void {
  const batch = app.batch!;
  const task = batch.task;
  const request = task ? (id: string) => Library.judgePaperInRun(id, task, batch.modelId, batch.runId) : (id: string) => Library.screenPaperInRun(id, batch.requirement, batch.modelId, batch.runId, batch.kind);
  void batch.run(request, id => updateBatch(app, id)).then(() => {
    app.toast(task ? t("批量判断已结束，答“是”的文献可一键采纳。") : t("批量筛选已结束，可查看结果并逐篇核对。"));
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
  return `<span class="batch-verdict ${esc(j.verdict)}">${t("建议：{label}", { label: verdictLabel(j.verdict, batch.task) })}</span><span class="muted small"> · ${j.basis === "fulltext" ? t("依据全文") : t("依据摘要")}${j.confidence == null ? "" : ` · ${esc(confidenceText(j.confidence))}`}${decision}</span><p>${esc(j.reason)}</p>`;
}

function showBatchProgress(app: App): void {
  const batch = app.batch!;
  const task = batch.task;
  app.showModal(batchTitle(batch.kind, task), `<div id="batch-progress"><div class="batch-visual" aria-hidden="true"><div class="batch-paper"><i></i><i></i><i></i><i></i><div class="batch-scan"></div></div><div class="batch-spark">${I("sparkle", 28)}</div><div class="batch-output">${I("check-circle", 30)}</div></div>
    <div class="batch-progress-heading"><h3 id="batch-status"></h3><strong id="batch-fraction"></strong></div><progress id="batch-meter" max="${batch.ids.length}" value="0" aria-label="${t("筛查进度")}"></progress>
    <p id="batch-counts" class="muted" role="status" aria-live="polite"></p><p class="form-hint">${task ? t("关闭此窗口后任务继续；退出应用会结束任务。结果只是建议，采纳后才会写入文献。") : t("关闭此窗口后任务继续；退出应用会结束任务。每条建议都会连同所用模型记录到“AI 对照”，采纳与否由你决定。")}</p>
    <details class="batch-requirement"><summary>${task ? t("本次判断设置") : t("本次筛选设置")}</summary><p>${esc(models(app.env.settings).find(p => p.id === batch.modelId)?.name ?? batch.modelId)} · ${t("并发文献数")}: ${batch.concurrency}${task ? ` · ${esc(targetText(task))}` : ""}</p><p>${esc(batch.requirement)}</p></details>
    <div class="batch-results">${batch.ids.map((id, i) => `<article class="batch-result" data-batch-row="${i}"><span class="batch-row-number">${i + 1}</span><div><strong>${esc(app.paperById(id)?.title ?? id)}</strong><div class="batch-row-result">${rowResult(app, batch, id)}</div></div></article>`).join("")}</div></div>`,
    `<button class="btn btn-secondary" data-action="close-modal">${t("收起")}</button><button class="btn btn-secondary" id="batch-stop" data-action="batch-stop">${t("停止后续任务")}</button><button class="btn btn-secondary" id="batch-retry" data-action="batch-retry">${t("重试失败及未开始项")}</button><button class="btn btn-secondary" id="batch-new" data-action="batch-new">${t("新一轮筛选")}</button>${task ? `<button class="btn btn-primary" id="batch-adopt" data-action="batch-adopt">${I("check-circle", 16)}${t("采纳答“是”的文献")}</button>` : `<button class="btn ${batch.kind === "rerun" ? "btn-secondary" : "btn-primary"}" id="batch-review" data-action="batch-review">${t("逐篇核对建议")}${I("arrow-right", 16)}</button>`}${batch.kind === "rerun" ? `<button class="btn btn-primary" id="batch-calibration" data-action="batch-calibration">${t("查看对照")}${I("arrow-right", 16)}</button>` : ""}`, true);
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
    const review = $<HTMLButtonElement>("#batch-review");
    if (review) review.disabled = batch.running || done === skipped;
    const adopt = $<HTMLButtonElement>("#batch-adopt");
    if (adopt) {
      const yes = adoptable(batch).length;
      adopt.disabled = batch.running || !yes;
      adopt.textContent = t("采纳答“是”的 {n} 篇", { n: yes });
    }
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
  if (label) label.textContent = batch.running ? t("AI 筛查中 {done}/{total}", { done: done + failed, total: batch.ids.length }) : batch.task ? t("批量判断结果") : t("批量筛选结果");
}

/** adoptable lists the recorded suggestions of a question run that answered yes. */
export const adoptable = (batch: ScreeningBatch): string[] => batch.ids.flatMap(id => { const r = batch.results.get(id); return r?.state === "done" && !r.judgement.skipped && r.judgement.verdict === "included" && r.judgement.id ? [r.judgement.id] : []; });

export function batchChange(app: App, target: HTMLElement): boolean {
  if (!app.batchDraft) return false;
  if (target instanceof HTMLSelectElement && target.id === "batch-task") { captureConfig(app); showConfig(app); return true; }
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
  "batch-adopt": () => {
    const batch = app.batch;
    if (!batch || batch.running || !batch.task) return;
    const ids = adoptable(batch);
    if (!ids.length) return;
    const button = $<HTMLButtonElement>("#batch-adopt");
    if (button) button.disabled = true;
    Library.adoptJudgements(ids).then(result => {
      app.toast(result.adopted ? t("已采纳 {n} 篇；{skipped} 篇原本已有标记或无需更改。", { n: result.adopted, skipped: result.skipped }) : t("没有需要更改的文献：标记原本就在。"));
      void app.reload();
    }, error => {
      app.toast(t("采纳失败：{reason}", { reason: error instanceof Error ? error.message : String(error) }), true);
      if (button) button.disabled = false;
    });
  },
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
