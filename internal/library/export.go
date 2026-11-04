package library

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Export formats. Each one turns the papers and datasets a person selected
// into one text file; nothing here reads the database, so the desktop can
// export the Demo library through the same code as the real one.
const (
	ExportBibTeX         = "bibtex"           // papers as BibTeX entries
	ExportPapersCSV      = "papers-csv"       // one row per paper
	ExportDatasetsCSV    = "datasets-csv"     // one row per dataset
	ExportDatasetSummary = "datasets-summary" // dataset counts per species, organ or modality
	ExportDownloadScript = "download-script"  // shell script fetching each dataset by accession
	ExportAccessions     = "accessions"       // one accession per line
	ExportReport         = "report"           // Markdown screening progress report
)

// ExportKinds lists every format in menu order.
var ExportKinds = []string{ExportBibTeX, ExportPapersCSV, ExportReport, ExportDatasetsCSV, ExportDatasetSummary, ExportDownloadScript, ExportAccessions}

// ExportRequest selects a format and carries the records to write. Papers
// are needed for paper formats and the report; datasets for dataset formats,
// the report, and the accession column of the paper CSV. Paper formats use
// Datasets only to look up links, and dataset formats use Papers the same way.
type ExportRequest struct {
	Kind     string    `json:"kind"`
	Papers   []Paper   `json:"papers"`
	Datasets []Dataset `json:"datasets"`
	// GroupBy picks the summary dimension: species, organ, or modality.
	GroupBy string `json:"groupBy"`
	// Title heads the report; the library name is used when empty.
	Title string `json:"title"`
}

// ExportFile is a finished export: a suggested file name, the text, and how
// many records (papers or datasets) it describes.
type ExportFile struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Count   int    `json:"count"`
}

// Export renders one file. now dates the file name and the report.
func Export(req ExportRequest, now time.Time) (ExportFile, error) {
	day := now.Format("2006-01-02")
	switch req.Kind {
	case ExportBibTeX:
		return ExportFile{Name: "bibliocell-papers-" + day + ".bib", Content: BibTeX(req.Papers), Count: len(req.Papers)}, nil
	case ExportPapersCSV:
		return ExportFile{Name: "bibliocell-papers-" + day + ".csv", Content: PapersCSV(req.Papers, req.Datasets), Count: len(req.Papers)}, nil
	case ExportDatasetsCSV:
		return ExportFile{Name: "bibliocell-datasets-" + day + ".csv", Content: DatasetsCSV(req.Datasets, req.Papers), Count: len(req.Datasets)}, nil
	case ExportDatasetSummary:
		by, err := summaryDimension(req.GroupBy)
		if err != nil {
			return ExportFile{}, err
		}
		return ExportFile{Name: "bibliocell-datasets-by-" + by + "-" + day + ".csv", Content: DatasetSummaryCSV(req.Datasets, by), Count: len(req.Datasets)}, nil
	case ExportDownloadScript:
		return ExportFile{Name: "bibliocell-download-" + day + ".sh", Content: DownloadScript(req.Datasets, req.Papers, now), Count: len(req.Datasets)}, nil
	case ExportAccessions:
		return ExportFile{Name: "bibliocell-accessions-" + day + ".txt", Content: AccessionList(req.Datasets), Count: len(req.Datasets)}, nil
	case ExportReport:
		return ExportFile{Name: "bibliocell-report-" + day + ".md", Content: Report(req.Papers, req.Datasets, req.Title, now), Count: len(req.Papers)}, nil
	}
	return ExportFile{}, fmt.Errorf("unknown export kind %q; one of %s", req.Kind, strings.Join(ExportKinds, ", "))
}

func summaryDimension(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "species":
		return "species", nil
	case "organ", "organs", "tissue":
		return "organ", nil
	case "modality", "modalities":
		return "modality", nil
	}
	return "", fmt.Errorf("unknown group %q; one of species, organ, modality", value)
}

// ---- shared helpers ----

const listJoin = "; "

