// Detail drawer for one paper or one dataset.
import type { App } from "./app.ts";
import type { DatasetPaperLink, Evidence, Paper } from "./mygo.ts";
import { AVAILABILITY, READING, RESOURCE_TYPES, ROLE, SCREEN, arr, datasetLocation, dateText, esc, fileSize, joinList, repositoryURL, safeURL, speciesDisplay, speciesNames, type Resource } from "./model.ts";
import { I, badge, datasetStatusBadge, demoBadge, options, screenBadge } from "./ui.ts";
import { listSeparator, locale, t, tv } from "./i18n.ts";

/** displayList joins stored values for display, translating known labels like modalities. */
const displayList = (values: string[] | null | undefined): string => joinList(arr(values).map(tv), listSeparator);

const fileRow = (r: Resource): string =>
  `<div class="resource-item"><span class="icon-wrap blue">${I("file", 18)}</span><div class="resource-content"><strong>${esc(r.title)}</strong><span class="muted">${r.type === "fulltext" ? t("全文") : t("补充材料")} · ${esc(fileSize(r.sizeBytes))}</span></div><button class="btn-icon" data-action="open-file" data-id="${esc(r.id)}" aria-label="${t("打开 {title}", { title: esc(r.title) })}">${I("arrow-up-right", 18)}</button><button class="btn-icon" data-action="remove-file" data-id="${esc(r.id)}" aria-label="${t("移除 {title}", { title: esc(r.title) })}">${I("x", 18)}</button></div>`;

export function resourceRows(resources: Resource[], emptyHint = t("可以先保存全文、补充材料或数据页面的链接。")): string {
  if (!resources.length) return `<div class="inline-empty">${I("folder", 24)}<p>${t("还没有记录资料")}</p><span class="muted">${emptyHint}</span></div>`;
  return resources.map((r) => {
    if (r.localPath) return fileRow(r);
    const url = safeURL(r.url);
    const link = url
      ? `<a class="btn-icon" href="${esc(url)}" target="_blank" rel="noopener noreferrer" aria-label="${t("打开 {title}", { title: esc(r.title) })}">${I("arrow-up-right", 18)}</a>`
      : `<span class="muted">${t("链接待补")}</span>`;
    return `<div class="resource-item"><span class="icon-wrap blue">${I(r.type === "fulltext" ? "file" : "link", 18)}</span><div class="resource-content"><strong>${esc(r.title || RESOURCE_TYPES[r.type] || t("资料链接"))}</strong><span class="muted">${esc(RESOURCE_TYPES[r.type] ?? t("其他资料"))} · ${t("已记录链接")}${r.isDemo ? ` · ${t("示例跳转至官方主页")}` : ""}</span></div>${link}</div>`;
  }).join("");
}

const drawerFrame = (id: string, title: string, subtitle: string, body: string): string =>
  `<div class="drawer-backdrop" data-action="close-drawer"></div><aside class="drawer" role="dialog" aria-modal="true" aria-labelledby="drawer-title"><div class="drawer-header"><div class="drawer-eyebrow"><span>${esc(id)}</span><button class="btn-icon" data-action="close-drawer" aria-label="${t("关闭详情")}">${I("x", 20)}</button></div><h2 id="drawer-title">${esc(title)}</h2>${subtitle}</div>${body}</aside>`;

const detail = (label: string, value: string, small = ""): string =>
  `<div class="detail-item"><span>${label}</span><strong>${value}</strong>${small}</div>`;

const proposalBox = (p: Paper): string => p.proposedScreening
  ? `<div class="notice-box compact proposal-box">${I("info", 16)}<span>${t("Agent 建议：")}<strong>${esc(SCREEN[p.proposedScreening] ?? p.proposedScreening)}</strong>${p.proposedReason ? `<br>${esc(p.proposedReason)}` : ""}</span><span class="section-actions"><button class="text-link" data-action="accept-proposal" data-id="${esc(p.id)}">${t("采纳")}</button><button class="text-link" data-action="dismiss-proposal" data-id="${esc(p.id)}">${t("忽略")}</button></span></div>`
  : "";

