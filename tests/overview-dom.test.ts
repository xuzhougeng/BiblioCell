// Overview dashboard and the keyboard shortcut sheet, in a real DOM.
import { $, $$, keydown, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";

const previewEnv = (): Env => ({
  desktop: false, backend: false, readOnly: false, web: null, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

function overviewApp(state = demoState(), screening = "") {
  resetDom("#overview");
  // The statistics default to the included papers; these tests count every status unless they say otherwise.
  localStorage.setItem("bibliocell-overview-screening", screening);
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: previewEnv() });
  app.start();
  return { app, state };
}

test("the overview dashboard sums up screening and resumes it on the first undecided paper", () => {
  const { app, state } = overviewApp();
  const undecided = state.papers.filter((p) => p.screening === "pending").length;
  assert.match(text(".dashboard-panel .panel-header p"), new RegExp(`已判断 ${state.papers.length - undecided} / ${state.papers.length} 篇`));
  assert.equal($$(".screen-bar-seg").length, new Set(state.papers.map((p) => p.screening)).size);
  assert.match(text(".dashboard-metric:nth-child(3)"), new RegExp(`待判断\\s*${undecided}`));
  // The assistant is a header button on the overview, not a card in the page.
  assert.equal(document.querySelector(".agent-entry"), null);
  assert.equal($("[data-overview-only]").hidden, false);

  $('[data-action="resume-screening"]').click();
  assert.ok(app.screen, "screening opened");
  const first = state.papers.find((p) => p.screening === "pending")!;
  assert.equal($(".screen-panel").dataset.paperId, first.id);
  assert.equal(app.screen!.ids.length, state.papers.length);
});

test("the overview charts years, techniques and tissues, and each bar filters the library", () => {
  const { app, state } = overviewApp();
  const year = String(state.papers.find((p) => p.year)!.year);
  const inYear = state.papers.filter((p) => String(p.year) === year).length;
  const col = $(`.year-col[data-value="${year}"]`);
  assert.match(col.title, new RegExp(`${year}：${inYear} 篇`));
  assert.ok($('[data-field="modality"]') && $('[data-field="organ"]') && $('[data-field="species"]'));

  app.filters.organ = "stale";
  col.click();
  assert.equal(app.page, "papers");
  assert.equal(app.filters.year, year);
  assert.equal(app.filters.organ, "", "a chart click starts from a clean filter");
  assert.equal($("[data-overview-only]").hidden, true, "the assistant button stays on the overview");
});

test("the overview counts article types and can leave preprints out of the statistics", () => {
  const { app, state } = overviewApp();
  assert.ok($(".overview-progress-grid .dashboard-panel") && $(".overview-progress-grid .coverage-panel"), "screening progress sits beside coverage");
  const research = state.papers.filter((p) => p.articleTypes.includes("原创研究"));
  const row = $('[data-field="articleType"][data-value="原创研究"]');
  assert.equal(row.querySelector("strong")!.textContent, String(research.length));

  const box = $<HTMLInputElement>("#overview-hide-preprint");
  box.checked = true;
  box.dispatchEvent(new Event("change", { bubbles: true }));
  assert.equal(localStorage.getItem("bibliocell-overview-hide-preprint"), "on");
  const published = research.filter((p) => p.publicationStatus !== "preprint").length;
  assert.ok(published < research.length, "the demo has preprints to leave out");
  assert.equal($('[data-field="articleType"][data-value="原创研究"] strong').textContent, String(published));
  assert.match(text(".coverage-panel"), new RegExp(`/ ${state.papers.length}`), "coverage still counts every paper");

  $('[data-field="articleType"][data-value="原创研究"]').click();
  assert.equal(app.page, "papers");
  assert.equal(app.filters.articleType, "原创研究");
  assert.equal(app.filters.preprint, "no", "the library shows the papers the bar counted");
  assert.equal($$("#app-content .paper-table tbody tr").length, published);
});

test("the overview counts included papers by default and never the irrelevant ones", () => {
  resetDom("#overview");
  localStorage.removeItem("bibliocell-overview-screening");
  const state = demoState();
  const dropped = state.papers.find((p) => p.screening === "pending")!;
  dropped.screening = "irrelevant";
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: previewEnv() });
  app.start();
  const active = state.papers.filter((p) => p.screening !== "irrelevant");
  const included = active.filter((p) => p.screening === "included");
  assert.deepEqual(app.overviewScreening, ["included"]);
  assert.equal($<HTMLInputElement>('[data-overview-screening="included"]').checked, true);
  assert.equal(document.querySelector('[data-overview-screening="irrelevant"]'), null, "no checkbox offers the irrelevant papers");
  assert.match(text(".stat-card:first-child .stat-value"), new RegExp(`^${active.length}`), "the paper count leaves the irrelevant one out");
  assert.match(text(".dashboard-panel .panel-header p"), new RegExp(`/ ${active.length} 篇`));
  assert.match(text(".overview-section-head"), new RegExp(`当前统计 ${included.length} 篇`));
  const year = String(included.find((p) => p.year)!.year);
  assert.match($(`.year-col[data-value="${year}"]`).title, new RegExp(`${year}：${included.filter((p) => String(p.year) === year).length} 篇`));
});

test("? opens the shortcut sheet unless typing, and the top bar has the same button", () => {
  overviewApp();
  assert.ok($("#shortcuts-help"));
  $("#app-content").focus();
  keydown("?");
  assert.match(text("#modal-root #modal-title"), /快捷键/);
  assert.match(text("#modal-root .shortcut-groups"), /逐篇筛选/);
  keydown("Escape");
  assert.equal(document.querySelector("#modal-root .modal"), null);
  const search = $<HTMLInputElement>("#global-search");
  search.focus();
  const typed = keydown("?", search);
  assert.equal(typed.defaultPrevented, false, "a ? typed into a field is text");
  assert.equal(document.querySelector("#modal-root .modal"), null);
  $("#shortcuts-help").click();
  assert.match(text("#modal-root #modal-title"), /快捷键/);
});
