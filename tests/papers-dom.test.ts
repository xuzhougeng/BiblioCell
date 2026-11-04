// Papers list interactions in a real DOM: paging, sorting, filter chips and the arrow keys.
import { $, $$, change, keydown, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import { filterPapers, type LibraryState, type Paper } from "../src/model.ts";

const previewEnv = (): Env => ({
  desktop: false, backend: false, readOnly: false, web: null, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" }, quitOnWindowClose: false, sourceCheck: { enabled: false, prompt: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

/** papersApp opens the demo library (14 papers, page size 10) on the papers page. */
function papersApp(state: LibraryState = demoState()) {
  resetDom("#papers");
  const saved: LibraryState[] = [];
  const app = new App({ initial: state, store: { load: async () => state, save: async (next) => { saved.push(next); } }, env: previewEnv() });
  app.start();
  return { app, state, saved };
}

const rowIds = (): string[] => $$("tbody tr[data-action=open-paper]").map((tr) => tr.dataset.id ?? "");
const years = (state: LibraryState): number[] => rowIds().map((id) => Number(state.papers.find((p) => p.id === id)!.year));
const sortedAsc = (values: number[]) => values.every((v, i) => i === 0 || values[i - 1]! <= v);

test("AI calibration and paper export remain reachable from the library", () => {
  const { app } = papersApp();
  $('[data-action="export-papers"]').click();
  assert.ok(document.querySelector('#modal-root [role="dialog"]'));
  $('#modal-root button[data-action="close-modal"]').click();

  $('[data-action="calibration-open"]').click();
  assert.equal(app.calibrationView, true);
  assert.ok(document.querySelector("#calibration-panel"));
  $('[data-action="all-papers"]').click();
  assert.equal(app.calibrationView, false);
  $('[data-action="export-papers"]').click();
  assert.ok(document.querySelector('#modal-root [role="dialog"]'));
});

test("the table pages through the list and clamps the page when the list shrinks", () => {
  const { app, state } = papersApp();
  assert.equal(state.papers.length, 14);
  assert.equal(rowIds().length, 10);
  assert.deepEqual(rowIds(), state.papers.slice(0, 10).map((p) => p.id));
  assert.match(text(".table-footer > span"), /1–10 \/ 14/);
  assert.equal($<HTMLButtonElement>('.pager-pages button[aria-label="上一页"]').disabled, true);

  $('.page-btn[data-id="2"]').click();
  assert.equal(app.paperPage, 2);
  assert.deepEqual(rowIds(), state.papers.slice(10).map((p) => p.id));
  assert.match(text(".table-footer > span"), /11–14 \/ 14/);
  assert.equal($<HTMLButtonElement>('.pager-pages button[aria-label="下一页"]').disabled, true);
  assert.equal($('.page-btn.active').dataset.id, "2");

  // A bigger page holds everything and returns to page 1.
  change($("select[data-page-size=papers]"), "50");
  assert.equal(app.paperPage, 1);
  assert.equal(rowIds().length, 14);
  assert.equal(localStorage.getItem("bibliocell-page-size"), "50");

  // Back to 10 per page, on page 2, a filter that leaves fewer than one page clamps to page 1.
  change($("select[data-page-size=papers]"), "10");
  $('.page-btn[data-id="2"]').click();
  change($("select[data-filter=screening]"), "pending");
  assert.equal(app.paperPage, 1);
  assert.ok(rowIds().length > 0 && rowIds().length <= 10);
});

test("column headers sort the list and flip direction; the sort select agrees", () => {
  const { app, state } = papersApp();
  $('.th-sort[data-key="year"]').click();
  assert.equal(app.filters.sort, "year-desc");
  assert.equal($('th[aria-sort]').getAttribute("aria-sort"), "descending");
  assert.ok(sortedAsc([...years(state)].reverse()), `years descending: ${years(state)}`);
  assert.equal($<HTMLSelectElement>("select[data-filter=sort]").value, "year-desc");

  $('.th-sort[data-key="year"]').click();
  assert.equal(app.filters.sort, "year-asc");
  assert.equal($('th[aria-sort]').getAttribute("aria-sort"), "ascending");
  assert.ok(sortedAsc(years(state)), `years ascending: ${years(state)}`);

  // The sort control is a chip in the filter row, and its visible text follows the choice.
  assert.ok($("select[data-filter=sort]").closest(".filter-row .sort-chip"));
  change($("select[data-filter=sort]"), "title-desc");
  assert.equal(app.filters.sort, "title-desc");
  assert.equal(text(".sort-chip .filter-chip-value"), "标题 Z→A");
  // The rows follow the model's order (its comparators have their own tests).
  assert.deepEqual(rowIds(), filterPapers(state.papers, app.filters).slice(0, 10).map((p) => p.id));
  assert.notDeepEqual(rowIds(), state.papers.slice(0, 10).map((p) => p.id));
  assert.equal($('th[aria-sort]').getAttribute("aria-sort"), "descending");
  assert.equal($(".th-sort.on").dataset.key, "title");

  // Sorting from page 2 lands on page 1, so the first rows of the new order are visible.
  $('.page-btn[data-id="2"]').click();
  $('.th-sort[data-key="title"]').click();
  assert.equal(app.paperPage, 1);
  assert.equal(app.filters.sort, "title-asc");
});

test("filter chips narrow the list, show what is active and clear one by one or all at once", () => {
  const { app, state } = papersApp();
  const pending = state.papers.filter((p) => p.screening === "pending").map((p) => p.id);
  change($("select[data-filter=screening]"), "pending");
  assert.deepEqual(rowIds(), pending);
  const chip = $("select[data-filter=screening]").closest<HTMLElement>(".filter-chip")!;
  assert.ok(chip.classList.contains("on"));
  assert.equal(text(".filter-chip-value", chip), "待判断");
  assert.equal(text(".filter-clear-all"), "清空 1 项筛选");

  const species = state.papers.find((p) => p.id === pending[0])!.species[0]!.name;
  change($("select[data-filter=species]"), species);
  assert.ok(rowIds().every((id) => pending.includes(id)));
  assert.ok(rowIds().every((id) => state.papers.find((p) => p.id === id)!.species.some((s) => s.name === species)));
  assert.equal($$(".filter-chip.on").length, 2);
  assert.equal(text(".filter-clear-all"), "清空 2 项筛选");

  // The × on a chip clears only that filter.
  $('.filter-chip-clear[data-filter-key="species"]').click();
  assert.equal(app.filters.species, "");
  assert.equal(app.filters.screening, "pending");
  assert.deepEqual(rowIds(), pending);

  $(".filter-clear-all").click();
  assert.equal(app.filters.screening, "");
  assert.equal(rowIds().length, 10);
  assert.equal(document.querySelector(".filter-clear-all"), null);

  // The tabs are filters too, combined with the chips.
  $("[data-action=starred-papers]").click();
  assert.deepEqual(rowIds(), state.papers.filter((p) => p.starred).map((p) => p.id));
  assert.ok($("[data-action=starred-papers]").classList.contains("active"));
  change($("select[data-filter=screening]"), "included");
  assert.deepEqual(rowIds(), state.papers.filter((p) => p.starred && p.screening === "included").map((p) => p.id));
  $("[data-action=all-papers]").click();
  assert.equal(app.filters.starred, false);
  assert.equal(app.filters.screening, "included", "the tab leaves the chips alone");
  $(".filter-clear-all").click();

  // An empty result still offers a way out.
  app.filters.search = "no such paper zzz";
  app.render();
  assert.equal(rowIds().length, 0);
  assert.ok($("[data-action=clear-filters]"));
});

test("arrow keys turn the page and move the row cursor; typing in a field is left alone", () => {
  const { app, state } = papersApp();
  $("#app-content").focus();
  keydown("ArrowRight");
  assert.equal(app.paperPage, 2);
  assert.deepEqual(rowIds(), state.papers.slice(10).map((p) => p.id));
  assert.equal($('.page-btn.active').dataset.id, "2");

  // Past the last page nothing changes, and the event is left for the browser.
  const past = keydown("ArrowRight");
  assert.equal(app.paperPage, 2);
  assert.equal(past.defaultPrevented, false);

  keydown("ArrowLeft");
  assert.equal(app.paperPage, 1);
  assert.equal(rowIds().length, 10);

  keydown("ArrowDown");
  const rows = $$("tbody tr[data-action=open-paper]");
  assert.equal(document.activeElement, rows[0]);
  assert.ok(rows[0]!.classList.contains("cursor-row"));
  keydown("ArrowDown");
  assert.equal(document.activeElement, $$("tbody tr[data-action=open-paper]")[1]);
  keydown("ArrowUp");
  assert.equal(document.activeElement, $$("tbody tr[data-action=open-paper]")[0]);
  assert.equal($$(".cursor-row").length, 1);

  // The cursor comes along to the next page.
  keydown("ArrowRight");
  assert.equal(app.paperPage, 2);
  assert.equal(document.activeElement, $$("tbody tr[data-action=open-paper]")[0]);

  // Enter on the focused row opens its details.
  keydown("Enter");
  assert.equal(app.drawer?.type, "paper");
  assert.equal(app.drawer?.id, rowIds()[0]);
  keydown("Escape");
  assert.equal(app.drawer, null);

  // Arrow keys inside the search box edit text, not the table.
  const search = $<HTMLInputElement>("#paper-search");
  search.focus();
  const typed = keydown("ArrowLeft", search);
  assert.equal(app.paperPage, 2);
  assert.equal(typed.defaultPrevented, false);
});

test("the search box filters after a pause and resets the page", async () => {
  const { app, state } = papersApp();
  $('.page-btn[data-id="2"]').click();
  const target: Paper = state.papers[3]!;
  const search = $<HTMLInputElement>("#paper-search");
  search.value = target.title.slice(0, 12);
  search.dispatchEvent(new Event("input", { bubbles: true }));
  await new Promise((resolve) => setTimeout(resolve, 200));
  assert.equal(app.paperPage, 1);
  assert.ok(rowIds().includes(target.id));
  assert.ok(rowIds().length < 14);
  assert.equal(document.activeElement, $("#paper-search"));
});

test("the heading's import and add-paper buttons appear only on pages about the library's records", () => {
  const { app } = papersApp();
  const actions = $(".heading-actions");
  for (const page of ["overview", "papers", "datasets", "tasks"]) {
    app.navigate(page);
    assert.equal(actions.hidden, false, `${page} keeps the heading actions`);
  }
  for (const page of ["settings", "agent", "sources"]) {
    app.navigate(page);
    assert.equal(actions.hidden, true, `${page} hides the heading actions`);
  }
});

test("ticking rows keeps them selected across pages and feeds the bulk bar's actions", async () => {
  const { app, state } = papersApp();
  const bar = () => $("#bulk-bar");
  assert.equal(bar().hidden, true);
  const first = $<HTMLInputElement>(`input[data-select-paper="${state.papers[0]!.id}"]`);
  first.checked = true;
  first.dispatchEvent(new Event("change", { bubbles: true }));
  assert.deepEqual([...app.selectedPapers], [state.papers[0]!.id]);
  assert.equal(bar().hidden, false);
  assert.match(text("#bulk-bar .bulk-bar-count"), /已勾选 1 篇/);
  assert.ok(first.closest("tr")!.classList.contains("is-selected"));
  // Ticking a box never opens the row's drawer.
  first.click();
  assert.equal(app.drawer, null);

  // Select-all covers the page; the selection survives turning the page and comes back.
  const all = $<HTMLInputElement>("input[data-select-all]");
  all.checked = true;
  all.dispatchEvent(new Event("change", { bubbles: true }));
  assert.equal(app.selectedPapers.size, 10);
  $('.page-btn[data-id="2"]').click();
  assert.equal($<HTMLInputElement>("input[data-select-all]").checked, false);
  assert.match(text("#bulk-bar .bulk-bar-count"), /已勾选 10 篇/);
  $('.page-btn[data-id="1"]').click();
  assert.equal($<HTMLInputElement>("input[data-select-all]").checked, true);
  assert.equal($$("tbody tr.is-selected").length, 10);

  // Batch screening starts from the ticked papers, export offers them as a range.
  $('[data-action="bulk-screen"]').click();
  assert.deepEqual(new Set(app.batchDraft!.current), app.selectedPapers);
  assert.equal(app.batchDraft!.selected.size, 10);
  $('#modal-root button[data-action="close-modal"]').click();
  app.batchDraft = null;
  $('[data-action="bulk-export"]').click();
  assert.equal($<HTMLInputElement>('#export-form input[name="range"][value="selected"]').checked, true);
  assert.match(text('#export-form .radio-choice:has(input[value="selected"])'), /已勾选的文献（10 篇）/);
  $('#modal-root button[data-action="close-modal"]').click();

  // Deleting asks once for the lot, then removes them and empties the selection.
  $('[data-action="bulk-delete"]').click();
  assert.match(text("#modal-root .modal-body"), /确定删除已勾选的 10 篇文献/);
  $('[data-action="confirm-delete-selected"]').click();
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(app.state.papers.length, 4);
  assert.equal(app.selectedPapers.size, 0);
  assert.equal(bar().hidden, true);
  assert.equal(document.querySelector("#modal-root .modal"), null);
});
