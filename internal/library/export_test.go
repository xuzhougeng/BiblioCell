package library

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"
)

func exportFixture() ([]Paper, []Dataset) {
	year := 2024
	papers := []Paper{
		{
			ID: "P-1", Title: "Single-cell atlas of Arabidopsis & rice roots: 100% coverage", DOI: "10.1000/abc_1", PMID: "12345",
			Authors: "Zhang, Wei; O'Brien, Ann; Li Si", Year: &year, Journal: "Nature Plants", PublicationStatus: "published",
			ArticleTypes: []string{"原创研究"}, Species: []SpeciesEntry{{Name: "Arabidopsis thaliana", Label: "拟南芥"}, {Name: "Oryza sativa", Label: "水稻"}},
			Organs: []string{"root", "leaf"}, Modalities: []string{"scRNA-seq", "snRNA-seq"}, Platforms: []string{"10x Genomics Chromium"},
			Screening: "included", Fulltext: "stored", Reading: "read", Tags: []string{"atlas"}, Source: "PubMed", AddedAt: "2026-10-01T00:00:00Z",
			Abstract: "Line one\nline two",
		},
		{
			ID: "P-2", Title: "A \"quoted\" title, with comma", Authors: "Zhang, Wei", Year: &year, Journal: "bioRxiv", PublicationStatus: "preprint",
			Screening: "excluded", ScreeningReason: "=review only", Fulltext: "missing", Reading: "unread",
		},
		{ID: "P-3", Title: "Pending paper", Screening: "pending", Fulltext: "missing", Reading: "unread", ProposedScreening: "included", AwaitingReview: true},
		{ID: "P-4", Title: "Uncertain paper", Screening: "uncertain", Fulltext: "missing", Reading: "unread", Species: []SpeciesEntry{{Name: "Zea mays"}}},
	}
	cells := 12000
	datasets := []Dataset{
		{
			ID: "DS-1", Title: "Root atlas data", Species: []SpeciesEntry{{Name: "Arabidopsis thaliana", Label: "拟南芥"}}, Organs: []string{"root"},
			Modalities: []string{"scRNA-seq"}, Platforms: []string{"10x Genomics Chromium"},
			Repositories: []DatasetRepository{{Repository: "geo", Accession: "GSE123456", Verified: true}, {Repository: "sra", Accession: "SRP111111"}},
			Status: "verified", Availability: "public", MatrixAvailability: "public", AnnotationAvailability: "unknown", CellsReported: &cells,
			PaperLinks: []DatasetPaperLink{{PaperID: "P-1", Role: "generated"}},
		},
		{
			ID: "DS-2", Title: "Rice ENA run", Species: []SpeciesEntry{{Name: "Oryza sativa"}}, Modalities: []string{"snRNA-seq"},
			Repositories: []DatasetRepository{{Repository: "ena", Accession: "ERR222222"}, {Repository: "bioproject", Accession: "PRJEB33333"}, {Repository: "gsa", Accession: "CRA004444", URL: "https://ngdc.cncb.ac.cn/gsa/browse/CRA004444"}},
			Status: "pending", Availability: "unknown", PaperLinks: []DatasetPaperLink{{PaperID: "P-1", Role: "reused"}, {PaperID: "P-2", Role: "unknown"}},
		},
		{ID: "DS-3", Title: "No accession yet", Status: "identified", Availability: "restricted"},
	}
	return papers, datasets
}

