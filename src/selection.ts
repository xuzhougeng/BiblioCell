// Multi-select on the papers table: a checkbox column, select-all for the
// page, and a floating bar with what can be done to the chosen papers at once.
// Ticking a box only touches the row and the bar, never the whole table, so
// the keyboard focus and scroll position stay where they are.
import type { App } from "./app.ts";
import { exportModal } from "./export.ts";
import { esc } from "./model.ts";
import { openBatch } from "./screen-batch.ts";
import { $, $$, I } from "./ui.ts";
import { t } from "./i18n.ts";

/** selectCell is the checkbox at the start of a row. */
export const selectCell = (id: string, title: string, checked: boolean): string =>
  `<td class="select-cell"><input type="checkbox" data-select-paper="${esc(id)}" aria-label="${esc(t("选择 {title}", { title }))}"${checked ? " checked" : ""}></td>`;

/** selectHeader is the select-all box for the rows on this page. */
export const selectHeader = (allChecked: boolean, any: boolean): string =>
  `<th class="col-select"><input type="checkbox" data-select-all aria-label="${t("选择本页全部文献")}"${allChecked && any ? " checked" : ""}></th>`;

/** bulkBar floats above the table footer while papers are ticked. */
export function bulkBar(app: App): string {
  const n = app.selectedPapers.size;
  const ids = [...app.selectedPapers].join(",");
  return `<div class="bulk-bar" id="bulk-bar" role="toolbar" aria-label="${t("批量操作")}"${n ? "" : " hidden"}>`
    + `<span class="bulk-bar-count" role="status">${t("已勾选 {n} 篇", { n })}</span>`
    + `<span class="bulk-bar-actions"><button class="btn btn-primary" data-action="bulk-screen" data-id="${esc(ids)}">${I("sparkle", 16)} ${t("批量 AI 筛选")}</button>`
    + `<button class="btn btn-secondary" data-action="bulk-export">${I("download", 16)} ${t("导出")}</button>`
    + `<button class="btn btn-danger" data-action="bulk-delete">${I("trash", 16)} ${t("删除")}</button></span>`
    + `<button class="text-link bulk-bar-clear" data-action="bulk-clear">${I("x", 13)} ${t("清除选择")}</button></div>`;
}

/** syncSelection brings the rows, the header box and the bar in line with the set. */
export function syncSelection(app: App): void {
  const boxes = $$<HTMLInputElement>("input[data-select-paper]");
  for (const box of boxes) {
    const on = app.selectedPapers.has(box.dataset.selectPaper ?? "");
    box.checked = on;
    box.closest("tr")?.classList.toggle("is-selected", on);
  }
  const all = $<HTMLInputElement>("input[data-select-all]");
  if (all) {
    const checked = boxes.filter((b) => b.checked).length;
    all.checked = boxes.length > 0 && checked === boxes.length;
    all.indeterminate = checked > 0 && checked < boxes.length;
  }
  const bar = $("#bulk-bar");
  if (bar) bar.outerHTML = bulkBar(app);
}

/** selectionChange handles the checkboxes; true when the change was one of them. */
export function selectionChange(app: App, target: HTMLElement): boolean {
  if (!(target instanceof HTMLInputElement)) return false;
  if (target.hasAttribute("data-select-all")) {
    for (const box of $$<HTMLInputElement>("input[data-select-paper]")) {
      const id = box.dataset.selectPaper ?? "";
      if (target.checked) app.selectedPapers.add(id); else app.selectedPapers.delete(id);
    }
    syncSelection(app);
    return true;
  }
  const id = target.dataset.selectPaper;
  if (id === undefined) return false;
  if (target.checked) app.selectedPapers.add(id); else app.selectedPapers.delete(id);
  syncSelection(app);
  return true;
}

/** pruneSelection drops papers that no longer exist, before a render. */
export function pruneSelection(app: App): void {
  for (const id of app.selectedPapers) if (!app.paperById(id)) app.selectedPapers.delete(id);
}

export const selectionActions = (app: App): Record<string, (el: HTMLElement, id: string) => void> => ({
  "bulk-screen": () => { const ids = [...app.selectedPapers]; if (ids.length) openBatch(app, "batch", ids); },
  "bulk-export": () => { if (app.selectedPapers.size) exportModal(app, "papers"); },
  "bulk-delete": () => {
    const n = app.selectedPapers.size;
    if (!n) return;
    app.showModal(t("删除文献"), `<p>${t("确定删除已勾选的 {n} 篇文献？", { n })}</p><p>${t("文献将移入回收站，附件、笔记和关联保留。来源更新与重新导入不会自动加回，可在设置中的回收站恢复。")}</p>`,
      `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-danger" data-action="confirm-delete-selected">${t("确认删除 {n} 篇", { n })}</button>`);
  },
  "confirm-delete-selected": (el) => { void app.deletePapers([...app.selectedPapers], el); },
  "bulk-clear": () => { app.selectedPapers.clear(); syncSelection(app); },
});
