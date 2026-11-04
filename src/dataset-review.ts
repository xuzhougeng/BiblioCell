// Dataset review mode: one dataset at a time, so a person can clean up what
// source updates brought in — keep it (marking it reviewed) or delete it on the
// spot — and move on with the keyboard. Like paper screening, the list is a
// snapshot of the filtered datasets at entry, so a decision never reshuffles it.
import type { App } from "./app.ts";
import type { Dataset, DatasetRepository } from "./mygo.ts";
import { AVAILABILITY, ROLE, arr, datasetLocation, esc, joinList, repositoryLabel, repositoryURL, speciesDisplay } from "./model.ts";
import { I, badge, datasetStatusBadge, demoBadge } from "./ui.ts";
import { listSeparator, t, tv } from "./i18n.ts";

export interface DatasetReviewState {
  /** Dataset ids in review order, fixed when the mode starts. */
  ids: string[];
  index: number;
  /** How many this run deleted, for the progress line. */
  deleted: number;
}

const kbd = (key: string): string => `<kbd class="screen-key">${esc(key)}</kbd>`;
const displayList = (values: string[] | null | undefined): string => joinList(arr(values).map(tv), listSeparator);

/** Review keys: the key, the action and what the shortcut sheet calls it. */
export const DATASET_REVIEW_KEYS = {
  keep: ["y", "k"],
  remove: ["d", "Delete", "Backspace"],
  edit: ["e"],
} as const;

/** reviewDatasets drops datasets deleted since the snapshot and clamps the cursor. */
export function reviewDatasets(app: App): Dataset[] {
  const review = app.datasetScreen;
  if (!review) return [];
  const datasets = review.ids.map((id) => app.state.datasets.find((d) => d.id === id)).filter((d): d is Dataset => Boolean(d));
  review.ids = datasets.map((d) => d.id);
  review.index = Math.min(Math.max(review.index, 0), Math.max(datasets.length - 1, 0));
  return datasets;
}

function queueRail(app: App, datasets: Dataset[], index: number): string {
  const items = datasets.map((d, i) =>
    `<button class="screen-queue-item${i === index ? " is-current" : ""}" data-action="dataset-review-go" data-index="${i}"${i === index ? ' aria-current="true"' : ""} title="${esc(d.title)}"><span class="screen-queue-dot ${d.awaitingReview ? "amber" : "green"}" aria-label="${d.awaitingReview ? t("待核查") : t("已核查")}"></span><span class="screen-queue-index">${i + 1}</span><span class="screen-queue-title">${esc(d.accession ? `${d.accession} · ${d.title}` : d.title)}</span></button>`,
  ).join("");
  return `<aside class="screen-queue${app.screenQueueOpen ? " is-open" : ""}" aria-label="${t("核查队列")}"><div class="screen-queue-head"><span>${t("队列")} <span class="count-pill">${datasets.length}</span></span><button class="btn-icon" data-action="screen-queue-toggle" aria-label="${t("收起队列")}">${I("x", 16)}</button></div><div class="screen-queue-list">${items}</div></aside>`;
}

const repositoryRow = (r: DatasetRepository): string => {
  const url = repositoryURL(r);
  const name = `<strong>${esc(repositoryLabel(r.repository))} ${esc(r.accession)}</strong>`;
  return `<div class="evidence-item"><div class="evidence-head">${url ? `<a class="text-link" href="${esc(url)}" target="_blank" rel="noopener noreferrer">${name}${I("arrow-up-right", 13)}</a>` : name}${badge(r.verified ? t("已核实") : t("待核实"), r.verified ? "green" : "amber")}</div>${r.evidenceText ? `<blockquote>${esc(r.evidenceText)}</blockquote>` : ""}</div>`;
};

