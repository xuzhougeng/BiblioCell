import { metadataButton } from "./metadata.ts";
import { canWrite, hasBackend } from "./env.ts";
// Screening mode: one paper at a time, so a person can read it and decide
// whether to keep it, then move on with the arrow keys. The list is a snapshot
// of the filtered papers at entry, so deciding never shuffles the order or
// drops the paper from under the reader.
import type { App } from "./app.ts";
import { editableDetail } from "./paper-fields.ts";
import { Library, type Evidence, type Paper, type ScreeningSuggestion } from "./mygo.ts";
import { FIELD_LABELS } from "./drawer.ts";
import { confidenceText } from "./calibration.ts";
import { configured, modelFor } from "./models.ts";
import { READING, SCREEN, arr, dateText, decideScreening, esc, evidenceValueText, joinList, speciesDisplay, speciesNames } from "./model.ts";
import { $, I, badge, demoBadge, errorMessage, screenBadge } from "./ui.ts";
import { listSeparator, t, tv } from "./i18n.ts";

export interface ScreenState {
  /** Paper ids in reading order, fixed when the mode starts. */
  ids: string[];
  index: number;
  /** AI suggestions for this run: the requirement the model judges against, and its answers so far. */
  ai: ScreenAI | null;
  /** Where leaving screening goes back to, when not the papers table. */
  returnTo?: "calibration";
}

export interface ScreenAI {
  requirement: string;
  results: Map<string, AIResult>;
}

export type AIResult =
  | { state: "loading" }
  | { state: "done"; judgement: ScreeningSuggestion }
  | { state: "error"; message: string };

export const AI_ASK_KEY = "bibliocell-screen-ai-ask";
export const AI_REQUIREMENT_KEY = "bibliocell-screen-ai-requirement";

/** Translation of the paper on screen: what was selected and what came back. */
export type Translation =
  | { paperId: string; source: string; state: "loading" }
  | { paperId: string; source: string; state: "done"; text: string }
  | { paperId: string; source: string; state: "error"; message: string };

export type AbstractTranslation = (Translation | { paperId: string; source: string; state: "checking" | "uncached" }) & { prompt: string; showOriginal: boolean };

export function currentAbstractTranslation(app: App, p: Paper): AbstractTranslation | undefined {
  const result = app.abstractTranslations.get(p.id);
  return result?.source === p.abstract.trim() && result.prompt === (app.env.settings.translationPrompt ?? "") ? result : undefined;
}

function abstractSection(app: App, p: Paper): string {
  const result = currentAbstractTranslation(app, p);
  let controls = "";
  if (result?.state === "done") controls = `<button class="btn btn-secondary" data-action="screen-abstract-toggle" data-id="${esc(p.id)}">${result.showOriginal ? t("显示译文") : t("显示原文")}</button>`;
  if (result?.state === "loading") controls = `<span class="muted" role="status">${t("正在翻译…")}</span>`;
  if (result?.state === "error") controls = `<span class="form-error" role="alert">${esc(result.message)}</span><button class="text-link" data-action="screen-translate" data-id="${esc(p.id)}">${t("重试")}</button>`;
  const text = result?.state === "done" && !result.showOriginal ? result.text : p.abstract;
  return `<section class="detail-section" id="screen-abstract" data-paper-id="${esc(p.id)}"><div class="screen-abstract-heading"><h3>${t("摘要")}</h3><div class="section-actions">${controls}</div></div><p class="abstract-text">${esc(text || t("摘要尚未补充。先保留这条文献，之后可以继续完善。"))}</p></section>`;
}

export function updateScreenAbstract(app: App, id: string): void {
  const region = $("#screen-abstract");
  const paper = app.paperById(id);
  if (region?.dataset.paperId === id && paper) region.outerHTML = abstractSection(app, paper);
}

