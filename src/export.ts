// Exports: BibTeX, CSV, dataset download lists and the Markdown progress
// report. The desktop sends the selected records to Go (internal/library/
// export.go), which renders the file and opens the save dialog, so Demo and
// real libraries export through the same code. The browser preview has no Go
// behind it, so renderExport below mirrors those formats in TypeScript and
// the file downloads instead.
import type { App } from "./app.ts";
import type { Dataset, Paper, SpeciesEntry } from "./mygo.ts";
import { Library } from "./mygo.ts";
import { arr, esc, filterDatasets, filterPapers } from "./model.ts";
import { t } from "./i18n.ts";
import { I, downloadFile, errorMessage, option } from "./ui.ts";

export type ExportKind = "bibtex" | "papers-csv" | "report" | "datasets-csv" | "datasets-summary" | "download-script" | "accessions";
export type ExportScope = "papers" | "datasets";

const PAPER_FORMATS: [ExportKind, string, string][] = [
  ["bibtex", "BibTeX (.bib)", t("导入 Zotero、EndNote 或 LaTeX 参考文献。")],
  ["papers-csv", t("CSV 表格 (.csv)"), t("每行一篇：题目、作者、期刊、年份、DOI、物种、组织、测序类型、平台、文献类型、收录状态、关联数据集 accession。")],
  ["report", t("筛选进度报告 (Markdown)"), t("纳入、不纳入、待核对数量，按物种 / 测序类型 / 组织的数据集统计，以及已收录文献清单。")],
];
const DATASET_FORMATS: [ExportKind, string, string][] = [
  ["datasets-csv", t("数据集清单 CSV (.csv)"), t("每行一个数据集：全部 accession、仓库类型、关联文献 DOI、整理状态、可用性。")],
  ["datasets-summary", t("汇总统计 CSV (.csv)"), t("按物种、组织或测序类型统计数据集数量、已核实数和公开数。")],
  ["download-script", t("批量下载脚本 (.sh)"), t("按 accession 类型调用 wget、prefetch / fasterq-dump、ENA 等工具的 bash 脚本，只生成文件，不在此运行。")],
  ["accessions", t("accession 列表 (.txt)"), t("每行一个 accession，方便交给下载工具或 xargs。")],
];
const GROUPS: Record<string, string> = { species: t("物种"), organ: t("组织"), modality: t("测序类型") };

const radio = (name: string, value: string, label: string, hint: string, checked: boolean): string =>
  `<label class="radio-choice"><input type="radio" name="${name}" value="${esc(value)}"${checked ? " checked" : ""}><span><strong>${esc(label)}</strong>${hint ? `<small class="muted">${esc(hint)}</small>` : ""}</span></label>`;

/** exportModal asks for a range and a format, then runs the export on submit. */
export function exportModal(app: App, scope: ExportScope, preset: ExportKind | "" = ""): void {
  let ranges: [string, string][];
  let formats: [ExportKind, string, string][];
  if (scope === "papers") {
    const filtered = filterPapers(app.state.papers, app.filters).length;
    const included = app.state.papers.filter((p) => p.screening === "included").length;
    ranges = [["filtered", t("当前筛选结果（{n} 篇）", { n: filtered })], ["included", t("全部已收录（{n} 篇）", { n: included })], ["all", t("全部文献（{n} 篇）", { n: app.state.papers.length })]];
    // Papers ticked in the table come first, so the bar's export button needs no extra step.
    if (app.selectedPapers.size) ranges.unshift(["selected", t("已勾选的文献（{n} 篇）", { n: app.selectedPapers.size })]);
    formats = PAPER_FORMATS;
  } else {
    const filtered = filterDatasets(app.state.datasets, app.datasetSearch, (id) => app.paperById(id)?.title ?? "", app.datasetReview).length;
    ranges = [["filtered", t("当前搜索结果（{n} 组）", { n: filtered })], ["all", t("全部数据集（{n} 组）", { n: app.state.datasets.length })]];
    formats = DATASET_FORMATS;
  }
  const chosen = preset || formats[0]![0];
  const body = `<form id="export-form"><input type="hidden" name="scope" value="${scope}">`
    + `<fieldset class="export-group"><legend>${t("范围")}</legend>${ranges.map(([value, label], i) => radio("range", value, label, "", i === 0)).join("")}</fieldset>`
    + `<fieldset class="export-group"><legend>${t("格式")}</legend>${formats.map(([value, label, hint]) => radio("kind", value, label, hint, value === chosen)).join("")}</fieldset>`
    + (scope === "datasets"
      ? `<label class="form-field export-option"><span>${t("汇总维度（仅汇总统计）")}</span><select class="input" name="groupBy">${Object.entries(GROUPS).map(([v, l]) => option(v, l, "species")).join("")}</select></label>`
      : `<label class="form-field export-option"><span>${t("报告标题（仅进度报告）")}</span><input class="input" name="title" type="text" maxlength="200" placeholder="BiblioCell screening progress"></label>`)
    + `<p class="muted small">${app.env.desktop ? t("导出会打开保存对话框，由你选择位置；什么都不会写入数据库。") : t("浏览器预览中文件直接下载。")}</p></form>`;
  app.showModal(scope === "papers" ? t("导出文献") : t("导出数据集"), body, `<button class="btn btn-secondary" data-action="close-modal">${t("取消")}</button><button class="btn btn-primary" type="submit" form="export-form">${I("download", 16)} ${t("导出")}</button>`);
}

