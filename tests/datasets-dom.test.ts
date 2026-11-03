// Datasets list in a real DOM: the table names its columns so narrow windows
// can fold the modality column into the title cell without the accession
// column losing its width (it used to overlap the status column at 1024px).
import { $, $$, resetDom } from "./dom.ts";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";

const previewEnv = (): Env => ({
  desktop: false, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

function datasetsApp() {
  resetDom("#datasets");
  localStorage.setItem("bibliocell-datasets-view", "table");
  const state = demoState();
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: previewEnv() });
  app.start();
  return { app, state };
}

const COLUMNS = ["col-title", "col-links", "col-species", "col-modality", "col-repo", "col-status", "col-arrow"];

test("every dataset row carries the same named columns as the header", () => {
  const { state } = datasetsApp();
  assert.ok(state.datasets.length > 0);
  const header = $$("thead th", $(".dataset-table"));
  assert.deepEqual(header.map((th) => COLUMNS.find((c) => th.classList.contains(c))), COLUMNS);
  for (const row of $$("tbody tr[data-action=open-dataset]", $(".dataset-table"))) {
    const cells = $$("td", row);
    assert.equal(cells.length, header.length, "a row with fewer cells than the header shifts every column after it");
    assert.deepEqual(cells.slice(0, -1).map((td) => COLUMNS.find((c) => td.classList.contains(c))), COLUMNS.slice(0, -1));
    // The title cell repeats the modality badges, so hiding the column keeps the information on the row.
    assert.ok($(".dataset-meta-modality .badge", row) || $(".col-modality .badge", row) === null);
  }
});

test("the stylesheet hides the modality column and shows its badges in the title cell below 1100px", () => {
  const css = readFileSync(new URL("../src/style.css", import.meta.url), "utf8");
  const narrow = css.slice(css.indexOf("@media (max-width: 1100px)"));
  const block = narrow.slice(0, narrow.indexOf("\n}\n") + 3);
  assert.match(block, /\.dataset-table \.col-modality \{ display: none \}/);
  assert.match(block, /\.dataset-table \.dataset-meta-modality \{ display: inline-flex/);
  // Outside the narrow range the badges stay in their own column only.
  assert.match(css, /\n\.dataset-meta-modality \{ display: none \}/);
  // Accessions wrap instead of overflowing into the next column.
  assert.match(css, /\.dataset-table \.repo-cell \{[^}]*overflow-wrap: anywhere/);
  assert.doesNotMatch(css, /\.dataset-table \.repo-cell \{[^}]*white-space: nowrap/);
});
