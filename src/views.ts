// Page renderers for overview, papers, datasets and tasks.
import { speciesText } from "./species.ts";
import { PAPER_COLUMNS, type PaperColumn } from "./columns.ts";
import type { App } from "./app.ts";
import type { Dataset, DatasetRepository, Paper } from "./mygo.ts";
import {
  FULL, PAGE_SIZES, ROLE, SORT_OPTIONS, SCREEN, arr, datasetLocation, esc, filterDatasets, filterPapers, joinList, pageList, paperPage, repositoryLabel, repositoryURL, safeURL, sortParts, speciesDisplay, uniqueValues, speciesLabelOf, listValues,
  type ListField,
  reviewConcern, type SortKey, type TaskRow, type TaskType,
} from "./model.ts";
import { I, badge, datasetStatusBadge, demoBadge, empty, fullBadge, modalityBadges, option, progressRow, screenBadge, selectFilter, sortChip } from "./ui.ts";
import { renderCalibration } from "./calibration.ts";
import { bulkBar, selectCell, selectHeader } from "./selection.ts";
import { listSeparator, locale, t, tv } from "./i18n.ts";

/** displayList joins stored values for display, translating known labels like modalities. */
const displayList = (values: string[] | null | undefined): string => joinList(arr(values).map(tv), listSeparator);

export function taskRows(items: TaskRow[]): string {
  return items.map((t) =>
    `<button class="task-item" data-action="open-paper" data-id="${esc(t.paper.id)}"><span class="icon-wrap ${t.color}">${I(t.icon, 18)}</span><span class="task-content"><span class="task-title">${esc(t.title)}</span><span class="task-paper">${esc(t.paper.title)}</span><span class="task-subtitle muted">${esc(t.subtitle)}</span></span>${I("chevron-right", 16)}</button>`,
  ).join("");
}

const TASK_GROUPS: [TaskType, string][] = [["screen", t("确认收录范围")], ["fulltext", t("待补全文")], ["data", t("待核实数据关联")]];
const TASK_GROUP_LIMIT = 3;

/** taskGroups lists upcoming tasks grouped by type: the task name once, then only paper titles. */
export function taskGroups(ts: TaskRow[]): string {
  const groups = TASK_GROUPS.map(([type, label]) => {
    const items = ts.filter((t) => t.type === type);
    if (!items.length) return "";
    const rows = items.slice(0, TASK_GROUP_LIMIT).map((t) =>
      `<button class="task-row" data-action="open-paper" data-id="${esc(t.paper.id)}"><span>${esc(t.paper.title)}</span>${I("chevron-right", 14)}</button>`,
    ).join("");
    const more = items.length > TASK_GROUP_LIMIT ? `<span class="task-more muted">${t("还有 {n} 篇", { n: items.length - TASK_GROUP_LIMIT })}</span>` : "";
    return `<div class="task-group"><div class="task-group-title"><span>${label}</span><span class="count-pill">${items.length}</span></div>${rows}${more}</div>`;
  }).join("");
  return groups || `<p class="muted task-empty">${t("暂无待补事项。")}</p>`;
}

/** datasetChips shows a paper's linked datasets; a chip opens the dataset's web location, or its drawer when none is known. */
function datasetChips(app: App, p: Paper): string {
  const linked = app.paperDatasets(p.id);
  if (!linked.length) return `<span class="muted">${t("未关联")}</span>`;
  const chips = linked.slice(0, 2).map((d) => {
    const label = d.accession || (d.repository ? repositoryLabel(d.repository) : "") || d.title;
    const url = datasetLocation(d);
    return url
      ? `<a class="link-chip" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="${t("{title} · 在仓库中打开", { title: esc(d.title) })}"><span class="link-chip-title">${esc(label)}</span>${I("arrow-up-right", 12)}</a>`
      : `<button class="link-chip" data-action="open-dataset" data-id="${esc(d.id)}" title="${esc(d.title)}"><span class="link-chip-title">${esc(label)}</span></button>`;
  }).join("");
  return `${chips}${linked.length > 2 ? `<span class="link-more">+${linked.length - 2}</span>` : ""}`;
}

/** fulltextEntry resolves where a linked fulltext opens: the recorded entry, or the DOI landing page. */
function fulltextEntry(app: App, p: Paper): string {
  if (p.fulltext !== "linked") return "";
  const resource = app.paperResources(p.id).find((r) => r.type === "fulltext" && safeURL(r.url));
  if (resource) return safeURL(resource.url);
  return p.doi ? `https://doi.org/${encodeURI(p.doi)}` : "";
}