/** runExport reads the modal form, picks the records, and hands them to Go (desktop) or downloads them (browser). */
export function runExport(app: App, form: HTMLFormElement): void {
  const data = new FormData(form);
  const scope = data.get("scope") === "datasets" ? "datasets" : "papers";
  const range = String(data.get("range") ?? "filtered");
  const kind = String(data.get("kind") ?? "") as ExportKind;
  const groupBy = String(data.get("groupBy") ?? "species");
  const title = String(data.get("title") ?? "").trim();
  let papers = app.state.papers;
  let datasets = app.state.datasets;
  if (scope === "papers") {
    if (range === "filtered") papers = filterPapers(papers, app.filters);
    else if (range === "included") papers = papers.filter((p) => p.screening === "included");
    else if (range === "selected") papers = papers.filter((p) => app.selectedPapers.has(p.id));
  } else if (range === "filtered") {
    datasets = filterDatasets(datasets, app.datasetSearch, (id) => app.paperById(id)?.title ?? "", app.datasetReview);
  }
  const count = scope === "papers" ? papers.length : datasets.length;
  if (!count) {
    app.toast(scope === "papers" ? t("所选范围内没有文献。") : t("所选范围内没有数据集。"), true);
    return;
  }
  app.closeModal();
  if (!app.env.desktop) {
    const file = renderExport({ kind, papers, datasets, groupBy, title }, new Date());
    downloadFile(file.name, file.content, MIME[kind] ?? "text/plain;charset=utf-8");
    app.toast(t("已导出 {n} 条记录。", { n: count }));
    return;
  }
  Library.exportLibrary({ kind, papers, datasets, groupBy, title }).then(
    (result) => {
      if (result.path) app.toast(t("已导出 {n} 条记录到 {path}", { n: result.count, path: result.path }));
    },
    (error) => app.toast(errorMessage(error, t("导出失败，请重试。")), true),
  );
}

const MIME: Record<string, string> = {
  bibtex: "application/x-bibtex;charset=utf-8", "papers-csv": "text/csv;charset=utf-8", "datasets-csv": "text/csv;charset=utf-8", "datasets-summary": "text/csv;charset=utf-8",
  "download-script": "text/x-shellscript;charset=utf-8", accessions: "text/plain;charset=utf-8", report: "text/markdown;charset=utf-8",
};

// ---- TypeScript rendering for the browser preview (mirrors internal/library/export.go) ----

export interface ExportInput {
  kind: ExportKind;
  papers: Paper[];
  datasets: Dataset[];
  groupBy?: string;
  title?: string;
}

export interface ExportFile {
  name: string;
  content: string;
}

const day = (now: Date): string => now.toISOString().slice(0, 10);
const JOIN = "; ";
const clean = (values: (string | null | undefined)[] | null | undefined): string[] => [...new Set(arr(values).map((v) => (v ?? "").trim()).filter(Boolean))];
const names = (species: SpeciesEntry[] | null | undefined): string[] => clean(arr(species).map((s) => s?.name));
const labels = (species: SpeciesEntry[] | null | undefined): string[] => clean(arr(species).map((s) => (s?.label || s?.name) ?? ""));
const yearText = (year: number | null | undefined): string => (year ? String(year) : "");
const datasetsOf = (datasets: Dataset[], paperId: string): Dataset[] => datasets.filter((d) => arr(d.paperLinks).some((l) => l.paperId === paperId));
const accessionsOf = (d: Dataset): string[] => clean(arr(d.repositories).map((r) => r.accession));