/** translationPop is the floating card that follows a selection: the original in brief, then the Chinese. */
export function translationPop(tr: Translation): string {
  const brief = tr.source.length > 80 ? `${tr.source.slice(0, 80)}…` : tr.source;
  const head = `<div class="translate-pop-head"><span class="ai-box-label">${I("sparkle", 14)} ${t("译文")}</span><button class="btn-icon" data-action="screen-translate-clear" aria-label="${t("收起译文")}">${I("x", 16)}</button></div><p class="translate-pop-source" title="${esc(tr.source)}">${esc(brief)}</p>`;
  if (tr.state === "loading") return `${head}<p class="muted translate-pop-wait"><span class="ai-spinner"></span>${t("正在翻译…")}</p>`;
  if (tr.state === "error") return `${head}<p class="translate-pop-error">${esc(tr.message)}</p><button class="text-link" data-action="screen-translate-selection">${t("重试")}</button>`;
  return `${head}<p class="translate-pop-text">${esc(tr.text)}</p>`;
}

/** Keep the reader in its own document so PDF search is scoped to the PDF. */
const paperFileURL = (id: string): string => `/pdf-viewer.html?file=${encodeURIComponent(id)}`;

/** pdfPane shows the stored PDF, or an upload entry when the paper has none. */
function pdfPane(app: App, p: Paper): string {
  if (!hasBackend(app.env)) return "";
  const file = app.paperResources(p.id).find((r) => r.type === "fulltext" && r.localPath && /\.pdf$/i.test(r.localPath));
  if (!file) {
    // Only the desktop can pick a file; on the web an admin fetches open-access full text from the drawer.
    const upload = app.env.desktop
      ? `<p>${t("上传这篇文献的 PDF，即可在右侧阅读全文。")}</p><button class="btn btn-primary" data-action="import-file" data-id="${esc(p.id)}" data-type="fulltext" data-screen-pdf="true">${I("upload", 16)} ${t("上传 PDF")}</button><p class="small muted">${t("文件会复制到文献库，并关联到当前文献。")}</p>`
      : `<p>${t("这篇文献还没有保存全文。")}</p><p class="small muted">${t("请在桌面应用中导入 PDF，或让管理员通过“获取开放全文”保存。")}</p>`;
    return `<aside class="screen-pdf screen-pdf-empty" aria-label="${t("全文")}"><div class="screen-pdf-head"><span>${I("file", 15)} ${t("全文")}</span></div><div class="screen-upload"><div class="screen-upload-card"><div class="empty-icon">${I("upload", 32)}</div><h3>${t("暂无 PDF 全文")}</h3>${upload}</div></div></aside>`;
  }
  if (!app.screenPdf) return "";
  return `<aside class="screen-pdf"><div class="screen-pdf-head"><span>${I("file", 15)} ${esc(file.title)}</span><span class="section-actions"><button class="text-link" data-action="open-file" data-id="${esc(file.id)}">${app.env.desktop ? t("在系统中打开") : t("新标签页打开")}</button><button class="text-link" data-action="screen-pdf-toggle">${t("隐藏")}</button></span></div><iframe class="screen-pdf-frame" src="${esc(paperFileURL(file.id))}" title="${esc(file.title)}"></iframe></aside>`;
}

/** hasPdf says whether the paper has a stored PDF, for the toolbar toggle. */
const hasPdf = (app: App, p: Paper): boolean => hasBackend(app.env) && app.paperResources(p.id).some((r) => r.type === "fulltext" && r.localPath && /\.pdf$/i.test(r.localPath));

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

/** aiBox shows where the model's answer for this paper stands, beside the decision
 * buttons: verdict, confidence and the adopt button in one row, the full reason on
 * its own line below, and the quoted passage and the model behind a disclosure.
 * Without a run it offers to start one. */