/** fulltextCell makes the status actionable: open the stored PDF file, or follow the recorded entry link. */
function fulltextCell(app: App, p: Paper): string {
  const supplement = () => `<button class="fulltext-status is-link" data-action="supplement-fulltext" data-id="${esc(p.id)}" title="${t("补充全文链接或文件")}" aria-label="${t("补充全文：{title}", { title: esc(p.title) })}">${fullBadge(p.fulltext)}${I("plus", 12)}</button>`;
  if (p.fulltext === "stored") {
    const file = app.paperResources(p.id).find((r) => r.localPath && r.type === "fulltext");
    if (file) return `<button class="fulltext-status is-link" data-action="open-file" data-id="${esc(file.id)}" title="${t("打开已保存的全文文件")}">${fullBadge(p.fulltext)}${I("file", 12)}</button>`;
    return supplement();
  }
  const url = p.fulltext === "linked" ? safeURL(p.fulltextUrl) || fulltextEntry(app, p) : "";
  return url
    ? `<a class="fulltext-status is-link" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="${t("打开全文入口")}">${fullBadge(p.fulltext)}${I("arrow-up-right", 12)}</a>`
    : supplement();
}

export function paperTable(items: Paper[], app: App, compact = false, sort = ""): string {
  if (!items.length) return empty(t("没有找到匹配文献"), t("尝试调整筛选条件，或添加一篇新文献。"), `<button class="btn btn-secondary" data-action="clear-filters">${t("清空筛选")}</button>`);
  const active = compact ? null : sortParts(sort);
  const th = (label: string, key?: SortKey): string => {
    if (compact || !key) return `<th>${label}</th>`;
    const on = active!.key === key;
    const dir = on ? active!.dir : "";
    return `<th${on ? ` aria-sort="${dir === "asc" ? "ascending" : "descending"}"` : ""}><button class="th-sort${on ? " on" : ""}" data-action="sort-column" data-key="${key}" aria-label="${t("按{label}排序", { label })}">${label}<span class="sort-mark">${on ? I(dir === "asc" ? "chevron-up" : "chevron-down", 12) : ""}</span></button></th>`;
  };
  const columns: PaperColumn[] = compact ? ["star", "title", "species", "modality", "screening"] : app.paperColumns;
  const labels = Object.fromEntries(PAPER_COLUMNS) as Record<PaperColumn, string>;
  const sortKeys: Partial<Record<PaperColumn, SortKey>> = { title: "title", year: "year", species: "species", modality: "modality" };
  const rows = items.map((p) => {
    const concern = reviewConcern(p);
    const concernLabel = concern === "excluded" ? t("建议排除") : concern === "source" ? t("来源新增") : t("建议核对");
    const concernReason = concern === "source" ? t("来自 {source} 的更新，尚未人工核查。", { source: p.source ? tv(p.source) : t("来源") })
      : p.proposedReason || (concern === "excluded" ? t("可能不符合收录范围，请查看详情确认。") : t("收录依据仍有疑问，请查看详情确认。"));
    const concernHint = concern === "source" ? t("查看详情，确认收录范围") : t("查看详情，核对依据");
    // The review state lives in the status column; the note under the title only appears when that column is hidden.
    const warning = concern && !columns.includes("screening") ? `<div class="paper-review-note ${concern}"><span class="paper-review-label">${I("alert", 14)}${t("{label} · 待你确认", { label: concernLabel })}</span><span class="paper-review-reason" title="${esc(concernReason)}">${esc(concernReason)}</span><span class="paper-review-hint">${concernHint} ${I("arrow-right", 12)}</span></div>` : "";
    const cells: Record<PaperColumn, string> = {
      star: `<td class="star-cell"><button class="btn-icon star-button${p.starred ? " is-starred" : ""}" data-action="toggle-star" data-id="${esc(p.id)}" aria-label="${p.starred ? t("取消收藏") : t("加入收藏")}" aria-pressed="${Boolean(p.starred)}">${I("star", 16)}</button></td>`,
      title: `<td class="paper-main-cell"><div class="paper-title">${esc(p.title)}</div><div class="paper-meta">${demoBadge(p)}<span>${esc(p.authors || t("作者待补"))}</span><span class="meta-separator">·</span><span>${esc(p.year || t("年份待补"))}</span>${p.publicationStatus === "preprint" ? `<span class="preprint-label">${t("预印本")}</span>` : ""}</div>${warning}</td>`,
      journal: `<td class="journal-cell">${p.journal ? esc(p.journal) : `<span class="muted">${t("期刊待补")}</span>`}</td>`,
      year: `<td class="year-cell">${p.year ? esc(String(p.year)) : `<span class="muted">${t("年份待补")}</span>`}</td>`,
      species: `<td><div class="species-label">${esc(speciesDisplay(p) || t("物种待补"))}</div><div class="species-name">${esc(displayList(p.organs) || t("组织待补"))}</div></td>`,
      datasets: `<td class="link-cell">${datasetChips(app, p)}</td>`,
      modality: `<td>${modalityBadges(p)}</td>`,
      fulltext: `<td>${fulltextCell(app, p)}</td>`,
      screening: `<td class="screening-cell">${screenBadge(p.screening)}${concern ? `<div class="screening-proposal ${concern}" title="${esc(concernReason)}">${I("alert", 13)}${t("{label} · 待你确认", { label: concernLabel })}<small>${concernHint}</small></div>` : ""}</td>`,
    };
    const selected = !compact && app.selectedPapers.has(p.id);
    const classes = [concern ? `paper-needs-review ${concern}` : "", selected ? "is-selected" : ""].filter(Boolean).join(" ");
    return `<tr${classes ? ` class="${classes}"` : ""} data-action="open-paper" data-id="${esc(p.id)}" tabindex="0" aria-label="${concern ? t("{label}，待确认：{title}", { label: concernLabel, title: esc(p.title) }) : t("查看 {title}", { title: esc(p.title) })}">${compact ? "" : selectCell(p.id, p.title, selected)}${columns.map(id => cells[id]).join("")}<td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`;
  }).join("");
  const selectAll = compact ? "" : selectHeader(items.every((p) => app.selectedPapers.has(p.id)), items.length > 0);
  const head = `<tr>${selectAll}${columns.map(id => id === "star" ? `<th class="col-star"><span class="sr-only">${t("收藏")}</span></th>` : th(labels[id], sortKeys[id])).join("")}<th class="col-arrow"></th></tr>`;

  return `<div class="table-wrap"><table class="paper-table${compact ? " compact-table" : ""}"><thead>${head}</thead><tbody>${rows}</tbody></table></div>`;
}

