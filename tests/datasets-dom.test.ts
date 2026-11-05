// Datasets list in a real DOM: the table names its columns so narrow windows
// can fold the modality column into the title cell without the accession
// column losing its width (it used to overlap the status column at 1024px).
import { $, $$, change, keydown, resetDom } from "./dom.ts";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";
import { App, type Env } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import { Library, type DatasetManifest } from "../src/mygo.ts";
import { matchesPreparation, referenceComplete } from "../src/dataset-assets.ts";

const previewEnv = (): Env => ({
  desktop: false, backend: false, readOnly: false, web: null, real: false,
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
  const css = readFileSync(new URL("../src/style.css", import.meta.url), "utf8").replaceAll("\r\n", "\n");
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

test("review mode walks the datasets one at a time: Y keeps, D deletes, Esc returns to the list", async () => {
  const { app } = datasetsApp();
  const total = app.state.datasets.length;
  assert.ok(total >= 3);
  app.state.datasets[0]!.awaitingReview = true;
  const [first, second, third] = app.state.datasets.map((d) => d.id);
  $<HTMLButtonElement>("[data-action=start-dataset-review]").click();
  assert.equal($(".dataset-review-panel").dataset.datasetId, first);
  keydown("y");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.equal(app.state.datasets.find((d) => d.id === first)?.awaitingReview, false);
  assert.equal($(".dataset-review-panel").dataset.datasetId, second);
  keydown("d");
  await new Promise((resolve) => setTimeout(resolve, 0));
  assert.ok(!app.state.datasets.some((d) => d.id === second), "D deletes the dataset on screen");
  assert.equal(app.state.datasets.length, total - 1);
  assert.equal($(".dataset-review-panel").dataset.datasetId, third, "the next dataset takes its place");
  assert.match($(".screen-counts").textContent ?? "", /本次删除 1/);
  keydown("Escape");
  assert.equal(document.querySelector(".dataset-review-panel"), null);
  assert.ok($(".dataset-table"));
});

function manifestFixture(id: string): DatasetManifest {
  return {datasetId:id,directory:`datasets/${id}`,allocated:false,revision:0,plan:{samples:[],references:[],notes:""},assets:[]};
}

test("dataset preparation filters distinguish directory allocation, local verification and reference matching", () => {
  const { app, state }=datasetsApp();app.env.real=true;
  const m=manifestFixture(state.datasets[0]!.id);state.datasetManifests=[m];app.render();
  m.allocated=true;
  assert.equal(matchesPreparation(m,"unallocated"),false);
  assert.equal(matchesPreparation(m,"verified"),false);
  m.plan.references=[{key:"ath",species:"Arabidopsis thaliana",assembly:"TAIR10",annotation:"Araport11",status:"matched",evidence:"Feature IDs checked"}];
  assert.equal(referenceComplete(m),false,"a reference claim alone is not a sequence bundle");
  m.assets=["pep","cds","gene_map"].map(kind=>({id:kind,datasetId:m.datasetId,title:kind,kind,url:"",sampleIds:[],matrixKind:"unknown",referenceKey:"ath",notes:"",localPath:`references/ath/${kind}`,sizeBytes:1,sha256:"a".repeat(64),checkedAt:"2026-11-05",localState:"verified"}));
  assert.equal(referenceComplete(m),true);
  m.assets[0]!.localState="changed";assert.equal(referenceComplete(m),false);
  change($<HTMLSelectElement>("#dataset-preparation-filter"),"verified");
  assert.equal($$("tbody tr[data-action=open-dataset]").length,1);
  app.openDataset(m.datasetId);
  assert.match($(".dataset-workspace").textContent??"",/不代表矩阵、注释和序列已通过分析验证/);
  assert.ok(!$(".drawer").textContent?.includes("分析已就绪"));
  app.closeDrawer();
  $<HTMLButtonElement>("[data-action=start-dataset-review]").click();
  assert.deepEqual(app.datasetScreen?.ids,[m.datasetId],"the review queue must use the preparation filter too");
  app.exitDatasetReview();
  change($<HTMLSelectElement>("#dataset-preparation-filter"),"reference_matched");
  assert.equal($$("tbody tr[data-action=open-dataset]").length,0);
  $<HTMLButtonElement>("[data-action=clear-filters]").click();
  assert.equal(app.datasetPreparation,"");
  assert.equal($$("tbody tr[data-action=open-dataset]").length,state.datasets.length);
});

test("dataset file form saves scope, matrix type and relative path through the real backend",async()=>{
  const {app,state}=datasetsApp();app.env.real=true;app.env.backend=true;
  const m=manifestFixture(state.datasets[0]!.id);m.plan.samples=[{id:"GSM1",title:"root",species:"",tissue:"",condition:"",replicate:"",notes:""}];state.datasetManifests=[m];
  const original=Library.saveDatasetAsset;let saved:Parameters<typeof Library.saveDatasetAsset>[0]|undefined;
  Library.saveDatasetAsset=async a=>{saved=a;return {...a,id:"DA-test"};};
  try{
    app.openDataset(m.datasetId);$<HTMLButtonElement>('[data-action="dataset-asset-edit"]').click();
    $<HTMLInputElement>('#dataset-asset-form [name="title"]').value='counts <script>.mtx';
    $<HTMLInputElement>('#dataset-asset-form [name="localPath"]').value=m.directory+'/original/counts.mtx';
    $<HTMLSelectElement>('#dataset-asset-form [name="kind"]').value='matrix';
    $<HTMLSelectElement>('#dataset-asset-form [name="matrixKind"]').value='counts';
    $<HTMLInputElement>('#dataset-asset-form [name="sampleIds"]').checked=true;
    $("#dataset-asset-form").dispatchEvent(new SubmitEvent("submit",{bubbles:true,cancelable:true}));
    await new Promise(r=>setTimeout(r,0));
    assert.equal(saved?.matrixKind,"counts");assert.deepEqual(saved?.sampleIds,["GSM1"]);assert.equal(saved?.localState,"none");
    assert.equal(saved?.localPath,m.directory+'/original/counts.mtx');
  }finally{Library.saveDatasetAsset=original;}
});

test("viewers can read dataset manifests without write controls or executable source markup",()=>{
  const {app,state}=datasetsApp();app.env.real=true;app.env.readOnly=true;
  const m=manifestFixture(state.datasets[0]!.id);m.plan.samples=[{id:'<img src=x onerror=alert(1)>',title:'unsafe',species:'',tissue:'',condition:'',replicate:'',notes:''}];state.datasetManifests=[m];
  app.openDataset(m.datasetId);
  assert.ok($(".dataset-workspace"));assert.equal(document.querySelector('[data-action="dataset-prepare"]'),null);assert.equal(document.querySelector('[data-action="dataset-asset-edit"]'),null);assert.equal(document.querySelector('.drawer img'),null);
});