function aiBox(app: App, p: Paper): string {
  const ai = app.screen?.ai;
  if (!ai) return app.canScreenWithAI() ? `<div class="ai-box is-off"><button class="text-link" data-action="screen-ai-open">${I("sparkle", 15)} ${t("获取 AI 建议")}</button></div>` : "";
  const result = ai.results.get(p.id);
  const head = `<span class="ai-box-label">${I("sparkle", 15)} ${t("AI 建议")}</span>`;
  if (!result || result.state === "loading") return `<div class="ai-box is-loading">${head}<span class="muted">${t("正在阅读这篇文献…")}</span></div>`;
  if (result.state === "error") return `<div class="ai-box is-error">${head}<span>${esc(result.message)}</span><button class="text-link" data-action="screen-ai-retry" data-id="${esc(p.id)}">${t("重试")}</button></div>`;
  const j = result.judgement;
  if (j.skipped) return `<div class="ai-box is-skipped">${head}<span class="muted">${t("没有全文也没有摘要，已跳过 AI 判断。")}</span></div>`;
  const adopt = p.screening === j.verdict
    ? `<span class="muted small">${t("与当前判断一致")}</span>`
    : `<button class="btn btn-secondary btn-compact" data-action="screen-ai-adopt" data-id="${esc(p.id)}">${kbd("A")} ${t("采纳为“{label}”", { label: SCREEN[j.verdict] ?? j.verdict })}</button>`;
  const model = j.profileName || j.model ? `<span class="ai-box-model muted small" title="${esc(j.requirement)}">${esc(j.profileName || j.profileId)}${j.model ? ` · ${esc(j.model)}` : ""}${j.createdAt ? ` · ${esc(dateText(j.createdAt))}` : ""}</span>` : "";
  const confidence = j.confidence == null ? "" : `<span class="ai-box-basis muted">${esc(confidenceText(j.confidence))}</span>`;
  const excerpt = j.excerpt ? `<blockquote class="ai-box-excerpt">${esc(j.excerpt)}${pageJump(app, p, j.page, j.excerpt)}</blockquote>` : "";
  const more = excerpt || model ? `<details class="ai-box-more"><summary>${t("依据与模型")}</summary>${excerpt}${model}</details>` : "";
  return `<div class="ai-box ${esc(j.verdict)}">${head}${badge(SCREEN[j.verdict] ?? j.verdict, VERDICT_COLOR[j.verdict] ?? "gray")}<span class="ai-box-basis muted">${esc(BASIS[j.basis] ?? j.basis)}</span>${confidence}<span class="section-actions">${adopt}</span><p class="ai-box-reason">${esc(j.reason)}</p>${more}</div>`;
}

/** Status dot colours of the queue rail, by screening status. */
const QUEUE_DOT: Record<string, string> = { included: "green", excluded: "red", uncertain: "amber", pending: "" };

/** queueRail lists the papers of this run with their decisions, so the reader can see
 * how far along they are and jump anywhere. Narrow windows show it as an overlay. */
function queueRail(app: App, papers: Paper[], index: number): string {
  const items = papers.map((x, i) =>
    `<button class="screen-queue-item${i === index ? " is-current" : ""}" data-action="screen-go" data-index="${i}"${i === index ? ' aria-current="true"' : ""} title="${esc(x.title)}"><span class="screen-queue-dot ${QUEUE_DOT[x.screening] ?? ""}" aria-label="${esc(SCREEN[x.screening] ?? x.screening)}"></span><span class="screen-queue-index">${i + 1}</span><span class="screen-queue-title">${esc(x.title)}</span></button>`,
  ).join("");
  return `<aside class="screen-queue${app.screenQueueOpen ? " is-open" : ""}" aria-label="${t("筛选队列")}"><div class="screen-queue-head"><span>${t("队列")} <span class="count-pill">${papers.length}</span></span><button class="btn-icon" data-action="screen-queue-toggle" aria-label="${t("收起队列")}">${I("x", 16)}</button></div><div class="screen-queue-list">${items}</div></aside>`;
}

/** pageJump is the button that takes the embedded PDF to a quoted passage: to its
 * recorded page, or by searching the PDF text when no page was recorded. */
function pageJump(app: App, p: Paper, page: number, excerpt: string): string {
  if (!hasPdf(app, p)) return "";
  const label = page > 0 ? t("PDF 第 {n} 页", { n: page }) : t("在 PDF 中查找");
  return `<button class="text-link evidence-jump" data-action="screen-jump-page" data-id="${esc(p.id)}" data-page="${page > 0 ? page : 0}" data-excerpt="${esc(excerpt)}" title="${t("在右侧 PDF 中定位这段原文")}">${I("file", 13)} ${label}</button>`;
}

