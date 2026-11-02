// Detail drawer for one paper or one dataset.
import type { App } from "./app.ts";
import type { DatasetPaperLink, Evidence, Paper } from "./mygo.ts";
import { AVAILABILITY, READING, RESOURCE_TYPES, ROLE, SCREEN, arr, datasetLocation, dateText, esc, fileSize, joinList, repositoryURL, safeURL, speciesDisplay, speciesNames, type Resource } from "./model.ts";
import { I, badge, datasetStatusBadge, demoBadge, options, screenBadge } from "./ui.ts";

const fileRow = (r: Resource): string =>
  `<div class="resource-item"><span class="icon-wrap blue">${I("file", 18)}</span><div class="resource-content"><strong>${esc(r.title)}</strong><span class="muted">${r.type === "fulltext" ? "全文" : "补充材料"} · ${esc(fileSize(r.sizeBytes))}</span></div><button class="btn-icon" data-action="open-file" data-id="${esc(r.id)}" aria-label="打开 ${esc(r.title)}">${I("arrow-up-right", 18)}</button><button class="btn-icon" data-action="remove-file" data-id="${esc(r.id)}" aria-label="移除 ${esc(r.title)}">${I("x", 18)}</button></div>`;

export function resourceRows(resources: Resource[], emptyHint = "可以先保存全文、补充材料或数据页面的链接。"): string {
  if (!resources.length) return `<div class="inline-empty">${I("folder", 24)}<p>还没有记录资料</p><span class="muted">${emptyHint}</span></div>`;
  return resources.map((r) => {
    if (r.localPath) return fileRow(r);
    const url = safeURL(r.url);
    const link = url
      ? `<a class="btn-icon" href="${esc(url)}" target="_blank" rel="noopener noreferrer" aria-label="打开 ${esc(r.title)}">${I("arrow-up-right", 18)}</a>`
      : '<span class="muted">链接待补</span>';
    return `<div class="resource-item"><span class="icon-wrap blue">${I(r.type === "fulltext" ? "file" : "link", 18)}</span><div class="resource-content"><strong>${esc(r.title || RESOURCE_TYPES[r.type] || "资料链接")}</strong><span class="muted">${esc(RESOURCE_TYPES[r.type] ?? "其他资料")} · 已记录链接${r.isDemo ? " · 示例跳转至官方主页" : ""}</span></div>${link}</div>`;
  }).join("");
}

const drawerFrame = (id: string, title: string, subtitle: string, body: string): string =>
  `<div class="drawer-backdrop" data-action="close-drawer"></div><aside class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title"><div class="drawer-header"><div class="drawer-eyebrow"><span>${esc(id)}</span><button class="btn-icon" data-action="close-drawer" aria-label="关闭详情">${I("x", 20)}</button></div><h2 id="drawer-title">${esc(title)}</h2>${subtitle}</div>${body}</aside>`;

const detail = (label: string, value: string, small = ""): string =>
  `<div class="detail-item"><span>${label}</span><strong>${value}</strong>${small}</div>`;

const proposalBox = (p: Paper): string => p.proposedScreening
  ? `<div class="notice-box compact proposal-box">${I("info", 16)}<span>Agent 建议：<strong>${esc(SCREEN[p.proposedScreening] ?? p.proposedScreening)}</strong>${p.proposedReason ? `<br>${esc(p.proposedReason)}` : ""}</span><span class="section-actions"><button class="text-link" data-action="accept-proposal" data-id="${esc(p.id)}">采纳</button><button class="text-link" data-action="dismiss-proposal" data-id="${esc(p.id)}">忽略</button></span></div>`
  : "";

/** reviewBox explains why a record a source update added waits in 需核对, and lets a person finish it. */
const reviewBox = (kind: "paper" | "dataset", id: string, text: string): string =>
  `<div class="notice-box compact proposal-box">${I("alert", 16)}<span><strong>来源新增 · 尚未人工核查</strong><br>${esc(text)}</span><span class="section-actions"><button class="text-link" data-action="finish-review" data-kind="${kind}" data-id="${esc(id)}">核查完成</button></span></div>`;

const FIELD_LABELS: Record<string, string> = {
  title: "题目", doi: "DOI", pmid: "PMID", authors: "作者", year: "年份", journal: "期刊", abstract: "摘要",
  species: "物种", organs: "组织", modalities: "测序类型", platforms: "测序平台", articleTypes: "文献类型",
  screening: "收录判断", repositories: "数据仓库", accession: "Accession", availability: "数据可用性", cellsReported: "细胞数",
};
const EVIDENCE_STATUS: Record<string, [string, string]> = { unreviewed: ["待核对", "amber"], verified: ["已确认", "green"], rejected: ["已驳回", "gray"] };