func speciesNames(entries []SpeciesEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if name := strings.TrimSpace(entry.Name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

func speciesLabels(entries []SpeciesEntry) []string {
	labels := make([]string, 0, len(entries))
	for _, entry := range entries {
		label := strings.TrimSpace(entry.Label)
		if label == "" {
			label = strings.TrimSpace(entry.Name)
		}
		if label != "" {
			labels = append(labels, label)
		}
	}
	return labels
}

func yearText(year *int) string {
	if year == nil || *year == 0 {
		return ""
	}
	return strconv.Itoa(*year)
}

// datasetsOf lists the datasets linked to a paper, in dataset order.
func datasetsOf(datasets []Dataset, paperID string) []Dataset {
	var linked []Dataset
	for _, d := range datasets {
		for _, link := range d.PaperLinks {
			if link.PaperID == paperID {
				linked = append(linked, d)
				break
			}
		}
	}
	return linked
}

// accessionsOf lists a dataset's accessions in record order, without repeats.
func accessionsOf(d Dataset) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range d.Repositories {
		acc := strings.TrimSpace(r.Accession)
		if acc == "" || seen[acc] {
			continue
		}
		seen[acc] = true
		out = append(out, acc)
	}
	return out
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// ---- BibTeX ----

var bibKeyStrip = regexp.MustCompile(`[^A-Za-z0-9]+`)

// BibTeX renders the papers as @article entries. Titles are brace-protected
// so BibTeX styles keep their capitalization; authors stored as "A; B" become
// "A and B". Keys are author-year-word and get a letter suffix on collision.
func BibTeX(papers []Paper) string {
	var b strings.Builder
	used := map[string]int{}
	for _, p := range papers {
		key := bibKey(p)
		if n := used[key]; n > 0 {
			key += string(rune('a' + n - 1))
		}
		used[bibKey(p)]++
		entry := "@article"
		fields := [][2]string{
			{"title", "{" + bibEscape(p.Title) + "}"},
			{"author", bibEscape(bibAuthors(p.Authors))},
			{"journal", bibEscape(p.Journal)},
			{"year", yearText(p.Year)},
			{"doi", bibEscape(p.DOI)},
		}
		if p.DOI != "" {
			fields = append(fields, [2]string{"url", "https://doi.org/" + bibEscape(p.DOI)})
		}
		if p.PMID != "" {
			fields = append(fields, [2]string{"pmid", bibEscape(p.PMID)})
		}
		if p.PublicationStatus == "preprint" {
			fields = append(fields, [2]string{"note", "Preprint"})
		}
		var keywords []string
		keywords = append(keywords, speciesNames(p.Species)...)
		keywords = append(keywords, p.Organs...)
		keywords = append(keywords, p.Modalities...)
		keywords = append(keywords, p.Platforms...)
		if len(keywords) > 0 {
			fields = append(fields, [2]string{"keywords", bibEscape(strings.Join(cleanStrings(keywords), ", "))})
		}
		if p.Abstract != "" {
			fields = append(fields, [2]string{"abstract", bibEscape(strings.Join(strings.Fields(p.Abstract), " "))})
		}
		fmt.Fprintf(&b, "%s{%s,\n", entry, key)
		for _, f := range fields {
			if f[1] == "" {
				continue
			}
			fmt.Fprintf(&b, "  %s = {%s},\n", f[0], f[1])
		}
		b.WriteString("}\n\n")
	}
	return b.String()
}

// bibKey builds "surname2024word" from the first author, the year, and the
// first title word longer than three letters, ASCII only.
func bibKey(p Paper) string {
	first := strings.TrimSpace(strings.Split(p.Authors, ";")[0])
	surname := first
	if i := strings.Index(first, ","); i >= 0 {
		surname = first[:i]
	} else if parts := strings.Fields(first); len(parts) > 0 {
		surname = parts[len(parts)-1]
	}
	surname = asciiKey(surname)
	if surname == "" {
		surname = "anon"
	}
	word := ""
	for _, w := range strings.Fields(p.Title) {
		w = asciiKey(w)
		if len(w) > 3 {
			word = w
			break
		}
	}
	key := strings.ToLower(surname + yearText(p.Year) + word)
	if key == "" {
		key = "paper"
	}
	return key
}

func asciiKey(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
		}
	}
	return bibKeyStrip.ReplaceAllString(b.String(), "")
}