/** fulltextEvidence lists what agents extracted from this paper's full text, each with a way into the PDF. */
function fulltextEvidence(app: App, p: Paper): string {
  const items = app.evidenceFor("paperId", p.id);
  if (!items.length) return "";
  const row = (e: Evidence): string => `<div class="evidence-item ${esc(e.status)}"><div class="evidence-head"><strong>${esc(FIELD_LABELS[e.field] ?? e.field)}${e.value ? `: ${esc(evidenceValueText(e.value))}` : ""}</strong>${badge(e.status === "verified" ? t("已确认") : e.status === "rejected" ? t("已驳回") : t("待核对"), e.status === "verified" ? "green" : e.status === "rejected" ? "gray" : "amber")}</div><blockquote>${esc(e.excerpt)}${e.fileId ? pageJump(app, p, e.page, e.excerpt) : ""}</blockquote>${e.extractor ? `<div class="evidence-foot"><span class="evidence-source muted small">${esc(e.extractor)}</span></div>` : ""}</div>`;
  return `<section class="detail-section screen-evidence"><h3>${t("全文依据")} <span class="count-pill">${items.length}</span></h3>${items.map(row).join("")}</section>`;
}

/** jumpToPdfPage takes the screening PDF to a page, searching the full text for
 * the passage when no page is known. Outside screening it opens the paper in
 * screening first, so the drawer's evidence can jump too. */
export async function jumpToPdfPage(app: App, paperId: string, page: number, excerpt: string): Promise<void> {
  if (!app.paperById(paperId) || !hasBackend(app.env)) return;
  if (!app.screen || screenPapers(app)[app.screen.index]?.id !== paperId) {
    app.closeDrawer();
    app.screen = { ids: [paperId], index: 0, ai: null };
    app.navigate("papers");
  }
  if (!app.screenPdf) app.toggleScreenPdf();
  let target = page > 0 ? page : 0;
  if (!target && excerpt) {
    try { target = await Library.locateFulltextPage(paperId, excerpt); } catch (error) { app.toast(errorMessage(error, t("读取 PDF 文本失败。")), true); return; }
  }
  if (!target) { app.toast(t("未在 PDF 文本中找到这段原文。"), true); return; }
  const frame = $<HTMLIFrameElement>(".screen-pdf-frame");
  if (!frame) { app.toast(t("这篇文献没有可显示的 PDF。"), true); return; }
  // The viewer reads #page= when the document loads, so the frame is reloaded at the page.
  const base = frame.getAttribute("src")?.split("#")[0] ?? "";
  frame.src = "about:blank";
  requestAnimationFrame(() => { frame.src = `${base}#page=${target}`; });
}

