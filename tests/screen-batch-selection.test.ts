import assert from "node:assert/strict";
import { test, type TestContext } from "node:test";
import type { App } from "../src/app.ts";
import { demoState } from "../src/demo-data.ts";
import { emptyFilters } from "../src/model.ts";
import { batchActions, openBatch } from "../src/screen-batch.ts";

function setup(t: TestContext) {
  const state = demoState();
  const paper = state.papers[0]!;
  state.papers = Array.from({ length: 874 }, (_, i) => ({ ...paper, id: `paper-${i}`, year: i < 30 ? 2020 : 2025, publicationStatus: i < 2 ? "preprint" : "published" }));
  const regions = {
    "#batch-candidates": { innerHTML: "" },
    "#batch-selected": { textContent: "" },
    "#batch-next": { disabled: false },
  };
  const previous = Object.getOwnPropertyDescriptor(globalThis, "document");
  Object.defineProperty(globalThis, "document", { configurable: true, value: {
    querySelector: (selector: string) => regions[selector as keyof typeof regions] ?? null,
  } });
  t.after(() => {
    if (previous) Object.defineProperty(globalThis, "document", previous);
    else Reflect.deleteProperty(globalThis, "document");
  });
  const app = {
    state, filters: emptyFilters(), batch: null, batchDraft: null,
    env: { settings: { models: [], tasks: {}, batchScreenConcurrency: 3 } },
    paperById: (id: string) => state.papers.find(p => p.id === id),
    showModal: () => {},
  } as unknown as App;
  return { app, regions };
}

test("reopening batch selection replaces a stale 874-paper draft with the current 28 filtered papers", t => {
  const { app, regions } = setup(t);
  openBatch(app);
  assert.equal(app.batchDraft!.selected.size, 874);
  Object.assign(app.batchDraft!, { scope: "all", search: "old query", requirement: "plants", modelId: "chosen", concurrency: 5 });
  app.filters.year = "2020";
  app.filters.preprint = "no";
  openBatch(app);
  const expected = Array.from({ length: 28 }, (_, i) => `paper-${i + 2}`);
  assert.deepEqual(new Set(app.batchDraft!.current), new Set(expected));
  assert.deepEqual(app.batchDraft!.selected, new Set(expected));
  assert.equal(app.batchDraft!.scope, "current");
  assert.equal(app.batchDraft!.search, "");
  assert.equal(regions["#batch-selected"].textContent, "已选择 28 篇");
  assert.equal((regions["#batch-candidates"].innerHTML.match(/data-batch-paper=/g) ?? []).length, 28);
  assert.equal(app.batchDraft!.requirement, "plants");
  assert.equal(app.batchDraft!.modelId, "chosen");
  assert.equal(app.batchDraft!.concurrency, 5);
  app.filters.year = "1999";
  openBatch(app);
  assert.equal(app.batchDraft!.selected.size, 0);
  assert.equal(regions["#batch-next"].disabled, true);
});

test("returning from model configuration preserves manual candidate selection", t => {
  const { app } = setup(t);
  app.filters.year = "2020";
  app.filters.preprint = "no";
  openBatch(app);
  app.batchDraft!.selected = new Set(["paper-5"]);
  batchActions(app)["batch-back"]!();
  assert.deepEqual([...app.batchDraft!.selected], ["paper-5"]);
});
