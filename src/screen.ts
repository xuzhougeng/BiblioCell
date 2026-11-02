// Screening mode: one paper at a time, so a person can read it and decide
// whether to keep it, then move on with the arrow keys. The list is a snapshot
// of the filtered papers at entry, so deciding never shuffles the order or
// drops the paper from under the reader.
import type { App } from "./app.ts";
import type { AgentConfig, Judgement, Paper } from "./mygo.ts";
import { READING, SCREEN, arr, dateText, decideScreening, esc, joinList, speciesDisplay, speciesNames } from "./model.ts";
import { $, I, badge, demoBadge, screenBadge } from "./ui.ts";
import { listSeparator, t, tv } from "./i18n.ts";

export interface ScreenState {
  /** Paper ids in reading order, fixed when the mode starts. */
  ids: string[];
  index: number;
  /** AI suggestions for this run: the requirement the model judges against, and its answers so far. */
  ai: ScreenAI | null;
}

export interface ScreenAI {
  requirement: string;
  results: Map<string, AIResult>;
}

export type AIResult =
  | { state: "loading" }
  | { state: "done"; judgement: Judgement }
  | { state: "error"; message: string };

export const AI_ASK_KEY = "bibliocell-screen-ai-ask";
export const AI_REQUIREMENT_KEY = "bibliocell-screen-ai-requirement";

/** aiConfigured says whether the settings name a model service the backend could call. */
export const aiConfigured = (ai: AgentConfig): boolean =>
  Boolean(ai.model || (ai.protocol || "anthropic") === "anthropic") && Boolean(ai.apiKey || ai.baseUrl);

const read = (key: string): string => {
  try { return localStorage.getItem(key) ?? ""; } catch { return ""; }
};
const write = (key: string, value: string): void => {
  try { localStorage.setItem(key, value); } catch { /* The choice then lasts for this session only. */ }
};

/** askAIDisabled is the person's standing choice not to be asked on entry. */
export const askAIDisabled = (): boolean => read(AI_ASK_KEY) === "off";
export const setAskAIDisabled = (off: boolean): void => write(AI_ASK_KEY, off ? "off" : "on");
export const lastRequirement = (): string => read(AI_REQUIREMENT_KEY);
export const rememberRequirement = (value: string): void => write(AI_REQUIREMENT_KEY, value);

/** screenAIModal asks for the requirement the model should judge papers against. */
export function screenAIModal(app: App, fromToolbar = false): void {
  const body = `<form id="screen-ai-form"><p class="muted">${t("写下你的筛选要求，AI 会结合每篇文献的全文（没有全文时用摘要）判断是否符合，给出建议供你采纳。没有全文也没有摘要的文献会自动跳过。")}</p>`
    + `<label class="form-field"><span>${t("筛选要求")} <b class="required">*</b></span><textarea class="textarea" name="requirement" rows="5" maxlength="4000" required placeholder="${t("例如：只保留对植物根组织做了单细胞或单核 RNA 测序、并公开了原始数据的研究")}">${esc(lastRequirement())}</textarea></label>`
    + (fromToolbar ? "" : `<label class="checkbox-choice"><input type="checkbox" name="noAsk"><span>${t("进入筛选时不再自动询问（可在筛选页的“AI 建议”按钮重新开启）")}</span></label>`)
    + `<p id="form-error" class="form-error" role="alert"></p></form>`;
  app.showModal(t("让 AI 一起筛选？"), body, `<button class="btn btn-secondary" data-action="screen-ai-skip">${t("不用，直接筛选")}</button><button class="btn btn-primary" type="submit" form="screen-ai-form">${I("sparkle", 16)} ${t("启用 AI 建议")}</button>`);
  // The modal's default lands on the first input, which here is the checkbox; the requirement is what to type.
  requestAnimationFrame(() => $<HTMLTextAreaElement>("#screen-ai-form textarea")?.focus());
}