/** reviewBox explains why a record a source update added waits in 需核对, and lets a person finish it. */
const reviewBox = (kind: "paper" | "dataset", id: string, text: string): string =>
  `<div class="notice-box compact proposal-box">${I("alert", 16)}<span><strong>${t("来源新增 · 尚未人工核查")}</strong><br>${esc(text)}</span><span class="section-actions"><button class="text-link" data-action="finish-review" data-kind="${kind}" data-id="${esc(id)}">${t("核查完成")}</button></span></div>`;

const FIELD_LABELS: Record<string, string> = {
  title: t("题目"), doi: "DOI", pmid: "PMID", authors: t("作者"), year: t("年份"), journal: t("期刊"), abstract: t("摘要"),
  species: t("物种"), organs: t("组织"), modalities: t("测序类型"), platforms: t("测序平台"), articleTypes: t("文献类型"),
  screening: t("收录判断"), repositories: t("数据仓库"), accession: "Accession", availability: t("数据可用性"), cellsReported: t("细胞数"),
};
const EVIDENCE_STATUS: Record<string, [string, string]> = { unreviewed: [t("待核对"), "amber"], verified: [t("已确认"), "green"], rejected: [t("已驳回"), "gray"] };

const sourceLink = (value: string): string => {
  const url = safeURL(value);
  return url ? `<a class="text-link" href="${esc(url)}" target="_blank" rel="noopener noreferrer">${t("来源")}${I("arrow-up-right", 13)}</a>` : "";
};

function evidenceRow(e: Evidence): string {
  const [label, color] = EVIDENCE_STATUS[e.status] ?? [t("待核对"), "amber"];
  const source = [
    sourceLink(e.sourceUrl),
    e.fileId ? `<button class="text-link" data-action="open-file" data-id="${esc(e.fileId)}">${t("打开文件")}</button>` : "",
    e.location ? `<span>${esc(e.location)}</span>` : "",
    e.extractor ? `<span>${esc(e.extractor)}</span>` : "",
  ].join("");
  const verdict = (status: string, text: string) => `<button class="text-link" data-action="review-evidence" data-id="${esc(e.id)}" data-status="${status}">${text}</button>`;
  const actions = e.status === "unreviewed" ? verdict("verified", t("确认")) + verdict("rejected", t("驳回")) : verdict("unreviewed", t("撤销"));
  return `<div class="evidence-item ${esc(e.status)}"><div class="evidence-head"><strong>${esc(FIELD_LABELS[e.field] ?? e.field)}${e.value ? `${locale === "en" ? ": " : "："}${esc(e.value)}` : ""}</strong>${badge(label, color)}</div><blockquote>${esc(e.excerpt)}</blockquote><div class="evidence-foot"><span class="evidence-source">${source}</span><span class="section-actions">${actions}</span></div></div>`;
}

const evidenceSection = (items: Evidence[]): string => items.length
  ? `<section class="detail-section"><h3>${t("Agent 证据")} <span class="count-pill">${items.length}</span></h3>${items.map(evidenceRow).join("")}</section>`
  : "";

/** linkReview shows why an agent linked a paper and dataset, for a person to verify. */
const linkReview = (app: App, datasetId: string, link: DatasetPaperLink): string => !app.env.real ? "" :
  `<div class="link-review"><span class="evidence-source"><span>${esc(link.evidenceText || (link.verified ? t("已核实") : t("未记录依据")))}</span>${sourceLink(link.evidenceUrl)}</span><button class="text-link" data-action="verify-link" data-paper="${esc(link.paperId)}" data-dataset="${esc(datasetId)}" data-role="${esc(link.role)}" data-verified="${!link.verified}">${link.verified ? t("撤销核实") : t("核实关联")}</button></div>`;