/** Update only the suggestion: rebuilding the page would reload the PDF iframe. */
export function updateScreenAI(app: App, id: string): void {
  const region = $("#screen-ai");
  const p = app.paperById(id);
  if (!region || region.dataset.paperId !== id || !p || !app.screen?.ai) return;
  if (screenPapers(app)[app.screen.index]?.id !== id) return;
  region.innerHTML = aiBox(app, p);
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



const kbd = (key: string): string => `<kbd class="screen-key">${esc(key)}</kbd>`;

/** Keep the live PDF and its ancestors attached when only paper metadata changes.
 * Even removing and reinserting the same iframe reloads its browsing context. */
export function updateScreeningContent(content: HTMLElement, html: string): void {
  const panel = content.querySelector<HTMLElement>(".screen-panel");
  const frame = panel?.querySelector<HTMLIFrameElement>(".screen-pdf-frame");
  if (panel && frame) {
    const template = document.createElement("template");
    template.innerHTML = html;
    const nextPanel = template.content.querySelector<HTMLElement>(".screen-panel");
    const nextFrame = nextPanel?.querySelector<HTMLIFrameElement>(".screen-pdf-frame");
    // A #page= fragment from a jump does not count as a different document.
    const base = (f: HTMLIFrameElement | null | undefined): string | undefined => f?.getAttribute("src")?.split("#")[0];
    if (panel.dataset.paperId && panel.dataset.paperId === nextPanel?.dataset.paperId
      && base(frame) === base(nextFrame)) {
      const regions = [".screen-toolbar", ".screen-counts", ".screen-queue", ".screen-body", ".screen-pdf-head"].map(selector =>
        [panel.querySelector(selector), nextPanel.querySelector(selector)] as const);
      if (regions.every(([current, next]) => current && next)) {
        for (const [current, next] of regions) current!.replaceWith(next!);
        frame.title = nextFrame!.title;
        return;
      }
    }
  }
  content.innerHTML = html;
}

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
  const pdf = pdfPane(app, p);
  const pdfToggle = hasPdf(app, p) && !app.screenPdf ? `<button class="btn btn-secondary" data-action="screen-pdf-toggle">${I("file", 16)} ${t("显示 PDF")}</button>` : "";
  const aiToggle = screen.ai
    ? `<button class="btn btn-secondary ai-toggle is-on" data-action="screen-ai-off" title="${esc(screen.ai.requirement)}">${I("sparkle", 16)} ${t("AI 建议：开")}</button>`
    : app.canScreenWithAI() ? `<button class="btn btn-secondary ai-toggle" data-action="screen-ai-open">${I("sparkle", 16)} ${t("AI 建议")}</button>` : "";
  const queueToggle = `<button class="btn btn-secondary screen-queue-toggle" data-action="screen-queue-toggle" aria-pressed="${app.screenQueueOpen}" title="${t("显示或隐藏筛选队列")}">${I("more", 16)} ${t("队列")}</button>`;

  return `<section class="panel screen-panel" data-paper-id="${esc(p.id)}" aria-label="${t("逐篇筛选")}">`
    + `<div class="screen-toolbar"><button class="btn btn-secondary" data-action="exit-screening">${I("x", 16)} ${t("退出筛选")} ${kbd("Esc")}</button><div class="screen-progress"><strong>${t("第 {n} / {total} 篇", { n: screen.index + 1, total })}</strong><span class="muted">${t("已判断 {done} 篇", { done: decided })}</span></div><div class="screen-nav">${queueToggle}${pdfToggle}${aiToggle}<button class="btn btn-secondary" data-action="screen-prev" ${screen.index === 0 ? "disabled" : ""} aria-label="${t("上一篇")}">${I("chevron-left", 16)} ${t("上一篇")}</button><button class="btn btn-secondary" data-action="screen-next" ${screen.index >= total - 1 ? "disabled" : ""} aria-label="${t("下一篇")}">${t("下一篇")} ${I("chevron-right", 16)}</button></div></div>`
    + `<div class="screen-counts">${counts}</div>`
    + `<div class="screen-split${pdf ? " has-pdf" : ""}">${queueRail(app, papers, screen.index)}<div class="screen-body">`
    + `<div class="screen-actionbar"><div class="screen-decisions">${decisionButtons}</div><div id="screen-ai" data-paper-id="${esc(p.id)}" aria-live="polite">${aiBox(app, p)}</div>${reason}<div class="screen-extra"><button class="btn ${p.starred ? "btn-primary" : "btn-secondary"}" data-action="toggle-star" data-id="${esc(p.id)}">${kbd("S")} ${I("star", 16)} ${p.starred ? t("已收藏") : t("收藏")}</button><button class="btn btn-secondary" data-action="open-paper" data-id="${esc(p.id)}">${kbd("Enter")} ${t("完整详情")}</button>${app.canTranslate() ? `<button class="btn btn-secondary" data-action="screen-translate" data-id="${esc(p.id)}" title="${t("翻译整段摘要并原位显示；划选文字可单独翻译")}">${kbd("T")} ${t("翻译摘要")}</button>` : ""}<span class="muted footer-hint">${t("←→ 前后切换 · Y 纳入 · N 排除 · U 证据不足 · 判断后自动跳到下一篇")}</span></div></div>`
    + `<div class="screen-status-row">${screenBadge(p.screening)}${badge(READING[p.reading] ?? t("未读"))}${p.publicationStatus === "preprint" ? badge(t("预印本"), "amber") : ""}${demoBadge(p)}${p.starred ? badge(t("已收藏"), "amber") : ""}<span class="muted screen-id">${esc(p.id)}</span></div>`
    + `<h2 class="screen-title">${esc(p.title)}</h2>`
    + `<p class="paper-meta">${esc(p.authors || t("作者待补"))} · ${esc(p.journal || t("期刊待补"))} · ${esc(p.year || t("年份待补"))}</p>`
    + `<div class="doi-bar"><span class="doi-bar-label">DOI</span>${doi}</div>`
    + proposal
    + metadataButton(app, p)
    + `<div class="detail-grid screen-grid">${editableDetail(p.id, "species", t("研究物种"), esc(speciesDisplay(p, false) || t("待补")) + (speciesNames(p).length ? `<small class="muted"> ${esc(joinList(speciesNames(p), listSeparator))}</small>` : ""))}${editableDetail(p.id, "organs", t("组织 / 部位"), esc(displayList(p.organs) || t("待补")))}${editableDetail(p.id, "modalities", t("测序类型"), esc(displayList(p.modalities) || t("待补")))}${editableDetail(p.id, "platforms", t("测序平台"), esc(displayList(p.platforms) || t("待补")))}${editableDetail(p.id, "articleTypes", t("文献类型"), esc(displayList(p.articleTypes) || t("待判断")))}${editableDetail(p.id, "datasets", t("关联数据集"), ds.length ? esc(ds.map((d) => d.accession || d.title).join(listSeparator)) : `<span class="muted">${t("未关联")}</span>`)}${editableDetail(p.id, "source", t("发现来源"), esc(tv(p.source || "手动添加")))}${editableDetail(p.id, "addedAt", t("加入时间"), dateText(p.addedAt))}</div>`
    + abstractSection(app, p)
    + fulltextEvidence(app, p)
    + (arr(p.tags).length ? `<section class="detail-section"><h3>${t("主题标签")}</h3><div class="tag-list">${arr(p.tags).map((tag) => `<span class="tag">${esc(tag)}</span>`).join("")}</div></section>` : "")
    + (p.notes ? `<section class="detail-section"><h3>${t("研究笔记")}</h3><p class="abstract-text">${esc(p.notes)}</p></section>` : "")
    + `</div>${pdf}</div>`
    + `</section>`;
}

