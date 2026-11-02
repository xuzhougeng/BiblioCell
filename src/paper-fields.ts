// Focused paper editing. Panels stay mounted so navigation never loses a draft.
import type { App } from "./app.ts";
import type { Dataset, Paper } from "./mygo.ts";
import { ARTICLE_TYPES, MODALITY_CHOICES, PLATFORM_CHOICES, ROLE, arr, clone, esc, filterDatasets, joinList, normalizeDataset, nowISO, splitList, uid } from "./model.ts";
import { t } from "./i18n.ts";
import { $, $$, I, checkboxGroup, customField, field, mergeChecked, options, readSpeciesRows, setFormError, speciesRow, text } from "./ui.ts";

const fields = [
  ["species", t("研究物种")], ["organs", t("组织 / 部位")], ["modalities", t("测序类型")],
  ["platforms", t("测序平台")], ["articleTypes", t("文献类型")], ["datasets", t("关联数据集")],
  ["source", t("发现来源")], ["addedAt", t("加入时间")], ["year", t("发表年份")], ["publicationStatus", t("发表状态")],
] as const;
type FieldKey = typeof fields[number][0];
const publicationStatuses = { unknown: t("尚未确认"), preprint: t("预印本"), published: t("正式发表"), unpublished: t("未发表") };
const repositories = { geo: "GEO", gsa: "GSA / CNCB", sra: "SRA", ena: "ENA", bioproject: "BioProject", biosample: "BioSample", biostudies: "BioStudies", zenodo: "Zenodo", figshare: "Figshare", dryad: "Dryad", other: t("其他") };
const snapshots = new WeakMap<HTMLFormElement, Partial<Paper>>();

export function editableDetail(paperId: string, key: FieldKey, label: string, value: string, small = ""): string {
  return `<button type="button" class="detail-item detail-edit" data-action="edit-paper-field" data-id="${esc(paperId)}" data-field="${key}" aria-label="${esc(t("编辑{field}", { field: label }))}"><span>${esc(label)}${I("edit", 13)}</span><strong>${value}</strong>${small}</button>`;
}

export function matchingDatasets(datasets: Dataset[], paperId: string, query: string): Dataset[] {
  // Normalize legacy repository/accession fields as well as multi-repository records.
  return filterDatasets(datasets.filter(d => !arr(d.paperLinks).some(l => l.paperId === paperId)).map(normalizeDataset), query, () => "");
}

function datasetPicker(app: App, paperId: string): string {
  const linked = app.paperDatasets(paperId);
  const available = matchingDatasets(app.state.datasets, paperId, "");
  return (linked.length ? `<p class="muted">${t("已关联：{names}", { names: esc(linked.map(d => d.accession || d.title).join(" · ")) })}</p>` : "")
    + `<div class="field-mode"><button type="button" class="btn btn-secondary" data-action="field-dataset-mode" data-mode="existing" aria-pressed="true">${t("检索并关联")}</button><button type="button" class="btn btn-secondary" data-action="field-dataset-mode" data-mode="new" aria-pressed="false">${t("新建并关联")}</button></div>`
    + `<input type="hidden" name="datasetMode" value="existing"><fieldset data-dataset-mode="existing"><label class="form-field"><span>${t("检索数据集")}</span><input class="input" type="search" id="field-dataset-search" placeholder="${t("名称、Accession、物种或组织")}"></label><div class="dataset-picker">`
    + available.map(d => `<label class="dataset-choice" data-dataset-choice="${esc(d.id)}"><input type="checkbox" name="datasetIds" value="${esc(d.id)}"><span><strong>${esc(d.title)}</strong><small>${esc(arr(d.repositories).map(r => [r.repository.toUpperCase(), r.accession].filter(Boolean).join(" ")).join(" · ") || d.id)}</small></span></label>`).join("")
    + `</div><p class="muted" id="field-dataset-empty" ${available.length ? "hidden" : ""}>${t("没有匹配的数据集，可以新建并关联。")}</p></fieldset>`
    + `<fieldset data-dataset-mode="new" hidden disabled>${field(t("数据集名称"), "datasetTitle", "", t("例如：拟南芥根尖发育数据"))}<label class="form-field"><span>${t("数据仓库")}</span><select class="input" name="datasetRepository"><option value="">${t("待补充")}</option>${options(repositories, "")}</select></label>${field("Accession", "datasetAccession", "", t("可先留空"))}<p class="muted small">${t("物种、组织和测序信息将沿用当前文献，之后可在数据集详情继续完善。")}</p></fieldset>`
    + `<label class="form-field"><span>${t("这篇论文与数据的关系")}</span><select class="input" name="datasetRole">${options(ROLE, "reused")}</select></label>`;
}

function localDateTime(value: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? "" : new Date(date.getTime() - date.getTimezoneOffset() * 60000).toISOString().slice(0, -1);
}

