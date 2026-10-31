// Page renderers for overview, papers, datasets, tasks and schema.
import type { App } from "./app.ts";
import type { Paper } from "./mygo.ts";
import {
  FULL, SCREEN, arr, esc, filterPapers, joinList, speciesDisplay, speciesNames, uniqueValues, speciesLabelOf,
  type TaskRow, type TaskType,
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

export function paperTable(items: Paper[], compact = false): string {
  if (!items.length) return empty("没有找到匹配文献", "尝试调整筛选条件，或添加一篇新文献。", '<button class="btn btn-secondary" data-action="clear-filters">清空筛选</button>');
  const rows = items.map((p) =>
    `<tr data-action="open-paper" data-id="${esc(p.id)}" tabindex="0" aria-label="查看 ${esc(p.title)}">`
    + `<td><button class="btn-icon star-button${p.starred ? " is-starred" : ""}" data-action="toggle-star" data-id="${esc(p.id)}" aria-label="${p.starred ? "取消收藏" : "收藏"}" aria-pressed="${Boolean(p.starred)}">${I("star", 16)}</button></td>`
    + `<td class="paper-main-cell"><div class="paper-title">${esc(p.title)}</div><div class="paper-meta">${demoBadge(p)}<span>${esc(p.authors || "作者待补")}</span><span class="meta-separator">·</span><span>${esc(p.year || "年份待补")}</span>${p.publicationStatus === "preprint" ? '<span class="preprint-label">预印本</span>' : ""}</div></td>`
    + `<td><div class="species-label">${esc(speciesDisplay(p) || "物种待补")}</div><div class="species-name">${esc(joinList(p.organs) || "组织待补")}</div></td>`
    + `<td>${modalityBadges(p)}</td>`
    + (compact ? "" : `<td><span class="fulltext-status">${fullBadge(p.fulltext)}</span></td>`)
    + `<td>${screenBadge(p.screening)}</td><td class="row-arrow">${I("arrow-up-right", 16)}</td></tr>`,
  ).join("");
  return `<div class="table-wrap"><table class="paper-table${compact ? " compact-table" : ""}"><thead><tr><th class="col-star"><span class="sr-only">收藏</span></th><th>文献</th><th>研究对象</th><th>技术</th>${compact ? "" : "<th>全文</th>"}<th>收录状态</th><th class="col-arrow"></th></tr></thead><tbody>${rows}</tbody></table></div>`;
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
    + `<div class="overview-grid"><section class="panel recent-panel"><div class="panel-header"><div><h2>最近收录</h2><p class="muted">新线索先留下，整理慢慢来</p></div><button class="text-link" data-page="papers">查看全部 ${I("arrow-right", 16)}</button></div>${paperTable(recent, true)}<div class="panel-footnote">${I("info", 14)} 当前条目为原型示例，可通过新增或导入建立自己的目录。</div></section>`
    + `<section class="panel tasks-panel"><div class="panel-header"><div><h2>接下来整理</h2><p class="muted">按需要补齐信息</p></div><span class="count-pill">${ts.length}</span></div><div class="task-groups">${taskGroups(ts)}</div><button class="panel-bottom-link" data-page="tasks">查看全部待补事项 ${I("arrow-right", 16)}</button></section></div>`
    + `<div class="overview-bottom-grid"><section class="panel"><div class="panel-header"><div><h2>整理覆盖</h2><p class="muted">收录与完善，各自向前一步</p></div>${I("grid", 18)}</div><div class="panel-body progress-list">${progressRow("收录范围已判断", total - pending, total)}${progressRow("已关联数据集", withData, total, "blue")}${progressRow("已记录全文入口", withFulltext, total, "gold")}</div></section>`
    + `<section class="panel"><div class="panel-header"><div><h2>研究物种</h2><p class="muted">当前目录中的文献分布</p></div><span class="muted">${uniqueValues(papers, "species").length} 个分类</span></div><div class="panel-body species-bars">${speciesBars}</div></section></div>`;
}

export function renderPapers(app: App): string {
  const { filters } = app;
  const papers = app.state.papers;
  const items = filterPapers(papers, filters);
  const filterKeys = ["species", "organ", "year", "modality", "platform", "fulltext", "screening", "source"] as const;
  const filterCount = filterKeys.filter((k) => filters[k]).length + (filters.starred ? 1 : 0);
  const entries = (map: Record<string, string>) => Object.entries(map).map(([value, label]) => ({ value, label }));
  return `<div class="section-toolbar"><div class="view-tabs"><button class="view-tab ${filters.starred ? "" : "active"}" data-action="all-papers">全部文献 <span>${papers.length}</span></button><button class="view-tab ${filters.starred ? "active" : ""}" data-action="starred-papers">${I("star", 15)} 我的收藏 <span>${papers.filter((p) => p.starred).length}</span></button></div><button class="btn btn-secondary" data-action="export-csv">${I("download", 16)} 导出列表</button></div>`
    + `<section class="panel papers-panel"><div class="toolbar"><label class="table-search">${I("search", 18)}<input id="paper-search" class="search-input" type="search" placeholder="搜索题目、物种、作者或 DOI…" value="${esc(filters.search)}" aria-label="搜索文献"></label><select class="filter-select" data-filter="sort" aria-label="排序方式">${option("newest", "最近收录", filters.sort)}${option("year-desc", "发表年份 ↓", filters.sort)}${option("title", "按标题排序", filters.sort)}</select></div>`
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
    + `</div>${paperTable(items)}<div class="table-footer"><span>显示 ${items.length} / ${papers.length} 篇文献</span><span class="muted">点击文献查看详情与整理记录</span></div></section>`;
}

export function renderDatasets(app: App): string {
  const q = app.datasetSearch.toLowerCase();
  const items = app.state.datasets.filter((d) => [
    d.title, d.id, speciesNames(d).join(" "), speciesDisplay(d), joinList(d.organs, " "), joinList(d.modalities, " "), joinList(d.platforms, " "),
    arr(d.repositories).map((r) => `${r.repository || ""} ${r.accession || ""}`).join(" "),
  ].join(" ").toLowerCase().includes(q));
  const cards = items.map((d) =>
    `<button class="dataset-card" data-action="open-dataset" data-id="${esc(d.id)}"><div class="dataset-top"><span class="icon-wrap green">${I("database", 20)}</span>${datasetStatusBadge(d)}</div><h3>${esc(d.title)}</h3><p class="species-name">${esc(speciesDisplay(d) || "物种待补")}</p><div class="tag-list">${badge(joinList(d.organs) || "组织待补")}${badge(joinList(d.modalities) || "技术待补", "blue")}${demoBadge(d)}</div><div class="dataset-accession"><span>${esc(d.repository ? d.repository.toUpperCase() : "仓库待补")}</span><code>${esc(d.accession || "编号待补")}</code></div><div class="dataset-footer"><span>${I("book", 15)} ${arr(d.paperLinks).length} 篇关联文献</span>${I("arrow-up-right", 17)}</div></button>`,
  ).join("");
  return `<div class="section-toolbar"><p class="muted">${app.state.datasets.length} 组数据 · 论文与数据集分别计数</p><button class="btn btn-secondary" data-action="new-dataset">${I("plus", 16)} 添加数据集</button></div>`
    + `<div class="toolbar standalone-toolbar"><label class="table-search">${I("search", 18)}<input id="dataset-search" class="search-input" type="search" placeholder="搜索物种、组织或 accession…" value="${esc(app.datasetSearch)}" aria-label="搜索数据集"></label></div>`
    + (items.length ? `<div class="dataset-grid">${cards}</div>` : empty("暂无匹配数据集", "从文献详情关联数据，或先创建一条数据集记录。"));
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
