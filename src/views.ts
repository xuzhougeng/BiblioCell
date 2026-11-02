// Page renderers for overview, papers, datasets and tasks.
import { AGENT_EXAMPLES } from "./agent.ts";
import { PAPER_COLUMNS, type PaperColumn } from "./columns.ts";
import type { App } from "./app.ts";
import type { Dataset, DatasetRepository, Paper } from "./mygo.ts";
import {
  FULL, PAGE_SIZES, ROLE, SORT_OPTIONS, SCREEN, arr, datasetLocation, esc, filterDatasets, filterPapers, joinList, pageList, paperPage, repositoryURL, safeURL, sortParts, speciesDisplay, uniqueValues, speciesLabelOf,
  reviewConcern, type SortKey, type TaskRow, type TaskType,
} from "./model.ts";
import { I, badge, datasetStatusBadge, demoBadge, empty, fullBadge, modalityBadges, option, progressRow, screenBadge, selectFilter } from "./ui.ts";
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
    const label = d.accession || (d.repository ? d.repository.toUpperCase() : "") || d.title;
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
    const warning = concern ? `<div class="paper-review-note ${concern}"><span class="paper-review-label">${I("alert", 14)}${t("{label} · 待你确认", { label: concernLabel })}</span><span class="paper-review-reason" title="${esc(concernReason)}">${esc(concernReason)}</span><span class="paper-review-hint">${concernHint} ${I("arrow-right", 12)}</span></div>` : "";
    const cells: Record<PaperColumn, string> = {
      star: `<td><button class="btn-icon star-button${p.starred ? " is-starred" : ""}" data-action="toggle-star" data-id="${esc(p.id)}" aria-label="${p.starred ? t("取消收藏") : t("加入收藏")}" aria-pressed="${Boolean(p.starred)}">${I("star", 16)}</button></td>`,
      title: `<td class="paper-main-cell"><div class="paper-title">${esc(p.title)}</div><div class="paper-meta">${demoBadge(p)}<span>${esc(p.authors || t("作者待补"))}</span><span class="meta-separator">·</span><span>${esc(p.year || t("年份待补"))}</span>${p.publicationStatus === "preprint" ? `<span class="preprint-label">${t("预印本")}</span>` : ""}</div>${warning}</td>`,
      journal: `<td class="journal-cell">${p.journal ? esc(p.journal) : `<span class="muted">${t("期刊待补")}</span>`}</td>`,
      year: `<td class="year-cell">${p.year ? esc(String(p.year)) : `<span class="muted">${t("年份待补")}</span>`}</td>`,
      species: `<td><div class="species-label">${esc(speciesDisplay(p) || t("物种待补"))}</div><div class="species-name">${esc(displayList(p.organs) || t("组织待补"))}</div></td>`,
      datasets: `<td class="link-cell">${datasetChips(app, p)}</td>`,
      modality: `<td>${modalityBadges(p)}</td>`,
      fulltext: `<td>${fulltextCell(app, p)}</td>`,
      screening: `<td>${screenBadge(p.screening)}${concern ? `<div class="screening-proposal ${concern}">${I("alert", 13)}${concernLabel}<small>${t("尚未确认")}</small></div>` : ""}</td>`,
    };
    return `<tr${concern ? ` class="paper-needs-review ${concern}"` : ""} data-action="open-paper" data-id="${esc(p.id)}" tabindex="0" aria-label="${concern ? t("{label}，待确认：{title}", { label: concernLabel, title: esc(p.title) }) : t("查看 {title}", { title: esc(p.title) })}">${columns.map(id => cells[id]).join("")}<td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`;
  }).join("");
  const head = `<tr>${columns.map(id => id === "star" ? `<th class="col-star"><span class="sr-only">${t("收藏")}</span></th>` : th(labels[id], sortKeys[id])).join("")}<th class="col-arrow"></th></tr>`;

  return `<div class="table-wrap"><table class="paper-table${compact ? " compact-table" : ""}"><thead>${head}</thead><tbody>${rows}</tbody></table></div>`;
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
  const counts = new Map<string, number>();
  for (const p of papers) {
    for (const s of arr(p.species)) {
      const label = s?.label || s?.name;
      if (label) counts.set(label, (counts.get(label) ?? 0) + 1);
    }
  }
  const species = [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
  const maxCount = Math.max(1, ...species.map((s) => s[1]));
  const stats: [string, number, string, string, string][] = [
    [t("文献条目"), total, t("已纳入 {n} 篇", { n: papers.filter((p) => p.screening === "included").length }), "book", "papers"],
    [t("关联数据集"), app.state.datasets.length, t("支持原始、复用与重新处理"), "database", "datasets"],
    [t("全文待补"), missing, t("已有入口，也可继续补正文"), "file", "fulltext"],
    [t("收录待判断"), pending, t("保留不确定项，减少遗漏"), "check", "screen"],
  ];
  const statCards = stats.map((s, i) =>
    `<button class="stat-card" data-action="stat-nav" data-target="${s[4]}"><div class="stat-top"><span class="stat-label">${s[0]}</span><span class="stat-icon">${I(s[3], 18)}</span></div><div class="stat-value">${s[1]}<span class="stat-unit">${i === 1 ? t("组") : t("篇")}</span></div><div class="stat-meta">${s[2]}${I("arrow-right", 14)}</div></button>`,
  ).join("");
  const speciesBars = species.map(([label, count]) =>
    `<button class="species-bar-row" data-action="species-filter" data-label="${esc(label)}"><span>${esc(label)}</span><div class="species-bar-track"><div style="width:${Math.round((count / maxCount) * 100)}%"></div></div><strong>${count}</strong></button>`,
  ).join("");
  const agentEntry = `<section class="panel agent-entry"><span class="agent-entry-icon">${I("sparkle", 22)}</span><div class="agent-entry-body"><h2>${t("AI 助手")}</h2><p class="muted">${t("用自己的模型，通过 MCP 工具检索、导入、补元数据、提收录建议。")}</p><div class="agent-entry-prompts">${AGENT_EXAMPLES.map((p) => `<button type="button" class="agent-entry-prompt" data-action="agent-open" data-prompt="${esc(p)}">${esc(p)}</button>`).join("")}</div></div><button class="btn btn-primary" data-action="agent-open">${I("sparkle", 15)} ${t("打开助手")}</button></section>`;
  return `<div class="stat-grid">${statCards}</div>${agentEntry}`
    + `<div class="overview-grid"><section class="panel recent-panel"><div class="panel-header"><div><h2>${t("最近收录")}</h2><p class="muted">${t("新线索先留下，整理慢慢来")}</p></div><button class="text-link" data-page="papers">${t("查看全部")} ${I("arrow-right", 16)}</button></div>${paperTable(recent, app, true)}<div class="panel-footnote">${I("info", 14)} ${t("当前条目为原型示例，可通过新增或导入建立自己的目录。")}</div></section>`
    + `<section class="panel tasks-panel"><div class="panel-header"><div><h2>${t("接下来整理")}</h2><p class="muted">${t("按需要补齐信息")}</p></div><span class="count-pill">${ts.length}</span></div><div class="task-groups">${taskGroups(ts)}</div><button class="panel-bottom-link" data-page="tasks">${t("查看全部待补事项")} ${I("arrow-right", 16)}</button></section></div>`
    + `<div class="overview-bottom-grid"><section class="panel"><div class="panel-header"><div><h2>${t("整理覆盖")}</h2><p class="muted">${t("收录与完善，各自向前一步")}</p></div>${I("grid", 18)}</div><div class="panel-body progress-list">${progressRow(t("收录范围已判断"), total - pending, total)}${progressRow(t("已关联数据集"), withData, total, "blue")}${progressRow(t("已记录全文入口"), withFulltext, total, "gold")}</div></section>`
    + `<section class="panel"><div class="panel-header"><div><h2>${t("研究物种")}</h2><p class="muted">${t("当前目录中的文献分布")}</p></div><span class="muted">${t("{n} 个分类", { n: uniqueValues(papers, "species").length })}</span></div><div class="panel-body species-bars">${speciesBars}</div></section></div>`;
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
  const filterKeys = ["species", "organ", "year", "modality", "platform", "fulltext", "screening", "source", "preprint"] as const;
  const filterCount = filterKeys.filter((k) => filters[k]).length + (filters.starred ? 1 : 0) + (filters.review ? 1 : 0);
  const entries = (map: Record<string, string>) => Object.entries(map).map(([value, label]) => ({ value, label }));
  return `<div class="section-toolbar"><div class="view-tabs"><button class="view-tab ${!filters.starred && !filters.review ? "active" : ""}" data-action="all-papers">${t("全部文献")} <span>${papers.length}</span></button><button class="view-tab review-tab ${filters.review ? "active" : ""}" data-action="review-papers">${I("alert", 15)} ${t("需核对")} <span>${concerns.length}</span></button><button class="view-tab ${filters.starred ? "active" : ""}" data-action="starred-papers">${I("star", 15)} ${t("我的收藏")} <span>${papers.filter((p) => p.starred).length}</span></button></div><div class="column-toolbar"><button class="btn btn-secondary" data-action="paper-columns">${t("显示列")}</button><button class="btn btn-secondary" data-action="export-csv">${I("download", 16)} ${t("导出列表")}</button></div></div>`
    + (concerns.length ? `<div class="review-summary">${I("alert", 20)}<div><strong>${t("有 {n} 篇文献的收录范围需要核对", { n: concerns.length })}</strong><p>${t("{summary}。确认前仍计入全部文献；打开详情可做出收录判断、采纳或忽略建议，或标记核查完成。", { summary: concernSummary })}</p></div><button class="btn btn-secondary" data-action="review-papers">${t("查看待核对")} ${I("arrow-right", 15)}</button></div>` : "")
    + `<section class="panel papers-panel"><div class="toolbar"><label class="table-search">${I("search", 18)}<input id="paper-search" class="search-input" type="search" placeholder="${t("搜索题目、物种、作者或 DOI…")}" value="${esc(filters.search)}" aria-label="${t("搜索文献")}"></label><select class="filter-select" data-filter="sort" aria-label="${t("排序方式")}">${SORT_OPTIONS.map(([value, label]) => option(value, label, filters.sort)).join("")}</select></div>`
    + `<div class="filter-row"><span class="filter-caption">${I("filter", 15)} ${t("筛选")}</span>`
    + selectFilter(filters, "species", t("全部物种"), uniqueValues(papers, "species").map((v) => ({ value: v, label: speciesLabelOf(papers, v) })))
    + selectFilter(filters, "organ", t("全部组织"), uniqueValues(papers, "organ"))
    + selectFilter(filters, "year", t("全部年份"), uniqueValues(papers, "year").reverse())
    + selectFilter(filters, "modality", t("全部技术"), uniqueValues(papers, "modality").map((v) => ({ value: v, label: tv(v) })))
    + selectFilter(filters, "platform", t("全部平台"), uniqueValues(papers, "platform"))
    + selectFilter(filters, "fulltext", t("全文情况"), entries(FULL))
    + selectFilter(filters, "screening", t("收录状态"), entries(SCREEN))
    + selectFilter(filters, "source", t("全部来源"), uniqueValues(papers, "source").map((v) => ({ value: v, label: tv(v) })))
    + selectFilter(filters, "preprint", t("是否为 preprint（全部）"), [{ value: "yes", label: t("是（预印本）") }, { value: "no", label: t("否（未标记为预印本）") }])
    + (filterCount || filters.search ? `<button class="text-link" data-action="clear-filters">${t("清空")}${filterCount ? ` (${filterCount})` : ""}</button>` : "")
    + `</div>${paperTable(items.slice(info.start, info.end), app, false, filters.sort)}<div class="table-footer"><span>${t("显示 {range} / {total} 篇文献", { range: items.length ? `${info.start + 1}–${info.end}` : 0, total: items.length })}</span><span class="muted footer-hint">${t("↑↓ 切换文献 · ←→ 翻页 · Enter 查看详情")}</span>${items.length ? tablePager("papers", app.paperPageSize, info) : ""}</div></section>`;
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
      const line = `<span>${esc(r.repository ? r.repository.toUpperCase() : t("仓库待补"))}</span><code>${esc(r.accession || t("编号待补"))}</code>`;
      const url = repositoryURL(r);
      return url
        ? `<a class="repo-line" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="${t("在仓库中打开")}">${line}</a>`
        : `<div class="repo-line">${line}</div>`;
    };
    const review = d.awaitingReview
      ? `<div class="paper-review-note source"><span class="paper-review-label">${I("alert", 14)}${t("{label} · 待你确认", { label: t("来源新增") })}</span><span class="paper-review-hint">${t("查看详情，核对仓库记录")} ${I("arrow-right", 12)}</span></div>`
      : "";
    return `<tr${d.awaitingReview ? ' class="paper-needs-review source"' : ""} data-action="open-dataset" data-id="${esc(d.id)}" tabindex="0" aria-label="${d.awaitingReview ? t("{label}，待确认：{title}", { label: t("来源新增"), title: esc(d.title) }) : t("查看 {title}", { title: esc(d.title) })}">`
      + `<td class="dataset-main-cell"><div class="dataset-title">${esc(d.title)}</div><div class="dataset-meta">${demoBadge(d)}<span>${esc(displayList(d.platforms) || t("平台待补"))}</span></div>${review}</td>`
      + `<td class="link-cell">${links.length ? `${chips}${links.length > 2 ? `<span class="link-more">+${links.length - 2}</span>` : ""}` : `<span class="muted">${t("未关联")}</span>`}</td>`
      + `<td><div class="species-label">${esc(speciesDisplay(d) || t("物种待补"))}</div><div class="species-name">${esc(displayList(d.organs) || t("组织待补"))}</div></td>`
      + `<td>${modalityBadges(d)}</td>`
      + `<td class="repo-cell">${repos.length ? `${repos.slice(0, 1).map(repoLine).join("")}${repos.length > 1 ? `<span class="link-more">+${repos.length - 1}</span>` : ""}` : `<span class="muted">${t("待补")}</span>`}</td>`
      + `<td>${datasetStatusBadge(d)}</td><td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`;
  }).join("");
  const head = `<tr><th>${t("数据集")}</th><th>${t("关联文献")}</th><th>${t("研究对象")}</th><th>${t("技术")}</th><th>${t("仓库编号")}</th><th>${t("数据状态")}</th><th class="col-arrow"></th></tr>`;
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
  const head = `<div class="section-toolbar">${tabs}<div class="page-actions">${toggle}<button class="btn btn-secondary" data-action="new-dataset">${I("plus", 16)} ${t("添加数据集")}</button></div></div>`
    + (awaiting && !app.datasetReview ? `<div class="review-summary">${I("alert", 20)}<div><strong>${t("有 {n} 组数据集需要核对", { n: awaiting })}</strong><p>${t("由来源更新从数据仓库新增，尚未人工核查。打开详情核对仓库记录、关联文献，修改整理状态或标记核查完成后移出。")}</p></div><button class="btn btn-secondary" data-action="review-datasets">${t("查看待核对")} ${I("arrow-right", 15)}</button></div>` : "")
    + `<div class="toolbar standalone-toolbar"><label class="table-search">${I("search", 18)}<input id="dataset-search" class="search-input" type="search" placeholder="${t("搜索物种、组织、accession 或关联文献…")}" value="${esc(app.datasetSearch)}" aria-label="${t("搜索数据集")}"></label></div>`;
  if (app.datasetView === "cards") {
    const cards = items.map((d) =>
      `<button class="dataset-card" data-action="open-dataset" data-id="${esc(d.id)}"><div class="dataset-top"><span class="icon-wrap green">${I("database", 20)}</span><span class="dataset-badges">${d.awaitingReview ? badge(t("待核查"), "amber") : ""}${datasetStatusBadge(d)}</span></div><h3>${esc(d.title)}</h3><p class="species-name">${esc(speciesDisplay(d) || t("物种待补"))}</p><div class="tag-list">${badge(displayList(d.organs) || t("组织待补"))}${badge(displayList(d.modalities) || t("技术待补"), "blue")}${demoBadge(d)}</div><div class="dataset-accession"><span>${esc(d.repository ? d.repository.toUpperCase() : t("仓库待补"))}</span><code>${esc(d.accession || t("编号待补"))}</code></div><div class="dataset-footer"><span>${I("book", 15)} ${t("{n} 篇关联文献", { n: arr(d.paperLinks).length })}</span>${I("arrow-up-right", 17)}</div></button>`,
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