/** countBy tallies papers per value of a list field; a paper counts once per distinct value. */
function countBy(papers: Paper[], field: ListField): Map<string, number> {
  const counts = new Map<string, number>();
  for (const p of papers) {
    for (const v of new Set(listValues[field](p).filter(Boolean))) counts.set(v, (counts.get(v) ?? 0) + 1);
  }
  return counts;
}

const DISTRIBUTION_LIMIT = 6;

/** distributionPanel renders the top values of a field as horizontal bars; each row filters the papers page. */
function distributionPanel(papers: Paper[], field: ListField, title: string, subtitle: string, label: (v: string) => string): string {
  const counts = countBy(papers, field);
  const top = [...counts.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0], locale)).slice(0, DISTRIBUTION_LIMIT);
  const max = Math.max(1, ...top.map((e) => e[1]));
  const rows = top.map(([value, count]) =>
    `<button class="species-bar-row" data-action="overview-filter" data-field="${field}" data-value="${esc(value)}" title="${esc(t("{label}：{n} 篇", { label: label(value), n: count }))}"><span>${esc(label(value))}</span><div class="species-bar-track"><div style="width:${Math.round((count / max) * 100)}%"></div></div><strong>${count}</strong></button>`,
  ).join("");
  return `<section class="panel"><div class="panel-header"><div><h2>${title}</h2><p class="muted">${subtitle}</p></div><span class="muted">${t("{n} 个分类", { n: counts.size })}</span></div><div class="panel-body species-bars">${rows || `<p class="muted">${t("暂无数据")}</p>`}</div></section>`;
}

const YEAR_SPAN = 15;

/** yearPanel charts papers per publication year: the latest YEAR_SPAN years, gaps included, older years folded together. */
function yearPanel(papers: Paper[]): string {
  const counts = countBy(papers, "year");
  const years = [...counts.keys()].map(Number).filter(Number.isInteger);
  const missing = papers.filter((p) => !p.year).length;
  const note = missing ? t("{n} 篇年份待补", { n: missing }) : t("{n} 个年份", { n: years.length });
  let body = `<p class="muted">${t("暂无数据")}</p>`;
  if (years.length) {
    const last = Math.max(...years);
    const first = Math.max(Math.min(...years), last - YEAR_SPAN + 1);
    const earlier = years.filter((y) => y < first).reduce((sum, y) => sum + (counts.get(String(y)) ?? 0), 0);
    const cols: { label: string; count: number; year?: string }[] = [];
    if (earlier) cols.push({ label: t("{year} 前", { year: first }), count: earlier });
    for (let y = first; y <= last; y++) cols.push({ label: String(y), count: counts.get(String(y)) ?? 0, year: String(y) });
    const max = Math.max(1, ...cols.map((c) => c.count));
    body = cols.map((c) => {
      const tip = esc(t("{label}：{n} 篇", { label: c.label, n: c.count }));
      const bar = `<span class="year-count">${c.count || ""}</span><span class="year-bar" style="height:${Math.round((c.count / max) * 100)}%"></span>`;
      const label = `<span class="year-label">${esc(c.label)}</span>`;
      return c.year && c.count
        ? `<button class="year-col" data-action="overview-filter" data-field="year" data-value="${c.year}" title="${tip}"><span class="year-plot">${bar}</span>${label}</button>`
        : `<div class="year-col${c.year ? "" : " year-earlier"}" title="${tip}"><span class="year-plot">${bar}</span>${label}</div>`;
    }).join("");
    body = `<div class="year-chart">${body}</div>`;
  }
  return `<section class="panel year-panel"><div class="panel-header"><div><h2>${t("年度发表")}</h2><p class="muted">${t("各年份发表的文献数量，点击柱子筛选")}</p></div><span class="muted">${note}</span></div><div class="panel-body">${body}</div></section>`;
}

