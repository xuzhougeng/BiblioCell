// Pure-logic tests, run with `npm test` (node:test with Node's built-in type stripping).
import assert from "node:assert/strict";
import { test } from "node:test";
import { demoState } from "../src/demo-data.ts";
import { changedItems, csvText, datasetLocation, decideScreening, emptyFilters, esc, fileSize, filterDatasets, filterPapers, nextRunText, normalizeDOI, normalizeDataset, pageList, paperPage, prepareImport, repositoryURL, shortTime, speciesDisplay, tasksFor, toggleSort, validDOI } from "../src/model.ts";

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

test("tasksFor queues unreviewed evidence on a decided paper and agent links on included ones", () => {
  const { papers, datasets } = demoState();
  const paper = { ...papers[0]!, screening: "excluded" };
  const claim = { id: "E-1", paperId: paper.id, datasetId: "", field: "organs", value: "root", sourceUrl: "", fileId: "", location: "", excerpt: "x", extractor: "", status: "unreviewed", observedAt: "" };
  const screen = (evidence: typeof claim[]) => tasksFor([paper], datasets, evidence).filter((row) => row.type === "screen");
  assert.match(screen([claim])[0]!.subtitle, /1 条证据待核对/);
  assert.equal(screen([{ ...claim, status: "verified" }]).length, 0);

  const included = { ...papers[0]!, id: "P-x", screening: "included" };
  const dataset = { ...datasets[0]!, accession: "GSE1", status: "verified", paperLinks: [{ paperId: "P-x", role: "generated", evidenceUrl: "", evidenceText: "we deposited", verified: false }] };
  const data = (d: typeof dataset) => tasksFor([included], [d]).filter((row) => row.type === "data");
  assert.equal(data(dataset).length, 1);
  assert.equal(data({ ...dataset, paperLinks: [{ ...dataset.paperLinks[0]!, verified: true }] }).length, 0);
});

test("filterPapers narrows by source", () => {
  const { papers } = demoState();
  const source = papers[0]!.source;
  const matched = filterPapers(papers, { ...emptyFilters(), source });
  assert.ok(matched.length > 0 && matched.every((p) => p.source === source));
});

test("paperPage clamps the requested page and returns slice bounds", () => {
  assert.deepEqual(paperPage(519, 10, 1), { page: 1, count: 52, start: 0, end: 10 });
  assert.deepEqual(paperPage(519, 10, 52), { page: 52, count: 52, start: 510, end: 519 });
  assert.deepEqual(paperPage(519, 10, 99), { page: 52, count: 52, start: 510, end: 519 }, "beyond the last page clamps down");
  assert.deepEqual(paperPage(519, 10, 0), { page: 1, count: 52, start: 0, end: 10 }, "non-positive requests become page 1");
  assert.deepEqual(paperPage(5, 100, 1), { page: 1, count: 1, start: 0, end: 5 });
  assert.deepEqual(paperPage(0, 50, 3), { page: 1, count: 1, start: 0, end: 0 });
  assert.deepEqual(paperPage(519, 80, 1).end, 10, "unknown sizes fall back to the smallest");
});

test("pageList keeps first, last and a window around the current page", () => {
  assert.deepEqual(pageList(3, 5), [1, 2, 3, 4, 5]);
  assert.deepEqual(pageList(1, 52), [1, 2, "gap", 52]);
  assert.deepEqual(pageList(5, 52), [1, "gap", 4, 5, 6, "gap", 52]);
  assert.deepEqual(pageList(52, 52), [1, "gap", 51, 52]);
  assert.deepEqual(pageList(2, 8), [1, 2, 3, "gap", 8]);
});

test("fileSize switches from KB to MB", () => {
  assert.equal(fileSize(10), "1 KB");
  assert.equal(fileSize(2048), "2 KB");
  assert.equal(fileSize(5 * 1024 * 1024), "5.0 MB");
});

test("nextRunText covers every automation state", () => {
  const now = Date.parse("2026-11-01T12:00:00Z");
  const base = { enabled: true, running: false, nextRunAt: "2026-11-02T12:00:00Z" };
  assert.equal(nextRunText({ ...base, running: true }, now), "正在更新");
  assert.equal(nextRunText({ ...base, enabled: false }, now), "未开启");
  assert.equal(nextRunText({ ...base, nextRunAt: "" }, now), "没有已启用的来源");
  assert.equal(nextRunText({ ...base, nextRunAt: "2026-11-01T11:59:00Z" }, now), "即将开始");
  assert.equal(nextRunText(base, now), shortTime(base.nextRunAt));
});

