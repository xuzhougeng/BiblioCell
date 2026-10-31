// Pure-logic tests, run with `npm test` (node:test with Node's built-in type stripping).
import assert from "node:assert/strict";
import { test } from "node:test";
import { demoState } from "../src/demo-data.ts";
import { changedItems, csvText, decideScreening, emptyFilters, esc, fileSize, filterPapers, normalizeDOI, normalizeDataset, prepareImport, tasksFor, validDOI } from "../src/model.ts";

test("esc neutralizes HTML metacharacters and tolerates null", () => {
  assert.equal(esc(`<a href="x">&'</a>`), "&lt;a href=&quot;x&quot;&gt;&amp;&#39;&lt;/a&gt;");
  assert.equal(esc(null), "");
  assert.equal(esc(2026), "2026");
});

test("changedItems returns only edited and new items with their synced versions", () => {
  const saved = [{ id: "a", v: 1 }, { id: "b", v: 1 }];
  const next = [{ id: "a", v: 1 }, { id: "b", v: 2 }, { id: "c", v: 1 }];
  assert.deepEqual(changedItems(next, saved), [[{ id: "b", v: 2 }, { id: "b", v: 1 }], [{ id: "c", v: 1 }, undefined]]);
});

test("decideScreening settles the proposal and pending clears the review", () => {
  const paper = { ...demoState().papers[0]!, proposedScreening: "excluded", proposedReason: "off topic" };
  decideScreening(paper, "included");
  assert.equal(paper.screening, "included");
  assert.equal(paper.proposedScreening, "");
  assert.ok(paper.reviewedAt);
  decideScreening(paper, "pending");
  assert.equal(paper.reviewedAt, "");
});

test("tasksFor queues an agent proposal that differs from the human decision", () => {
  const paper = { ...demoState().papers[0]!, screening: "included", proposedScreening: "excluded" };
  const rows = tasksFor([paper], []).filter((row) => row.type === "screen");
  assert.equal(rows.length, 1);
  assert.match(rows[0]!.subtitle, /Agent 建议/);
  assert.equal(tasksFor([{ ...paper, proposedScreening: "included" }], []).filter((row) => row.type === "screen").length, 0);
});

test("fileSize switches from KB to MB", () => {
  assert.equal(fileSize(10), "1 KB");
  assert.equal(fileSize(2048), "2 KB");
  assert.equal(fileSize(5 * 1024 * 1024), "5.0 MB");
});

test("normalizeDOI strips resolver prefixes and lowercases", () => {
  assert.equal(normalizeDOI(" https://doi.org/10.1000/ABC "), "10.1000/abc");
  assert.equal(normalizeDOI("http://dx.doi.org/10.1000/abc"), "10.1000/abc");
  assert.equal(normalizeDOI("DOI: 10.1000/abc"), "10.1000/abc");
  assert.equal(normalizeDOI(undefined), "");
  assert.ok(validDOI(""));
  assert.ok(validDOI("10.1000/abc.123"));
  assert.ok(!validDOI("11.1000/abc"));
  assert.ok(!validDOI("10.1000/with space"));
});

test("filterPapers matches any of a paper's species, applies starred and sorts by title", () => {
  const { papers } = demoState();
  const rice = filterPapers(papers, { ...emptyFilters(), species: "Oryza sativa" });
  assert.ok(rice.length >= 3);
  assert.ok(rice.every((p) => p.species.some((s) => s.name === "Oryza sativa")));

  const starred = filterPapers(papers, { ...emptyFilters(), starred: true });
  assert.ok(starred.length > 0 && starred.every((p) => p.starred));

  const search = filterPapers(papers, { ...emptyFilters(), search: "根瘤" });
  assert.deepEqual(search.map((p) => p.id), ["DEMO-P-006"]);

  const byTitle = filterPapers(papers, { ...emptyFilters(), sort: "title" }).map((p) => p.title);
  assert.deepEqual(byTitle, [...byTitle].sort((a, b) => a.localeCompare(b, "zh-CN")));
});