const WEEK = 7 * 24 * 60 * 60 * 1000;

/** screeningDashboard shows how far screening has come and where to pick it up:
 * a bar of decisions, what arrived this week, what waits for review, and a
 * button that opens screening on the first undecided paper. */
export function screeningDashboard(app: App, now = Date.now()): string {
  const papers = app.state.papers;
  const total = papers.length;
  const by = (status: string) => papers.filter((p) => p.screening === status).length;
  const undecided = by("pending");
  const decided = total - undecided;
  const thisWeek = papers.filter((p) => Date.parse(p.addedAt) >= now - WEEK).length;
  const concerns = papers.filter((p) => reviewConcern(p)).length;
  const segments = (["included", "excluded", "uncertain", "pending"] as const).map((status) => {
    const n = by(status);
    return n ? `<span class="screen-bar-seg ${status}" style="width:${(n / total) * 100}%" title="${esc(SCREEN[status]!)} ${n}"></span>` : "";
  }).join("");
  const legend = (["included", "excluded", "uncertain", "pending"] as const).map((status) =>
    `<span class="screen-bar-key"><span class="screen-queue-dot ${status === "included" ? "green" : status === "excluded" ? "red" : status === "uncertain" ? "amber" : ""}"></span>${esc(SCREEN[status]!)} <strong>${by(status)}</strong></span>`).join("");
  const metric = (label: string, value: number, action: string, target = ""): string =>
    `<button class="dashboard-metric" data-action="${action}"${target ? ` data-target="${target}"` : ""}><span class="dashboard-metric-label">${label}</span><strong>${value}</strong></button>`;
  return `<section class="panel dashboard-panel" aria-label="${t("筛选进度")}"><div class="panel-header"><div><h2>${t("筛选进度")}</h2><p class="muted">${t("已判断 {done} / {total} 篇", { done: decided, total })}</p></div><div class="section-actions"><button class="text-link" data-action="export-report">${I("download", 15)} ${t("导出进度报告")}</button><button class="btn btn-primary" data-action="resume-screening"${undecided ? "" : " disabled"}>${I("check-circle", 16)} ${t("继续筛选")}</button></div></div>`
    + `<div class="panel-body"><div class="screen-bar" role="img" aria-label="${t("已判断 {done} / {total} 篇", { done: decided, total })}">${segments}</div><div class="screen-bar-legend">${legend}</div>`
    + `<div class="dashboard-metrics">${metric(t("本周新增"), thisWeek, "stat-nav", "papers")}${metric(t("需核对"), concerns, "review-papers-nav")}${metric(t("待判断"), undecided, "stat-nav", "screen")}${metric(t("全文待补"), papers.filter((p) => p.screening === "included" && p.fulltext !== "stored").length, "stat-nav", "fulltext")}</div></div></section>`;
}

