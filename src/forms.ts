// Modal forms: paper, dataset, dataset link, resource link, exclusion reason,
// demo reset and import confirmation.
import type { App } from "./app.ts";
import type { Dataset, DatasetRepository, Paper } from "./mygo.ts";
import {
  ARTICLE_TYPES, AVAILABILITY, MODALITY_CHOICES, PLATFORM_CHOICES, RESOURCE_TYPES, ROLE, arr, clone, decideScreening, emptyFilters, emptyRepository, esc, joinList,
  normalizeDOI, normalizeDataset, nowISO, safeURL, splitList, uid, validDOI, type ImportStats,
} from "./model.ts";
import { t } from "./i18n.ts";
import { I, checkboxGroup, customField, field, mergeChecked, option, options, readSpeciesRows, setFormError, speciesRow, text } from "./ui.ts";

export const cancelButton = `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button>`;
const PUBLICATION_STATUSES: Record<string, string> = { unknown: t("尚未确认"), preprint: t("预印本"), published: t("正式发表"), unpublished: t("未发表") };
const REPOSITORIES: Record<string, string> = { geo: "GEO", gsa: "GSA / CNCB", sra: "SRA", ena: "ENA", bioproject: "BioProject", biosample: "BioSample", biostudies: "BioStudies", zenodo: "Zenodo", figshare: "Figshare", dryad: "Dryad", other: t("其他") };

export function paperModal(app: App, id = ""): void {
  const existing = id ? app.paperById(id) : undefined;
  if (id && !existing) return;
  const p: Partial<Paper> = existing ?? {};
  const species = arr(p.species).length ? arr(p.species) : [{}];
  const form = `<form id="paper-form"><input type="hidden" name="id" value="${esc(id)}"><div class="notice-box compact">${I("info", 16)}<span>${existing ? t("补充文献信息，已有笔记、标签、阅读进度和数据关联会保留。") : t("先填写题目即可收录，其余信息之后再补。新条目不会标记为演示数据。")}</span></div><div class="form-grid">`
    + `<div class="form-field full-width"><span>DOI</span><div class="doi-input-row"><input class="input" name="doi" type="text" value="${esc(p.doi ?? "")}" placeholder="${t("10.xxxx/… 或 DOI 链接")}" maxlength="500" aria-describedby="doi-status"><button type="button" class="btn btn-secondary" id="doi-lookup" data-action="lookup-doi">${I("download", 16)} ${t("获取信息")}</button></div><p id="doi-status" class="doi-status muted small" role="status">${t("填好 DOI 后点击获取，自动补全题目、作者、期刊和摘要；获取不到时可直接手动填写。")}</p></div>`
    + field(t("文献题目"), "title", p.title ?? "", t("输入论文题目"), true, "text", true)
    + field(t("作者"), "authors", p.authors ?? "", t("作者或研究团队"))
    + field(t("发表年份"), "year", p.year ? String(p.year) : "", t("例如 2026"), false, "number")
    + field(t("期刊 / 出版来源"), "journal", p.journal ?? "", t("暂时没有可以留空"))
    + field("PMID", "pmid", p.pmid ?? "", t("可选，例如 39012345"))
    + `<label class="form-field"><span>${t("发表状态")}</span><select class="input" name="publicationStatus">${options(PUBLICATION_STATUSES, p.publicationStatus || "unknown")}</select></label>`
    + `<label class="form-field"><span>${t("文献类型（可多选）")}</span>${checkboxGroup("articleTypes", ARTICLE_TYPES, arr(p.articleTypes).filter((v) => ARTICLE_TYPES.includes(v)))}</label>`
    + `<div class="form-field full-width"><span>${t("研究物种（一篇文献可涉及多个）")}</span><div class="species-rows" id="species-rows">${species.map((s) => speciesRow(s)).join("")}</div><button type="button" class="text-link" data-action="add-species-row">${I("plus", 15)} ${t("添加物种")}</button></div>`
    + `<label class="form-field"><span>${t("组织 / 部位（逗号分隔）")}</span><input class="input" name="organs" type="text" value="${esc(joinList(p.organs, ", "))}" placeholder="${t("例如 根, 叶")}" maxlength="2000"></label>`
    + `<div class="form-field full-width"><span>${t("测序类型（可多选）")}</span>${checkboxGroup("modalities", MODALITY_CHOICES, p.modalities, customField("modalitiesCustom", t("其他类型，逗号分隔")))}</div>`
    + `<div class="form-field full-width"><span>${t("测序平台（可多选）")}</span>${checkboxGroup("platforms", PLATFORM_CHOICES, p.platforms, customField("platformsCustom", t("其他平台，逗号分隔")))}</div>`
    + `<label class="form-field full-width"><span>${t("摘要")}</span><textarea class="textarea" name="abstract" rows="4" maxlength="50000" placeholder="${t("可粘贴摘要，暂时没有也可以留空")}">${esc(p.abstract ?? "")}</textarea></label>`
    + `<label class="form-field full-width"><span>${t("收录判断依据")}${p.screening === "excluded" ? ' <b class="required">*</b>' : ""}</span><textarea class="textarea" name="screeningReason" rows="2" maxlength="3000" placeholder="${t("例如：实际分析了植物单核数据；或仅包含空间转录组")}">${esc(p.screeningReason ?? "")}</textarea></label>`
    + '</div><p id="form-error" class="form-error" role="alert"></p></form>';
  app.showModal(existing ? t("编辑文献信息") : t("添加文献"), form, `${cancelButton}<button class="btn btn-primary" type="submit" form="paper-form">${I(existing ? "check" : "plus", 16)} ${existing ? t("保存修改") : t("添加到文献库")}</button>`, true);
}