export function renderDatasetReview(app: App): string {
  const datasets = reviewDatasets(app);
  const review = app.datasetScreen!;
  if (!datasets.length) {
    const done = review.deleted ? t("本次删除了 {n} 组数据集。", { n: review.deleted }) : t("当前筛选结果为空。回到列表调整筛选条件后再进入逐个核查。");
    return `<div class="empty-state"><div class="empty-icon">${I("check-circle", 28)}</div><h3>${t("没有可核查的数据集")}</h3><p>${done}</p><button class="btn btn-secondary" data-action="exit-dataset-review">${t("返回列表")}</button></div>`;
  }
  const d = datasets[review.index]!;
  const total = datasets.length;
  const awaiting = datasets.filter((x) => x.awaitingReview).length;
  const counts = `<span class="screen-count uncertain">${t("待核查")} <strong>${awaiting}</strong></span><span class="screen-count included">${t("已核查")} <strong>${total - awaiting}</strong></span><span class="screen-count excluded">${t("本次删除")} <strong>${review.deleted}</strong></span>`;
  const home = datasetLocation(d);
  const accession = home
    ? `<a class="text-link" href="${esc(home)}" target="_blank" rel="noopener noreferrer">${esc(d.accession || t("查看仓库"))}${I("arrow-up-right", 13)}</a>`
    : `<span>${esc(d.accession || t("编号待补"))}</span>`;
  const links = arr(d.paperLinks).map((l) => ({ role: l.role, paper: app.paperById(l.paperId) })).filter((l) => l.paper);
  const papers = links.length
    ? links.map((l) => `<button class="related-dataset" data-action="open-paper" data-id="${esc(l.paper!.id)}"><span class="icon-wrap blue">${I("book", 18)}</span><span><strong>${esc(l.paper!.title)}</strong><small>${esc(l.paper!.year || t("年份待补"))}</small></span>${badge(ROLE[l.role] ?? t("关联"))}</button>`).join("")
    : `<p class="muted">${t("尚未关联文献。")}</p>`;
  const detail = (label: string, value: string): string => `<div class="detail-item"><span>${label}</span><strong>${value}</strong></div>`;
  const keep = d.awaitingReview
    ? `<button class="btn btn-primary screen-decision" data-action="dataset-review-keep" data-id="${esc(d.id)}">${kbd("Y")} ${t("保留，核查完成")}</button>`
    : `<button class="btn btn-primary screen-decision is-current" data-action="dataset-review-keep" data-id="${esc(d.id)}" aria-pressed="true">${kbd("Y")} ${t("保留（已核查）")}</button>`;
  const actions = `${keep}<button class="btn btn-danger screen-decision" data-action="dataset-review-delete" data-id="${esc(d.id)}">${kbd("D")} ${t("删除")}</button>`;
  const extra = `<button class="btn btn-secondary" data-action="edit-dataset" data-id="${esc(d.id)}">${kbd("E")} ${t("编辑信息")}</button><button class="btn btn-secondary" data-action="open-dataset" data-id="${esc(d.id)}">${kbd("Enter")} ${t("完整详情")}</button><span class="muted footer-hint">${t("←→ 前后切换 · Y 保留 · D 删除（不可撤销） · 处理后自动跳到下一组")}</span>`;
  const queueToggle = `<button class="btn btn-secondary screen-queue-toggle" data-action="screen-queue-toggle" aria-pressed="${app.screenQueueOpen}" title="${t("显示或隐藏核查队列")}">${I("more", 16)} ${t("队列")}</button>`;

  return `<section class="panel screen-panel dataset-review-panel" data-dataset-id="${esc(d.id)}" aria-label="${t("逐个核查数据集")}">`
    + `<div class="screen-toolbar"><button class="btn btn-secondary" data-action="exit-dataset-review">${I("x", 16)} ${t("退出核查")} ${kbd("Esc")}</button><div class="screen-progress"><strong>${t("第 {n} / {total} 组", { n: review.index + 1, total })}</strong><span class="muted">${t("待核查 {n} 组", { n: awaiting })}</span></div><div class="screen-nav">${queueToggle}<button class="btn btn-secondary" data-action="dataset-review-prev" ${review.index === 0 ? "disabled" : ""}>${I("chevron-left", 16)} ${t("上一组")}</button><button class="btn btn-secondary" data-action="dataset-review-next" ${review.index >= total - 1 ? "disabled" : ""}>${t("下一组")} ${I("chevron-right", 16)}</button></div></div>`
    + `<div class="screen-counts">${counts}</div>`
    + `<div class="screen-split">${queueRail(app, datasets, review.index)}<div class="screen-body">`
    + `<div class="screen-actionbar"><div class="screen-decisions">${actions}</div><div class="screen-extra">${extra}</div></div>`
    + `<div class="screen-status-row">${d.awaitingReview ? badge(t("待核查"), "amber") : badge(t("已核查"), "green")}${datasetStatusBadge(d)}${demoBadge(d)}<span class="muted screen-id">${esc(d.id)}</span></div>`
    + `<h2 class="screen-title">${esc(d.title)}</h2>`
    + `<div class="doi-bar"><span class="doi-bar-label">${esc(d.repository ? repositoryLabel(d.repository) : t("仓库待补"))}</span>${accession}</div>`
    + `<div class="detail-grid screen-grid">${detail(t("研究物种"), esc(speciesDisplay(d) || t("待补")))}${detail(t("组织 / 部位"), esc(displayList(d.organs) || t("待补")))}${detail(t("测序类型"), esc(displayList(d.modalities) || t("待补")))}${detail(t("测序平台"), esc(displayList(d.platforms) || t("待补")))}${detail(t("细胞数"), d.cellsReported == null ? t("尚未整理") : esc(d.cellsReported))}${detail(t("原始数据"), esc(AVAILABILITY[d.availability] ?? t("尚未核实")))}</div>`
    + `<section class="detail-section"><h3>${t("描述")}</h3><p class="abstract-text">${esc(d.description || t("仓库记录没有描述。"))}</p></section>`
    + (arr(d.repositories).length > 1 || arr(d.repositories).some((r) => r.evidenceText) ? `<section class="detail-section"><h3>${t("数据仓库")}</h3>${arr(d.repositories).map(repositoryRow).join("")}</section>` : "")
    + `<section class="detail-section"><h3>${t("关联文献")} <span class="count-pill">${links.length}</span></h3>${papers}</section>`
    + (d.notes ? `<section class="detail-section"><h3>${t("整理备注")}</h3><p class="abstract-text">${esc(d.notes)}</p></section>` : "")
    + `</div></div></section>`;
}

/** datasetReviewActions builds the handlers of the buttons in dataset review mode. */
export const datasetReviewActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "start-dataset-review": () => app.startDatasetReview(),
  "exit-dataset-review": () => app.exitDatasetReview(),
  "dataset-review-prev": () => app.moveDatasetReview(-1),
  "dataset-review-next": () => app.moveDatasetReview(1),
  "dataset-review-go": (el) => app.goToDatasetReview(Number(el.dataset.index)),
  "dataset-review-keep": (_el, id) => { void app.keepInDatasetReview(id); },
  "dataset-review-delete": (el, id) => { void app.deleteDatasets([id], el); },
});