const PUBLICATION_LABELS: Record<string, string> = { preprint: t("预印本"), published: t("正式论文"), unpublished: t("未发表") };

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
      body = (p.awaitingReview ? reviewBox("paper", p.id, t("来自 {source} 的更新。做出收录判断后自动移出需核对；暂不判断也可直接标记核查完成。", { source: p.source ? tv(p.source) : t("来源") })) : "")
        + `<div class="drawer-status-row">${screenBadge(p.screening)}${badge(READING[p.reading] ?? t("未读"))}${demoBadge(p)}<button class="btn btn-secondary" data-action="edit-paper" data-id="${esc(p.id)}">${t("编辑信息")}</button><button class="btn btn-danger" data-action="delete-paper" data-id="${esc(p.id)}">${t("删除")}</button></div>`
        + `<div class="detail-grid">${detail(t("研究物种"), esc(speciesDisplay(p) || t("待补")), `<small>${esc(joinList(speciesNames(p), listSeparator))}</small>`)}${detail(t("组织 / 部位"), esc(displayList(p.organs) || t("待补")))}${detail(t("测序类型"), esc(displayList(p.modalities) || t("待补")))}${detail(t("测序平台"), esc(displayList(p.platforms) || t("待补")))}${detail(t("发表年份"), esc(p.year || t("待补")))}${detail(t("文献类型"), esc(displayList(p.articleTypes) || t("待判断")))}${detail(t("发表状态"), PUBLICATION_LABELS[p.publicationStatus] ?? t("尚未确认"))}</div>`
        + `<section class="detail-section"><h3>${t("摘要")}</h3><p class="abstract-text">${esc(p.abstract || t("摘要尚未补充。先保留这条文献，之后可以继续完善。"))}</p></section>`
        + `<section class="detail-section"><h3>${t("主题标签")}</h3><div class="tag-list">${arr(p.tags).length ? arr(p.tags).map((t) => `<span class="tag">${esc(t)}</span>`).join("") : `<span class="muted">${t("暂未设置标签")}</span>`}</div></section>`
        + evidenceSection(app.evidenceFor("paperId", p.id))
        + `<section class="detail-section"><h3>${t("来源与标识")}</h3><dl class="detail-dl"><dt>DOI</dt><dd>${p.doi ? `<a class="text-link" href="https://doi.org/${esc(encodeURI(p.doi))}" target="_blank" rel="noopener noreferrer">${esc(p.doi)}${I("arrow-up-right", 13)}</a>` : t("尚未记录")}</dd><dt>PMID</dt><dd>${esc(p.pmid || t("尚未记录"))}</dd><dt>${t("发现来源")}</dt><dd>${esc(tv(p.source || "手动添加"))}</dd>${p.batchId ? `<dt>${t("导入批次")}</dt><dd>${esc(p.batchId)}</dd>` : ""}<dt>${t("加入时间")}</dt><dd>${dateText(p.addedAt)}</dd></dl></section>`
        + `<section class="detail-section"><h3>${t("收录判断")}</h3>${proposalBox(p)}<label class="form-field"><span class="sr-only">${t("收录判断")}</span><select class="input" data-paper-setting="screening" data-id="${esc(p.id)}">${options(SCREEN, p.screening)}</select></label><p class="muted small">${esc(p.screeningReason || t("可在笔记中补充判断依据。"))}</p>${app.env.real ? `<p class="muted small">${p.reviewedAt ? t("人工确认于 {date}", { date: dateText(p.reviewedAt) }) : t("尚未人工确认")}</p>` : ""}</section>`;
    } else if (tab === "resources") {
      const related = ds.length
        ? ds.map((d) => `<button class="related-dataset" data-action="open-dataset" data-id="${esc(d.id)}"><span class="icon-wrap green">${I("database", 18)}</span><span><strong>${esc(d.title)}</strong><small>${esc(d.accession || t("仓库编号待补"))}</small></span>${badge(ROLE[d.paperLinks.find((l) => l.paperId === p.id)?.role ?? ""] ?? t("关联"), "gray")}${I("chevron-right", 15)}</button>${d.paperLinks.filter((l) => l.paperId === p.id).map((l) => linkReview(app, d.id, l)).join("")}`).join("")
        : `<p class="muted">${t("尚未关联数据。已有公共数据可以复用关联，无需重复创建。")}</p>`;
      const addResource = app.env.real
        ? `<span class="section-actions"><button class="text-link" data-action="fetch-fulltext" data-id="${esc(p.id)}">${I("download", 15)} ${t("获取开放全文")}</button><button class="text-link" data-action="import-file" data-id="${esc(p.id)}" data-type="fulltext">${I("plus", 15)} ${t("导入全文")}</button><button class="text-link" data-action="import-file" data-id="${esc(p.id)}" data-type="supplement">${I("plus", 15)} ${t("导入补充材料")}</button></span>`
        : `<button class="text-link" data-action="add-resource" data-id="${esc(p.id)}" data-kind="paper">${I("plus", 15)} ${t("添加链接")}</button>`;
      body = `<section class="detail-section"><div class="section-title"><h3>${t("关联数据集")} <span class="count-pill">${ds.length}</span></h3><button class="text-link" data-action="link-dataset" data-id="${esc(p.id)}">${I("plus", 15)} ${t("关联数据")}</button></div>${related}</section>`
        + `<section class="detail-section"><div class="section-title"><h3>${t("资料与附件")} <span class="count-pill">${resources.length}</span></h3>${addResource}</div>${app.env.real ? resourceRows(resources, t("导入 PDF 或补充材料后，会复制到存放文件夹保存。")) : resourceRows(resources)}</section>`
        + `<div class="notice-box compact">${I("info", 16)}<span>${app.env.real ? t("导入时复制到存放文件夹的 papers/，原文件保持不动。") : t("保存入口只表示链接已记录，不代表全文或数据文件已经获取。")}</span></div>`;
    } else {
      body = `<section class="detail-section"><h3>${t("研究笔记")}</h3><p class="muted">${t("记录关注的问题、判断依据和下一步计划。")}</p><label class="form-field"><span class="sr-only">${t("研究笔记")}</span><textarea class="textarea notes-textarea" id="paper-notes" rows="15" maxlength="50000" placeholder="${t("这篇文献值得关注什么？还有哪些信息需要补齐？")}">${esc(p.notes || "")}</textarea></label><button class="btn btn-primary" data-action="save-notes" data-id="${esc(p.id)}">${I("check", 16)} ${t("保存笔记")}</button></section>`
        + `<section class="detail-section"><h3>${t("主题标签")}</h3><label class="form-field"><span class="muted">${t("用逗号分隔，例如：根发育，跨物种，重点阅读")}</span><input class="input" id="paper-tags" value="${esc(arr(p.tags).join(locale === "en" ? ", " : "，"))}" maxlength="1000"></label><button class="btn btn-secondary" data-action="save-tags" data-id="${esc(p.id)}">${t("保存标签")}</button></section>`;
    }
    const tabs: [string, string][] = [["overview", t("文献概览")], ["resources", t("数据与资料")], ["notes", t("我的笔记")]];
    const tabButtons = tabs.map(([v, l]) => `<button class="drawer-tab ${tab === v ? "active" : ""}" data-action="drawer-tab" data-tab="${v}">${l}</button>`).join("");
    const footer = `<div class="drawer-footer"><label class="reading-select"><span>${t("阅读进度")}</span><select class="input" data-paper-setting="reading" data-id="${esc(p.id)}">${options(READING, p.reading)}</select></label><button class="btn ${p.starred ? "btn-primary" : "btn-secondary"}" data-action="toggle-star" data-id="${esc(p.id)}">${I("star", 16)}${p.starred ? ` ${t("已收藏")}` : ` ${t("收藏")}`}</button></div>`;
    return drawerFrame(p.id, p.title, `<p class="paper-meta">${esc(p.authors || t("作者待补"))} · ${esc(p.journal || t("期刊待补"))}</p>`, `<div class="drawer-tabs">${tabButtons}</div><div class="drawer-body">${body}</div>${footer}`);
  }

  const d = app.state.datasets.find((x) => x.id === drawer.id);
  if (!d) return null;
  const links = arr(d.paperLinks).map((l) => {
    const p = app.paperById(l.paperId);
    return p ? `<button class="related-dataset" data-action="open-paper" data-id="${esc(p.id)}"><span class="icon-wrap blue">${I("book", 18)}</span><span><strong>${esc(p.title)}</strong><small>${esc(p.year || t("年份待补"))}</small></span>${badge(ROLE[l.role] ?? t("关联"))}</button>${linkReview(app, d.id, l)}` : "";
  }).join("");
  const repositories = arr(d.repositories).map((r) =>
    `<div class="evidence-item"><div class="evidence-head"><strong>${esc(r.repository.toUpperCase())} ${esc(r.accession)}</strong>${badge(r.verified ? t("已核实") : t("待核实"), r.verified ? "green" : "amber")}</div>${r.evidenceText ? `<blockquote>${esc(r.evidenceText)}</blockquote>` : ""}<div class="evidence-foot"><span class="evidence-source">${sourceLink(safeURL(r.url) || repositoryURL(r))}${r.evidenceUrl && r.evidenceUrl !== r.url ? sourceLink(r.evidenceUrl) : ""}</span><span class="section-actions"><button class="text-link" data-action="verify-repository" data-id="${esc(d.id)}" data-record="${esc(r.id)}" data-verified="${!r.verified}">${r.verified ? t("撤销核实") : t("核实编号")}</button></span></div></div>`,
  ).join("");
  const datasetHome = datasetLocation(d);
  const accessionValue = datasetHome
    ? `<a class="text-link" href="${esc(datasetHome)}" target="_blank" rel="noopener noreferrer">${esc(d.accession || t("查看仓库"))}${I("arrow-up-right", 13)}</a>`
    : esc(d.accession || t("编号待补"));
  const body = `<div class="drawer-body">`
    + (d.awaitingReview ? reviewBox("dataset", d.id, t("来自数据仓库的更新，不会自动关联文献。核对仓库记录后修改整理状态即可移出需核对，也可直接标记核查完成。")) : "")
    + `<div class="drawer-status-row">${datasetStatusBadge(d)}${demoBadge(d)}</div>`
    + `<div class="detail-grid">${detail(t("仓库"), esc(d.repository || t("待补")))}${detail("Accession", accessionValue)}${detail(t("研究组织"), esc(displayList(d.organs) || t("待补")))}${detail(t("测序类型"), esc(displayList(d.modalities) || t("待补")))}${detail(t("测序平台"), esc(displayList(d.platforms) || t("待补")))}${detail(t("细胞数"), d.cellsReported == null ? t("尚未整理") : esc(d.cellsReported))}${detail(t("原始数据"), esc(AVAILABILITY[d.availability] ?? t("尚未核实")))}</div>`
    + (app.env.real && repositories ? `<section class="detail-section"><h3>${t("数据仓库")}</h3>${repositories}</section>` : "")
    + `<section class="detail-section"><h3>${t("关联文献")}</h3>${arr(d.paperLinks).length ? links : `<p class="muted">${t("尚未关联文献。")}</p>`}</section>`
    + evidenceSection(app.evidenceFor("datasetId", d.id))
    + `<section class="detail-section"><div class="section-title"><h3>${t("数据与资料入口")}</h3><button class="text-link" data-action="add-resource" data-id="${esc(d.id)}" data-kind="dataset">${I("plus", 15)} ${t("添加链接")}</button></div>${resourceRows(app.state.resources.filter((r) => r.datasetId === d.id))}</section>`
    + `<section class="detail-section"><h3>${t("整理备注")}</h3><p class="abstract-text">${esc(d.notes || t("尚未填写。"))}</p></section></div>`
    + `<div class="drawer-footer"><span class="muted">${t("独立记录数据，保留复用关系")}</span><button class="btn btn-primary" data-action="edit-dataset" data-id="${esc(d.id)}">${t("编辑信息")}</button></div>`;
  return drawerFrame(d.id, d.title, `<p class="species-name">${esc(speciesDisplay(d))}</p>`, body);
}