func TestBibTeXEscapesAndKeys(t *testing.T) {
	papers, _ := exportFixture()
	out := BibTeX(papers)
	for _, want := range []string{
		"@article{zhang2024singlecell,",
		`title = {{Single-cell atlas of Arabidopsis \& rice roots: 100\% coverage}},`,
		"author = {Zhang, Wei and O'Brien, Ann and Li Si},",
		`doi = {10.1000/abc\_1},`,
		`url = {https://doi.org/10.1000/abc\_1},`,
		"pmid = {12345},",
		"keywords = {Arabidopsis thaliana, Oryza sativa, root, leaf, scRNA-seq, snRNA-seq, 10x Genomics Chromium},",
		"abstract = {Line one line two},",
		"@article{zhang2024quoted,",
		"note = {Preprint},",
		"@article{anonpending,",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("BibTeX missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "journal = {},") || strings.Contains(out, "doi = {},") {
		t.Errorf("empty fields must be omitted:\n%s", out)
	}
	// Two papers with the same key get letter suffixes.
	dup := []Paper{papers[0], papers[0], papers[0]}
	keys := BibTeX(dup)
	for _, want := range []string{"{zhang2024singlecell,", "{zhang2024singlecella,", "{zhang2024singlecellb,"} {
		if !strings.Contains(keys, want) {
			t.Errorf("duplicate keys not suffixed: missing %q in\n%s", want, keys)
		}
	}
}

func TestBibEscapeCoversLaTeXSpecials(t *testing.T) {
	got := bibEscape(`a\b & c % d $ e # f _ g {h} i ~ j ^ k`)
	want := `a\textbackslash{}b \& c \% d \$ e \# f \_ g \{h\} i \textasciitilde{} j \textasciicircum{} k`
	if got != want {
		t.Fatalf("bibEscape = %q, want %q", got, want)
	}
	if got := bibAuthors("Alice Smith, Bob Jones"); got != "Alice Smith and Bob Jones" {
		t.Errorf("comma-separated plain names: %q", got)
	}
	if got := bibAuthors("Smith, Alice"); got != "Smith, Alice" {
		t.Errorf("a single Surname, Given name must not split: %q", got)
	}
}

func parseCSV(t *testing.T, text string) [][]string {
	t.Helper()
	if !strings.HasPrefix(text, "\uFEFF") {
		t.Fatalf("CSV lacks the UTF-8 BOM")
	}
	if !strings.Contains(text, "\r\n") {
		t.Fatalf("CSV must use CRLF line ends")
	}
	rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\uFEFF"))).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v\n%s", err, text)
	}
	return rows
}

func TestPapersCSVJoinsMultiValues(t *testing.T) {
	papers, datasets := exportFixture()
	rows := parseCSV(t, PapersCSV(papers, datasets))
	if len(rows) != 5 {
		t.Fatalf("want header + 4 rows, got %d", len(rows))
	}
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	first := rows[1]
	checks := map[string]string{
		"title": "Single-cell atlas of Arabidopsis & rice roots: 100% coverage", "year": "2024", "doi": "10.1000/abc_1",
		"species": "Arabidopsis thaliana; Oryza sativa", "species_label": "拟南芥; 水稻", "organs": "root; leaf",
		"modalities": "scRNA-seq; snRNA-seq", "platforms": "10x Genomics Chromium", "article_types": "原创研究",
		"screening": "included", "dataset_accessions": "GSE123456; SRP111111; ERR222222; PRJEB33333; CRA004444", "dataset_ids": "DS-1; DS-2",
	}
	for name, want := range checks {
		if got := first[col[name]]; got != want {
			t.Errorf("%s = %q, want %q", name, got, want)
		}
	}
	second := rows[2]
	if got := second[col["title"]]; got != `A "quoted" title, with comma` {
		t.Errorf("quotes and commas not round-tripped: %q", got)
	}
	if got := second[col["screening_reason"]]; got != "'=review only" {
		t.Errorf("formula-looking cell not neutralised: %q", got)
	}
	if got := rows[3][col["year"]]; got != "" {
		t.Errorf("missing year must be empty, got %q", got)
	}
}