export function savePaper(app: App, form: HTMLFormElement): void {
  const f = new FormData(form);
  const id = text(f, "id");
  const existing = id ? app.paperById(id) : undefined;
  const title = text(f, "title");
  const doi = normalizeDOI(f.get("doi"));
  const pmid = text(f, "pmid").replace(/^pmid:?\s*/i, "");
  const yearRaw = text(f, "year");
  const year = yearRaw ? Number(yearRaw) : null;
  const reason = text(f, "screeningReason");
  const publicationStatus = text(f, "publicationStatus") || "unknown";
  let error = "";
  if (id && !existing) error = t("原条目不存在，请关闭弹窗后重试。");
  else if (!title) error = t("请填写文献题目。");
  else if (!validDOI(doi)) error = t("DOI 格式不正确，请填写 10.xxxx/… 或完整 DOI 链接。");
  else if (doi && app.state.papers.some((p) => p.id !== id && normalizeDOI(p.doi) === doi)) error = t("这个 DOI 已在文献库中，请直接整理已有条目。");
  else if (pmid && !/^\d{1,10}$/.test(pmid)) error = t("PMID 应为纯数字。");
  else if (year !== null && (!Number.isInteger(year) || year < 1800 || year > 2100)) error = t("请填写 1800–2100 之间的年份，或留空。");
  else if (!Object.hasOwn(PUBLICATION_STATUSES, publicationStatus)) error = t("请选择有效的发表状态。");
  else if (existing && existing.screening === "excluded" && !reason) error = t("相关 / 排除条目需要保留判断理由。");
  if (error) {
    setFormError(error);
    return;
  }
  const now = nowISO();
  const values = {
    title, doi, pmid, authors: text(f, "authors"), year, journal: text(f, "journal"), publicationStatus,
    species: readSpeciesRows(form), organs: splitList(f.get("organs")).slice(0, 50),
    modalities: mergeChecked(f, "modalities", "modalitiesCustom"), platforms: mergeChecked(f, "platforms", "platformsCustom"),
    articleTypes: f.getAll("articleTypes").map(String), abstract: text(f, "abstract"), screeningReason: reason, updatedAt: now,
  };
  let p: Paper;
  if (existing) {
    p = Object.assign(existing, values);
  } else {
    p = { ...values, id: uid("P"), screening: "pending", reviewedAt: "", proposedScreening: "", proposedReason: "", awaitingReview: false, batchId: "", fulltext: "missing", fulltextUrl: "", reading: "unread", starred: false, tags: [], notes: "", source: "手动添加", addedAt: now, isDemo: false }; // i18n-ignore
    app.state.papers.unshift(p);
  }
  app.logActivity(existing ? t("更新文献信息：{title}", { title: p.title }) : t("添加文献：{title}", { title: p.title }), existing ? "edit" : "add");
  app.persist();
  app.closeModal();
  if (existing) {
    app.render();
    app.renderDrawer();
    app.toast(t("文献信息已更新，笔记与关联保持不变。"));
  } else {
    app.filters = { ...emptyFilters(), sort: app.filters.sort };
    app.navigate("papers");
    app.toast(t("已收录，全文与数据可以之后补充。"));
    app.openPaper(p.id);
  }
}