export function renderOverview(app: App): string {
  const papers = app.state.papers;
  const total = papers.length;
  const missing = papers.filter((p) => p.screening === "included" && p.fulltext !== "stored").length;
  const pending = papers.filter((p) => p.screening === "pending" || p.screening === "uncertain").length;
  const withData = papers.filter((p) => app.paperDatasets(p.id).length).length;
  const withFulltext = papers.filter((p) => p.fulltext !== "missing").length;
  const recent = [...papers].sort((a, b) => String(b.addedAt).localeCompare(String(a.addedAt))).slice(0, 5);
  const ts = app.tasks();
  const stats: [string, number, string, string, string][] = [
    [t("文献条目"), total, t("已纳入 {n} 篇", { n: papers.filter((p) => p.screening === "included").length }), "book", "papers"],
    [t("关联数据集"), app.state.datasets.length, t("支持原始、复用与重新处理"), "database", "datasets"],
    [t("全文待补"), missing, t("已有入口，也可继续补正文"), "file", "fulltext"],
    [t("收录待判断"), pending, t("保留不确定项，减少遗漏"), "check", "screen"],
  ];
  const statCards = stats.map((s, i) =>
    `<button class="stat-card" data-action="stat-nav" data-target="${s[4]}"><div class="stat-top"><span class="stat-label">${s[0]}</span><span class="stat-icon">${I(s[3], 18)}</span></div><div class="stat-value">${s[1]}<span class="stat-unit">${i === 1 ? t("组") : t("篇")}</span></div><div class="stat-meta">${s[2]}${I("arrow-right", 14)}</div></button>`,
  ).join("");
  const coverage = `<section class="panel coverage-panel"><div class="panel-header"><div><h2>${t("整理覆盖")}</h2><p class="muted">${t("收录与完善，各自向前一步")}</p></div></div><div class="panel-body progress-list">${progressRow(t("收录范围已判断"), total - pending, total)}${progressRow(t("已关联数据集"), withData, total, "blue")}${progressRow(t("已记录全文入口"), withFulltext, total, "gold")}</div></section>`;
  // The statistics can leave preprints out; screening progress and coverage always count every paper.
  const preprints = papers.filter((p) => p.publicationStatus === "preprint").length;
  const screening = app.overviewScreening;
  const counted = papers.filter((p) => (!app.overviewHidePreprint || p.publicationStatus !== "preprint") && (!screening.length || screening.includes(p.screening)));
  const screeningChoices = `<div class="overview-screening" role="group" aria-label="${t("按收录状态统计")}"><span class="muted">${t("收录状态")}</span>`
    + Object.entries(SCREEN).map(([status, label]) => `<label class="checkbox-choice"><input type="checkbox" data-overview-screening="${status}"${screening.includes(status) ? " checked" : ""}><span>${esc(label)} (${papers.filter((p) => p.screening === status).length})</span></label>`).join("")
    + `</div>`;
  return `<div class="stat-grid">${statCards}</div><div class="overview-progress-grid">${screeningDashboard(app)}${coverage}</div>`
    + `<div class="overview-grid"><section class="panel recent-panel"><div class="panel-header"><div><h2>${t("最近收录")}</h2><p class="muted">${t("新线索先留下，整理慢慢来")}</p></div><button class="text-link" data-page="papers">${t("查看全部")} ${I("arrow-right", 16)}</button></div>${paperTable(recent, app, true)}<div class="panel-footnote">${I("info", 14)} ${t("当前条目为原型示例，可通过新增或导入建立自己的目录。")}</div></section>`
    + `<section class="panel tasks-panel"><div class="panel-header"><div><h2>${t("接下来整理")}</h2><p class="muted">${t("按需要补齐信息")}</p></div><span class="count-pill">${ts.length}</span></div><div class="task-groups">${taskGroups(ts)}</div><button class="panel-bottom-link" data-page="tasks">${t("查看全部待补事项")} ${I("arrow-right", 16)}</button></section></div>`
    + `<div class="overview-section-head"><div><h2>${t("文献统计")}</h2><p class="muted">${t("点击条目筛选文献库")}${screening.length ? ` · ${t("当前统计 {n} 篇", { n: counted.length })}` : ""}</p></div><div class="overview-stat-options">${screeningChoices}<label class="checkbox-choice"><input type="checkbox" id="overview-hide-preprint"${app.overviewHidePreprint ? " checked" : ""}><span>${t("不展示预印本")}${preprints ? ` (${preprints})` : ""}</span></label></div></div>`
    + `<div class="overview-bottom-grid">${yearPanel(counted)}`
    + distributionPanel(counted, "species", t("研究物种"), t("当前目录中的文献分布"), (v) => speciesLabelOf(papers, v))
    + distributionPanel(counted, "modality", t("研究技术"), t("按测序或成像技术统计"), tv)
    + distributionPanel(counted, "organ", t("研究组织"), t("按组织或器官统计"), tv)
    + distributionPanel(counted, "articleType", t("文献类型"), t("按原创研究、方法、综述等统计"), tv)
    + `</div>`;
}

/** tablePager renders the shared page-size select and page buttons for the papers/datasets table footers. */
function tablePager(scope: "papers" | "datasets", size: number, info: ReturnType<typeof paperPage>): string {
  const unit = scope === "papers" ? t("篇") : t("组");
  const pages = pageList(info.page, info.count).map((n) =>
    n === "gap" ? '<span class="pager-gap">…</span>'
      : `<button class="page-btn${n === info.page ? " active" : ""}" data-action="table-page" data-scope="${scope}" data-id="${n}"${n === info.page ? ' aria-current="page"' : ""} aria-label="${t("第 {n} 页", { n })}">${n}</button>`,
  ).join("");
  return `<div class="pager"><label class="page-size"><span>${t("每页")}</span><select class="filter-select" data-page-size="${scope}" aria-label="${t("每页显示条数")}">${PAGE_SIZES.map((s) => option(String(s), `${s} ${unit}`, size)).join("")}</select></label>`
    + `<nav class="pager-pages" aria-label="${scope === "papers" ? t("文献分页") : t("数据集分页")}"><button class="page-btn" data-action="table-page" data-scope="${scope}" data-id="${info.page - 1}"${info.page <= 1 ? " disabled" : ""} aria-label="${t("上一页")}">${I("chevron-left", 15)}</button>${pages}<button class="page-btn" data-action="table-page" data-scope="${scope}" data-id="${info.page + 1}"${info.page >= info.count ? " disabled" : ""} aria-label="${t("下一页")}">${I("chevron-right", 15)}</button></nav></div>`;
}