export function renderExport(input: ExportInput, now: Date): ExportFile {
  const { papers, datasets } = input;
  switch (input.kind) {
    case "bibtex": return { name: `bibliocell-papers-${day(now)}.bib`, content: bibtex(papers) };
    case "papers-csv": return { name: `bibliocell-papers-${day(now)}.csv`, content: papersCSV(papers, datasets) };
    case "datasets-csv": return { name: `bibliocell-datasets-${day(now)}.csv`, content: datasetsCSV(datasets, papers) };
    case "datasets-summary": {
      const by = summaryDimension(input.groupBy);
      return { name: `bibliocell-datasets-by-${by}-${day(now)}.csv`, content: summaryCSV(datasets, by) };
    }
    case "download-script": return { name: `bibliocell-download-${day(now)}.sh`, content: downloadScript(datasets, papers, now) };
    case "accessions": return { name: `bibliocell-accessions-${day(now)}.txt`, content: accessionList(datasets) };
    case "report": return { name: `bibliocell-report-${day(now)}.md`, content: report(papers, datasets, input.title ?? "", now) };
  }
  throw new Error(`unknown export kind ${String(input.kind)}`);
}

function summaryDimension(value = ""): "species" | "organ" | "modality" {
  const v = value.trim().toLowerCase();
  if (v === "organ" || v === "organs" || v === "tissue") return "organ";
  if (v === "modality" || v === "modalities") return "modality";
  return "species";
}