export function exclusionReasonModal(app: App, p: Paper): void {
  app.showModal(t("补充排除理由"), `<form id="exclusion-reason-form"><input type="hidden" name="paperId" value="${esc(p.id)}"><p class="muted">${t("为了保留筛选依据，请记录为何将这篇文献归为相关资料或排除。")}</p><label class="form-field"><span>${t("判断理由")} <b class="required">*</b></span><textarea class="textarea" name="reason" rows="4" maxlength="3000" required placeholder="${t("例如：仅包含空间转录组，没有实际分析单细胞或单核测序数据")}"></textarea></label><p id="form-error" class="form-error" role="alert"></p></form>`, `${cancelButton}<button class="btn btn-primary" type="submit" form="exclusion-reason-form">${t("保存判断")}</button>`);
}

export function saveExclusionReason(app: App, form: HTMLFormElement): void {
  const f = new FormData(form);
  const p = app.paperById(text(f, "paperId"));
  const reason = text(f, "reason");
  if (!p || !reason) {
    setFormError(t("请填写判断理由。"));
    return;
  }
  decideScreening(p, "excluded", reason);
  app.logActivity(t("保存排除判断：{title}", { title: p.title }));
  app.persist();
  app.closeModal();
  app.render();
  app.renderDrawer();
  app.toast(t("判断与理由已保存。"));
}

export function datasetModal(app: App, id = "", paperId = ""): void {
  const existing = app.state.datasets.find((d) => d.id === id);
  const d: Partial<Dataset> = existing ?? {};
  const repository = arr(d.repositories)[0] ?? { repository: d.repository ?? "", accession: d.accession ?? "" };
  const speciesRows = arr(d.species).length ? arr(d.species).map((s) => speciesRow(s)).join("") : speciesRow();
  const form = `<form id="dataset-form"><input type="hidden" name="id" value="${esc(id)}"><input type="hidden" name="paperId" value="${esc(paperId)}"><div class="form-grid">`
    + field(t("数据集名称"), "title", d.title ?? "", t("例如：拟南芥根尖发育数据"), true, "text", true)
    + `<div class="form-field full-width"><span>${t("研究物种")}</span><div class="species-rows" id="species-rows">${speciesRows}</div><button class="text-link" type="button" data-action="add-species-row">${I("plus", 14)} ${t("添加物种")}</button></div>`
    + `<label class="form-field full-width"><span>${t("组织 / 部位")}</span><input class="input" name="organs" value="${esc(joinList(d.organs, "，"))}" placeholder="${t("多个组织用逗号分隔，例如：根，叶，茎尖")}"></label>`
    + `<div class="form-field full-width"><span>${t("测序类型")}</span>${checkboxGroup("modalities", MODALITY_CHOICES, d.modalities, customField("modalitiesCustom", t("补充其他类型，多个用逗号分隔")))}</div>`
    + `<div class="form-field full-width"><span>${t("测序平台")}</span>${checkboxGroup("platforms", PLATFORM_CHOICES, d.platforms, customField("platformsCustom", t("补充其他平台，多个用逗号分隔")))}</div>`
    + `<label class="form-field"><span>${t("数据仓库")}</span><select class="input" name="repository">${option("", t("待补充"), repository.repository)}${options(REPOSITORIES, repository.repository)}</select></label>`
    + field("Accession", "accession", repository.accession ?? "", t("可先留空"))
    + `<label class="form-field"><span>${t("整理状态")}</span><select class="input" name="status">${option("pending", t("待整理"), d.status || "pending")}${option("identified", t("线索已记录"), d.status)}${option("verified", t("已人工核实"), d.status)}</select></label>`
    + `<label class="form-field"><span>${t("原始数据可用性")}</span><select class="input" name="availability">${options(AVAILABILITY, d.availability || "unknown")}</select></label>`
    + `<label class="form-field full-width"><span>${t("整理备注")}</span><textarea class="textarea" name="notes" rows="3" maxlength="50000">${esc(d.notes ?? "")}</textarea></label></div><p id="form-error" class="form-error" role="alert"></p></form>`;
  app.showModal(existing ? t("编辑数据集") : t("添加数据集"), form, `${cancelButton}<button class="btn btn-primary" type="submit" form="dataset-form">${t("保存数据集")}</button>`, true);
}