export function renderPapers(app: App): string {
  const { filters } = app;
  const papers = app.state.papers;
  const items = filterPapers(papers, filters);
  const concerns = papers.filter((p) => reviewConcern(p));
  const count = (kind: string) => concerns.filter((p) => reviewConcern(p) === kind).length;
  const fromSource = count("source");
  const excluded = count("excluded");
  const uncertain = count("uncertain");
  const concernSummary = [fromSource ? t("{n} 篇由来源更新新增", { n: fromSource }) : "", excluded ? t("{n} 篇建议排除", { n: excluded }) : "", uncertain ? t("{n} 篇建议核对", { n: uncertain }) : ""].filter(Boolean).join(locale === "en" ? "; " : "，");
  // Writing the clamped page back keeps state valid as filters or edits shrink the list.
  const info = paperPage(items.length, app.paperPageSize, app.paperPage);
  app.paperPage = info.page;
  const filterKeys = ["species", "organ", "year", "modality", "platform", "articleType", "fulltext", "screening", "source", "preprint"] as const;
  const filterCount = filterKeys.filter((k) => filters[k]).length + (filters.starred ? 1 : 0) + (filters.review ? 1 : 0);
  const entries = (map: Record<string, string>) => Object.entries(map).map(([value, label]) => ({ value, label }));
  const calibration = app.calibrationView;
  // The breakdown of what waits for review sits on the tab; the rows themselves carry the reason.
  const tabs = `<div class="view-tabs"><button class="view-tab ${!filters.starred && !filters.review && !calibration ? "active" : ""}" data-action="all-papers">${t("全部文献")} <span>${papers.length}</span></button><button class="view-tab review-tab ${filters.review ? "active" : ""}" data-action="review-papers"${concernSummary ? ` title="${esc(concernSummary)}"` : ""}>${I("alert", 15)} ${t("需核对")} <span>${concerns.length}</span></button><button class="view-tab ${filters.starred ? "active" : ""}" data-action="starred-papers">${I("star", 15)} ${t("我的收藏")} <span>${papers.filter((p) => p.starred).length}</span></button><button class="view-tab ${calibration ? "active" : ""}" data-action="calibration-open" title="${t("AI 建议与人工判定的一致率、混淆矩阵和不一致样本")}">${I("sparkle", 15)} ${t("AI 对照")}${app.calibration?.report ? ` <span>${app.calibration.report.compared}</span>` : ""}</button></div>`;
  if (calibration) return `<div class="section-toolbar">${tabs}</div>${renderCalibration(app)}`;
  return `<div class="section-toolbar">${tabs}<div class="column-toolbar"><button class="btn btn-primary" data-action="batch-open">${I("sparkle", 16)} <span id="batch-entry-label">${app.batch ? (app.batch.running ? t("AI 正在筛查文献") : t("批量筛选结果")) : t("批量 AI 筛选")}</span></button><button class="btn btn-secondary" data-action="start-screening" title="${t("以单篇文献为中心逐篇查看并做收录判断")}">${I("check-circle", 16)} ${t("逐篇筛选")}</button><button class="btn btn-secondary" data-action="paper-columns">${t("显示列")}</button><button class="btn btn-secondary" data-action="export-papers">${I("download", 16)} ${t("导出")}</button></div></div>`
    + `<section class="panel papers-panel"><div class="toolbar"><label class="table-search">${I("search", 18)}<input id="paper-search" class="search-input" type="search" placeholder="${t("搜索题目、物种、作者或 DOI…")}" value="${esc(filters.search)}" aria-label="${t("搜索文献")}"></label></div>`
    + `<div class="filter-row"><span class="filter-caption">${I("filter", 15)} ${t("筛选")}</span>${sortChip(filters.sort, SORT_OPTIONS)}`
    + selectFilter(filters, "species", t("物种"), uniqueValues(papers, "species").map((v) => ({ value: v, label: speciesLabelOf(papers, v) })))
    + selectFilter(filters, "organ", t("组织"), uniqueValues(papers, "organ").map((v) => ({ value: v, label: tv(v) })))
    + selectFilter(filters, "year", t("年份"), uniqueValues(papers, "year").reverse())
    + selectFilter(filters, "modality", t("技术"), uniqueValues(papers, "modality").map((v) => ({ value: v, label: tv(v) })))
    + selectFilter(filters, "platform", t("平台"), uniqueValues(papers, "platform"))
    + selectFilter(filters, "articleType", t("文献类型"), uniqueValues(papers, "articleType").map((v) => ({ value: v, label: tv(v) })))
    + selectFilter(filters, "fulltext", t("全文"), entries(FULL))
    + selectFilter(filters, "screening", t("收录"), entries(SCREEN))
    + selectFilter(filters, "source", t("来源"), uniqueValues(papers, "source").map((v) => ({ value: v, label: tv(v) })))
    + selectFilter(filters, "preprint", t("预印本"), [{ value: "yes", label: t("是") }, { value: "no", label: t("否") }])
    + (filterCount || filters.search ? `<button class="text-link filter-clear-all" data-action="clear-filters">${I("x", 13)} ${filterCount ? t("清空 {n} 项筛选", { n: filterCount }) : t("清空")}</button>` : "")
    + `</div>${paperTable(items.slice(info.start, info.end), app, false, filters.sort)}${bulkBar(app)}<div class="table-footer"><span>${t("显示 {range} / {total} 篇文献", { range: items.length ? `${info.start + 1}–${info.end}` : 0, total: items.length })}</span><span class="muted footer-hint">${t("↑↓ 切换文献 · ←→ 翻页 · Enter 查看详情")}</span>${items.length ? tablePager("papers", app.paperPageSize, info) : ""}</div></section>`;
}

