// Browser-side export rendering (src/export.ts), the TypeScript mirror of
// internal/library/export.go used by the browser preview.
import assert from "node:assert/strict";
import { test } from "node:test";
import { accessionList, bibAuthors, bibEscape, bibtex, classifyAccession, csvTable, datasetsCSV, downloadScript, geoSeriesURL, papersCSV, renderExport, report, summaryCSV } from "../src/export.ts";
import { normalizeDataset } from "../src/model.ts";
import type { Dataset, Paper } from "../src/mygo.ts";

const paper = (over: Partial<Paper>): Paper => ({
  id: "P-1", title: "Single-cell atlas of Arabidopsis & rice roots: 100% coverage", doi: "10.1000/abc_1", pmid: "12345", authors: "Zhang, Wei; O'Brien, Ann", year: 2024,
  journal: "Nature Plants", publicationStatus: "published", articleTypes: ["原创研究"], species: [{ name: "Arabidopsis thaliana", label: "拟南芥" }, { name: "Oryza sativa", label: "水稻" }],
  organs: ["root", "leaf"], modalities: ["scRNA-seq", "snRNA-seq"], platforms: ["10x Genomics Chromium"], abstract: "Line one\nline two", notes: "", tags: ["atlas"],
  screening: "included", screeningReason: "", reviewedAt: "", proposedScreening: "", proposedReason: "", awaitingReview: false, reading: "read", fulltext: "stored", fulltextUrl: "",
  starred: false, source: "PubMed", batchId: "", addedAt: "2026-10-01T00:00:00Z", updatedAt: "", isDemo: false, ...over,
});
const papers: Paper[] = [
  paper({}),
  paper({ id: "P-2", title: 'A "quoted" title, with comma', authors: "Zhang, Wei", journal: "bioRxiv", publicationStatus: "preprint", screening: "excluded", screeningReason: "=review only", doi: "", pmid: "", species: [], organs: [], modalities: [], platforms: [], abstract: "", tags: [], fulltext: "missing" }),
  paper({ id: "P-3", title: "Pending paper", authors: "", year: null, screening: "pending", proposedScreening: "included", awaitingReview: true, doi: "", pmid: "", species: [], organs: [], modalities: [], platforms: [], abstract: "", journal: "", fulltext: "missing" }),
  paper({ id: "P-4", title: "Uncertain paper", authors: "", year: null, screening: "uncertain", doi: "", pmid: "", species: [{ name: "Zea mays", label: "" }], organs: [], modalities: [], platforms: [], abstract: "", journal: "", fulltext: "missing" }),
];
const datasets: Dataset[] = [
  normalizeDataset({ id: "DS-1", title: "Root atlas data", species: [{ name: "Arabidopsis thaliana", label: "拟南芥" }], organs: ["root"], modalities: ["scRNA-seq"], platforms: ["10x Genomics Chromium"], repositories: [{ repository: "geo", accession: "GSE123456", verified: true }, { repository: "sra", accession: "SRP111111" }], status: "verified", availability: "public", matrixAvailability: "public", cellsReported: 12000, paperLinks: [{ paperId: "P-1", role: "generated" }] }),
  normalizeDataset({ id: "DS-2", title: "Rice ENA run", species: [{ name: "Oryza sativa" }], modalities: ["snRNA-seq"], repositories: [{ repository: "ena", accession: "ERR222222" }, { repository: "bioproject", accession: "PRJEB33333" }, { repository: "gsa", accession: "CRA004444", url: "https://ngdc.cncb.ac.cn/gsa/browse/CRA004444" }], status: "pending", paperLinks: [{ paperId: "P-1", role: "reused" }, { paperId: "P-2", role: "unknown" }] }),
  normalizeDataset({ id: "DS-3", title: "No accession yet", status: "identified", availability: "restricted" }),
];
const parseCSV = (text: string): string[][] => {
  assert.ok(text.startsWith("﻿"), "BOM");
  return text.slice(1).trimEnd().split("\r\n").map((line) => {
    const cells: string[] = [];
    const re = /"((?:[^"]|"")*)"|([^,]*)/y;
    for (let i = 0; i <= line.length;) {
      re.lastIndex = i;
      const m = re.exec(line)!;
      cells.push(m[1] !== undefined ? m[1].replaceAll('""', '"') : m[2]!);
      i = re.lastIndex + 1;
      if (re.lastIndex === line.length) break;
    }
    return cells;
  });
};