func TestDatasetsCSVAndSummary(t *testing.T) {
	papers, datasets := exportFixture()
	rows := parseCSV(t, DatasetsCSV(datasets, papers))
	col := map[string]int{}
	for i, name := range rows[0] {
		col[name] = i
	}
	first, second, third := rows[1], rows[2], rows[3]
	if got := first[col["accessions"]]; got != "GSE123456; SRP111111" {
		t.Errorf("accessions = %q", got)
	}
	if got := first[col["accession_kinds"]]; got != "geo/series; sra/study" {
		t.Errorf("accession_kinds = %q", got)
	}
	if got := first[col["verified_accessions"]]; got != "GSE123456" {
		t.Errorf("verified_accessions = %q", got)
	}
	if got := first[col["paper_dois"]]; got != "10.1000/abc_1" {
		t.Errorf("paper_dois = %q", got)
	}
	if got := first[col["cells_reported"]]; got != "12000" {
		t.Errorf("cells_reported = %q", got)
	}
	if got := second[col["repositories"]]; got != "ena; gsa" {
		t.Errorf("repositories (sorted, unique, prefix wins over recorded bioproject) = %q", got)
	}
	if got := second[col["paper_roles"]]; got != "reused; unknown" {
		t.Errorf("paper_roles = %q", got)
	}
	if got := third[col["accessions"]]; got != "" {
		t.Errorf("dataset without accession must have an empty cell, got %q", got)
	}

	summary := parseCSV(t, DatasetSummaryCSV(datasets, "species"))
	want := [][]string{
		{"species", "datasets", "verified", "public", "linked_papers", "accessions"},
		{"(unspecified)", "1", "0", "0", "0", "0"},
		{"Arabidopsis thaliana", "1", "1", "1", "1", "2"},
		{"Oryza sativa", "1", "0", "0", "2", "3"},
	}
	if len(summary) != len(want) {
		t.Fatalf("summary rows = %v", summary)
	}
	for i := range want {
		if strings.Join(summary[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("summary row %d = %v, want %v", i, summary[i], want[i])
		}
	}
	byModality := parseCSV(t, DatasetSummaryCSV(datasets, "modality"))
	if len(byModality) != 4 || byModality[1][0] != "(unspecified)" {
		t.Errorf("modality summary = %v", byModality)
	}
}

func TestClassifyAccession(t *testing.T) {
	cases := []struct {
		repo, acc  string
		repository string
		kind       string
	}{
		{"geo", "GSE123456", "geo", "series"},
		{"", "gse1", "geo", "series"},
		{"geo", "GSM999", "geo", "sample"},
		{"sra", "SRP111", "sra", "study"},
		{"sra", "SRX111", "sra", "experiment"},
		{"sra", "SRR111", "sra", "run"},
		{"sra", "SRS111", "sra", "sample"},
		{"ena", "ERR111", "ena", "run"},
		{"ena", "ERP111", "ena", "study"},
		{"ena", "PRJEB111", "ena", "project"},
		{"bioproject", "PRJNA111", "bioproject", "project"},
		{"ddbj", "DRR111", "ddbj", "run"},
		{"", "PRJDB111", "ddbj", "project"},
		{"biosample", "SAMN0001", "biosample", "sample"},
		{"", "SAMEA123", "biosample", "sample"},
		{"gsa", "CRA001", "gsa", "study"},
		{"gsa", "PRJCA001", "gsa", "project"},
		{"gsa", "HRA001", "gsa", "human-study"},
		{"other", "E-MTAB-1234", "arrayexpress", "experiment"},
		{"biostudies", "S-BSST123", "biostudies", "study"},
		{"stomics", "STT0000001", "stomics", "dataset"},
		{"zenodo", "10.5281/zenodo.1234", "zenodo", "record"},
		{"zenodo", "1234", "zenodo", "record"},
		{"figshare", "10.6084/m9.figshare.123.v1", "figshare", "record"},
		{"dryad", "10.5061/dryad.abc", "dryad", "record"},
		{"", "mystery-1", "other", "record"},
		{"Custom DB", "X1", "custom db", "record"},
		// The prefix wins over a mislabeled repository.
		{"geo", "SRR42", "sra", "run"},
	}
	for _, c := range cases {
		got := ClassifyAccession(c.repo, c.acc)
		if got.Repository != c.repository || got.Kind != c.kind {
			t.Errorf("ClassifyAccession(%q, %q) = %+v, want %s/%s", c.repo, c.acc, got, c.repository, c.kind)
		}
	}
}

func TestAccessionListAndDownloadScript(t *testing.T) {
	papers, datasets := exportFixture()
	datasets = append(datasets, datasets[0]) // repeated accessions appear once
	list := AccessionList(datasets)
	if list != "GSE123456\nSRP111111\nERR222222\nPRJEB33333\nCRA004444\n" {
		t.Errorf("AccessionList = %q", list)
	}

	script := DownloadScript(datasets, papers, time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC))
	for _, want := range []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"geo_series GSE123456 'https://ftp.ncbi.nlm.nih.gov/geo/series/GSE123nnn/GSE123456/'",
		"sra SRP111111",
		"ena ERR222222",
		"ena PRJEB33333",
		"manual 'CRA004444' 'https://ngdc.cncb.ac.cn/gsa/browse/CRA004444'",
		"# Root atlas data (doi: 10.1000/abc_1)",
		"# ---- GEO (series) ----",
		"# ---- Manual (gsa) ----",
	} {
		if !strings.Contains(script, want) {
			t.Errorf("script missing %q:\n%s", want, script)
		}
	}
	if strings.Count(script, "geo_series GSE123456") != 1 {
		t.Errorf("duplicate dataset must download once:\n%s", script)
	}
	if got := geoSeriesURL("GSE7"); got != "https://ftp.ncbi.nlm.nih.gov/geo/series/GSEnnn/GSE7/" {
		t.Errorf("short series URL = %q", got)
	}
	if got := shellQuote("it's"); got != `'it'\''s'` {
		t.Errorf("shellQuote = %q", got)
	}
	empty := DownloadScript(nil, nil, time.Now())
	if !strings.Contains(empty, "No accessions recorded") {
		t.Errorf("empty script should say so:\n%s", empty)
	}
}