/** datasetTable renders the datasets list: one row per dataset with its linked papers. */
export function datasetTable(items: Dataset[], app: App, none = empty(t("暂无匹配数据集"), t("从文献详情关联数据，或先创建一条数据集记录。"), `<button class="btn btn-secondary" data-action="clear-filters">${t("清空筛选")}</button>`)): string {
  if (!items.length) return none;
  const rows = items.map((d) => {
    const links = arr(d.paperLinks)
      .map((l) => ({ role: l.role, paper: app.paperById(l.paperId) }))
      .filter((l) => l.paper);
    const chips = links.slice(0, 2).map((l) =>
      `<button class="link-chip" data-action="open-paper" data-id="${esc(l.paper!.id)}" title="${esc(l.paper!.title)}"><span class="link-chip-title">${esc(l.paper!.title)}</span><span class="link-chip-role">${esc(ROLE[l.role] ?? t("关联"))}</span></button>`,
    ).join("");
    const repos = arr(d.repositories);
    const repoLine = (r: DatasetRepository): string => {
      const line = `<span>${esc(r.repository ? repositoryLabel(r.repository) : t("仓库待补"))}</span><code>${esc(r.accession || t("编号待补"))}</code>`;
      const url = repositoryURL(r);
      return url
        ? `<a class="repo-line" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="${t("在仓库中打开")}">${line}</a>`
        : `<div class="repo-line">${line}</div>`;
    };
    const review = d.awaitingReview
      ? `<div class="paper-review-note source"><span class="paper-review-label">${I("alert", 14)}${t("{label} · 待你确认", { label: t("来源新增") })}</span><span class="paper-review-hint">${t("查看详情，核对仓库记录")} ${I("arrow-right", 12)}</span></div>`
      : "";
    return `<tr${d.awaitingReview ? ' class="paper-needs-review source"' : ""} data-action="open-dataset" data-id="${esc(d.id)}" tabindex="0" aria-label="${d.awaitingReview ? t("{label}，待确认：{title}", { label: t("来源新增"), title: esc(d.title) }) : t("查看 {title}", { title: esc(d.title) })}">`
      // The modality badges repeat inside the title cell for narrow windows, where the column itself is hidden.
      + `<td class="dataset-main-cell col-title"><div class="dataset-title">${esc(d.title)}</div><div class="dataset-meta">${demoBadge(d)}<span>${esc(displayList(d.platforms) || t("平台待补"))}</span><span class="dataset-meta-modality">${modalityBadges(d)}</span></div>${review}</td>`
      + `<td class="link-cell col-links">${links.length ? `${chips}${links.length > 2 ? `<span class="link-more">+${links.length - 2}</span>` : ""}` : `<span class="muted">${t("未关联")}</span>`}</td>`
      + `<td class="col-species"><div class="species-label">${esc(speciesDisplay(d) || t("物种待补"))}</div><div class="species-name">${esc(displayList(d.organs) || t("组织待补"))}</div></td>`
      + `<td class="col-modality">${modalityBadges(d)}</td>`
      + `<td class="repo-cell col-repo">${repos.length ? `${repos.slice(0, 1).map(repoLine).join("")}${repos.length > 1 ? `<span class="link-more">+${repos.length - 1}</span>` : ""}` : `<span class="muted">${t("待补")}</span>`}</td>`
      + `<td class="col-status">${datasetStatusBadge(d)}</td><td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`;
  }).join("");
  const head = `<tr><th class="col-title">${t("数据集")}</th><th class="col-links">${t("关联文献")}</th><th class="col-species">${t("研究对象")}</th><th class="col-modality">${t("技术")}</th><th class="col-repo">${t("仓库编号")}</th><th class="col-status">${t("数据状态")}</th><th class="col-arrow"></th></tr>`;
  return `<div class="table-wrap"><table class="paper-table dataset-table"><thead>${head}</thead><tbody>${rows}</tbody></table></div>`;
}