test("bibtex escapes LaTeX specials, joins authors and suffixes duplicate keys", () => {
  const out = bibtex(papers);
  for (const want of ["@article{zhang2024singlecell,", "title = {{Single-cell atlas of Arabidopsis \\& rice roots: 100\\% coverage}},", "author = {Zhang, Wei and O'Brien, Ann},", "doi = {10.1000/abc\\_1},", "url = {https://doi.org/10.1000/abc\\_1},", "keywords = {Arabidopsis thaliana, Oryza sativa, root, leaf, scRNA-seq, snRNA-seq, 10x Genomics Chromium},", "abstract = {Line one line two},", "@article{zhang2024quoted,", "note = {Preprint},", "@article{anonpending,"]) {
    assert.ok(out.includes(want), `missing ${want}\n${out}`);
  }
  assert.ok(!out.includes("journal = {},"));
  const dup = bibtex([papers[0]!, papers[0]!, papers[0]!]);
  assert.ok(dup.includes("{zhang2024singlecella,") && dup.includes("{zhang2024singlecellb,"));
  assert.equal(bibEscape("a\\b & c % d $ e # f _ g {h} i ~ j ^ k"), "a\\textbackslash{}b \\& c \\% d \\$ e \\# f \\_ g \\{h\\} i \\textasciitilde{} j \\textasciicircum{} k");
  assert.equal(bibAuthors("Alice Smith, Bob Jones"), "Alice Smith and Bob Jones");
  assert.equal(bibAuthors("Smith, Alice"), "Smith, Alice");
});

test("papersCSV joins multi-valued fields and neutralises formulas", () => {
  const rows = parseCSV(papersCSV(papers, datasets));
  const col = Object.fromEntries(rows[0]!.map((name, i) => [name, i]));
  const first = rows[1]!;
  assert.equal(first[col.species!], "Arabidopsis thaliana; Oryza sativa");
  assert.equal(first[col.species_label!], "拟南芥; 水稻");
  assert.equal(first[col.modalities!], "scRNA-seq; snRNA-seq");
  assert.equal(first[col.dataset_accessions!], "GSE123456; SRP111111; ERR222222; PRJEB33333; CRA004444");
  assert.equal(first[col.dataset_ids!], "DS-1; DS-2");
  assert.equal(rows[2]![col.title!], 'A "quoted" title, with comma');
  assert.equal(rows[2]![col.screening_reason!], "'=review only");
  assert.equal(rows[3]![col.year!], "");
  assert.equal(csvTable(["a"], [["x,y"], ["-1"]]), '﻿a\r\n"x,y"\r\n\'-1\r\n');
});

test("datasetsCSV lists every accession with its kind; summary groups by dimension", () => {
  const rows = parseCSV(datasetsCSV(datasets, papers));
  const col = Object.fromEntries(rows[0]!.map((name, i) => [name, i]));
  assert.equal(rows[1]![col.accessions!], "GSE123456; SRP111111");
  assert.equal(rows[1]![col.accession_kinds!], "geo/series; sra/study");
  assert.equal(rows[1]![col.verified_accessions!], "GSE123456");
  assert.equal(rows[1]![col.paper_dois!], "10.1000/abc_1");
  assert.equal(rows[1]![col.cells_reported!], "12000");
  assert.equal(rows[2]![col.repositories!], "ena; gsa");
  assert.equal(rows[2]![col.paper_roles!], "reused; unknown");
  assert.equal(rows[3]![col.accessions!], "");
  assert.deepEqual(parseCSV(summaryCSV(datasets, "species")), [
    ["species", "datasets", "verified", "public", "linked_papers", "accessions"],
    ["(unspecified)", "1", "0", "0", "0", "0"],
    ["Arabidopsis thaliana", "1", "1", "1", "1", "2"],
    ["Oryza sativa", "1", "0", "0", "2", "3"],
  ]);
});

