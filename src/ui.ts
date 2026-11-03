// Small HTML fragment builders and DOM helpers. Every dynamic value passes
// through esc(); fragments that are already HTML are composed as strings.
import { speciesText } from "./species.ts";
import type { Dataset, SpeciesEntry } from "./mygo.ts";
import { icon } from "./icons.ts";
import { FULL, SCREEN, arr, esc, splitList, type Filters } from "./model.ts";
import { t, tv } from "./i18n.ts";
import { distinctChoices, plantChoices, rememberChoices, repositoryChoices, type ChoiceKind } from "./research-options.ts";
import type { LibraryState } from "./model.ts";

export const I = icon;
export const $ = <T extends Element = HTMLElement>(selector: string, root: ParentNode = document): T | null => root.querySelector<T>(selector);
export const $$ = <T extends Element = HTMLElement>(selector: string, root: ParentNode = document): T[] => Array.from(root.querySelectorAll<T>(selector));

export const badge = (label: string, color = "gray"): string => `<span class="badge ${color}">${esc(label)}</span>`;
export const demoBadge = (item: { isDemo: boolean }): string => (item.isDemo ? `<span class="demo-label">${t("示例")}</span>` : "");
export const screenBadge = (status: string): string =>
  badge(SCREEN[status] ?? t("待判断"), status === "included" ? "green" : status === "pending" || status === "uncertain" ? "amber" : "gray");
export const fullBadge = (status: string): string =>
  `<span class="status-dot ${status === "missing" ? "amber" : "green"}"></span><span>${esc(FULL[status] ?? t("待补入口"))}</span>`;
export const datasetStatusBadge = (d: Dataset): string =>
  badge(d.status === "verified" ? t("已核实") : d.status === "identified" ? t("线索已记录") : t("待整理"), d.status === "verified" ? "green" : "gray");

export const empty = (title: string, text: string, action = ""): string =>
  `<div class="empty-state"><div class="empty-icon">${I("search", 28)}</div><h3>${esc(title)}</h3><p>${esc(text)}</p>${action}</div>`;

export const option = (value: string, label: string, selected: unknown): string =>
  `<option value="${esc(value)}"${String(value) === String(selected ?? "") ? " selected" : ""}>${esc(label)}</option>`;

export const options = (entries: Record<string, string>, selected: unknown): string =>
  Object.entries(entries).map(([value, label]) => option(value, label, selected)).join("");

type FilterValue = string | { value: string; label: string };
// A filter renders as a "key: value" chip. The native <select> sits invisibly on
// top so it keeps keyboard access and the platform dropdown; the visible text is
// drawn by the chip, which an active filter can clear with its own × button.
export function selectFilter(filters: Filters, name: keyof Filters, key: string, values: FilterValue[]): string {
  const current = String(filters[name] ?? "");
  const items = values.map((v) => (typeof v === "object" ? v : { value: v, label: v }));
  const selected = items.find((v) => v.value === current);
  const on = Boolean(current && selected);
  const label = on ? selected!.label : t("全部");
  return `<div class="filter-chip${on ? " on" : ""}"><label class="filter-chip-body"><span class="filter-chip-key">${esc(key)}</span><span class="filter-chip-value">${esc(label)}</span>${I("chevron-down", 13)}`
    + `<select class="filter-chip-select" data-filter="${name}" aria-label="${esc(key)}">${option("", t("全部"), current)}${items.map((v) => option(v.value, v.label, current)).join("")}</select></label>`
    + (on ? `<button class="filter-chip-clear" data-action="clear-filter" data-filter-key="${name}" aria-label="${esc(t("清除{key}筛选", { key }))}">${I("x", 12)}</button>` : "")
    + `</div>`;
}

/** sortChip is the sort order drawn like a filter chip, so it sits in the same row as the filters. */
export function sortChip(current: string, options: [string, string][]): string {
  const label = options.find(([value]) => value === current)?.[1] ?? options[0]?.[1] ?? "";
  return `<div class="filter-chip sort-chip"><label class="filter-chip-body"><span class="filter-chip-key">${t("排序")}</span><span class="filter-chip-value">${esc(label)}</span>${I("chevron-down", 13)}`
    + `<select class="filter-chip-select" data-filter="sort" aria-label="${t("排序方式")}">${options.map(([value, text]) => option(value, text, current)).join("")}</select></label></div>`;
}

export function modalityBadges(item: { modalities: string[] | null | undefined }): string {
  const list = arr(item.modalities);
  if (!list.length) return badge(t("待补"));
  const color = (m: string) => (m === "snRNA-seq" || m === "snATAC-seq" ? "purple" : m === "scATAC-seq" ? "amber" : "blue");
  return `<div class="tech-badges">${list.map((m) => badge(tv(m), color(m))).join("")}</div>`;
}

export function progressRow(label: string, value: number, total: number, color = ""): string {
  const percentage = total ? Math.round((value / total) * 100) : 0;
  return `<div class="progress-row"><div class="progress-label"><span>${esc(label)}</span><span><strong>${value}</strong> / ${total}</span></div><div class="progress-track"><div class="progress-fill ${color}" style="width:${percentage}%"></div></div></div>`;
}

export function field(label: string, name: string, value = "", placeholder = "", required = false, type = "text", span = false): string {
  return `<label class="form-field${span ? " full-width" : ""}"><span>${esc(label)}${required ? ' <b class="required">*</b>' : ""}</span><input class="input" name="${name}" type="${type}" value="${esc(value)}" placeholder="${esc(placeholder)}"${required ? " required" : ""} maxlength="${name === "title" ? 2000 : 500}"></label>`;
}