export function renderDatasets(app: App): string {
  const datasets = app.state.datasets;
  const items = filterDatasets(datasets, app.datasetSearch, (id) => app.paperById(id)?.title ?? "", app.datasetReview);
  const awaiting = datasets.filter((d) => d.awaitingReview).length;
  const reviewDone = app.datasetReview && !awaiting
    ? empty(t("没有需核对的数据集"), t("来源更新新增的数据集都已核查。"), `<button class="btn btn-secondary" data-action="all-datasets">${t("查看全部数据集")}</button>`)
    : undefined;
  const toggle = `<div class="view-toggle" role="group" aria-label="${t("数据集视图切换")}"><button class="view-option${app.datasetView === "cards" ? " active" : ""}" data-action="datasets-view" data-view="cards" aria-pressed="${app.datasetView === "cards"}">${I("grid", 15)} ${t("卡片")}</button><button class="view-option${app.datasetView === "table" ? " active" : ""}" data-action="datasets-view" data-view="table" aria-pressed="${app.datasetView === "table"}">${I("more", 15)} ${t("列表")}</button></div>`;
  const tabs = `<div class="view-tabs"><button class="view-tab ${!app.datasetReview ? "active" : ""}" data-action="all-datasets">${t("全部数据集")} <span>${datasets.length}</span></button><button class="view-tab review-tab ${app.datasetReview ? "active" : ""}" data-action="review-datasets">${I("alert", 15)} ${t("需核对")} <span>${awaiting}</span></button></div>`;
  const head = `<div class="section-toolbar">${tabs}<div class="page-actions">${toggle}${app.env.readOnly ? "" : `<button class="btn btn-primary" data-action="start-dataset-review" title="${t("逐组查看数据集，保留或直接删除")}"${items.length ? "" : " disabled"}>${I("check-circle", 16)} ${t("逐个核查")}</button>`}<button class="btn btn-secondary" data-action="export-datasets">${I("download", 16)} ${t("导出")}</button><button class="btn btn-secondary" data-action="new-dataset">${I("plus", 16)} ${t("添加数据集")}</button></div></div>`
    + `<div class="toolbar standalone-toolbar"><label class="table-search">${I("search", 18)}<input id="dataset-search" class="search-input" type="search" placeholder="${t("搜索物种、组织、accession 或关联文献…")}" value="${esc(app.datasetSearch)}" aria-label="${t("搜索数据集")}"></label></div>`;
  if (app.datasetView === "cards") {
    const cards = items.map((d) =>
      `<button class="dataset-card" data-action="open-dataset" data-id="${esc(d.id)}"><div class="dataset-top"><span class="icon-wrap green">${I("database", 20)}</span><span class="dataset-badges">${d.awaitingReview ? badge(t("待核查"), "amber") : ""}${datasetStatusBadge(d)}</span></div><h3>${esc(d.title)}</h3><p class="species-name">${esc(speciesDisplay(d) || t("物种待补"))}</p><div class="tag-list">${badge(displayList(d.organs) || t("组织待补"))}${badge(displayList(d.modalities) || t("技术待补"), "blue")}${demoBadge(d)}</div><div class="dataset-accession"><span>${esc(d.repository ? repositoryLabel(d.repository) : t("仓库待补"))}</span><code>${esc(d.accession || t("编号待补"))}</code></div><div class="dataset-footer"><span>${I("book", 15)} ${t("{n} 篇关联文献", { n: arr(d.paperLinks).length })}</span>${I("arrow-up-right", 17)}</div></button>`,
    ).join("");
    return head + (items.length ? `<div class="dataset-grid">${cards}</div>` : reviewDone ?? empty(t("暂无匹配数据集"), t("从文献详情关联数据，或先创建一条数据集记录。")));
  }
  // Writing the clamped page back keeps state valid as the search narrows the list.
  const info = paperPage(items.length, app.datasetPageSize, app.datasetPage);
  app.datasetPage = info.page;
  return head
    + `<section class="panel datasets-panel">${datasetTable(items.slice(info.start, info.end), app, reviewDone)}<div class="table-footer"><span>${t("显示 {range} / {total} 组数据集", { range: items.length ? `${info.start + 1}–${info.end}` : 0, total: items.length })}</span><span class="muted footer-hint">${t("↑↓ 切换数据集 · ←→ 翻页 · 点击文献条目可跳转")}</span>${items.length ? tablePager("datasets", app.datasetPageSize, info) : ""}</div></section>`;
}

export function renderTasks(app: App): string {
  const all = app.tasks();
  const items = app.taskFilter === "all" ? all : all.filter((t) => t.type === app.taskFilter);
  const tabs: [TaskType | "all", string][] = [["all", t("全部待补")], ["screen", t("收录待判断")], ["fulltext", t("全文待补")], ["data", t("数据待核实")]];
  const tabButtons = tabs.map(([key, label]) =>
    `<button class="view-tab ${app.taskFilter === key ? "active" : ""}" data-action="task-filter" data-filter-value="${key}">${label} <span>${key === "all" ? all.length : all.filter((t) => t.type === key).length}</span></button>`,
  ).join("");
  return `<div class="notice-box">${I("info", 18)}<span>${t("待补事项根据文献与数据状态自动生成。完成对应整理后，事项会自动移出列表。")}</span></div><div class="section-toolbar"><div class="view-tabs">${tabButtons}</div></div><section class="panel"><div class="task-list full-task-list">${items.length ? taskRows(items) : empty(t("这部分已整理完成"), t("新的文献或数据线索进入后，需要补充的事项会出现在这里。"))}</div></section>`;
}