test("normalizeDOI strips resolver prefixes and lowercases", () => {
  assert.equal(normalizeDOI(" https://doi.org/10.1000/ABC "), "10.1000/abc");
  assert.equal(normalizeDOI("http://dx.doi.org/10.1000/abc"), "10.1000/abc");
  assert.equal(normalizeDOI("DOI: 10.1000/abc"), "10.1000/abc");
  assert.equal(normalizeDOI("https://www.doi.org/10.1000/abc."), "10.1000/abc");
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

  const byTitle = filterPapers(papers, { ...emptyFilters(), sort: "title-asc" }).map((p) => p.title);
  assert.deepEqual(byTitle, [...byTitle].sort((a, b) => a.localeCompare(b, "zh-CN")));
});

test("toggleSort flips direction and applies column defaults", () => {
  assert.equal(toggleSort("newest", "year"), "year-desc");
  assert.equal(toggleSort("year-desc", "year"), "year-asc");
  assert.equal(toggleSort("year-asc", "year"), "year-desc");
  assert.equal(toggleSort("newest", "title"), "title-asc");
  assert.equal(toggleSort("title-asc", "title"), "title-desc");
  assert.equal(toggleSort("modality-desc", "species"), "species-asc");
});

test("filterPapers sorts by year with missing years last, and by species display", () => {
  const { papers } = demoState();
  const withMissing = [...papers, { ...papers[0]!, id: "P-none", year: null }];
  const asc = filterPapers(withMissing, { ...emptyFilters(), sort: "year-asc" }).map((p) => p.year);
  const known = asc.filter((y) => y != null);
  assert.deepEqual(known, [...known].sort((a, b) => a - b));
  assert.equal(asc.at(-1), null);
  const desc = filterPapers(withMissing, { ...emptyFilters(), sort: "year-desc" }).map((p) => p.year);
  const knownDesc = desc.filter((y) => y != null);
  assert.deepEqual(knownDesc, [...knownDesc].sort((a, b) => b - a));
  assert.equal(desc.at(-1), null);

  const species = filterPapers(papers, { ...emptyFilters(), sort: "species-asc" }).map((p) => speciesDisplay(p));
  assert.deepEqual(species, [...species].sort((a, b) => a.localeCompare(b, "zh-CN")));
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

test("filterDatasets matches own fields, repositories and linked paper titles", () => {
  const { papers, datasets } = demoState();
  const title = (id: string) => papers.find((p) => p.id === id)?.title ?? "";
  assert.equal(filterDatasets(datasets, "", title).length, datasets.length);
  assert.equal(filterDatasets(datasets, "  ", title).length, datasets.length);
  const linked = datasets[0]!;
  const firstPaper = papers.find((p) => p.id === linked.paperLinks[0]!.paperId)!;
  // A dataset without repositories still surfaces through its linked paper's title.
  assert.deepEqual(filterDatasets(datasets, firstPaper.title.slice(0, 4), title).map((d) => d.id), [linked.id]);
  const withRepo = normalizeDataset({ id: "DS-GEO", title: "geo row", repository: "geo", accession: "GSE123456" });
  assert.deepEqual(filterDatasets([withRepo], "gse123456", title).map((d) => d.id), ["DS-GEO"]);
  assert.deepEqual(filterDatasets([withRepo], "拟南芥", title), []);
});

test("repositoryURL prefers the recorded url and falls back to known repositories", () => {
  assert.equal(repositoryURL({ repository: "geo", accession: "GSE123456", url: "https://example.org/data" }), "https://example.org/data");
  assert.equal(repositoryURL({ repository: "geo", accession: "GSE123456", url: "" }), "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=GSE123456");
  assert.equal(repositoryURL({ repository: "GEO", accession: "GSE1", url: "" }), "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=GSE1");
  assert.equal(repositoryURL({ repository: "ena", accession: "ERP123", url: "" }), "https://www.ebi.ac.uk/ena/browser/view/ERP123");
  assert.equal(repositoryURL({ repository: "zenodo", accession: "1234567", url: "" }), "https://zenodo.org/records/1234567");
  assert.equal(repositoryURL({ repository: "geo", accession: "", url: "" }), "", "no accession means no location");
  assert.equal(repositoryURL({ repository: "other", accession: "X1", url: "" }), "", "unknown repositories stay unlinkable");
  assert.equal(repositoryURL({ repository: "geo", accession: "GSE1", url: "javascript:alert(1)" }), "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=GSE1", "an unsafe url falls back to the repository link");
  assert.equal(datasetLocation(normalizeDataset({ id: "DS", title: "t", repository: "sra", accession: "SRR000001" })), "https://www.ncbi.nlm.nih.gov/sra/?term=SRR000001");
  assert.equal(datasetLocation(normalizeDataset({ id: "DS2", title: "t" })), "");
});