// BibTeX
const BIB_ESCAPES: Record<string, string> = {
  "\\": "\\textbackslash{}", "&": "\\&", "%": "\\%", $: "\\$", "#": "\\#", _: "\\_", "{": "\\{", "}": "\\}", "~": "\\textasciitilde{}", "^": "\\textasciicircum{}",
};
/** bibEscape protects LaTeX specials in one pass, so an escape's own braces stay intact. */
export const bibEscape = (value: string): string => value.trim().replace(/[\\&%$#_{}~^]/g, (c) => BIB_ESCAPES[c] ?? c);
const asciiKey = (value: string): string => value.replace(/[^A-Za-z0-9]/g, "");
export function bibKey(p: Paper): string {
  const first = (p.authors ?? "").split(";")[0]!.trim();
  let surname = first.includes(",") ? first.slice(0, first.indexOf(",")) : (first.split(/\s+/).at(-1) ?? "");
  surname = asciiKey(surname) || "anon";
  const word = (p.title ?? "").split(/\s+/).map(asciiKey).find((w) => w.length > 3) ?? "";
  return (surname + yearText(p.year) + word).toLowerCase() || "paper";
}
export function bibAuthors(authors: string): string {
  let sep = ";";
  if (!authors.includes(";") && !authors.includes(" and ") && authors.includes(",") && authors.split(",").every((piece) => piece.trim().split(/\s+/).length >= 2)) sep = ",";
  return authors.split(sep).map((n) => n.trim()).filter(Boolean).join(" and ");
}
export function bibtex(papers: Paper[]): string {
  const used = new Map<string, number>();
  return papers.map((p) => {
    const base = bibKey(p);
    const n = used.get(base) ?? 0;
    used.set(base, n + 1);
    const key = n ? base + String.fromCharCode(96 + n) : base;
    const keywords = clean([...names(p.species), ...arr(p.organs), ...arr(p.modalities), ...arr(p.platforms)]);
    const fields: [string, string][] = [
      ["title", `{${bibEscape(p.title ?? "")}}`], ["author", bibEscape(bibAuthors(p.authors ?? ""))], ["journal", bibEscape(p.journal ?? "")],
      ["year", yearText(p.year)], ["doi", bibEscape(p.doi ?? "")], ["url", p.doi ? `https://doi.org/${bibEscape(p.doi)}` : ""], ["pmid", bibEscape(p.pmid ?? "")],
      ["note", p.publicationStatus === "preprint" ? "Preprint" : ""], ["keywords", bibEscape(keywords.join(", "))],
      ["abstract", bibEscape((p.abstract ?? "").split(/\s+/).join(" "))],
    ];
    return `@article{${key},\n${fields.filter(([, v]) => v).map(([k, v]) => `  ${k} = {${v}},\n`).join("")}}\n\n`;
  }).join("");
}

// CSV
export function csvTable(header: string[], rows: string[][]): string {
  const cell = (value: string): string => {
    const s = /^[=+\-@\t\r]/.test(value) ? `'${value}` : value;
    return /[",\r\n]/.test(s) ? `"${s.replaceAll('"', '""')}"` : s;
  };
  return "﻿" + [header, ...rows].map((r) => r.map(cell).join(",") + "\r\n").join("");
}
export function papersCSV(papers: Paper[], datasets: Dataset[]): string {
  const header = ["id", "title", "authors", "journal", "year", "doi", "pmid", "publication_status", "species", "species_label", "organs", "modalities", "platforms", "article_types", "screening", "screening_reason", "fulltext", "reading", "tags", "dataset_accessions", "dataset_ids", "source", "added_at"];
  return csvTable(header, papers.map((p) => {
    const linked = datasetsOf(datasets, p.id);
    return [
      p.id, p.title, p.authors, p.journal, yearText(p.year), p.doi, p.pmid, p.publicationStatus,
      names(p.species).join(JOIN), labels(p.species).join(JOIN), arr(p.organs).join(JOIN), arr(p.modalities).join(JOIN), arr(p.platforms).join(JOIN), arr(p.articleTypes).join(JOIN),
      p.screening, p.screeningReason, p.fulltext, p.reading, arr(p.tags).join(JOIN),
      linked.flatMap(accessionsOf).join(JOIN), linked.map((d) => d.id).join(JOIN), p.source, p.addedAt,
    ].map((v) => v ?? "");
  }));
}
export function datasetsCSV(datasets: Dataset[], papers: Paper[]): string {
  const header = ["id", "title", "species", "species_label", "organs", "modalities", "platforms", "accessions", "repositories", "accession_kinds", "verified_accessions", "status", "curation_level", "availability", "matrix_availability", "annotation_availability", "cells_reported", "paper_dois", "paper_ids", "paper_roles", "awaiting_review", "added_at"];
  const byId = new Map(papers.map((p) => [p.id, p]));
  return csvTable(header, datasets.map((d) => {
    const records = arr(d.repositories).filter((r) => r.accession?.trim());
    const classes = records.map((r) => classifyAccession(r.repository, r.accession));
    const links = arr(d.paperLinks);
    return [
      d.id, d.title, names(d.species).join(JOIN), labels(d.species).join(JOIN), arr(d.organs).join(JOIN), arr(d.modalities).join(JOIN), arr(d.platforms).join(JOIN),
      records.map((r) => r.accession.trim()).join(JOIN), [...new Set(classes.map((c) => c.repository))].sort().join(JOIN), classes.map((c) => `${c.repository}/${c.kind}`).join(JOIN),
      records.filter((r) => r.verified).map((r) => r.accession.trim()).join(JOIN),
      d.status, d.curationLevel, d.availability, d.matrixAvailability, d.annotationAvailability, d.cellsReported == null ? "" : String(d.cellsReported),
      links.map((l) => byId.get(l.paperId)?.doi ?? "").filter(Boolean).join(JOIN), links.map((l) => l.paperId).join(JOIN), links.map((l) => l.role).join(JOIN),
      String(d.awaitingReview === true), d.addedAt,
    ].map((v) => v ?? "");
  }));
}

interface Group { key: string; datasets: number; verified: number; public: number; papers: Set<string>; accessions: number }
const UNSPECIFIED = "(unspecified)";
export function summarize(datasets: Dataset[], by: "species" | "organ" | "modality"): Group[] {
  const groups = new Map<string, Group>();
  for (const d of datasets) {
    let keys = clean(by === "organ" ? d.organs : by === "modality" ? d.modalities : names(d.species));
    if (!keys.length) keys = [UNSPECIFIED];
    for (const key of keys) {
      const g = groups.get(key) ?? { key, datasets: 0, verified: 0, public: 0, papers: new Set<string>(), accessions: 0 };
      groups.set(key, g);
      g.datasets++;
      if (d.status === "verified") g.verified++;
      if (d.availability === "public") g.public++;
      for (const l of arr(d.paperLinks)) g.papers.add(l.paperId);
      g.accessions += accessionsOf(d).length;
    }
  }
  return [...groups.values()].sort((a, b) => b.datasets - a.datasets || (a.key < b.key ? -1 : a.key > b.key ? 1 : 0));
}
export const summaryCSV = (datasets: Dataset[], by: "species" | "organ" | "modality"): string =>
  csvTable([by, "datasets", "verified", "public", "linked_papers", "accessions"], summarize(datasets, by).map((g) => [g.key, String(g.datasets), String(g.verified), String(g.public), String(g.papers.size), String(g.accessions)]));

// Accessions
export interface AccessionClass { repository: string; kind: string }
const PREFIXES: [RegExp, string, string][] = [
  [/^GSE\d+$/, "geo", "series"], [/^GSM\d+$/, "geo", "sample"], [/^GDS\d+$/, "geo", "dataset"], [/^GPL\d+$/, "geo", "platform"],
  [/^SRP\d+$/, "sra", "study"], [/^SRX\d+$/, "sra", "experiment"], [/^SRR\d+$/, "sra", "run"], [/^SRS\d+$/, "sra", "sample"], [/^SRA\d+$/, "sra", "submission"],
  [/^ERP\d+$/, "ena", "study"], [/^ERX\d+$/, "ena", "experiment"], [/^ERR\d+$/, "ena", "run"], [/^ERS\d+$/, "ena", "sample"],
  [/^DRP\d+$/, "ddbj", "study"], [/^DRX\d+$/, "ddbj", "experiment"], [/^DRR\d+$/, "ddbj", "run"], [/^DRS\d+$/, "ddbj", "sample"],
  [/^PRJNA\d+$/, "bioproject", "project"], [/^PRJEB\d+$/, "ena", "project"], [/^PRJDB\d+$/, "ddbj", "project"], [/^SAM[NED][A-Z]?\d+$/, "biosample", "sample"],
  [/^PRJCA\d+$/, "gsa", "project"], [/^CRA\d+$/, "gsa", "study"], [/^CRX\d+$/, "gsa", "experiment"], [/^CRR\d+$/, "gsa", "run"], [/^HRA\d+$/, "gsa", "human-study"],
  [/^OEP\d+$/, "nodeomics", "project"], [/^E-[A-Z]{4}-\d+$/, "arrayexpress", "experiment"], [/^S-[A-Z]{4}\d+$/, "biostudies", "study"], [/^STT\d+$/, "stomics", "dataset"],
  [/^10\.5281\/ZENODO\.\d+$/, "zenodo", "record"], [/^10\.6084\/M9\.FIGSHARE\.\d+/, "figshare", "record"], [/^10\.5061\/DRYAD\./, "dryad", "record"],
];
/** classifyAccession recognises the prefix first and falls back to the recorded repository. */
export function classifyAccession(repository: string, accession: string): AccessionClass {
  const acc = (accession ?? "").trim().toUpperCase();
  for (const [re, repo, kind] of PREFIXES) if (re.test(acc)) return { repository: repo, kind };
  return { repository: (repository ?? "").trim().toLowerCase() || "other", kind: "record" };
}
export const accessionList = (datasets: Dataset[]): string => {
  const all = [...new Set(datasets.flatMap(accessionsOf))];
  return all.length ? all.join("\n") + "\n" : "";
};
export function geoSeriesURL(accession: string): string {
  const acc = accession.toUpperCase();
  const digits = acc.slice(3);
  const stem = digits.length > 3 ? `GSE${digits.slice(0, -3)}nnn` : "GSEnnn";
  return `https://ftp.ncbi.nlm.nih.gov/geo/series/${stem}/${acc}/`;
}
const shellQuote = (value: string): string => `'${value.replaceAll("'", "'\\''")}'`;
const SCRIPT_HEAD = `# Review before running. Needs: wget or curl; sra-tools (prefetch, fasterq-dump)
# for SRA accessions; entrez-direct (esearch, efetch) for BioProjects.
# Usage: OUT=/data/scrna bash bibliocell-download.sh   (OUT defaults to ./bibliocell-data)
set -euo pipefail
OUT="\${OUT:-./bibliocell-data}"
mkdir -p "$OUT"
THREADS="\${THREADS:-4}"

geo_series() { # GEO series: supplementary files plus the series SOFT family file
  local acc=$1 url=$2
  mkdir -p "$OUT/geo/$acc"
  wget -q --show-progress -r -np -nd -N -P "$OUT/geo/$acc" "\${url}suppl/" || echo "no supplementary files for $acc" >&2
  wget -q --show-progress -N -P "$OUT/geo/$acc" "\${url}soft/\${acc}_family.soft.gz" || true
}
geo_sample() { # GEO sample: its own supplementary files
  local acc=$1 stem=\${1:0:\${#1}-3}nnn
  mkdir -p "$OUT/geo/$acc"
  wget -q --show-progress -r -np -nd -N -P "$OUT/geo/$acc" "https://ftp.ncbi.nlm.nih.gov/geo/samples/$stem/$acc/suppl/"
}
sra() { # SRA study, experiment, sample, or run: prefetch resolves it to runs
  local acc=$1 run
  mkdir -p "$OUT/sra" "$OUT/sra/fastq"
  prefetch --max-size u --output-directory "$OUT/sra" "$acc"
  for run in "$OUT/sra"/[SED]RR*/; do
    [ -d "$run" ] || continue
    run=$(basename "$run")
    [ -e "$OUT/sra/fastq/\${run}_1.fastq" ] || [ -e "$OUT/sra/fastq/\${run}.fastq" ] \\
      || fasterq-dump --split-files --threads "$THREADS" --outdir "$OUT/sra/fastq" "$OUT/sra/$run"
  done
}
ena() { # ENA / DDBJ accession: fastq links from the ENA portal file report
  local acc=$1
  mkdir -p "$OUT/ena/$acc"
  curl -s "https://www.ebi.ac.uk/ena/portal/api/filereport?accession=$acc&result=read_run&fields=run_accession,fastq_ftp&format=tsv" \\
    | tail -n +2 | cut -f2 | tr ';' '\\n' | grep . | sed 's#^#https://#' \\
    | wget -q --show-progress -N -P "$OUT/ena/$acc" -i -
}
bioproject() { # NCBI BioProject: list its SRA runs, then prefetch each
  local acc=$1
  mkdir -p "$OUT/sra"
  esearch -db sra -query "$acc" | efetch -format runinfo | cut -d, -f1 | grep -E '^[SED]RR' | sort -u \\
    | while read -r run; do sra "$run"; done
}
manual() { echo "manual: $1  $2" >&2; }

`;
const section = (c: AccessionClass): string =>
  c.repository === "geo" ? `GEO (${c.kind})` : c.repository === "sra" ? "SRA (prefetch + fasterq-dump)" : c.repository === "ena" || c.repository === "ddbj" ? "ENA / DDBJ (portal file report)" : c.repository === "bioproject" ? "BioProject (entrez-direct + prefetch)" : `Manual (${c.repository})`;
function accessionPage(c: AccessionClass, accession: string): string {
  switch (c.repository) {
    case "gsa": return `https://ngdc.cncb.ac.cn/search/all?q=${accession}`;
    case "biosample": return `https://www.ncbi.nlm.nih.gov/biosample/${accession}`;
    case "arrayexpress": case "biostudies": return `https://www.ebi.ac.uk/biostudies/studies/${accession}`;
    case "stomics": return "https://db.cngb.org/stomics/";
    case "zenodo": return `https://zenodo.org/records/${accession.toLowerCase().replace(/^10\.5281\/zenodo\./, "")}`;
    case "figshare": case "dryad": return `https://doi.org/${accession}`;
    case "ena": case "ddbj": return `https://www.ebi.ac.uk/ena/browser/view/${accession}`;
  }
  return `https://www.google.com/search?q=${accession}`;
}
export function downloadScript(datasets: Dataset[], papers: Paper[], now: Date): string {
  const byId = new Map(papers.map((p) => [p.id, p]));
  const sections = new Map<string, string[]>();
  const seen = new Set<string>();
  for (const d of datasets) {
    for (const r of arr(d.repositories)) {
      const acc = (r.accession ?? "").trim();
      if (!acc || seen.has(acc)) continue;
      seen.add(acc);
      const c = classifyAccession(r.repository, acc);
      const dois = arr(d.paperLinks).map((l) => byId.get(l.paperId)?.doi ?? "").filter(Boolean);
      const note = `${d.title}${dois.length ? ` (doi: ${dois.join(", ")})` : ""}`.split(/\s+/).join(" ");
      const upper = acc.toUpperCase();
      let line: string;
      if (c.repository === "geo" && c.kind === "series") line = `geo_series ${upper} ${shellQuote(geoSeriesURL(upper))}`;
      else if (c.repository === "geo" && c.kind === "sample") line = `geo_sample ${upper}`;
      else if (c.repository === "sra") line = `sra ${upper}`;
      else if ((c.repository === "ena" || c.repository === "ddbj") && c.kind !== "sample") line = `ena ${upper}`;
      else if (c.repository === "bioproject") line = `bioproject ${upper}`;
      else line = `manual ${shellQuote(acc)} ${shellQuote(r.url?.trim() || accessionPage(c, acc))}`;
      const key = section(c);
      sections.set(key, [...(sections.get(key) ?? []), `# ${note}\n${line}\n`]);
    }
  }
  const head = `#!/usr/bin/env bash\n# BiblioCell download list, ${day(now)}. ${datasets.length} datasets.\n${SCRIPT_HEAD}`;
  if (!sections.size) return head + "# No accessions recorded for the selected datasets.\n";
  return head + [...sections.keys()].sort().map((key) => `# ---- ${key} ----\n${sections.get(key)!.join("")}\n`).join("");
}

// Report
export function report(papers: Paper[], datasets: Dataset[], title: string, now: Date): string {
  const heading = title.trim() || "BiblioCell screening progress";
  const stamp = now.toISOString().slice(0, 16).replace("T", " ");
  const count = (s: string) => papers.filter((p) => p.screening === s).length;
  const proposals = papers.filter((p) => p.proposedScreening && p.proposedScreening !== p.screening).length;
  const awaiting = papers.filter((p) => p.awaitingReview).length;
  const included = papers.filter((p) => p.screening === "included");
  const fulltext = included.filter((p) => p.fulltext === "stored").length;
  const lines = [`# ${heading}`, "", `Generated ${stamp} · ${papers.length} papers · ${datasets.length} datasets`, "", "## Screening", "", "| Status | Papers |", "|---|---:|",
    `| Included | ${count("included")} |`, `| Excluded | ${count("excluded")} |`, `| Uncertain | ${count("uncertain")} |`, `| Pending | ${count("pending")} |`,
    `| **Needs a decision** (pending + uncertain) | **${count("pending") + count("uncertain")}** |`, `| Open agent proposals | ${proposals} |`, `| Added by source updates, unchecked | ${awaiting} |`];
  if (included.length) lines.push(`| Included with stored full text | ${fulltext} / ${included.length} |`);
  const status = (s: string) => datasets.filter((d) => d.status === s).length;
  lines.push("", "## Datasets", "", "| Curation | Datasets |", "|---|---:|", `| Verified | ${status("verified")} |`, `| Identified | ${status("identified")} |`, `| Pending | ${status("pending")} |`, "", "| Availability | Datasets |", "|---|---:|");
  for (const key of ["public", "restricted", "embargoed", "unavailable", "unknown"]) {
    const n = datasets.filter((d) => d.availability === key).length;
    if (n) lines.push(`| ${key} | ${n} |`);
  }
  lines.push("");
  const dims: ["species" | "modality" | "organ", string, (p: Paper) => string[]][] = [["species", "By species", (p) => names(p.species)], ["modality", "By modality", (p) => arr(p.modalities)], ["organ", "By organ", (p) => arr(p.organs)]];
  for (const [by, headingText, pick] of dims) {
    const groups = summarize(datasets, by);
    const paperCounts = new Map<string, number>();
    for (const p of included) for (const key of clean(pick(p))) paperCounts.set(key, (paperCounts.get(key) ?? 0) + 1);
    const listed = new Set(groups.map((g) => g.key));
    for (const key of [...paperCounts.keys()].filter((k) => !listed.has(k)).sort()) groups.push({ key, datasets: 0, verified: 0, public: 0, papers: new Set(), accessions: 0 });
    if (!groups.length) continue;
    lines.push(`## ${headingText}`, "", `| ${by[0]!.toUpperCase() + by.slice(1)} | Datasets | Verified | Public | Included papers |`, "|---|---:|---:|---:|---:|");
    for (const g of groups) lines.push(`| ${g.key} | ${g.datasets} | ${g.verified} | ${g.public} | ${paperCounts.get(g.key) ?? 0} |`);
    lines.push("");
  }
  if (included.length) {
    lines.push("## Included papers", "");
    for (const p of included) {
      const accs = datasetsOf(datasets, p.id).flatMap(accessionsOf);
      lines.push(`- ${p.title.trim()}${p.year ? ` (${p.year})` : ""}${p.doi ? ` — https://doi.org/${p.doi}` : ""}${accs.length ? ` · ${accs.join(", ")}` : ""}`);
    }
  }
  return lines.join("\n") + "\n";
}
