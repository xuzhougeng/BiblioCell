import assert from "node:assert/strict";
import { test } from "node:test";
import type { App } from "../src/app.ts";
import { defaultPaperColumns } from "../src/columns.ts";
import { demoState } from "../src/demo-data.ts";
import { decideScreening, emptyFilters, filterPapers, screeningConcern } from "../src/model.ts";
import { paperTable, renderPapers } from "../src/views.ts";

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