export function paperFieldModal(app: App, id: string, selected = "species"): void {
  const p = app.paperById(id);
  if (!p) return;
  const content: Record<FieldKey, string> = {
    species: `<div class="species-rows" id="species-rows">${(arr(p.species).length ? p.species : [{}]).map(speciesRow).join("")}</div><button type="button" class="text-link" data-action="add-species-row">${I("plus", 15)} ${t("添加物种")}</button>`,
    organs: field(t("组织 / 部位（逗号分隔）"), "organs", joinList(p.organs, ", "), t("例如 根, 叶")),
    modalities: checkboxGroup("modalities", MODALITY_CHOICES, p.modalities, customField("modalitiesCustom", t("其他类型，逗号分隔"))),
    platforms: checkboxGroup("platforms", PLATFORM_CHOICES, p.platforms, customField("platformsCustom", t("其他平台，逗号分隔"))),
    articleTypes: checkboxGroup("articleTypes", ARTICLE_TYPES, p.articleTypes),
    datasets: datasetPicker(app, id),
    source: field(t("发现来源"), "source", p.source),
    addedAt: field(t("加入时间"), "addedAt", localDateTime(p.addedAt), "", false, "datetime-local").replace('type="datetime-local"', 'type="datetime-local" step="any"'),
    year: field(t("发表年份"), "year", p.year ? String(p.year) : "", t("例如 2026"), false, "number"),
    publicationStatus: `<label class="form-field"><span>${t("发表状态")}</span><select class="input" name="publicationStatus">${options(publicationStatuses, p.publicationStatus || "unknown")}</select></label>`,
  };
  const body = `<p class="muted small">${t("切换字段会保留草稿，点击保存后统一生效。← → 切换；输入时用 Alt + ← →。")}</p><form id="paper-fields-form" novalidate data-paper-id="${esc(id)}"><nav class="field-tabs" aria-label="${t("编辑字段")}">${fields.map(([key, label]) => `<button type="button" class="btn btn-secondary" data-action="select-paper-field" data-field="${key}">${esc(label)}</button>`).join("")}</nav>`
    + fields.map(([key, label]) => `<section class="field-panel" data-field-panel="${key}" aria-label="${esc(label)}" hidden>${content[key]}</section>`).join("")
    + `<p id="form-error" class="form-error" role="alert"></p></form>`;
  app.showModal(t("编辑信息"), body, `<div class="field-step"><button type="button" class="btn btn-secondary" data-action="step-paper-field" data-step="-1" aria-label="${t("上一个字段")}">${I("chevron-left", 16)}</button><button type="button" class="btn btn-secondary" data-action="step-paper-field" data-step="1" aria-label="${t("下一个字段")}">${I("chevron-right", 16)}</button></div><button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-primary" type="submit" form="paper-fields-form">${t("保存修改")}</button>`, true);
  const form = $<HTMLFormElement>("#paper-fields-form");
  if (!form) return;
  snapshots.set(form, readPaperFields(form));
  selectPaperField(fields.some(([key]) => key === selected) ? selected : "species");
}

export function selectPaperField(key: string): void {
  const form = $("#paper-fields-form");
  if (!form || !fields.some(([id]) => id === key)) return;
  form.dataset.field = key;
  $$("[data-field-panel]", form).forEach(panel => { (panel as HTMLElement).hidden = (panel as HTMLElement).dataset.fieldPanel !== key; });
  $$("[data-action=select-paper-field]", form).forEach(tab => tab.setAttribute("aria-pressed", String((tab as HTMLElement).dataset.field === key)));
  const title = $("#modal-title");
  if (title) title.textContent = t("编辑{field}", { field: fields.find(([id]) => id === key)![1] });
  $<HTMLElement>(`[data-action=select-paper-field][data-field="${key}"]`, form)?.focus();
}

export function stepPaperField(step: number): void {
  const current = $("#paper-fields-form")?.dataset.field;
  const index = fields.findIndex(([key]) => key === current);
  if (index >= 0) selectPaperField(fields[(index + step + fields.length) % fields.length]![0]);
}

export function paperFieldKey(e: KeyboardEvent): boolean {
  if (!$("#paper-fields-form") || e.isComposing || e.ctrlKey || e.metaKey || e.shiftKey || !["ArrowLeft", "ArrowRight"].includes(e.key)) return false;
  const target = e.target as HTMLElement | null;
  if (!e.altKey && target?.closest("input,textarea,select,[contenteditable=true]")) return false;
  e.preventDefault();
  stepPaperField(e.key === "ArrowRight" ? 1 : -1);
  return true;
}

export function datasetPickerMode(mode: string): void {
  const form = $("#paper-fields-form");
  if (!form || !["new", "existing"].includes(mode)) return;
  $<HTMLInputElement>("[name=datasetMode]", form)!.value = mode;
  $$<HTMLFieldSetElement>("[data-dataset-mode]", form).forEach(panel => { panel.hidden = panel.disabled = panel.dataset.datasetMode !== mode; });
  $$<HTMLElement>("[data-action=field-dataset-mode]", form).forEach(button => button.setAttribute("aria-pressed", String(button.dataset.mode === mode)));
  $<HTMLSelectElement>("[name=datasetRole]", form)!.value = mode === "new" ? "generated" : "reused";
}