func TestReport(t *testing.T) {
	papers, datasets := exportFixture()
	out := Report(papers, datasets, "", time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC))
	for _, want := range []string{
		"# BiblioCell screening progress",
		"Generated 2026-11-02 09:30 · 4 papers · 3 datasets",
		"| Included | 1 |", "| Related (kept for reference) | 1 |", "| Uncertain | 1 |", "| Pending | 1 |",
		"| **Needs a decision** (pending + uncertain) | **2** |",
		"| Open agent proposals | 1 |", "| Added by source updates, unchecked | 1 |",
		"| Included with stored full text | 1 / 1 |",
		"| Verified | 1 |\n| Identified | 1 |\n| Pending | 1 |",
		"| public | 1 |", "| restricted | 1 |", "| unknown | 1 |",
		"## By species", "| Arabidopsis thaliana | 1 | 1 | 1 | 1 |", "| Oryza sativa | 1 | 0 | 0 | 1 |",
		"## By modality", "| scRNA-seq | 1 | 1 | 1 | 1 |",
		"## By organ", "| root | 1 | 1 | 1 | 1 |", "| leaf | 0 | 0 | 0 | 1 |",
		"## Included papers", "- Single-cell atlas of Arabidopsis & rice roots: 100% coverage (2024) — https://doi.org/10.1000/abc_1 · GSE123456, SRP111111, ERR222222, PRJEB33333, CRA004444",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report missing %q:\n%s", want, out)
		}
	}
	titled := Report(nil, nil, "  My review  ", time.Now())
	if !strings.HasPrefix(titled, "# My review\n") {
		t.Errorf("custom title not used: %q", titled[:40])
	}
}

func TestExportDispatch(t *testing.T) {
	papers, datasets := exportFixture()
	now := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	names := map[string]string{
		ExportBibTeX: "bibliocell-papers-2026-11-02.bib", ExportPapersCSV: "bibliocell-papers-2026-11-02.csv",
		ExportDatasetsCSV: "bibliocell-datasets-2026-11-02.csv", ExportDatasetSummary: "bibliocell-datasets-by-species-2026-11-02.csv",
		ExportDownloadScript: "bibliocell-download-2026-11-02.sh", ExportAccessions: "bibliocell-accessions-2026-11-02.txt",
		ExportReport: "bibliocell-report-2026-11-02.md",
	}
	for _, kind := range ExportKinds {
		file, err := Export(ExportRequest{Kind: kind, Papers: papers, Datasets: datasets}, now)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if file.Name != names[kind] || file.Content == "" || file.Count == 0 {
			t.Errorf("%s: %+v", kind, ExportFile{Name: file.Name, Count: file.Count})
		}
	}
	file, err := Export(ExportRequest{Kind: ExportDatasetSummary, Datasets: datasets, GroupBy: "organs"}, now)
	if err != nil || file.Name != "bibliocell-datasets-by-organ-2026-11-02.csv" {
		t.Errorf("organ summary: %v %q", err, file.Name)
	}
	if _, err := Export(ExportRequest{Kind: ExportDatasetSummary, GroupBy: "year"}, now); err == nil {
		t.Error("unknown group must fail")
	}
	if _, err := Export(ExportRequest{Kind: "pdf"}, now); err == nil {
		t.Error("unknown kind must fail")
	}
}
