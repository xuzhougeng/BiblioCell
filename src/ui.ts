// Small HTML fragment builders and DOM helpers. Every dynamic value passes
// through esc(); fragments that are already HTML are composed as strings.
import type { Dataset, SpeciesEntry } from "./mygo.ts";
import { icon } from "./icons.ts";
import { FULL, SCREEN, arr, esc, splitList, type Filters } from "./model.ts";

export const I = icon;
export const $ = <T extends Element = HTMLElement>(selector: string, root: ParentNode = document): T | null => root.querySelector<T>(selector);
export const $$ = <T extends Element = HTMLElement>(selector: string, root: ParentNode = document): T[] => Array.from(root.querySelectorAll<T>(selector));

export const badge = (label: string, color = "gray"): string => `<span class="badge ${color}">${esc(label)}</span>`;
export const demoBadge = (item: { isDemo: boolean }): string => (item.isDemo ? '<span class="demo-label">示例</span>' : "");
export const screenBadge = (status: string): string =>
  badge(SCREEN[status] ?? "待判断", status === "included" ? "green" : status === "pending" || status === "uncertain" ? "amber" : "gray");
export const fullBadge = (status: string): string =>
  `<span class="status-dot ${status === "missing" ? "amber" : "green"}"></span><span>${esc(FULL[status] ?? "待补入口")}</span>`;
export const datasetStatusBadge = (d: Dataset): string =>
  badge(d.status === "verified" ? "已核实" : d.status === "identified" ? "线索已记录" : "待整理", d.status === "verified" ? "green" : "gray");

export const empty = (title: string, text: string, action = ""): string =>
  `<div class="empty-state"><div class="empty-icon">${I("search", 28)}</div><h3>${esc(title)}</h3><p>${esc(text)}</p>${action}</div>`;

export const option = (value: string, label: string, selected: unknown): string =>
  `<option value="${esc(value)}"${String(value) === String(selected ?? "") ? " selected" : ""}>${esc(label)}</option>`;

export const options = (entries: Record<string, string>, selected: unknown): string =>
  Object.entries(entries).map(([value, label]) => option(value, label, selected)).join("");

type FilterValue = string | { value: string; label: string };
export function selectFilter(filters: Filters, name: keyof Filters, label: string, values: FilterValue[]): string {
  const current = filters[name];
  const items = values.map((v) => (typeof v === "object" ? option(v.value, v.label, current) : option(v, v, current))).join("");
  return `<label class="filter-field"><span class="sr-only">${esc(label)}</span><select class="filter-select" data-filter="${name}" aria-label="${esc(label)}">${option("", label, current)}${items}</select></label>`;
}

export function modalityBadges(item: { modalities: string[] | null | undefined }): string {
  const list = arr(item.modalities);
  if (!list.length) return badge("待补");
  const color = (m: string) => (m === "snRNA-seq" || m === "snATAC-seq" ? "purple" : m === "scATAC-seq" ? "amber" : "blue");
  return `<div class="tech-badges">${list.map((m) => badge(m, color(m))).join("")}</div>`;
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
  return `<div class="checkbox-group" role="group">${items.map((v) => `<label class="checkbox-choice"><input type="checkbox" name="${name}" value="${esc(v)}"${chosen.includes(v) ? " checked" : ""}><span>${esc(v)}</span></label>`).join("")}</div>${extraCustom}`;
}

export const customField = (name: string, placeholder: string): string =>
  `<input class="input checkbox-custom" type="text" name="${name}" placeholder="${esc(placeholder)}" maxlength="500" aria-label="自定义补充">`;

export const speciesRow = (entry: Partial<SpeciesEntry> = {}): string =>
  `<div class="species-row"><input class="input" name="speciesName" type="text" value="${esc(entry.name ?? "")}" placeholder="例如 Arabidopsis thaliana" maxlength="500" aria-label="物种学名"><input class="input" name="speciesLabel" type="text" value="${esc(entry.label ?? "")}" placeholder="例如 拟南芥" maxlength="500" aria-label="物种中文名"><button type="button" class="btn-icon" data-action="remove-species-row" aria-label="移除该物种">${I("x", 16)}</button></div>`;

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
  return [...new Set([...data.getAll(name).map(String), ...splitList(data.get(custom))])].slice(0, 50);
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