// bibAuthors turns "A; B" into "A and B". A comma-separated list is split
// only when every piece holds several words, so "Smith, Alice" stays one name
// while "Alice Smith, Bob Jones" becomes two.
func bibAuthors(authors string) string {
	sep := ";"
	if !strings.Contains(authors, ";") && !strings.Contains(authors, " and ") && strings.Contains(authors, ",") {
		sep = ","
		for _, piece := range strings.Split(authors, ",") {
			if len(strings.Fields(piece)) < 2 {
				sep = ";"
				break
			}
		}
	}
	var names []string
	for _, name := range strings.Split(authors, sep) {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return strings.Join(names, " and ")
}

var bibEscaper = strings.NewReplacer(
	`\`, `\textbackslash{}`,
	`&`, `\&`, `%`, `\%`, `$`, `\$`, `#`, `\#`, `_`, `\_`,
	`{`, `\{`, `}`, `\}`,
	`~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`,
)

// bibEscape protects the characters BibTeX and LaTeX treat specially.
func bibEscape(value string) string {
	return bibEscaper.Replace(strings.TrimSpace(value))
}

// ---- CSV ----

// csvTable writes rows as RFC 4180 CSV with CRLF line ends and a UTF-8 BOM so
// spreadsheets open it correctly. Cells starting with a formula character are
// prefixed with an apostrophe so no spreadsheet evaluates them.
func csvTable(header []string, rows [][]string) string {
	var buf bytes.Buffer
	buf.WriteString("\uFEFF")
	w := csv.NewWriter(&buf)
	w.UseCRLF = true
	_ = w.Write(header)
	for _, row := range rows {
		for i, cell := range row {
			if cell != "" && strings.ContainsRune("=+-@\t\r", rune(cell[0])) {
				row[i] = "'" + cell
			}
		}
		_ = w.Write(row)
	}
	w.Flush()
	return buf.String()
}

// PapersCSV writes one row per paper with multi-valued fields joined by "; ".
func PapersCSV(papers []Paper, datasets []Dataset) string {
	header := []string{
		"id", "title", "authors", "journal", "year", "doi", "pmid", "publication_status",
		"species", "species_label", "organs", "modalities", "platforms", "article_types",
		"screening", "screening_reason", "fulltext", "reading", "tags",
		"dataset_accessions", "dataset_ids", "source", "added_at",
	}
	rows := make([][]string, 0, len(papers))
	for _, p := range papers {
		var accessions, ids []string
		for _, d := range datasetsOf(datasets, p.ID) {
			accessions = append(accessions, accessionsOf(d)...)
			ids = append(ids, d.ID)
		}
		rows = append(rows, []string{
			p.ID, p.Title, p.Authors, p.Journal, yearText(p.Year), p.DOI, p.PMID, p.PublicationStatus,
			strings.Join(speciesNames(p.Species), listJoin), strings.Join(speciesLabels(p.Species), listJoin),
			strings.Join(p.Organs, listJoin), strings.Join(p.Modalities, listJoin), strings.Join(p.Platforms, listJoin), strings.Join(p.ArticleTypes, listJoin),
			p.Screening, p.ScreeningReason, p.Fulltext, p.Reading, strings.Join(p.Tags, listJoin),
			strings.Join(accessions, listJoin), strings.Join(ids, listJoin), p.Source, p.AddedAt,
		})
	}
	return csvTable(header, rows)
}

// DatasetsCSV writes one row per dataset: every accession with its repository
// and kind, the linked papers' DOIs, curation status, and availability.
func DatasetsCSV(datasets []Dataset, papers []Paper) string {
	header := []string{
		"id", "title", "species", "species_label", "organs", "modalities", "platforms",
		"accessions", "repositories", "accession_kinds", "verified_accessions",
		"status", "curation_level", "availability", "matrix_availability", "annotation_availability",
		"cells_reported", "paper_dois", "paper_ids", "paper_roles", "awaiting_review", "added_at",
	}
	byID := make(map[string]Paper, len(papers))
	for _, p := range papers {
		byID[p.ID] = p
	}
	rows := make([][]string, 0, len(datasets))
	for _, d := range datasets {
		var accessions, repos, kinds, verified []string
		for _, r := range d.Repositories {
			acc := strings.TrimSpace(r.Accession)
			if acc == "" {
				continue
			}
			class := ClassifyAccession(r.Repository, acc)
			accessions = append(accessions, acc)
			repos = append(repos, class.Repository)
			kinds = append(kinds, class.Repository+"/"+class.Kind)
			if r.Verified {
				verified = append(verified, acc)
			}
		}
		var dois, ids, roles []string
		for _, link := range d.PaperLinks {
			ids = append(ids, link.PaperID)
			roles = append(roles, link.Role)
			if p, ok := byID[link.PaperID]; ok && p.DOI != "" {
				dois = append(dois, p.DOI)
			}
		}
		cells := ""
		if d.CellsReported != nil {
			cells = strconv.Itoa(*d.CellsReported)
		}
		rows = append(rows, []string{
			d.ID, d.Title, strings.Join(speciesNames(d.Species), listJoin), strings.Join(speciesLabels(d.Species), listJoin),
			strings.Join(d.Organs, listJoin), strings.Join(d.Modalities, listJoin), strings.Join(d.Platforms, listJoin),
			strings.Join(accessions, listJoin), strings.Join(uniqueSorted(repos), listJoin), strings.Join(kinds, listJoin), strings.Join(verified, listJoin),
			d.Status, d.CurationLevel, d.Availability, d.MatrixAvailability, d.AnnotationAvailability,
			cells, strings.Join(dois, listJoin), strings.Join(ids, listJoin), strings.Join(roles, listJoin), strconv.FormatBool(d.AwaitingReview), d.AddedAt,
		})
	}
	return csvTable(header, rows)
}

// summaryGroup is one line of a dataset summary.
type summaryGroup struct {
	Key        string
	Datasets   int
	Verified   int
	Public     int
	Papers     map[string]bool
	Accessions int
}

const unspecified = "(unspecified)"

// summarize counts datasets per value of one dimension. A dataset with
// several values counts under each; one with none counts as unspecified.
func summarize(datasets []Dataset, by string) []summaryGroup {
	groups := map[string]*summaryGroup{}
	for _, d := range datasets {
		var keys []string
		switch by {
		case "organ":
			keys = d.Organs
		case "modality":
			keys = d.Modalities
		default:
			keys = speciesNames(d.Species)
		}
		keys = cleanStrings(keys)
		if len(keys) == 0 {
			keys = []string{unspecified}
		}
		for _, key := range keys {
			g := groups[key]
			if g == nil {
				g = &summaryGroup{Key: key, Papers: map[string]bool{}}
				groups[key] = g
			}
			g.Datasets++
			if d.Status == "verified" {
				g.Verified++
			}
			if d.Availability == "public" {
				g.Public++
			}
			for _, link := range d.PaperLinks {
				g.Papers[link.PaperID] = true
			}
			g.Accessions += len(accessionsOf(d))
		}
	}
	out := make([]summaryGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Datasets != out[j].Datasets {
			return out[i].Datasets > out[j].Datasets
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// DatasetSummaryCSV counts datasets per species, organ, or modality.
func DatasetSummaryCSV(datasets []Dataset, by string) string {
	header := []string{by, "datasets", "verified", "public", "linked_papers", "accessions"}
	var rows [][]string
	for _, g := range summarize(datasets, by) {
		rows = append(rows, []string{g.Key, strconv.Itoa(g.Datasets), strconv.Itoa(g.Verified), strconv.Itoa(g.Public), strconv.Itoa(len(g.Papers)), strconv.Itoa(g.Accessions)})
	}
	return csvTable(header, rows)
}

// ---- accessions ----

// AccessionClass says which repository an accession belongs to and what it
// names there, from the accession's own prefix first and the recorded
// repository name as a fallback.
type AccessionClass struct {
	Repository string `json:"repository"` // geo, sra, ena, ddbj, bioproject, biosample, gsa, arrayexpress, biostudies, stomics, zenodo, figshare, dryad, other
	Kind       string `json:"kind"`       // series, sample, platform, study, experiment, run, project, record, …
}

var accessionPrefixes = []struct {
	pattern *regexp.Regexp
	class   AccessionClass
}{
	{regexp.MustCompile(`^GSE\d+$`), AccessionClass{"geo", "series"}},
	{regexp.MustCompile(`^GSM\d+$`), AccessionClass{"geo", "sample"}},
	{regexp.MustCompile(`^GDS\d+$`), AccessionClass{"geo", "dataset"}},
	{regexp.MustCompile(`^GPL\d+$`), AccessionClass{"geo", "platform"}},
	{regexp.MustCompile(`^SRP\d+$`), AccessionClass{"sra", "study"}},
	{regexp.MustCompile(`^SRX\d+$`), AccessionClass{"sra", "experiment"}},
	{regexp.MustCompile(`^SRR\d+$`), AccessionClass{"sra", "run"}},
	{regexp.MustCompile(`^SRS\d+$`), AccessionClass{"sra", "sample"}},
	{regexp.MustCompile(`^SRA\d+$`), AccessionClass{"sra", "submission"}},
	{regexp.MustCompile(`^ERP\d+$`), AccessionClass{"ena", "study"}},
	{regexp.MustCompile(`^ERX\d+$`), AccessionClass{"ena", "experiment"}},
	{regexp.MustCompile(`^ERR\d+$`), AccessionClass{"ena", "run"}},
	{regexp.MustCompile(`^ERS\d+$`), AccessionClass{"ena", "sample"}},
	{regexp.MustCompile(`^DRP\d+$`), AccessionClass{"ddbj", "study"}},
	{regexp.MustCompile(`^DRX\d+$`), AccessionClass{"ddbj", "experiment"}},
	{regexp.MustCompile(`^DRR\d+$`), AccessionClass{"ddbj", "run"}},
	{regexp.MustCompile(`^DRS\d+$`), AccessionClass{"ddbj", "sample"}},
	{regexp.MustCompile(`^PRJNA\d+$`), AccessionClass{"bioproject", "project"}},
	{regexp.MustCompile(`^PRJEB\d+$`), AccessionClass{"ena", "project"}},
	{regexp.MustCompile(`^PRJDB\d+$`), AccessionClass{"ddbj", "project"}},
	{regexp.MustCompile(`^SAM[NED][A-Z]?\d+$`), AccessionClass{"biosample", "sample"}},
	{regexp.MustCompile(`^PRJCA\d+$`), AccessionClass{"gsa", "project"}},
	{regexp.MustCompile(`^CRA\d+$`), AccessionClass{"gsa", "study"}},
	{regexp.MustCompile(`^CRX\d+$`), AccessionClass{"gsa", "experiment"}},
	{regexp.MustCompile(`^CRR\d+$`), AccessionClass{"gsa", "run"}},
	{regexp.MustCompile(`^HRA\d+$`), AccessionClass{"gsa", "human-study"}},
	{regexp.MustCompile(`^OEP\d+$`), AccessionClass{"nodeomics", "project"}},
	{regexp.MustCompile(`^E-[A-Z]{4}-\d+$`), AccessionClass{"arrayexpress", "experiment"}},
	{regexp.MustCompile(`^S-[A-Z]{4}\d+$`), AccessionClass{"biostudies", "study"}},
	{regexp.MustCompile(`^STT\d+$`), AccessionClass{"stomics", "dataset"}},
	{regexp.MustCompile(`^10\.5281/ZENODO\.\d+$`), AccessionClass{"zenodo", "record"}},
	{regexp.MustCompile(`^10\.6084/M9\.FIGSHARE\.\d+`), AccessionClass{"figshare", "record"}},
	{regexp.MustCompile(`^10\.5061/DRYAD\.`), AccessionClass{"dryad", "record"}},
}

// ClassifyAccession recognises the accession's prefix (GSE, SRR, PRJEB, …)
// and falls back to the recorded repository with kind "record". An accession
// whose prefix contradicts the recorded repository is classified by its prefix,
// since that is what the download tools go by.
func ClassifyAccession(repository, accession string) AccessionClass {
	acc := strings.ToUpper(strings.TrimSpace(accession))
	for _, entry := range accessionPrefixes {
		if entry.pattern.MatchString(acc) {
			return entry.class
		}
	}
	repo := strings.ToLower(strings.TrimSpace(repository))
	if repo == "" {
		repo = "other"
	}
	return AccessionClass{repo, "record"}
}

// AccessionList writes every accession once, one per line, in dataset order,
// ready for xargs or a download tool's batch mode.
func AccessionList(datasets []Dataset) string {
	seen := map[string]bool{}
	var b strings.Builder
	for _, d := range datasets {
		for _, acc := range accessionsOf(d) {
			if seen[acc] {
				continue
			}
			seen[acc] = true
			b.WriteString(acc)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// geoSeriesURL is the FTP mirror folder holding a series' supplementary files:
// GSE123456 lives under series/GSE123nnn/GSE123456/.
func geoSeriesURL(accession string) string {
	acc := strings.ToUpper(accession)
	stem := "GSEnnn"
	if digits := acc[3:]; len(digits) > 3 {
		stem = "GSE" + digits[:len(digits)-3] + "nnn"
	}
	return "https://ftp.ncbi.nlm.nih.gov/geo/series/" + stem + "/" + acc + "/"
}

// shellQuote wraps a value in single quotes for a POSIX shell.
func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// shellComment keeps a comment on one line.
func shellComment(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// DownloadScript writes a bash script that fetches every dataset's data by
// accession with the usual public tools: wget for GEO supplementary files,
// prefetch and fasterq-dump for SRA, the ENA portal for ENA/DDBJ runs, and
// entrez-direct to expand BioProjects. Repositories without a batch tool are
// listed as manual steps with their page. Nothing runs when the file is
// written; a person reviews it and runs it where the data should land.
func DownloadScript(datasets []Dataset, papers []Paper, now time.Time) string {
	byID := make(map[string]Paper, len(papers))
	for _, p := range papers {
		byID[p.ID] = p
	}
	var b strings.Builder
	b.WriteString("#!/usr/bin/env bash\n")
	fmt.Fprintf(&b, "# BiblioCell download list, %s. %d datasets.\n", now.Format("2006-01-02"), len(datasets))
	b.WriteString(`# Review before running. Needs: wget or curl; sra-tools (prefetch, fasterq-dump)
# for SRA accessions; entrez-direct (esearch, efetch) for BioProjects.
# Usage: OUT=/data/scrna bash ` + "bibliocell-download.sh" + `   (OUT defaults to ./bibliocell-data)
set -euo pipefail
OUT="${OUT:-./bibliocell-data}"
mkdir -p "$OUT"
THREADS="${THREADS:-4}"

geo_series() { # GEO series: supplementary files plus the series SOFT family file
  local acc=$1 url=$2
  mkdir -p "$OUT/geo/$acc"
  wget -q --show-progress -r -np -nd -N -P "$OUT/geo/$acc" "${url}suppl/" || echo "no supplementary files for $acc" >&2
  wget -q --show-progress -N -P "$OUT/geo/$acc" "${url}soft/${acc}_family.soft.gz" || true
}
geo_sample() { # GEO sample: its own supplementary files
  local acc=$1 stem=${1:0:${#1}-3}nnn
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
    [ -e "$OUT/sra/fastq/${run}_1.fastq" ] || [ -e "$OUT/sra/fastq/${run}.fastq" ] \
      || fasterq-dump --split-files --threads "$THREADS" --outdir "$OUT/sra/fastq" "$OUT/sra/$run"
  done
}
ena() { # ENA / DDBJ accession: fastq links from the ENA portal file report
  local acc=$1
  mkdir -p "$OUT/ena/$acc"
  curl -s "https://www.ebi.ac.uk/ena/portal/api/filereport?accession=$acc&result=read_run&fields=run_accession,fastq_ftp&format=tsv" \
    | tail -n +2 | cut -f2 | tr ';' '\n' | grep . | sed 's#^#https://#' \
    | wget -q --show-progress -N -P "$OUT/ena/$acc" -i -
}
bioproject() { # NCBI BioProject: list its SRA runs, then prefetch each
  local acc=$1
  mkdir -p "$OUT/sra"
  esearch -db sra -query "$acc" | efetch -format runinfo | cut -d, -f1 | grep -E '^[SED]RR' | sort -u \
    | while read -r run; do sra "$run"; done
}
manual() { echo "manual: $1  $2" >&2; }

`)
	type item struct {
		class   AccessionClass
		acc     string
		dataset Dataset
	}
	sections := map[string][]item{}
	var order []string
	seen := map[string]bool{}
	for _, d := range datasets {
		for _, r := range d.Repositories {
			acc := strings.TrimSpace(r.Accession)
			if acc == "" || seen[acc] {
				continue
			}
			seen[acc] = true
			class := ClassifyAccession(r.Repository, acc)
			section := scriptSection(class)
			if _, ok := sections[section]; !ok {
				order = append(order, section)
			}
			sections[section] = append(sections[section], item{class, acc, d})
		}
	}
	sort.Strings(order)
	for _, section := range order {
		fmt.Fprintf(&b, "# ---- %s ----\n", section)
		for _, it := range sections[section] {
			var dois []string
			for _, link := range it.dataset.PaperLinks {
				if p, ok := byID[link.PaperID]; ok && p.DOI != "" {
					dois = append(dois, p.DOI)
				}
			}
			note := it.dataset.Title
			if len(dois) > 0 {
				note += " (doi: " + strings.Join(dois, ", ") + ")"
			}
			fmt.Fprintf(&b, "# %s\n", shellComment(note))
			upper := strings.ToUpper(it.acc)
			switch {
			case it.class.Repository == "geo" && it.class.Kind == "series":
				fmt.Fprintf(&b, "geo_series %s %s\n", upper, shellQuote(geoSeriesURL(upper)))
			case it.class.Repository == "geo" && it.class.Kind == "sample":
				fmt.Fprintf(&b, "geo_sample %s\n", upper)
			case it.class.Repository == "sra":
				fmt.Fprintf(&b, "sra %s\n", upper)
			case (it.class.Repository == "ena" || it.class.Repository == "ddbj") && it.class.Kind != "sample":
				fmt.Fprintf(&b, "ena %s\n", upper)
			case it.class.Repository == "bioproject":
				fmt.Fprintf(&b, "bioproject %s\n", upper)
			default:
				url := ""
				for _, r := range it.dataset.Repositories {
					if strings.EqualFold(strings.TrimSpace(r.Accession), it.acc) && strings.TrimSpace(r.URL) != "" {
						url = strings.TrimSpace(r.URL)
						break
					}
				}
				if url == "" {
					url = accessionPage(it.class, it.acc)
				}
				fmt.Fprintf(&b, "manual %s %s\n", shellQuote(it.acc), shellQuote(url))
			}
		}
		b.WriteString("\n")
	}
	if len(order) == 0 {
		b.WriteString("# No accessions recorded for the selected datasets.\n")
	}
	return b.String()
}

func scriptSection(class AccessionClass) string {
	switch class.Repository {
	case "geo":
		return "GEO (" + class.Kind + ")"
	case "sra":
		return "SRA (prefetch + fasterq-dump)"
	case "ena", "ddbj":
		return "ENA / DDBJ (portal file report)"
	case "bioproject":
		return "BioProject (entrez-direct + prefetch)"
	}
	return "Manual (" + class.Repository + ")"
}

// accessionPage is the public page for an accession when no download tool
// handles it, so a person can fetch it by hand.
func accessionPage(class AccessionClass, accession string) string {
	switch class.Repository {
	case "gsa":
		return "https://ngdc.cncb.ac.cn/search/all?q=" + accession
	case "biosample":
		return "https://www.ncbi.nlm.nih.gov/biosample/" + accession
	case "arrayexpress", "biostudies":
		return "https://www.ebi.ac.uk/biostudies/studies/" + accession
	case "stomics":
		return "https://db.cngb.org/stomics/"
	case "zenodo":
		return "https://zenodo.org/records/" + strings.TrimPrefix(strings.ToLower(accession), "10.5281/zenodo.")
	case "figshare", "dryad":
		return "https://doi.org/" + accession
	case "ena", "ddbj":
		return "https://www.ebi.ac.uk/ena/browser/view/" + accession
	}
	return "https://www.google.com/search?q=" + accession
}

// ---- report ----

// Report writes a Markdown screening progress summary: how many papers are
// included, excluded, uncertain, or still waiting, and how the datasets split
// by species, modality, and organ. It is meant to be pasted or sent as is.
func Report(papers []Paper, datasets []Dataset, title string, now time.Time) string {
	if strings.TrimSpace(title) == "" {
		title = "BiblioCell screening progress"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", strings.TrimSpace(title))
	fmt.Fprintf(&b, "Generated %s · %d papers · %d datasets\n\n", now.Format("2006-01-02 15:04"), len(papers), len(datasets))

	count := map[string]int{}
	proposals, awaiting, fulltext := 0, 0, 0
	for _, p := range papers {
		count[p.Screening]++
		if p.ProposedScreening != "" && p.ProposedScreening != p.Screening {
			proposals++
		}
		if p.AwaitingReview {
			awaiting++
		}
		if p.Screening == "included" && p.Fulltext == "stored" {
			fulltext++
		}
	}
	b.WriteString("## Screening\n\n| Status | Papers |\n|---|---:|\n")
	fmt.Fprintf(&b, "| Included | %d |\n", count["included"])
	fmt.Fprintf(&b, "| Excluded | %d |\n", count["excluded"])
	fmt.Fprintf(&b, "| Uncertain | %d |\n", count["uncertain"])
	fmt.Fprintf(&b, "| Pending | %d |\n", count["pending"])
	if count["irrelevant"] > 0 {
		fmt.Fprintf(&b, "| Irrelevant (kept only to skip on import) | %d |\n", count["irrelevant"])
	}
	fmt.Fprintf(&b, "| **Needs a decision** (pending + uncertain) | **%d** |\n", count["pending"]+count["uncertain"])
	fmt.Fprintf(&b, "| Open agent proposals | %d |\n", proposals)
	fmt.Fprintf(&b, "| Added by source updates, unchecked | %d |\n", awaiting)
	if count["included"] > 0 {
		fmt.Fprintf(&b, "| Included with stored full text | %d / %d |\n", fulltext, count["included"])
	}
	b.WriteString("\n")

	status := map[string]int{}
	availability := map[string]int{}
	for _, d := range datasets {
		status[d.Status]++
		availability[d.Availability]++
	}
	b.WriteString("## Datasets\n\n| Curation | Datasets |\n|---|---:|\n")
	fmt.Fprintf(&b, "| Verified | %d |\n| Identified | %d |\n| Pending | %d |\n\n", status["verified"], status["identified"], status["pending"])
	b.WriteString("| Availability | Datasets |\n|---|---:|\n")
	for _, key := range []string{"public", "restricted", "embargoed", "unavailable", "unknown"} {
		if availability[key] > 0 {
			fmt.Fprintf(&b, "| %s | %d |\n", key, availability[key])
		}
	}
	b.WriteString("\n")

	paperCounts := func(pick func(Paper) []string) map[string]int {
		counts := map[string]int{}
		for _, p := range papers {
			if p.Screening != "included" {
				continue
			}
			for _, key := range cleanStrings(pick(p)) {
				counts[key]++
			}
		}
		return counts
	}
	for _, dim := range []struct {
		by, heading string
		pick        func(Paper) []string
	}{
		{"species", "By species", func(p Paper) []string { return speciesNames(p.Species) }},
		{"modality", "By modality", func(p Paper) []string { return p.Modalities }},
		{"organ", "By organ", func(p Paper) []string { return p.Organs }},
	} {
		groups := summarize(datasets, dim.by)
		included := paperCounts(dim.pick)
		// Species with included papers but no dataset yet still belong in the table.
		listed := map[string]bool{}
		for _, g := range groups {
			listed[g.Key] = true
		}
		var extra []string
		for key := range included {
			if !listed[key] {
				extra = append(extra, key)
			}
		}
		sort.Strings(extra)
		for _, key := range extra {
			groups = append(groups, summaryGroup{Key: key, Papers: map[string]bool{}})
		}
		if len(groups) == 0 {
			continue
		}
		fmt.Fprintf(&b, "## %s\n\n| %s | Datasets | Verified | Public | Included papers |\n|---|---:|---:|---:|---:|\n", dim.heading, strings.ToUpper(dim.by[:1])+dim.by[1:])
		for _, g := range groups {
			fmt.Fprintf(&b, "| %s | %d | %d | %d | %d |\n", g.Key, g.Datasets, g.Verified, g.Public, included[g.Key])
		}
		b.WriteString("\n")
	}

	if count["included"] > 0 {
		b.WriteString("## Included papers\n\n")
		for _, p := range papers {
			if p.Screening != "included" {
				continue
			}
			line := "- " + strings.TrimSpace(p.Title)
			if year := yearText(p.Year); year != "" {
				line += " (" + year + ")"
			}
			if p.DOI != "" {
				line += " — https://doi.org/" + p.DOI
			}
			if accs := paperAccessions(datasets, p.ID); len(accs) > 0 {
				line += " · " + strings.Join(accs, ", ")
			}
			b.WriteString(line + "\n")
		}
	}
	return b.String()
}

func paperAccessions(datasets []Dataset, paperID string) []string {
	var out []string
	for _, d := range datasetsOf(datasets, paperID) {
		out = append(out, accessionsOf(d)...)
	}
	return out
}
