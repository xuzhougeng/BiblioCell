// Page renderers for overview, papers, datasets, tasks and schema.
import type { App } from "./app.ts";
import type { Dataset, DatasetRepository, Paper } from "./mygo.ts";
import {
  FULL, PAGE_SIZES, ROLE, SORT_OPTIONS, SCREEN, arr, datasetLocation, esc, filterDatasets, filterPapers, joinList, pageList, paperPage, repositoryURL, safeURL, sortParts, speciesDisplay, uniqueValues, speciesLabelOf,
  type SortKey, type TaskRow, type TaskType,
} from "./model.ts";
import { I, badge, datasetStatusBadge, demoBadge, empty, fullBadge, modalityBadges, option, progressRow, screenBadge, selectFilter } from "./ui.ts";

export function taskRows(items: TaskRow[]): string {
  return items.map((t) =>
    `<button class="task-item" data-action="open-paper" data-id="${esc(t.paper.id)}"><span class="icon-wrap ${t.color}">${I(t.icon, 18)}</span><span class="task-content"><span class="task-title">${esc(t.title)}</span><span class="task-paper">${esc(t.paper.title)}</span><span class="task-subtitle muted">${esc(t.subtitle)}</span></span>${I("chevron-right", 16)}</button>`,
  ).join("");
}

const TASK_GROUPS: [TaskType, string][] = [["screen", "确认收录范围"], ["fulltext", "待补全文"], ["data", "待核实数据关联"]];
const TASK_GROUP_LIMIT = 3;

/** taskGroups lists upcoming tasks grouped by type: the task name once, then only paper titles. */
export function taskGroups(ts: TaskRow[]): string {
  const groups = TASK_GROUPS.map(([type, label]) => {
    const items = ts.filter((t) => t.type === type);
    if (!items.length) return "";
    const rows = items.slice(0, TASK_GROUP_LIMIT).map((t) =>
      `<button class="task-row" data-action="open-paper" data-id="${esc(t.paper.id)}"><span>${esc(t.paper.title)}</span>${I("chevron-right", 14)}</button>`,
    ).join("");
    const more = items.length > TASK_GROUP_LIMIT ? `<span class="task-more muted">还有 ${items.length - TASK_GROUP_LIMIT} 篇</span>` : "";
    return `<div class="task-group"><div class="task-group-title"><span>${label}</span><span class="count-pill">${items.length}</span></div>${rows}${more}</div>`;
  }).join("");
  return groups || '<p class="muted task-empty">暂无待补事项。</p>';
}