const sourceLink = (value: string): string => {
  const url = safeURL(value);
  return url ? `<a class="text-link" href="${esc(url)}" target="_blank" rel="noopener noreferrer">来源${I("arrow-up-right", 13)}</a>` : "";
};

function evidenceRow(e: Evidence): string {
  const [label, color] = EVIDENCE_STATUS[e.status] ?? ["待核对", "amber"];
  const source = [
    sourceLink(e.sourceUrl),
    e.fileId ? `<button class="text-link" data-action="open-file" data-id="${esc(e.fileId)}">打开文件</button>` : "",
    e.location ? `<span>${esc(e.location)}</span>` : "",
    e.extractor ? `<span>${esc(e.extractor)}</span>` : "",
  ].join("");
  const verdict = (status: string, text: string) => `<button class="text-link" data-action="review-evidence" data-id="${esc(e.id)}" data-status="${status}">${text}</button>`;
  const actions = e.status === "unreviewed" ? verdict("verified", "确认") + verdict("rejected", "驳回") : verdict("unreviewed", "撤销");
  return `<div class="evidence-item ${esc(e.status)}"><div class="evidence-head"><strong>${esc(FIELD_LABELS[e.field] ?? e.field)}${e.value ? `：${esc(e.value)}` : ""}</strong>${badge(label, color)}</div><blockquote>${esc(e.excerpt)}</blockquote><div class="evidence-foot"><span class="evidence-source">${source}</span><span class="section-actions">${actions}</span></div></div>`;
}

const evidenceSection = (items: Evidence[]): string => items.length
  ? `<section class="detail-section"><h3>Agent 证据 <span class="count-pill">${items.length}</span></h3>${items.map(evidenceRow).join("")}</section>`
  : "";

/** linkReview shows why an agent linked a paper and dataset, for a person to verify. */
const linkReview = (app: App, datasetId: string, link: DatasetPaperLink): string => !app.env.real ? "" :
  `<div class="link-review"><span class="evidence-source"><span>${esc(link.evidenceText || (link.verified ? "已核实" : "未记录依据"))}</span>${sourceLink(link.evidenceUrl)}</span><button class="text-link" data-action="verify-link" data-paper="${esc(link.paperId)}" data-dataset="${esc(datasetId)}" data-role="${esc(link.role)}" data-verified="${!link.verified}">${link.verified ? "撤销核实" : "核实关联"}</button></div>`;

const PUBLICATION_LABELS: Record<string, string> = { preprint: "预印本", published: "正式论文", unpublished: "未发表" };