/** screenActions builds the handlers of the buttons in screening mode. */
export const screenActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "start-screening": () => app.startScreening(),
  "exit-screening": () => app.exitScreening(),
  "screen-prev": () => app.moveScreening(-1),
  "screen-next": () => app.moveScreening(1),
  "screen-go": (el) => app.goToScreening(Number(el.dataset.index)),
  "screen-queue-toggle": () => app.toggleScreenQueue(),
  "screen-decide": (el, id) => app.decideInScreening(id, el.dataset.status ?? ""),
  "screen-ai-open": () => screenAIModal(app, true),
  "screen-ai-skip": () => app.closeModal(),
  "screen-ai-off": () => app.disableScreeningAI(),
  "screen-ai-retry": (_el, id) => app.requestJudgement(id, true),
  "screen-ai-adopt": (_el, id) => app.adoptJudgement(id),
  "screen-translate": (_el, id) => app.translateInScreening(id),
  "screen-abstract-toggle": (_el, id) => app.toggleAbstractTranslation(id),
  "screen-translate-selection": () => app.retrySelectionTranslation(),
  "screen-translate-clear": () => app.clearTranslation(),
  "screen-pdf-toggle": () => app.toggleScreenPdf(),
  "screen-jump-page": (el, id) => { void jumpToPdfPage(app, id, Number(el.dataset.page ?? 0), el.dataset.excerpt ?? ""); },
  "evidence-jump": (el, id) => { void jumpToPdfPage(app, id, Number(el.dataset.page ?? 0), el.dataset.excerpt ?? ""); },
});

/** screenModelReady: a real library that may be written, with a model bound to screening. */
export const screenModelReady = (app: App): boolean => canWrite(app.env) && configured(modelFor(app.env.settings, "screen"));
export const translateModelReady = (app: App): boolean => canWrite(app.env) && configured(modelFor(app.env.settings, "translate"));


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