export function saveDataset(app: App, form: HTMLFormElement): void {
  const f = new FormData(form);
  const title = text(f, "title");
  const id = text(f, "id");
  const accession = text(f, "accession");
  const status = text(f, "status") || "pending";
  if (!title || (status === "verified" && !accession)) {
    setFormError(!title ? t("请填写数据集名称。") : t("标记为已核实前，请先填写仓库编号。"));
    return;
  }
  const repository = text(f, "repository").toLowerCase();
  if (accession && !repository) {
    setFormError(t("填写 Accession 时，请同时选择数据仓库。"));
    return;
  }
  const index = app.state.datasets.findIndex((d) => d.id === id);
  const existing = index >= 0 ? app.state.datasets[index] : undefined;
  const repositories = arr(existing?.repositories).map(clone);
  const previous: Partial<DatasetRepository> | null = repositories[0]
    ?? (existing && (existing.repository || existing.accession) ? { repository: existing.repository, accession: existing.accession } : null);
  if (!repository && !accession && !previous?.id) {
    // Nothing to keep: an unsaved empty entry would be rejected by the backend.
    repositories.splice(0, 1);
  } else if (previous || repository || accession) {
    repositories[0] = { ...emptyRepository(), ...previous, repository, accession };
  }
  const now = nowISO();
  const values = {
    title, species: readSpeciesRows(form), organs: splitList(f.get("organs")).slice(0, 50),
    modalities: mergeChecked(f, "modalities", "modalitiesCustom"), platforms: mergeChecked(f, "platforms", "platformsCustom"),
    repositories, repository, accession, status, availability: text(f, "availability") || "unknown", notes: text(f, "notes"), updatedAt: now,
  };
  let saved: Dataset;
  if (existing) {
    saved = normalizeDataset({ ...existing, ...values });
    // A changed status is the person's review, as the backend records it.
    if (status !== existing.status) saved.awaitingReview = false;
    app.state.datasets[index] = saved;
  } else {
    saved = normalizeDataset({ ...values, id: uid("DS"), matrixAvailability: "unknown", annotationAvailability: "unknown", cellsReported: null, cellCountUnit: "", cellCountScope: "", paperLinks: [], isDemo: false, addedAt: now });
    const pid = text(f, "paperId");
    if (app.paperById(pid)) saved.paperLinks.push({ paperId: pid, role: "generated", evidenceUrl: "", evidenceText: "", verified: false });
    app.state.datasets.push(saved);
  }
  app.logActivity(id ? t("更新数据集：{title}", { title }) : t("添加数据集：{title}", { title }));
  app.persist();
  app.closeModal();
  app.render();
  app.openDataset(saved.id);
  app.toast(t("数据集已保存。"));
}

export function linkDatasetModal(app: App, paperId: string): void {
  const available = app.state.datasets.filter((d) => !arr(d.paperLinks).some((l) => l.paperId === paperId));
  app.showModal(t("关联已有数据集"), `<p class="muted">${t("关联已有数据可保留复用关系，避免重复创建同一批数据。")}</p><form id="link-dataset-form"><input type="hidden" name="paperId" value="${esc(paperId)}"><label class="form-field"><span>${t("选择数据集")}</span><select class="input" name="datasetId" required><option value="">${t("请选择")}</option>${available.map((d) => option(d.id, d.title, "")).join("")}</select></label><label class="form-field"><span>${t("这篇论文与数据的关系")}</span><select class="input" name="role">${options(ROLE, "reused")}</select></label><p id="form-error" class="form-error" role="alert"></p></form>`, `<button class="btn btn-secondary" data-action="new-dataset-for-paper" data-id="${esc(paperId)}">${t("创建新数据集")}</button><button class="btn btn-primary" type="submit" form="link-dataset-form">${t("确认关联")}</button>`);
}