test("tasksFor generates screening, fulltext and data rows from state", () => {
  const { papers, datasets } = demoState();
  const rows = tasksFor(papers, datasets);
  const types = (id: string) => rows.filter((r) => r.paper.id === id).map((r) => r.type).sort();
  assert.deepEqual(types("DEMO-P-003"), ["screen"]); // pending paper
  assert.deepEqual(types("DEMO-P-001"), ["data", "fulltext"]); // included, linked fulltext, unverified dataset
  assert.deepEqual(types("DEMO-P-014"), []); // excluded paper
});

test("normalizeDataset wraps legacy single values and mirrors the first repository", () => {
  const d = normalizeDataset({ id: "DS", title: "x", species: "Zea mays", speciesLabel: "玉米", organ: "叶", modality: "scRNA-seq", technology: "10x", repository: "GEO", accession: "GSE1" });
  assert.deepEqual(d.species, [{ name: "Zea mays", label: "玉米" }]);
  assert.deepEqual([d.organs, d.modalities, d.platforms], [["叶"], ["scRNA-seq"], ["10x"]]);
  assert.equal(d.repositories.length, 1);
  assert.equal(d.repository, "geo");
  assert.equal(d.accession, "GSE1");
  assert.deepEqual(normalizeDataset({ id: "DS2", title: "y" }).repositories, []);
});

test("prepareImport validates structure, dedupes by DOI and remaps dataset links", () => {
  const current = demoState();
  const existing = current.papers[0]!;
  existing.doi = "10.1000/existing";
  assert.throws(() => prepareImport({ schemaVersion: 3, papers: [], datasets: [], resources: [] }, current), /文件结构不匹配/);
  assert.throws(() => prepareImport({ schemaVersion: 2, papers: [{ id: "a", title: "" }], datasets: [], resources: [] }, current), /题目缺失/);
  assert.throws(() => prepareImport({ schemaVersion: 2, papers: [{ id: "a", title: "t", year: 1500 }], datasets: [], resources: [] }, current), /年份/);
  assert.throws(() => prepareImport({ schemaVersion: 2, papers: [], datasets: [{ id: "d", title: "d", paperLinks: [{ paperId: "missing" }] }], resources: [] }, current), /不存在的文献/);

  const { next, stats } = prepareImport({
    schemaVersion: 1,
    papers: [
      { id: "dup", title: "duplicate", doi: "https://doi.org/10.1000/EXISTING" },
      { id: "new", title: "brand new", species: "Zea mays", organ: "叶", modality: "scRNA-seq", articleType: "原创研究", tags: ["a"] },
    ],
    datasets: [{ id: "ds", title: "data", species: "Zea mays", speciesLabel: "玉米", paperLinks: [{ paperId: "new", role: "generated" }, { paperId: "dup" }] }],
    resources: [{ paperId: "new", url: "https://example.org/x", type: "fulltext" }, { paperId: "new", url: "javascript:alert(1)" }].slice(0, 1),
  }, current);
  assert.deepEqual(stats, { papers: 1, datasets: 1, resources: 1, duplicates: 1, renamed: 0 });
  assert.equal(current.papers.length, next.papers.length - 1, "current state is left untouched");
  const added = next.papers.find((p) => p.id === "new")!;
  assert.deepEqual(added.species, [{ name: "Zea mays", label: "" }]);
  assert.deepEqual(added.articleTypes, ["原创研究"]);
  const dataset = next.datasets.find((d) => d.id === "ds")!;
  assert.deepEqual(dataset.paperLinks.map((l) => [l.paperId, l.role]), [["new", "generated"], [existing.id, "reused"]]);
  assert.equal(dataset.species[0]?.label, "玉米");
  assert.throws(() => prepareImport({ schemaVersion: 2, papers: [], datasets: [], resources: [{ paperId: existing.id, url: "javascript:alert(1)" }] }, current), /有效的 HTTP/);
});

test("csvText escapes quotes and neutralizes spreadsheet formulas", () => {
  const state = demoState();
  const p = state.papers[0]!;
  p.title = '=SUM(1) "quoted"';
  const lines = csvText([p], state.datasets).split("\r\n");
  assert.equal(lines.length, 2);
  assert.ok(lines[1]!.includes(`"'=SUM(1) ""quoted"""`));
});