export function checkboxGroup(name: string, values: string[], selected: string[] | null | undefined, extraCustom = ""): string {
  const chosen = arr(selected);
  const items = [...new Set([...values, ...chosen])];
  return `<div class="checkbox-group" role="group">${items.map((v) => `<label class="checkbox-choice"><input type="checkbox" name="${name}" value="${esc(v)}"${chosen.includes(v) ? " checked" : ""}><span>${esc(tv(v))}</span></label>`).join("")}</div>${extraCustom}`;
}

export const customField = (name: string, placeholder: string): string =>
  `<div class="metadata-custom" data-choice-kind="${name.startsWith("modalities") ? "modalities" : "platforms"}"><input class="input checkbox-custom" type="text" name="${name}" placeholder="${esc(placeholder)}" maxlength="500" aria-label="${t("自定义补充")}"><button type="button" class="btn btn-secondary" data-action="add-metadata-choice">${t("新增并选中")}</button></div>`;

export function speciesRow(entry: Partial<SpeciesEntry> = {}, choices = plantChoices()): string {
  const selected = choices.find(s => s.name.toLowerCase() === entry.name?.trim().toLowerCase());
  return `<div class="species-row"><select class="input species-picker" data-species-picker aria-label="${t("选择植物物种")}">${option("", t("选择常见植物，或在下方填写其他物种"), selected?.name ?? "")}${choices.map(s => `<option value="${esc(s.name)}" data-label="${esc(s.label)}"${s === selected ? " selected" : ""}>${esc(speciesText(s, true))}</option>`).join("")}</select><input class="input" name="speciesName" type="text" value="${esc(entry.name ?? "")}" placeholder="${t("例如 Arabidopsis thaliana")}" maxlength="500" aria-label="${t("物种学名")}"><input class="input" name="speciesLabel" type="text" value="${esc(entry.label ?? "")}" placeholder="${t("例如 拟南芥")}" maxlength="500" aria-label="${t("物种中文名")}"><button type="button" class="btn-icon" data-action="remove-species-row" aria-label="${t("移除该物种")}">${I("x", 16)}</button></div>`;
}

export function selectPlant(select: HTMLSelectElement): void {
  if (!select.value) return; // Returning to manual entry keeps the current draft.
  const row = select.closest(".species-row");
  if (!row) return;
  $<HTMLInputElement>("[name=speciesName]", row)!.value = select.value;
  $<HTMLInputElement>("[name=speciesLabel]", row)!.value = select.selectedOptions[0]?.dataset.label ?? "";
}

export function repositoryField(state: LibraryState, name: string, selected = ""): string {
  return `<div class="form-field repository-field"><label><span>${t("数据仓库")}</span><select class="input" name="${name}">${option("", t("待补充"), selected)}${options(repositoryChoices(state, selected), selected.toLowerCase())}</select></label><div class="metadata-custom" data-choice-kind="repositories"><input class="input" name="${name}Custom" maxlength="100" placeholder="${t("新增仓库名称")}" aria-label="${t("新增仓库名称")}"><button type="button" class="btn btn-secondary" data-action="add-metadata-choice">${t("新增并选中")}</button></div></div>`;
}

export function readRepository(data: FormData, name: string): string {
  return (text(data, `${name}Custom`) || text(data, name)).toLowerCase();
}

/** Add in place so the current modal and every draft field remain mounted. */
export function addMetadataChoice(button: HTMLElement): void {
  const group = button.closest<HTMLElement>(".metadata-custom");
  const input = group?.querySelector<HTMLInputElement>("input");
  const kind = group?.dataset.choiceKind as ChoiceKind | undefined;
  if (!input || !kind || !input.value.trim()) return;
  const values = kind === "repositories" ? [input.value.trim()] : distinctChoices(splitList(input.value));
  if (kind === "repositories") {
    const select = group!.parentElement!.querySelector("select")!;
    for (const value of values) {
      const key = value.toLowerCase();
      if (!Array.from(select.options).some(option => option.value === key)) select.add(new Option(value, key));
      select.value = key;
    }
  } else {
    const checkboxes = group!.previousElementSibling!;
    for (const value of values) {
      const existing = Array.from(checkboxes.querySelectorAll<HTMLInputElement>("input")).find(input => input.value.toLowerCase() === value.toLowerCase());
      if (existing) existing.checked = true;
      else checkboxes.insertAdjacentHTML("beforeend", `<label class="checkbox-choice"><input type="checkbox" name="${kind}" value="${esc(value)}" checked><span>${esc(value)}</span></label>`);
    }
  }
  rememberChoices(kind, values);
  input.value = "";
  input.focus();
}

/** readSpeciesRows collects the dynamic species rows of a form, dropping rows without a scientific name. */
export function readSpeciesRows(form: HTMLFormElement): SpeciesEntry[] {
  return $$(".species-row", form)
    .map((row) => ({
      name: ($<HTMLInputElement>("[name=speciesName]", row)?.value ?? "").trim(),
      label: ($<HTMLInputElement>("[name=speciesLabel]", row)?.value ?? "").trim(),
    }))
    .filter((s) => s.name);
}

/** mergeChecked joins checkbox values with the comma-separated custom input, deduplicated, at most 50. */
export function mergeChecked(data: FormData, name: string, custom: string): string[] {
  return distinctChoices([...data.getAll(name).map(String), ...splitList(data.get(custom))]).slice(0, 50);
}

export const text = (data: FormData, name: string): string => String(data.get(name) ?? "").trim();

export function setFormError(message: string): void {
  const el = $("#form-error");
  if (el) el.textContent = message;
}

export function downloadFile(filename: string, content: string, mime: string): void {
  const url = URL.createObjectURL(new Blob([content], { type: mime }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 5000);
}

export const errorMessage = (error: unknown, fallback: string): string =>
  error instanceof Error && error.message ? error.message : fallback;