export function searchDatasetPicker(app: App, query: string): void {
  const form = $("#paper-fields-form");
  if (!form) return;
  const ids = new Set(matchingDatasets(app.state.datasets, form.dataset.paperId ?? "", query).map(d => d.id));
  $$<HTMLElement>("[data-dataset-choice]", form).forEach(row => { row.hidden = !ids.has(row.dataset.datasetChoice!); });
  const empty = $("#field-dataset-empty");
  if (empty) empty.hidden = ids.size > 0;
}

function readPaperFields(form: HTMLFormElement): Partial<Paper> {
  const data = new FormData(form);
  return {
    species: readSpeciesRows(form), organs: splitList(data.get("organs")).slice(0, 50),
    modalities: mergeChecked(data, "modalities", "modalitiesCustom"), platforms: mergeChecked(data, "platforms", "platformsCustom"),
    articleTypes: data.getAll("articleTypes").map(String), source: text(data, "source"), addedAt: text(data, "addedAt"),
    year: text(data, "year") ? Number(text(data, "year")) : null, publicationStatus: text(data, "publicationStatus"),
  };
}

export function savePaperFields(app: App, form: HTMLFormElement): void {
  const paper = app.paperById(form.dataset.paperId ?? "");
  if (!paper) { setFormError(t("原条目不存在，请关闭弹窗后重试。")); return; }
  const data = new FormData(form);
  const values = readPaperFields(form);
  const before = snapshots.get(form);
  if (!before) return;
  const fail = (key: FieldKey, message: string) => { selectPaperField(key); setFormError(message); };
  if (values.year !== null && (!Number.isInteger(values.year) || values.year! < 1800 || values.year! > 2100)) return fail("year", t("请填写 1800–2100 之间的年份，或留空。"));
  if (values.addedAt !== before.addedAt && (!values.addedAt || Number.isNaN(new Date(values.addedAt).getTime()))) return fail("addedAt", t("请填写有效的加入时间。"));
  if ($$<HTMLInputElement>('[name="speciesLabel"]', form).some(input => input.value.trim() && !input.closest(".species-row")?.querySelector<HTMLInputElement>('[name="speciesName"]')?.value.trim())) return fail("species", t("请为物种填写学名。"));
  const patch = Object.fromEntries(Object.entries(values).filter(([key, value]) => JSON.stringify(value) !== JSON.stringify(before[key as keyof Paper]))) as Partial<Paper>;
  if (patch.addedAt) patch.addedAt = new Date(patch.addedAt).toISOString();
  const nextPaper = { ...paper, ...patch };
  const selected = data.getAll("datasetIds").map(String);
  const role = text(data, "datasetRole");
  const create = text(data, "datasetMode") === "new";
  let created: Dataset | undefined;
  if (create) {
    const title = text(data, "datasetTitle"), repository = text(data, "datasetRepository"), accession = text(data, "datasetAccession");
    if (!title) return fail("datasets", t("请填写数据集名称。"));
    if (accession && !repository) return fail("datasets", t("填写 Accession 时，请同时选择数据仓库。"));
    if (accession && app.state.datasets.some(d => normalizeDataset(d).repositories.some(r => r.repository === repository && r.accession.toLowerCase() === accession.toLowerCase()))) return fail("datasets", t("这个仓库编号已存在，请检索并关联已有数据集。"));
    created = normalizeDataset({ id: uid("DS"), title, repository, accession, species: clone(nextPaper.species), organs: clone(nextPaper.organs), modalities: clone(nextPaper.modalities), platforms: clone(nextPaper.platforms), paperLinks: [{ paperId: paper.id, role, evidenceUrl: "", evidenceText: "", verified: false }], addedAt: nowISO(), updatedAt: nowISO() });
  } else if (selected.some(id => !app.state.datasets.some(d => d.id === id))) return fail("datasets", t("请选择一个有效的数据集。"));
  if (!Object.hasOwn(ROLE, role)) return fail("datasets", t("请选择有效的数据关联关系。"));
  // All validation precedes mutations, including creation and linking.
  if (Object.keys(patch).length) Object.assign(paper, patch, { updatedAt: nowISO() });
  if (created) app.state.datasets.push(created);
  if (!create) for (const id of selected) {
    const dataset = app.state.datasets.find(d => d.id === id)!;
    if (!arr(dataset.paperLinks).some(link => link.paperId === paper.id)) {
      dataset.paperLinks = [...arr(dataset.paperLinks), { paperId: paper.id, role, evidenceUrl: "", evidenceText: "", verified: false }];
      dataset.updatedAt = nowISO();
    }
  }
  app.logActivity(t("更新文献信息：{title}", { title: paper.title }), "edit");
  app.persist();
  app.closeModal();
  app.render();
  app.renderDrawer();
  app.toast(t("修改已保存。"));
}