test("classifyAccession goes by prefix, then the recorded repository", () => {
  const cases: [string, string, string, string][] = [
    ["geo", "GSE123456", "geo", "series"], ["", "gse1", "geo", "series"], ["geo", "GSM999", "geo", "sample"],
    ["sra", "SRP111", "sra", "study"], ["sra", "SRR111", "sra", "run"], ["ena", "ERR111", "ena", "run"], ["ena", "PRJEB111", "ena", "project"],
    ["bioproject", "PRJNA111", "bioproject", "project"], ["", "PRJDB111", "ddbj", "project"], ["", "SAMEA123", "biosample", "sample"],
    ["gsa", "CRA001", "gsa", "study"], ["gsa", "HRA001", "gsa", "human-study"], ["other", "E-MTAB-1234", "arrayexpress", "experiment"],
    ["zenodo", "10.5281/zenodo.1234", "zenodo", "record"], ["zenodo", "1234", "zenodo", "record"], ["", "mystery-1", "other", "record"], ["geo", "SRR42", "sra", "run"],
  ];
  for (const [repo, acc, repository, kind] of cases) assert.deepEqual(classifyAccession(repo, acc), { repository, kind }, `${repo} ${acc}`);
});

test("accession list and download script cover each accession once, by tool", () => {
  assert.equal(accessionList([...datasets, datasets[0]!]), "GSE123456\nSRP111111\nERR222222\nPRJEB33333\nCRA004444\n");
  const script = downloadScript([...datasets, datasets[0]!], papers, new Date("2026-11-02T00:00:00Z"));
  for (const want of ["#!/usr/bin/env bash", "set -euo pipefail", "geo_series GSE123456 'https://ftp.ncbi.nlm.nih.gov/geo/series/GSE123nnn/GSE123456/'", "sra SRP111111", "ena ERR222222", "ena PRJEB33333", "manual 'CRA004444' 'https://ngdc.cncb.ac.cn/gsa/browse/CRA004444'", "# Root atlas data (doi: 10.1000/abc_1)", "# ---- GEO (series) ----", "# ---- Manual (gsa) ----"]) {
    assert.ok(script.includes(want), `missing ${want}\n${script}`);
  }
  assert.equal(script.split("geo_series GSE123456").length, 2);
  assert.equal(geoSeriesURL("GSE7"), "https://ftp.ncbi.nlm.nih.gov/geo/series/GSEnnn/GSE7/");
  assert.ok(downloadScript([], [], new Date()).includes("No accessions recorded"));
});

test("report counts screening, datasets and included papers", () => {
  const out = report(papers, datasets, "", new Date("2026-11-02T09:30:00Z"));
  for (const want of ["# BiblioCell screening progress", "Generated 2026-11-02 09:30 · 4 papers · 3 datasets", "| Included | 1 |", "| Excluded | 1 |", "| **Needs a decision** (pending + uncertain) | **2** |", "| Open agent proposals | 1 |", "| Added by source updates, unchecked | 1 |", "| Included with stored full text | 1 / 1 |", "| Verified | 1 |\n| Identified | 1 |\n| Pending | 1 |", "| public | 1 |", "| restricted | 1 |", "## By species", "| Arabidopsis thaliana | 1 | 1 | 1 | 1 |", "## By organ", "| leaf | 0 | 0 | 0 | 1 |", "- Single-cell atlas of Arabidopsis & rice roots: 100% coverage (2024) — https://doi.org/10.1000/abc_1 · GSE123456, SRP111111, ERR222222, PRJEB33333, CRA004444"]) {
    assert.ok(out.includes(want), `missing ${want}\n${out}`);
  }
  assert.ok(report([], [], "  My review  ", new Date()).startsWith("# My review\n"));
});

test("renderExport names files by kind and date", () => {
  const now = new Date("2026-11-02T00:00:00Z");
  const names = { bibtex: "bibliocell-papers-2026-11-02.bib", "papers-csv": "bibliocell-papers-2026-11-02.csv", report: "bibliocell-report-2026-11-02.md", "datasets-csv": "bibliocell-datasets-2026-11-02.csv", "datasets-summary": "bibliocell-datasets-by-species-2026-11-02.csv", "download-script": "bibliocell-download-2026-11-02.sh", accessions: "bibliocell-accessions-2026-11-02.txt" } as const;
  for (const [kind, name] of Object.entries(names)) {
    const file = renderExport({ kind: kind as keyof typeof names, papers, datasets }, now);
    assert.equal(file.name, name);
    assert.ok(file.content.length > 0);
  }
  assert.equal(renderExport({ kind: "datasets-summary", papers, datasets, groupBy: "organs" }, now).name, "bibliocell-datasets-by-organ-2026-11-02.csv");
});