/** renderDrawer returns the drawer HTML, or null when the referenced record no longer exists. */
export function renderDrawer(app: App): string | null {
  const { drawer } = app;
  if (!drawer) return null;
  if (drawer.type === "paper") {
    const p = app.paperById(drawer.id);
    if (!p) return null;
    const ds = app.paperDatasets(p.id);
    const resources = app.paperResources(p.id);
    const tab = app.drawerTab;
    let body = "";
    if (tab === "overview") {
      body = (p.awaitingReview ? reviewBox("paper", p.id, `来自 ${p.source || "来源"} 的更新。做出收录判断后自动移出需核对；暂不判断也可直接标记核查完成。`) : "")
        + `<div class="drawer-status-row">${screenBadge(p.screening)}${badge(READING[p.reading] ?? "未读")}${demoBadge(p)}<button class="btn btn-secondary" data-action="edit-paper" data-id="${esc(p.id)}">编辑信息</button><button class="btn btn-danger" data-action="delete-paper" data-id="${esc(p.id)}">删除</button></div>`
        + `<div class="detail-grid">${detail("研究物种", esc(speciesDisplay(p) || "待补"), `<small>${esc(joinList(speciesNames(p)))}</small>`)}${detail("组织 / 部位", esc(joinList(p.organs) || "待补"))}${detail("测序类型", esc(joinList(p.modalities) || "待补"))}${detail("测序平台", esc(joinList(p.platforms) || "待补"))}${detail("发表年份", esc(p.year || "待补"))}${detail("文献类型", esc(joinList(p.articleTypes) || "待判断"))}${detail("发表状态", PUBLICATION_LABELS[p.publicationStatus] ?? "尚未确认")}</div>`
        + `<section class="detail-section"><h3>摘要</h3><p class="abstract-text">${esc(p.abstract || "摘要尚未补充。先保留这条文献，之后可以继续完善。")}</p></section>`
        + `<section class="detail-section"><h3>主题标签</h3><div class="tag-list">${arr(p.tags).length ? arr(p.tags).map((t) => `<span class="tag">${esc(t)}</span>`).join("") : '<span class="muted">暂未设置标签</span>'}</div></section>`
        + evidenceSection(app.evidenceFor("paperId", p.id))
        + `<section class="detail-section"><h3>来源与标识</h3><dl class="detail-dl"><dt>DOI</dt><dd>${p.doi ? `<a class="text-link" href="https://doi.org/${esc(encodeURI(p.doi))}" target="_blank" rel="noopener noreferrer">${esc(p.doi)}${I("arrow-up-right", 13)}</a>` : "尚未记录"}</dd><dt>PMID</dt><dd>${esc(p.pmid || "尚未记录")}</dd><dt>发现来源</dt><dd>${esc(p.source || "手动添加")}</dd>${p.batchId ? `<dt>导入批次</dt><dd>${esc(p.batchId)}</dd>` : ""}<dt>加入时间</dt><dd>${dateText(p.addedAt)}</dd></dl></section>`
        + `<section class="detail-section"><h3>收录判断</h3>${proposalBox(p)}<label class="form-field"><span class="sr-only">收录判断</span><select class="input" data-paper-setting="screening" data-id="${esc(p.id)}">${options(SCREEN, p.screening)}</select></label><p class="muted small">${esc(p.screeningReason || "可在笔记中补充判断依据。")}</p>${app.env.real ? `<p class="muted small">${p.reviewedAt ? `人工确认于 ${dateText(p.reviewedAt)}` : "尚未人工确认"}</p>` : ""}</section>`;
    } else if (tab === "resources") {
      const related = ds.length
        ? ds.map((d) => `<button class="related-dataset" data-action="open-dataset" data-id="${esc(d.id)}"><span class="icon-wrap green">${I("database", 18)}</span><span><strong>${esc(d.title)}</strong><small>${esc(d.accession || "仓库编号待补")}</small></span>${badge(ROLE[d.paperLinks.find((l) => l.paperId === p.id)?.role ?? ""] ?? "关联", "gray")}${I("chevron-right", 15)}</button>${d.paperLinks.filter((l) => l.paperId === p.id).map((l) => linkReview(app, d.id, l)).join("")}`).join("")
        : '<p class="muted">尚未关联数据。已有公共数据可以复用关联，无需重复创建。</p>';
      const addResource = app.env.real
        ? `<span class="section-actions"><button class="text-link" data-action="fetch-fulltext" data-id="${esc(p.id)}">${I("download", 15)} 获取开放全文</button><button class="text-link" data-action="import-file" data-id="${esc(p.id)}" data-type="fulltext">${I("plus", 15)} 导入全文</button><button class="text-link" data-action="import-file" data-id="${esc(p.id)}" data-type="supplement">${I("plus", 15)} 导入补充材料</button></span>`
        : `<button class="text-link" data-action="add-resource" data-id="${esc(p.id)}" data-kind="paper">${I("plus", 15)} 添加链接</button>`;
      body = `<section class="detail-section"><div class="section-title"><h3>关联数据集 <span class="count-pill">${ds.length}</span></h3><button class="text-link" data-action="link-dataset" data-id="${esc(p.id)}">${I("plus", 15)} 关联数据</button></div>${related}</section>`
        + `<section class="detail-section"><div class="section-title"><h3>资料与附件 <span class="count-pill">${resources.length}</span></h3>${addResource}</div>${app.env.real ? resourceRows(resources, "导入 PDF 或补充材料后，会复制到存放文件夹保存。") : resourceRows(resources)}</section>`
        + `<div class="notice-box compact">${I("info", 16)}<span>${app.env.real ? "导入时复制到存放文件夹的 papers/，原文件保持不动。" : "保存入口只表示链接已记录，不代表全文或数据文件已经获取。"}</span></div>`;
    } else {
      body = `<section class="detail-section"><h3>研究笔记</h3><p class="muted">记录关注的问题、判断依据和下一步计划。</p><label class="form-field"><span class="sr-only">研究笔记</span><textarea class="textarea notes-textarea" id="paper-notes" rows="15" maxlength="50000" placeholder="这篇文献值得关注什么？还有哪些信息需要补齐？">${esc(p.notes || "")}</textarea></label><button class="btn btn-primary" data-action="save-notes" data-id="${esc(p.id)}">${I("check", 16)} 保存笔记</button></section>`
        + `<section class="detail-section"><h3>主题标签</h3><label class="form-field"><span class="muted">用逗号分隔，例如：根发育，跨物种，重点阅读</span><input class="input" id="paper-tags" value="${esc(arr(p.tags).join("，"))}" maxlength="1000"></label><button class="btn btn-secondary" data-action="save-tags" data-id="${esc(p.id)}">保存标签</button></section>`;
    }
    const tabs: [string, string][] = [["overview", "文献概览"], ["resources", "数据与资料"], ["notes", "我的笔记"]];
    const tabButtons = tabs.map(([v, l]) => `<button class="drawer-tab ${tab === v ? "active" : ""}" data-action="drawer-tab" data-tab="${v}">${l}</button>`).join("");
    const footer = `<div class="drawer-footer"><label class="reading-select"><span>阅读进度</span><select class="input" data-paper-setting="reading" data-id="${esc(p.id)}">${options(READING, p.reading)}</select></label><button class="btn ${p.starred ? "btn-primary" : "btn-secondary"}" data-action="toggle-star" data-id="${esc(p.id)}">${I("star", 16)}${p.starred ? " 已收藏" : " 收藏"}</button></div>`;
    return drawerFrame(p.id, p.title, `<p class="paper-meta">${esc(p.authors || "作者待补")} · ${esc(p.journal || "期刊待补")}</p>`, `<div class="drawer-tabs">${tabButtons}</div><div class="drawer-body">${body}</div>${footer}`);
  }

  const d = app.state.datasets.find((x) => x.id === drawer.id);
  if (!d) return null;
  const links = arr(d.paperLinks).map((l) => {
    const p = app.paperById(l.paperId);
    return p ? `<button class="related-dataset" data-action="open-paper" data-id="${esc(p.id)}"><span class="icon-wrap blue">${I("book", 18)}</span><span><strong>${esc(p.title)}</strong><small>${esc(p.year || "年份待补")}</small></span>${badge(ROLE[l.role] ?? "关联")}</button>${linkReview(app, d.id, l)}` : "";
  }).join("");
  const repositories = arr(d.repositories).map((r) =>
    `<div class="evidence-item"><div class="evidence-head"><strong>${esc(r.repository.toUpperCase())} ${esc(r.accession)}</strong>${badge(r.verified ? "已核实" : "待核实", r.verified ? "green" : "amber")}</div>${r.evidenceText ? `<blockquote>${esc(r.evidenceText)}</blockquote>` : ""}<div class="evidence-foot"><span class="evidence-source">${sourceLink(safeURL(r.url) || repositoryURL(r))}${r.evidenceUrl && r.evidenceUrl !== r.url ? sourceLink(r.evidenceUrl) : ""}</span><span class="section-actions"><button class="text-link" data-action="verify-repository" data-id="${esc(d.id)}" data-record="${esc(r.id)}" data-verified="${!r.verified}">${r.verified ? "撤销核实" : "核实编号"}</button></span></div></div>`,
  ).join("");
  const datasetHome = datasetLocation(d);
  const accessionValue = datasetHome
    ? `<a class="text-link" href="${esc(datasetHome)}" target="_blank" rel="noopener noreferrer">${esc(d.accession || "查看仓库")}${I("arrow-up-right", 13)}</a>`
    : esc(d.accession || "编号待补");
  const body = `<div class="drawer-body">`
    + (d.awaitingReview ? reviewBox("dataset", d.id, "来自数据仓库的更新，不会自动关联文献。核对仓库记录后修改整理状态即可移出需核对，也可直接标记核查完成。") : "")
    + `<div class="drawer-status-row">${datasetStatusBadge(d)}${demoBadge(d)}</div>`
    + `<div class="detail-grid">${detail("仓库", esc(d.repository || "待补"))}${detail("Accession", accessionValue)}${detail("研究组织", esc(joinList(d.organs) || "待补"))}${detail("测序类型", esc(joinList(d.modalities) || "待补"))}${detail("测序平台", esc(joinList(d.platforms) || "待补"))}${detail("细胞数", d.cellsReported == null ? "尚未整理" : esc(d.cellsReported))}${detail("原始数据", esc(AVAILABILITY[d.availability] ?? "尚未核实"))}</div>`
    + (app.env.real && repositories ? `<section class="detail-section"><h3>数据仓库</h3>${repositories}</section>` : "")
    + `<section class="detail-section"><h3>关联文献</h3>${arr(d.paperLinks).length ? links : '<p class="muted">尚未关联文献。</p>'}</section>`
    + evidenceSection(app.evidenceFor("datasetId", d.id))
    + `<section class="detail-section"><div class="section-title"><h3>数据与资料入口</h3><button class="text-link" data-action="add-resource" data-id="${esc(d.id)}" data-kind="dataset">${I("plus", 15)} 添加链接</button></div>${resourceRows(app.state.resources.filter((r) => r.datasetId === d.id))}</section>`
    + `<section class="detail-section"><h3>整理备注</h3><p class="abstract-text">${esc(d.notes || "尚未填写。")}</p></section></div>`
    + `<div class="drawer-footer"><span class="muted">独立记录数据，保留复用关系</span><button class="btn btn-primary" data-action="edit-dataset" data-id="${esc(d.id)}">编辑信息</button></div>`;
  return drawerFrame(d.id, d.title, `<p class="species-name">${esc(speciesDisplay(d))}</p>`, body);
}