/** datasetChips shows a paper's linked datasets; a chip opens the dataset's web location, or its drawer when none is known. */
function datasetChips(app: App, p: Paper): string {
  const linked = app.paperDatasets(p.id);
  if (!linked.length) return '<span class="muted">未关联</span>';
  const chips = linked.slice(0, 2).map((d) => {
    const label = d.accession || (d.repository ? d.repository.toUpperCase() : "") || d.title;
    const url = datasetLocation(d);
    return url
      ? `<a class="link-chip" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="${esc(d.title)} · 在仓库中打开"><span class="link-chip-title">${esc(label)}</span>${I("arrow-up-right", 12)}</a>`
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
  if (p.fulltext === "stored") {
    const file = app.paperResources(p.id).find((r) => r.localPath && r.type === "fulltext");
    if (file) return `<button class="fulltext-status is-link" data-action="open-file" data-id="${esc(file.id)}" title="打开已保存的全文文件">${fullBadge(p.fulltext)}${I("file", 12)}</button>`;
    return `<span class="fulltext-status">${fullBadge(p.fulltext)}</span>`;
  }
  const url = p.fulltext === "linked" ? safeURL(p.fulltextUrl) || fulltextEntry(app, p) : "";
  return url
    ? `<a class="fulltext-status is-link" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="打开全文入口">${fullBadge(p.fulltext)}${I("arrow-up-right", 12)}</a>`
    : `<span class="fulltext-status">${fullBadge(p.fulltext)}</span>`;
}

export function paperTable(items: Paper[], app: App, compact = false, sort = ""): string {
  if (!items.length) return empty("没有找到匹配文献", "尝试调整筛选条件，或添加一篇新文献。", '<button class="btn btn-secondary" data-action="clear-filters">清空筛选</button>');
  const active = compact ? null : sortParts(sort);
  const th = (label: string, key?: SortKey): string => {
    if (compact || !key) return `<th>${label}</th>`;
    const on = active!.key === key;
    const dir = on ? active!.dir : "";
    return `<th${on ? ` aria-sort="${dir === "asc" ? "ascending" : "descending"}"` : ""}><button class="th-sort${on ? " on" : ""}" data-action="sort-column" data-key="${key}" aria-label="按${label}排序">${label}<span class="sort-mark">${on ? I(dir === "asc" ? "chevron-up" : "chevron-down", 12) : ""}</span></button></th>`;
  };
  const rows = items.map((p) =>
    `<tr data-action="open-paper" data-id="${esc(p.id)}" tabindex="0" aria-label="查看 ${esc(p.title)}">`
    + `<td><button class="btn-icon star-button${p.starred ? " is-starred" : ""}" data-action="toggle-star" data-id="${esc(p.id)}" aria-label="${p.starred ? "取消收藏" : "收藏"}" aria-pressed="${Boolean(p.starred)}">${I("star", 16)}</button></td>`
    + `<td class="paper-main-cell"><div class="paper-title">${esc(p.title)}</div><div class="paper-meta">${demoBadge(p)}<span>${esc(p.authors || "作者待补")}</span><span class="meta-separator">·</span><span>${esc(p.year || "年份待补")}</span>${p.publicationStatus === "preprint" ? '<span class="preprint-label">预印本</span>' : ""}</div></td>`
    + (compact ? "" : `<td class="year-cell">${p.year ? esc(String(p.year)) : '<span class="muted">年份待补</span>'}</td>`)
    + `<td><div class="species-label">${esc(speciesDisplay(p) || "物种待补")}</div><div class="species-name">${esc(joinList(p.organs) || "组织待补")}</div></td>`
    + (compact ? "" : `<td class="link-cell">${datasetChips(app, p)}</td>`)
    + `<td>${modalityBadges(p)}</td>`
    + (compact ? "" : `<td>${fulltextCell(app, p)}</td>`)
    + `<td>${screenBadge(p.screening)}</td><td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`,
  ).join("");
  const head = `<tr><th class="col-star"><span class="sr-only">收藏</span></th>${th("文献", "title")}${compact ? "" : th("发表时间", "year")}${th("研究对象", "species")}${compact ? "" : "<th>关联数据集</th>"}${th("技术", "modality")}${compact ? "" : "<th>全文</th>"}<th>收录状态</th><th class="col-arrow"></th></tr>`;
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
    ["文献条目", total, `已纳入 ${papers.filter((p) => p.screening === "included").length} 篇`, "book", "papers"],
    ["关联数据集", app.state.datasets.length, "支持原始、复用与重新处理", "database", "datasets"],
    ["全文待补", missing, "已有入口，也可继续补正文", "file", "fulltext"],
    ["收录待判断", pending, "保留不确定项，减少遗漏", "check", "screen"],
  ];
  const statCards = stats.map((s, i) =>
    `<button class="stat-card" data-action="stat-nav" data-target="${s[4]}"><div class="stat-top"><span class="stat-label">${s[0]}</span><span class="stat-icon">${I(s[3], 18)}</span></div><div class="stat-value">${s[1]}<span class="stat-unit">${i === 1 ? "组" : "篇"}</span></div><div class="stat-meta">${s[2]}${I("arrow-right", 14)}</div></button>`,
  ).join("");
  const speciesBars = species.map(([label, count]) =>
    `<button class="species-bar-row" data-action="species-filter" data-label="${esc(label)}"><span>${esc(label)}</span><div class="species-bar-track"><div style="width:${Math.round((count / maxCount) * 100)}%"></div></div><strong>${count}</strong></button>`,
  ).join("");
  return `<div class="stat-grid">${statCards}</div>`
    + `<div class="overview-grid"><section class="panel recent-panel"><div class="panel-header"><div><h2>最近收录</h2><p class="muted">新线索先留下，整理慢慢来</p></div><button class="text-link" data-page="papers">查看全部 ${I("arrow-right", 16)}</button></div>${paperTable(recent, app, true)}<div class="panel-footnote">${I("info", 14)} 当前条目为原型示例，可通过新增或导入建立自己的目录。</div></section>`
    + `<section class="panel tasks-panel"><div class="panel-header"><div><h2>接下来整理</h2><p class="muted">按需要补齐信息</p></div><span class="count-pill">${ts.length}</span></div><div class="task-groups">${taskGroups(ts)}</div><button class="panel-bottom-link" data-page="tasks">查看全部待补事项 ${I("arrow-right", 16)}</button></section></div>`
    + `<div class="overview-bottom-grid"><section class="panel"><div class="panel-header"><div><h2>整理覆盖</h2><p class="muted">收录与完善，各自向前一步</p></div>${I("grid", 18)}</div><div class="panel-body progress-list">${progressRow("收录范围已判断", total - pending, total)}${progressRow("已关联数据集", withData, total, "blue")}${progressRow("已记录全文入口", withFulltext, total, "gold")}</div></section>`
    + `<section class="panel"><div class="panel-header"><div><h2>研究物种</h2><p class="muted">当前目录中的文献分布</p></div><span class="muted">${uniqueValues(papers, "species").length} 个分类</span></div><div class="panel-body species-bars">${speciesBars}</div></section></div>`;
}

/** tablePager renders the shared page-size select and page buttons for the papers/datasets table footers. */
function tablePager(scope: "papers" | "datasets", size: number, info: ReturnType<typeof paperPage>): string {
  const unit = scope === "papers" ? "篇" : "组";
  const pages = pageList(info.page, info.count).map((n) =>
    n === "gap" ? '<span class="pager-gap">…</span>'
      : `<button class="page-btn${n === info.page ? " active" : ""}" data-action="table-page" data-scope="${scope}" data-id="${n}"${n === info.page ? ' aria-current="page"' : ""} aria-label="第 ${n} 页">${n}</button>`,
  ).join("");
  return `<div class="pager"><label class="page-size"><span>每页</span><select class="filter-select" data-page-size="${scope}" aria-label="每页显示条数">${PAGE_SIZES.map((s) => option(String(s), `${s} ${unit}`, size)).join("")}</select></label>`
    + `<nav class="pager-pages" aria-label="${scope === "papers" ? "文献" : "数据集"}分页"><button class="page-btn" data-action="table-page" data-scope="${scope}" data-id="${info.page - 1}"${info.page <= 1 ? " disabled" : ""} aria-label="上一页">${I("chevron-left", 15)}</button>${pages}<button class="page-btn" data-action="table-page" data-scope="${scope}" data-id="${info.page + 1}"${info.page >= info.count ? " disabled" : ""} aria-label="下一页">${I("chevron-right", 15)}</button></nav></div>`;
}

export function renderPapers(app: App): string {
  const { filters } = app;
  const papers = app.state.papers;
  const items = filterPapers(papers, filters);
  // Writing the clamped page back keeps state valid as filters or edits shrink the list.
  const info = paperPage(items.length, app.paperPageSize, app.paperPage);
  app.paperPage = info.page;
  const filterKeys = ["species", "organ", "year", "modality", "platform", "fulltext", "screening", "source"] as const;
  const filterCount = filterKeys.filter((k) => filters[k]).length + (filters.starred ? 1 : 0);
  const entries = (map: Record<string, string>) => Object.entries(map).map(([value, label]) => ({ value, label }));
  return `<div class="section-toolbar"><div class="view-tabs"><button class="view-tab ${filters.starred ? "" : "active"}" data-action="all-papers">全部文献 <span>${papers.length}</span></button><button class="view-tab ${filters.starred ? "active" : ""}" data-action="starred-papers">${I("star", 15)} 我的收藏 <span>${papers.filter((p) => p.starred).length}</span></button></div><button class="btn btn-secondary" data-action="export-csv">${I("download", 16)} 导出列表</button></div>`
    + `<section class="panel papers-panel"><div class="toolbar"><label class="table-search">${I("search", 18)}<input id="paper-search" class="search-input" type="search" placeholder="搜索题目、物种、作者或 DOI…" value="${esc(filters.search)}" aria-label="搜索文献"></label><select class="filter-select" data-filter="sort" aria-label="排序方式">${SORT_OPTIONS.map(([value, label]) => option(value, label, filters.sort)).join("")}</select></div>`
    + `<div class="filter-row"><span class="filter-caption">${I("filter", 15)} 筛选</span>`
    + selectFilter(filters, "species", "全部物种", uniqueValues(papers, "species").map((v) => ({ value: v, label: speciesLabelOf(papers, v) })))
    + selectFilter(filters, "organ", "全部组织", uniqueValues(papers, "organ"))
    + selectFilter(filters, "year", "全部年份", uniqueValues(papers, "year").reverse())
    + selectFilter(filters, "modality", "全部技术", uniqueValues(papers, "modality"))
    + selectFilter(filters, "platform", "全部平台", uniqueValues(papers, "platform"))
    + selectFilter(filters, "fulltext", "全文情况", entries(FULL))
    + selectFilter(filters, "screening", "收录状态", entries(SCREEN))
    + selectFilter(filters, "source", "全部来源", uniqueValues(papers, "source"))
    + (filterCount || filters.search ? `<button class="text-link" data-action="clear-filters">清空${filterCount ? ` (${filterCount})` : ""}</button>` : "")
    + `</div>${paperTable(items.slice(info.start, info.end), app, false, filters.sort)}<div class="table-footer"><span>显示 ${items.length ? `${info.start + 1}–${info.end}` : 0} / ${items.length} 篇文献</span><span class="muted footer-hint">↑↓ 切换文献 · ←→ 翻页 · Enter 查看详情</span>${items.length ? tablePager("papers", app.paperPageSize, info) : ""}</div></section>`;
}

/** datasetTable renders the datasets list: one row per dataset with its linked papers. */
export function datasetTable(items: Dataset[], app: App): string {
  if (!items.length) return empty("暂无匹配数据集", "从文献详情关联数据，或先创建一条数据集记录。", '<button class="btn btn-secondary" data-action="clear-filters">清空筛选</button>');
  const rows = items.map((d) => {
    const links = arr(d.paperLinks)
      .map((l) => ({ role: l.role, paper: app.paperById(l.paperId) }))
      .filter((l) => l.paper);
    const chips = links.slice(0, 2).map((l) =>
      `<button class="link-chip" data-action="open-paper" data-id="${esc(l.paper!.id)}" title="${esc(l.paper!.title)}"><span class="link-chip-title">${esc(l.paper!.title)}</span><span class="link-chip-role">${esc(ROLE[l.role] ?? "关联")}</span></button>`,
    ).join("");
    const repos = arr(d.repositories);
    const repoLine = (r: DatasetRepository): string => {
      const line = `<span>${esc((r.repository || "仓库待补").toUpperCase())}</span><code>${esc(r.accession || "编号待补")}</code>`;
      const url = repositoryURL(r);
      return url
        ? `<a class="repo-line" href="${esc(url)}" target="_blank" rel="noopener noreferrer" title="在仓库中打开">${line}</a>`
        : `<div class="repo-line">${line}</div>`;
    };
    return `<tr data-action="open-dataset" data-id="${esc(d.id)}" tabindex="0" aria-label="查看 ${esc(d.title)}">`
      + `<td class="dataset-main-cell"><div class="dataset-title">${esc(d.title)}</div><div class="dataset-meta">${demoBadge(d)}<span>${esc(joinList(d.platforms) || "平台待补")}</span></div></td>`
      + `<td class="link-cell">${links.length ? `${chips}${links.length > 2 ? `<span class="link-more">+${links.length - 2}</span>` : ""}` : '<span class="muted">未关联</span>'}</td>`
      + `<td><div class="species-label">${esc(speciesDisplay(d) || "物种待补")}</div><div class="species-name">${esc(joinList(d.organs) || "组织待补")}</div></td>`
      + `<td>${modalityBadges(d)}</td>`
      + `<td class="repo-cell">${repos.length ? `${repos.slice(0, 1).map(repoLine).join("")}${repos.length > 1 ? `<span class="link-more">+${repos.length - 1}</span>` : ""}` : '<span class="muted">待补</span>'}</td>`
      + `<td>${datasetStatusBadge(d)}</td><td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`;
  }).join("");
  const head = `<tr><th>数据集</th><th>关联文献</th><th>研究对象</th><th>技术</th><th>仓库编号</th><th>数据状态</th><th class="col-arrow"></th></tr>`;
  return `<div class="table-wrap"><table class="paper-table dataset-table"><thead>${head}</thead><tbody>${rows}</tbody></table></div>`;
}

export function renderDatasets(app: App): string {
  const items = filterDatasets(app.state.datasets, app.datasetSearch, (id) => app.paperById(id)?.title ?? "");
  const toggle = `<div class="view-toggle" role="group" aria-label="数据集视图切换"><button class="view-option${app.datasetView === "cards" ? " active" : ""}" data-action="datasets-view" data-view="cards" aria-pressed="${app.datasetView === "cards"}">${I("grid", 15)} 卡片</button><button class="view-option${app.datasetView === "table" ? " active" : ""}" data-action="datasets-view" data-view="table" aria-pressed="${app.datasetView === "table"}">${I("more", 15)} 列表</button></div>`;
  const head = `<div class="section-toolbar"><p class="muted">${app.state.datasets.length} 组数据 · 论文与数据集分别计数</p><div class="page-actions">${toggle}<button class="btn btn-secondary" data-action="new-dataset">${I("plus", 16)} 添加数据集</button></div></div>`
    + `<div class="toolbar standalone-toolbar"><label class="table-search">${I("search", 18)}<input id="dataset-search" class="search-input" type="search" placeholder="搜索物种、组织、accession 或关联文献…" value="${esc(app.datasetSearch)}" aria-label="搜索数据集"></label></div>`;
  if (app.datasetView === "cards") {
    const cards = items.map((d) =>
      `<button class="dataset-card" data-action="open-dataset" data-id="${esc(d.id)}"><div class="dataset-top"><span class="icon-wrap green">${I("database", 20)}</span>${datasetStatusBadge(d)}</div><h3>${esc(d.title)}</h3><p class="species-name">${esc(speciesDisplay(d) || "物种待补")}</p><div class="tag-list">${badge(joinList(d.organs) || "组织待补")}${badge(joinList(d.modalities) || "技术待补", "blue")}${demoBadge(d)}</div><div class="dataset-accession"><span>${esc(d.repository ? d.repository.toUpperCase() : "仓库待补")}</span><code>${esc(d.accession || "编号待补")}</code></div><div class="dataset-footer"><span>${I("book", 15)} ${arr(d.paperLinks).length} 篇关联文献</span>${I("arrow-up-right", 17)}</div></button>`,
    ).join("");
    return head + (items.length ? `<div class="dataset-grid">${cards}</div>` : empty("暂无匹配数据集", "从文献详情关联数据，或先创建一条数据集记录。"));
  }
  // Writing the clamped page back keeps state valid as the search narrows the list.
  const info = paperPage(items.length, app.datasetPageSize, app.datasetPage);
  app.datasetPage = info.page;
  return head
    + `<section class="panel datasets-panel">${datasetTable(items.slice(info.start, info.end), app)}<div class="table-footer"><span>显示 ${items.length ? `${info.start + 1}–${info.end}` : 0} / ${items.length} 组数据集</span><span class="muted footer-hint">↑↓ 切换数据集 · ←→ 翻页 · 点击文献条目可跳转</span>${items.length ? tablePager("datasets", app.datasetPageSize, info) : ""}</div></section>`;
}

export function renderTasks(app: App): string {
  const all = app.tasks();
  const items = app.taskFilter === "all" ? all : all.filter((t) => t.type === app.taskFilter);
  const tabs: [TaskType | "all", string][] = [["all", "全部待补"], ["screen", "收录待判断"], ["fulltext", "全文待补"], ["data", "数据待核实"]];
  const tabButtons = tabs.map(([key, label]) =>
    `<button class="view-tab ${app.taskFilter === key ? "active" : ""}" data-action="task-filter" data-filter-value="${key}">${label} <span>${key === "all" ? all.length : all.filter((t) => t.type === key).length}</span></button>`,
  ).join("");
  return `<div class="notice-box">${I("info", 18)}<span>待补事项根据文献与数据状态自动生成。完成对应整理后，事项会自动移出列表。</span></div><div class="section-toolbar"><div class="view-tabs">${tabButtons}</div></div><section class="panel"><div class="task-list full-task-list">${items.length ? taskRows(items) : empty("这部分已整理完成", "新的文献或数据线索进入后，需要补充的事项会出现在这里。")}</div></section>`;
}

export function renderSchema(): string {
  const entities: [string, string, string, string, string][] = [
    ["01", "文献", "papers", "题目、摘要、发表信息、收录与阅读状态。全文缺失也可以先收录。", "book"],
    ["02", "数据集", "datasets", "独立内部 ID，关联多个仓库编号，并记录数据可用性。", "database"],
    ["03", "资料与附件", "resources", "正文、补充材料、数据入口、矩阵和代码，保存来源与获取状态。", "folder"],
  ];
  const cards = entities.map((e) =>
    `<section class="schema-card"><span class="schema-number">${e[0]}</span><span class="icon-wrap green">${I(e[4], 22)}</span><h2>${e[1]}</h2><code>${e[2]}</code><p>${e[3]}</p></section>`,
  ).join("");
  return `<div class="section-toolbar"><p class="muted">面向个人使用，逐步补齐信息</p><div class="page-actions"><a class="btn btn-secondary" href="./downloads/plantcell-library-v0.1.zip" download>下载完整文件包</a><a class="btn btn-secondary" href="./downloads/README.md" download>${I("file", 16)} 设计说明</a><a class="btn btn-primary" href="./downloads/schema.sql" download>${I("download", 16)} 下载建表 SQL</a></div></div>`
    + `<div class="schema-grid">${cards}</div>`
    + `<section class="panel"><div class="panel-header"><div><h2>关系如何连接</h2><p class="muted">关系单独保存，避免重复统计</p></div>${I("link", 20)}</div><div class="panel-body relation-list"><div class="relation-note"><strong>文献 ↔ 数据集</strong><span>多对多，标记新生成、复用或重新处理；一篇文献可以关联多组数据。</span><code>paper_datasets</code></div><div class="relation-note"><strong>文献 / 数据集 ↔ 资料</strong><span>同一份补充材料可以服务多个对象，链接与实际获取文件分别记录。</span><code>resources + 关联表</code></div><div class="relation-note"><strong>内部对象 ↔ 外部标识</strong><span>DOI、PMID 和仓库 accession 用于识别与关联；保留原始来源信息。</span><code>identifiers / accession</code></div></div></section>`
    + `<div class="overview-bottom-grid"><section class="panel"><div class="panel-header"><h2>状态分别记录</h2></div><div class="panel-body status-definitions"><p><strong>收录：</strong>待判断 · 证据不足 · 已纳入 · 相关 / 排除</p><p><strong>全文：</strong>待补入口 · 已有入口 · 已保存文件</p><p><strong>数据：</strong>待整理 · 线索已记录 · 已核实</p><p><strong>阅读：</strong>未读 · 已浏览 · 已精读</p></div></section><section class="panel"><div class="panel-header"><h2>下一步可逐步扩展</h2></div><div class="panel-body"><p class="muted">在相同数据集下补充生物样本、文库、测序 Run、参考基因组与细胞注释。每日采集使用独立日志与断点，人工笔记保留。</p><p class="muted">SQL 文件是后端数据库结构；当前网页原型在本浏览器保存操作，可通过 JSON 备份迁移。</p></div></section></div>`;
}