export function saveDatasetLink(app: App, form: HTMLFormElement): void {
  const f = new FormData(form);
  const d = app.state.datasets.find((x) => x.id === f.get("datasetId"));
  const pid = text(f, "paperId");
  if (!d || !app.paperById(pid)) {
    setFormError(t("请选择一个有效的数据集。"));
    return;
  }
  if (!arr(d.paperLinks).some((l) => l.paperId === pid)) d.paperLinks.push({ paperId: pid, role: text(f, "role"), evidenceUrl: "", evidenceText: "", verified: false });
  app.logActivity(t("关联数据集：{title}", { title: d.title }));
  app.persist();
  app.closeModal();
  app.render();
  app.renderDrawer();
  app.toast(t("文献与数据集已关联。"));
}

export function resourceModal(app: App, kind: string, id: string): void {
  app.showModal(t("添加资料链接"), `<form id="resource-form"><input type="hidden" name="kind" value="${esc(kind)}"><input type="hidden" name="ownerId" value="${esc(id)}"><label class="form-field"><span>${t("资料类型")}</span><select class="input" name="type">${options(RESOURCE_TYPES, kind === "paper" ? "fulltext" : "data")}</select></label>${field(t("资料名称"), "title", "", t("例如：期刊全文页面"), true)}${field(t("链接地址"), "url", "", "https://…", true, "url")}<p class="muted small">${t("仅保存链接，不会自动下载正文或数据文件。")}</p><p id="form-error" class="form-error" role="alert"></p></form>`, `${cancelButton}<button class="btn btn-primary" type="submit" form="resource-form">${t("保存链接")}</button>`);
}

export function saveResource(app: App, form: HTMLFormElement): void {
  const f = new FormData(form);
  const url = safeURL(f.get("url"));
  const title = text(f, "title");
  const kind = text(f, "kind");
  const ownerId = text(f, "ownerId");
  const type = text(f, "type");
  if (!url || !title) {
    setFormError(!title ? t("请填写资料名称。") : t("请填写有效的 HTTP 或 HTTPS 地址。"));
    return;
  }
  if (app.state.resources.some((r) => r.url === url && (kind === "paper" ? r.paperId === ownerId : r.datasetId === ownerId))) {
    setFormError(t("这条链接已经保存。"));
    return;
  }
  app.state.resources.push({ id: uid("R"), paperId: kind === "paper" ? ownerId : null, datasetId: kind === "dataset" ? ownerId : null, title, url, type, status: "linked", isDemo: false, addedAt: nowISO() });
  if (kind === "paper" && type === "fulltext") {
    const p = app.paperById(ownerId);
    if (p && p.fulltext === "missing") p.fulltext = "linked";
  }
  app.logActivity(t("保存资料入口：{title}", { title }));
  app.persist();
  app.closeModal();
  app.render();
  app.renderDrawer();
  app.toast(t("链接已保存；文件获取状态保持未下载。"));
}

export function resetDemoModal(app: App): void {
  app.showModal(t("恢复演示数据"), `<p>${t("这会清除本浏览器里的新增条目、修改和笔记，并恢复最初的示例内容。")}</p><p class="muted">${t("如有需要保留的整理结果，请先导出 JSON 备份。")}</p>`, `<button class="btn btn-secondary" data-action="export-json">${t("先导出备份")}</button>${cancelButton}<button class="btn btn-danger" data-action="confirm-reset">${t("恢复演示数据")}</button>`);
}

export function importSummaryModal(app: App, stats: ImportStats): void {
  const summary = `<p>${t("检查完成，将追加以下内容：")}</p><div class="import-summary"><div><strong>${stats.papers}</strong><span>${t("篇新文献")}</span></div><div><strong>${stats.datasets}</strong><span>${t("组新数据集")}</span></div><div><strong>${stats.resources}</strong><span>${t("个新链接")}</span></div></div><p class="muted">${t("{n} 条已存在记录保留原值，不覆盖笔记与整理状态。", { n: stats.duplicates })}${stats.renamed ? ` ${t("{n} 个冲突 ID 将自动分配新 ID。", { n: stats.renamed })}` : ""}</p><p class="muted small">${t("导入只合并记录，不会删除现有内容。导入文件中的文件保存标记仅作整理记录，本原型不会搬运 PDF 文件。")}</p>`;
  app.showModal(t("确认合并导入"), summary, `${cancelButton}<button class="btn btn-primary" data-action="confirm-import">${t("合并导入")}</button>`);
}
