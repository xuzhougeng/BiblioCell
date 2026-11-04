import { $, change, resetDom, text } from "./dom.ts";
import assert from "node:assert/strict";
import { test } from "node:test";
import type { Env } from "../src/app.ts";

// Load the normal entry points after selecting English, as a language-switch reload does.
localStorage.setItem("bibliocell-locale", "en");
const { App } = await import("../src/app.ts");
const { demoState } = await import("../src/demo-data.ts");
const { speciesDisplay, speciesLabelOf, speciesNameFor, emptyFilters, filterPapers, filterDatasets } = await import("../src/model.ts");
const { COMMON_PLANTS, speciesCommonName, speciesText } = await import("../src/species.ts");
const { speciesRow, selectPlant, readSpeciesRows } = await import("../src/ui.ts");
const { renderDatasets } = await import("../src/views.ts");
const { metadataValue } = await import("../src/metadata.ts");
const { tv } = await import("../src/i18n.ts");
const { research } = await import("../src/locales/en/research.ts");

const previewEnv = (): Env => ({
  desktop: false, backend: false, readOnly: false, web: null, real: false,
  settings: { storagePath: "", fontSize: 15, batchScreenConcurrency: 3, translationPrompt: "", mcpToken: "", ncbiApiKey: "", contactEmail: "", models: [], tasks: { chat: "", screen: "", translate: "" } },
  mcp: { enabled: false, endpoint: "", token: "", tools: [], resources: [] },
  status: null, sources: [],
  automation: { enabled: false, intervalHours: 24, running: false, lastRunAt: "", nextRunAt: "", added: 0, skipped: 0, failures: [] },
});

function englishApp(hash = "#papers") {
  resetDom(hash);
  localStorage.setItem("bibliocell-locale", "en");
  document.documentElement.lang = "en";
  const state = demoState();
  const app = new App({ initial: state, store: { load: async () => state, save: async () => {} }, env: previewEnv() });
  app.start();
  return { app, state };
}

test("all built-in plants have English and Latin names, including older Chinese labels", () => {
  for (const plant of COMMON_PLANTS) {
    assert.ok(plant.englishLabel.trim());
    assert.doesNotMatch(plant.englishLabel, /[一-鿿]/);
    assert.equal(speciesDisplay({ species: [plant] }), `${plant.englishLabel} · ${plant.name}`);
    assert.equal(speciesCommonName(plant, "zh-CN"), plant.label);
  }
  assert.equal(speciesText({ name: " Triticum aestivum ", label: "小麦" }), "Bread wheat · Triticum aestivum");
  assert.equal(speciesText({ name: "ORYZA SATIVA", label: "" }), "Rice · ORYZA SATIVA");
  assert.equal(speciesText({ name: "Custom species", label: "Custom label" }), "Custom label · Custom species");
  assert.equal(speciesText({ name: "Custom species" }), "Custom species");
  assert.equal(speciesText({}), "");
});

test("organ vocabulary and English searches work with existing Chinese records", () => {
  for (const [stored, translated] of Object.entries(research)) assert.equal(tv(stored), translated);
  assert.equal(tv("根瘤"), "Root nodule");
  assert.equal(tv("叶肉"), "Mesophyll");
  assert.equal(tv("Unlisted tissue"), "Unlisted tissue");
  const state = demoState();
  const paper = { ...state.papers[0]!, species: [{ name: "Oryza sativa", label: "水稻" }], organs: ["根瘤", "叶肉"] };
  const dataset = { ...state.datasets[0]!, species: paper.species, organs: paper.organs };
  const original = JSON.stringify([paper, dataset]);
  for (const search of ["Rice", "Oryza sativa", "水稻", "Mesophyll", "根瘤"]) {
    assert.equal(filterPapers([paper], { ...emptyFilters(), search }).length, 1, search);
    assert.equal(filterDatasets([dataset], search, () => "").length, 1, search);
  }
  assert.equal(JSON.stringify([paper, dataset]), original);
  assert.equal(metadataValue({ field: "organs", values: ["根瘤", "叶肉"] } as Parameters<typeof metadataValue>[0]), "Root nodule; Mesophyll");
});

test("English lists, filters, overview navigation and dataset cards use translated research labels", () => {
  const { app, state } = englishApp();
  assert.match(text(".paper-table"), /Thale cress · Arabidopsis thaliana/);
  assert.equal(text('[data-filter="organ"] option[value="根"]'), "Root");
  assert.equal(text('[data-filter="species"] option[value="Oryza sativa"]'), "Rice · Oryza sativa");
  change($<HTMLSelectElement>('[data-filter="organ"]'), "根");
  assert.equal(app.filters.organ, "根");
  assert.match(text(".paper-table .species-name"), /Root/);
  assert.equal(speciesLabelOf(state.papers, "Oryza sativa"), "Rice · Oryza sativa");
  assert.equal(speciesNameFor(state.papers, "Rice · Oryza sativa"), "Oryza sativa");
  assert.match(renderDatasets(app), /Thale cress · Arabidopsis thaliana/);
  $('.paper-table [data-action="open-paper"]').click();
  assert.match(text("#drawer-root .detail-grid"), /Thale cress/);
  assert.match(text("#drawer-root .detail-grid"), /Arabidopsis thaliana/);
  assert.match(text("#drawer-root .detail-grid"), /Root/);

  englishApp("#overview");
  assert.match(text(".species-bars"), /Thale cress · Arabidopsis thaliana/);
  $('[data-action="overview-filter"][data-field="species"][data-value="Arabidopsis thaliana"]').click();
  assert.equal($<HTMLSelectElement>('[data-filter="species"]').value, "Arabidopsis thaliana");
});

test("translated species options round-trip the stored names and labels", () => {
  resetDom();
  const entry = { name: "Oryza sativa", label: "水稻" };
  document.body.innerHTML = `<form>${speciesRow(entry)}</form>`;
  assert.equal(text('option[value="Oryza sativa"]'), "Rice · Oryza sativa");
  assert.deepEqual(readSpeciesRows($("form")), [entry]);
  const select = $<HTMLSelectElement>("[data-species-picker]");
  select.value = "Arachis hypogaea";
  selectPlant(select);
  assert.deepEqual(readSpeciesRows($("form")), [{ name: "Arachis hypogaea", label: "花生" }]);
});