/** saveScreenAI reads the modal and turns suggestions on for this run. */
export function saveScreenAI(app: App, form: HTMLFormElement): void {
  const data = new FormData(form);
  const requirement = String(data.get("requirement") ?? "").trim();
  if (!requirement) {
    const error = form.querySelector("#form-error");
    if (error) error.textContent = t("请填写筛选要求。");
    return;
  }
  if (data.get("noAsk")) setAskAIDisabled(true);
  rememberRequirement(requirement);
  app.closeModal();
  app.enableScreeningAI(requirement);
}

const displayList = (values: string[] | null | undefined): string => joinList(arr(values).map(tv), listSeparator);

/** Decision keys: the letter a person presses, the status it records and the button colour. */
const VERDICT_COLOR: Record<string, string> = { included: "green", excluded: "gray", uncertain: "amber" };
const BASIS: Record<string, string> = { fulltext: t("依据全文"), abstract: t("依据摘要") };

/** aiBox shows where the model's answer for this paper stands, with a way to adopt it. */
function aiBox(app: App, p: Paper): string {
  const ai = app.screen?.ai;
  if (!ai) return "";
  const result = ai.results.get(p.id);
  const head = `<span class="ai-box-label">${I("sparkle", 15)} ${t("AI 建议")}</span>`;
  if (!result || result.state === "loading") return `<div class="ai-box is-loading">${head}<span class="muted">${t("正在阅读这篇文献…")}</span></div>`;
  if (result.state === "error") return `<div class="ai-box is-error">${head}<span>${esc(result.message)}</span><button class="text-link" data-action="screen-ai-retry" data-id="${esc(p.id)}">${t("重试")}</button></div>`;
  const j = result.judgement;
  if (j.skipped) return `<div class="ai-box is-skipped">${head}<span class="muted">${t("没有全文也没有摘要，已跳过 AI 判断。")}</span></div>`;
  const adopt = p.screening === j.verdict
    ? `<span class="muted small">${t("与当前判断一致")}</span>`
    : `<button class="btn btn-secondary btn-compact" data-action="screen-ai-adopt" data-id="${esc(p.id)}">${kbd("A")} ${t("采纳为“{label}”", { label: SCREEN[j.verdict] ?? j.verdict })}</button>`;
  return `<div class="ai-box ${esc(j.verdict)}">${head}${badge(SCREEN[j.verdict] ?? j.verdict, VERDICT_COLOR[j.verdict] ?? "gray")}<span class="ai-box-basis muted">${esc(BASIS[j.basis] ?? j.basis)}</span><p class="ai-box-reason">${esc(j.reason)}</p><span class="section-actions">${adopt}</span></div>`;
}

/** Decision keys: the letter a person presses, the status it records and the button colour. */
export const DECISIONS: [key: string, status: string, color: string][] = [
  ["y", "included", "btn-primary"],
  ["n", "excluded", "btn-danger"],
  ["u", "uncertain", "btn-secondary"],
  ["p", "pending", "btn-secondary"],
];

/** screenPapers drops papers deleted since the snapshot and clamps the cursor. */
export function screenPapers(app: App): Paper[] {
  const screen = app.screen;
  if (!screen) return [];
  const papers = screen.ids.map((id) => app.paperById(id)).filter((p): p is Paper => Boolean(p));
  screen.ids = papers.map((p) => p.id);
  screen.index = Math.min(Math.max(screen.index, 0), Math.max(papers.length - 1, 0));
  return papers;
}

const detail = (label: string, value: string): string => `<div class="detail-item"><span>${label}</span><strong>${value}</strong></div>`;

const kbd = (key: string): string => `<kbd class="screen-key">${esc(key)}</kbd>`;

