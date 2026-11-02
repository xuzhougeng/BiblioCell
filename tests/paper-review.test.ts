import assert from "node:assert/strict";
import { test } from "node:test";
import type { App } from "../src/app.ts";
import { defaultPaperColumns } from "../src/columns.ts";
import { demoState } from "../src/demo-data.ts";
import { decideScreening, emptyFilters, filterDatasets, filterPapers, reviewConcern, screeningConcern } from "../src/model.ts";
import { paperTable, renderDatasets, renderPapers } from "../src/views.ts";

test("review filter keeps negative proposals distinct from actual screening decisions", () => {
  const seed = demoState().papers[0]!;
  const pending = { ...seed, id: "pending", screening: "pending", proposedScreening: "" };
  const excluded = { ...seed, id: "excluded", screening: "pending", proposedScreening: "excluded" };
  const uncertain = { ...seed, id: "uncertain", screening: "included", proposedScreening: "uncertain" };
  const positive = { ...seed, id: "positive", screening: "pending", proposedScreening: "included" };
  const settled = { ...seed, id: "settled", screening: "excluded", proposedScreening: "excluded" };
  const papers = [pending, excluded, uncertain, positive, settled];
  const filters = { ...emptyFilters(), review: "concern" };
  assert.deepEqual(new Set(filterPapers(papers, filters).map((p) => p.id)), new Set(["excluded", "uncertain"]));
  assert.equal(filterPapers(papers, { ...filters, screening: "included" })[0]!.id, "uncertain");
  assert.equal(filterPapers(papers, emptyFilters()).length, 5);
  decideScreening(excluded, "excluded", "Confirmed off topic");
  assert.equal(screeningConcern(excluded), null);
  uncertain.proposedScreening = ""; // Existing dismiss-proposal action.
  assert.equal(filterPapers(papers, filters).length, 0);
});

test("list exposes the proposal reason safely and explains why the total stays unchanged", () => {
  const paper = { ...demoState().papers[0]!, screening: "pending", proposedScreening: "excluded", proposedReason: '<img src=x onerror="bad()">研究对象为人类' };
  const app = {
    state: { ...demoState(), papers: [paper] }, filters: emptyFilters(),
    paperPage: 1, paperPageSize: 10, paperColumns: defaultPaperColumns(),
    paperDatasets: () => [], paperResources: () => [],
  } as unknown as App;
  const html = renderPapers(app);
  assert.match(html, /需核对 <span>1<\/span>/);
  assert.match(html, /全部文献 <span>1<\/span>/);
  assert.match(html, /确认前仍计入全部文献/);
  assert.match(html, /建议排除 · 待你确认/);
  assert.match(html, /&lt;img src=x onerror=&quot;bad\(\)&quot;&gt;/);
  assert.doesNotMatch(html, /<img src=x/);
  assert.match(html, /尚未确认/);
  // Warnings remain discoverable when the user hides the status column.
  app.paperColumns = ["title"];
  assert.match(paperTable([paper], app), /建议排除 · 待你确认/);
  paper.proposedScreening = "";
  const dismissed = renderPapers(app);
  assert.match(dismissed, /需核对 <span>0<\/span>/);
  assert.doesNotMatch(dismissed, /class="review-summary"|class="paper-review-note/);
});

test("papers a source update added wait in review until a person decides or finishes it", () => {
  const seed = demoState().papers[0]!;
  const added = { ...seed, id: "added", screening: "pending", proposedScreening: "", awaitingReview: true, source: "PubMed" };
  const flagged = { ...seed, id: "flagged", screening: "pending", proposedScreening: "excluded", awaitingReview: true };
  const own = { ...seed, id: "own", screening: "pending", proposedScreening: "", awaitingReview: false };
  const filters = { ...emptyFilters(), review: "concern" };
  assert.equal(reviewConcern(added), "source");
  assert.equal(reviewConcern(flagged), "excluded"); // An agent's warning says more than where it came from.
  assert.deepEqual(filterPapers([added, flagged, own], filters).map((p) => p.id).sort(), ["added", "flagged"]);

  const app = {
    state: { ...demoState(), papers: [added, own] }, filters: emptyFilters(),
    paperPage: 1, paperPageSize: 10, paperColumns: defaultPaperColumns(),
    paperDatasets: () => [], paperResources: () => [],
  } as unknown as App;
  const html = renderPapers(app);
  assert.match(html, /需核对 <span>1<\/span>/);
  assert.match(html, /1 篇由来源更新新增/);
  assert.match(html, /来源新增 · 待你确认/);
  assert.match(html, /来自 PubMed 的更新/);

  decideScreening(added, "pending"); // Any decision is the person's review.
  assert.equal(added.awaitingReview, false);
  assert.equal(reviewConcern(added), null);
});

test("datasets page has its own review tab for what a source update added", () => {
  const { datasets: [first, second] } = demoState();
  const added = { ...first!, id: "DS-added", title: "GEO series", awaitingReview: true };
  const known = { ...second!, id: "DS-known", awaitingReview: false };
  assert.deepEqual(filterDatasets([added, known], "", () => "", true).map((d) => d.id), ["DS-added"]);
  assert.equal(filterDatasets([added, known], "", () => "").length, 2);

  const app = {
    state: { ...demoState(), datasets: [added, known] }, datasetSearch: "", datasetReview: false,
    datasetView: "table", datasetPage: 1, datasetPageSize: 10, paperById: () => undefined,
  } as unknown as App;
  const all = renderDatasets(app);
  assert.match(all, /全部数据集 <span>2<\/span>/);
  assert.match(all, /需核对 <span>1<\/span>/);
  assert.match(all, /有 1 组数据集需要核对/);
  app.datasetReview = true;
  const review = renderDatasets(app);
  assert.match(review, /GEO series/);
  assert.doesNotMatch(review, /data-id="DS-known"/);
  added.awaitingReview = false;
  assert.match(renderDatasets(app), /没有需核对的数据集/);
});