export function renderScreening(app: App): string {
  const papers = screenPapers(app);
  const screen = app.screen!;
  if (!papers.length) {
    return `<div class="empty-state"><div class="empty-icon">${I("check-circle", 28)}</div><h3>${t("没有可筛选的文献")}</h3><p>${t("当前筛选结果为空。回到列表调整筛选条件后再进入逐篇筛选。")}</p><button class="btn btn-secondary" data-action="exit-screening">${t("返回列表")}</button></div>`;
  }
  const p = papers[screen.index]!;
  const total = papers.length;
  const decided = papers.filter((x) => x.screening !== "pending").length;
  const counts = Object.keys(SCREEN).map((s) => `<span class="screen-count ${s}">${esc(SCREEN[s]!)} <strong>${papers.filter((x) => x.screening === s).length}</strong></span>`).join("");
  const ds = app.paperDatasets(p.id);
  const proposal = p.proposedScreening
    ? `<div class="notice-box compact proposal-box">${I("info", 16)}<span>${t("Agent 建议：")}<strong>${esc(SCREEN[p.proposedScreening] ?? p.proposedScreening)}</strong>${p.proposedReason ? `<br>${esc(p.proposedReason)}` : ""}</span><span class="section-actions"><button class="text-link" data-action="accept-proposal" data-id="${esc(p.id)}">${t("采纳")}</button><button class="text-link" data-action="dismiss-proposal" data-id="${esc(p.id)}">${t("忽略")}</button></span></div>`
    : "";
  const doi = p.doi
    ? `<a class="text-link" href="https://doi.org/${esc(encodeURI(p.doi))}" target="_blank" rel="noopener noreferrer">${esc(p.doi)}${I("arrow-up-right", 13)}</a>`
    : `<span class="muted">${t("尚未记录")}</span>`;
  const decisionButtons = DECISIONS.map(([key, status, color]) =>
    `<button class="btn ${color} screen-decision${p.screening === status ? " is-current" : ""}" data-action="screen-decide" data-id="${esc(p.id)}" data-status="${status}" aria-pressed="${p.screening === status}">${kbd(key.toUpperCase())} ${esc(SCREEN[status]!)}</button>`,
  ).join("");
  const reason = p.screeningReason ? `<p class="muted small screen-reason">${t("判断依据：{reason}", { reason: esc(p.screeningReason) })}</p>` : "";
  const aiToggle = screen.ai
    ? `<button class="btn btn-secondary ai-toggle is-on" data-action="screen-ai-off" title="${esc(screen.ai.requirement)}">${I("sparkle", 16)} ${t("AI 建议：开")}</button>`
    : app.canScreenWithAI() ? `<button class="btn btn-secondary ai-toggle" data-action="screen-ai-open">${I("sparkle", 16)} ${t("AI 建议")}</button>` : "";

  return `<section class="panel screen-panel" aria-label="${t("逐篇筛选")}">`
    + `<div class="screen-toolbar"><button class="btn btn-secondary" data-action="exit-screening">${I("x", 16)} ${t("退出筛选")} ${kbd("Esc")}</button><div class="screen-progress"><strong>${t("第 {n} / {total} 篇", { n: screen.index + 1, total })}</strong><span class="muted">${t("已判断 {done} 篇", { done: decided })}</span></div><div class="screen-nav">${aiToggle}<button class="btn btn-secondary" data-action="screen-prev" ${screen.index === 0 ? "disabled" : ""} aria-label="${t("上一篇")}">${I("chevron-left", 16)} ${t("上一篇")}</button><button class="btn btn-secondary" data-action="screen-next" ${screen.index >= total - 1 ? "disabled" : ""} aria-label="${t("下一篇")}">${t("下一篇")} ${I("chevron-right", 16)}</button></div></div>`
    + `<div class="screen-counts">${counts}</div>`
    + `<div class="screen-body">`
    + `<div class="screen-status-row">${screenBadge(p.screening)}${badge(READING[p.reading] ?? t("未读"))}${p.publicationStatus === "preprint" ? badge(t("预印本"), "amber") : ""}${demoBadge(p)}${p.starred ? badge(t("已收藏"), "amber") : ""}<span class="muted screen-id">${esc(p.id)}</span></div>`
    + `<h2 class="screen-title">${esc(p.title)}</h2>`
    + `<p class="paper-meta">${esc(p.authors || t("作者待补"))} · ${esc(p.journal || t("期刊待补"))} · ${esc(p.year || t("年份待补"))}</p>`
    + `<div class="doi-bar"><span class="doi-bar-label">DOI</span>${doi}</div>`
    + proposal
    + aiBox(app, p)
    + `<div class="detail-grid screen-grid">${detail(t("研究物种"), esc(speciesDisplay(p) || t("待补")) + (speciesNames(p).length ? `<small class="muted"> ${esc(joinList(speciesNames(p), listSeparator))}</small>` : ""))}${detail(t("组织 / 部位"), esc(displayList(p.organs) || t("待补")))}${detail(t("测序类型"), esc(displayList(p.modalities) || t("待补")))}${detail(t("测序平台"), esc(displayList(p.platforms) || t("待补")))}${detail(t("文献类型"), esc(displayList(p.articleTypes) || t("待判断")))}${detail(t("关联数据集"), ds.length ? esc(ds.map((d) => d.accession || d.title).join(listSeparator)) : `<span class="muted">${t("未关联")}</span>`)}${detail(t("发现来源"), esc(tv(p.source || "手动添加")))}${detail(t("加入时间"), dateText(p.addedAt))}</div>`
    + `<section class="detail-section"><h3>${t("摘要")}</h3><p class="abstract-text">${esc(p.abstract || t("摘要尚未补充。先保留这条文献，之后可以继续完善。"))}</p></section>`
    + (arr(p.tags).length ? `<section class="detail-section"><h3>${t("主题标签")}</h3><div class="tag-list">${arr(p.tags).map((tag) => `<span class="tag">${esc(tag)}</span>`).join("")}</div></section>` : "")
    + (p.notes ? `<section class="detail-section"><h3>${t("研究笔记")}</h3><p class="abstract-text">${esc(p.notes)}</p></section>` : "")
    + `</div>`
    + `<div class="screen-footer"><div class="screen-decisions">${decisionButtons}</div>${reason}<div class="screen-extra"><button class="btn ${p.starred ? "btn-primary" : "btn-secondary"}" data-action="toggle-star" data-id="${esc(p.id)}">${kbd("S")} ${I("star", 16)} ${p.starred ? t("已收藏") : t("收藏")}</button><button class="btn btn-secondary" data-action="open-paper" data-id="${esc(p.id)}">${kbd("Enter")} ${t("完整详情")}</button><span class="muted footer-hint">${t("←→ 前后切换 · Y 纳入 · N 排除 · U 证据不足 · 判断后自动跳到下一篇")}</span></div></div>`
    + `</section>`;
}

/** screenActions builds the handlers of the buttons in screening mode. */
export const screenActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "start-screening": () => app.startScreening(),
  "exit-screening": () => app.exitScreening(),
  "screen-prev": () => app.moveScreening(-1),
  "screen-next": () => app.moveScreening(1),
  "screen-decide": (el, id) => app.decideInScreening(id, el.dataset.status ?? ""),
  "screen-ai-open": () => screenAIModal(app, true),
  "screen-ai-skip": () => app.closeModal(),
  "screen-ai-off": () => app.disableScreeningAI(),
  "screen-ai-retry": (_el, id) => app.requestJudgement(id, true),
  "screen-ai-adopt": (_el, id) => app.adoptJudgement(id),
});

/** applyDecision records the status and reports whether it was applied (an exclusion may first need a reason). */
export function applyDecision(app: App, p: Paper, status: string, askReason: (p: Paper) => void, reason = p.screeningReason): boolean {
  if (!Object.hasOwn(SCREEN, status)) return false;
  if (status === "excluded" && !String(reason || "").trim()) {
    askReason(p);
    return false;
  }
  decideScreening(p, status, reason);
  app.logActivity(t("更新收录判断：{title}", { title: p.title }));
  app.persist();
  return true;
}
