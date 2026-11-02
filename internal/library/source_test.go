package library

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeHarvester struct {
	records []harvestRecord
	err     error
}

func (f fakeHarvester) Harvest(context.Context, SourceConfig) ([]harvestRecord, error) {
	return f.records, f.err
}

func TestSourceConfigurationRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	sources, err := store.ListSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 6 {
		t.Fatalf("source count = %d, want 6", len(sources))
	}
	if sources[0].Query == "" || !sources[0].Enabled || sources[5].Supported {
		t.Errorf("unexpected defaults: %+v / %+v", sources[0], sources[5])
	}

	config := SourceConfig{ID: "sequence-repositories", Enabled: true, Query: `description="*single*" AND tax_tree(33090)`, MaxResults: 7, Provider: "ena"}
	status, err := store.SaveSourceConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != "ena" || status.MaxResults != 7 || status.Query != config.Query {
		t.Errorf("saved source = %+v", status)
	}
	reopened, err := Open(store.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	status, err = reopened.SourceStatus(context.Background(), config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != "ena" || status.MaxResults != 7 || status.Query != config.Query {
		t.Errorf("reopened source = %+v", status)
	}

	config.MaxResults = 101
	if _, err := store.SaveSourceConfig(context.Background(), config); err == nil {
		t.Fatal("result limit above 100 succeeded")
	}
}

func TestSyncSourceIsIdempotentAndPreservesCuration(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	year := 2026
	paper := Paper{Title: "A plant single-cell study", DOI: "10.1000/plant.1", PMID: "12345", Year: &year, Source: "PubMed"}
	harvester := fakeHarvester{records: []harvestRecord{{
		ExternalID: "12345", Title: paper.Title, URL: "https://pubmed.ncbi.nlm.nih.gov/12345/",
		Paper: &paper, Payload: map[string]string{"title": paper.Title},
	}}}

	first, err := store.syncSource(ctx, "pubmed", harvester)
	if err != nil {
		t.Fatal(err)
	}
	if first.PapersAdded != 1 || first.Duplicates != 0 || first.RecordsNew != 1 || first.Status.LastRun.Status != "succeeded" {
		t.Errorf("first sync = %+v", first)
	}
	paperID := first.Papers[0].ID
	if _, err := store.UpdatePaper(ctx, paperID, PaperPatch{Notes: stringPointer("keep this note"), Reading: stringPointer("read")}); err != nil {
		t.Fatal(err)
	}

	second, err := store.syncSource(ctx, "pubmed", harvester)
	if err != nil {
		t.Fatal(err)
	}
	if second.PapersAdded != 0 || second.Duplicates != 1 || second.RecordsNew != 0 || second.RecordsUpdated != 0 {
		t.Errorf("second sync = %+v", second)
	}
	preserved, err := store.GetPaper(ctx, paperID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Notes != "keep this note" || preserved.Reading != "read" {
		t.Errorf("sync replaced curation: %+v", preserved)
	}
	if second.Status.RecordCount != 1 || len(second.Status.RecentRecords) != 1 || second.Status.RecentRecords[0].PaperID != paperID {
		t.Errorf("source records = %+v", second.Status)
	}
}

func TestSyncSourceFailureDoesNotCreateRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.syncSource(ctx, "pubmed", fakeHarvester{err: errors.New("remote unavailable")}); err == nil {
		t.Fatal("failed remote sync succeeded")
	}
	status, err := store.SourceStatus(ctx, "pubmed")
	if err != nil {
		t.Fatal(err)
	}
	if status.LastRun == nil || status.LastRun.Status != "failed" || status.RecordCount != 0 || status.LastSuccessAt != "" {
		t.Errorf("failed source state = %+v", status)
	}
	papers, err := store.ListPapers(ctx)
	if err != nil || len(papers) != 0 {
		t.Errorf("papers after failed sync = %d, %v", len(papers), err)
	}
}

func openTestLibrary(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func harvested(externalID string, paper Paper) harvestRecord {
	return harvestRecord{ExternalID: externalID, Title: paper.Title, Paper: &paper, Payload: paper}
}

func syncRecords(t *testing.T, store *Store, sourceID string, records ...harvestRecord) SyncResult {
	t.Helper()
	result, err := store.syncSource(context.Background(), sourceID, fakeHarvester{records: records})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestIdentifiersAndTitlesNormalize(t *testing.T) {
	for _, value := range []string{
		"10.1000/ABC.1", " doi:10.1000/abc.1", "DOI: 10.1000/abc.1", "doi 10.1000/abc.1", "https://doi.org/10.1000/abc.1",
		"http://dx.doi.org/10.1000/ABC.1", "https://www.doi.org/10.1000/abc.1", "doi.org/10.1000/abc.1", "10.1000/abc.1.",
	} {
		if got := normalizeDOI(value); got != "10.1000/abc.1" {
			t.Errorf("normalizeDOI(%q) = %q", value, got)
		}
	}
	for _, value := range []string{"12345", " PMID:12345", "pmid: 12345", "PMID 12345", "https://pubmed.ncbi.nlm.nih.gov/12345/", "https://www.ncbi.nlm.nih.gov/pubmed/12345"} {
		if got := normalizePMID(value); got != "12345" {
			t.Errorf("normalizePMID(%q) = %q", value, got)
		}
	}
	for _, value := range []string{"Single-cell atlas of <i>Arabidopsis</i> roots.", "SINGLE CELL ATLAS OF ARABIDOPSIS ROOTS", "Single‑cell atlas: of Arabidopsis roots"} {
		if got := titleKey(value); got != "singlecellatlasofarabidopsisroots" {
			t.Errorf("titleKey(%q) = %q", value, got)
		}
	}
	if got := titleKey("植物单细胞图谱。"); got != "植物单细胞图谱" {
		t.Errorf("Chinese title key = %q", got)
	}
	if got := titleKey("???"); got != "" {
		t.Errorf("symbol title key = %q", got)
	}
}

func TestSyncDeduplicatesAcrossSources(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	year := 2025
	runs := []struct {
		source string
		record harvestRecord
	}{
		{"pubmed", harvested("12345", Paper{Title: "Single-cell atlas of Arabidopsis roots.", DOI: "10.1000/ATLAS", PMID: "12345", Year: &year, Source: "PubMed"})},
		{"crossref", harvested("10.1000/atlas", Paper{Title: "A title Crossref spells differently", DOI: "https://doi.org/10.1000/Atlas", Source: "Crossref"})},
		{"europe-pmc", harvested("MED:12345", Paper{Title: "A title Europe PMC spells differently", PMID: "PMID: 12345", Source: "Europe PMC"})},
		{"biorxiv", harvested("no-identifiers", Paper{Title: "SINGLE CELL ATLAS OF ARABIDOPSIS ROOTS", Year: &year, Source: "bioRxiv"})},
	}
	for round := range 2 {
		for _, run := range runs {
			result := syncRecords(t, store, run.source, run.record)
			wantAdded := 0
			if round == 0 && run.source == "pubmed" {
				wantAdded = 1
			}
			if result.PapersAdded != wantAdded || result.Duplicates != 1-wantAdded {
				t.Errorf("round %d %s = added %d, skipped %d", round, run.source, result.PapersAdded, result.Duplicates)
			}
			if result.Status.LastRun.PapersAdded != result.PapersAdded || result.Status.LastRun.Duplicates != result.Duplicates {
				t.Errorf("run did not keep its counts: %+v", result.Status.LastRun)
			}
		}
	}
	papers, err := store.ListPapers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(papers) != 1 || papers[0].DOI != "10.1000/atlas" || papers[0].PMID != "12345" || papers[0].Source != "PubMed" {
		t.Errorf("papers = %+v", papers)
	}
}

func TestSyncSharesIdentifiersFoundThroughTitles(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	syncRecords(t, store, "crossref", harvested("10.1000/maize", Paper{Title: "Root cell types in maize", DOI: "10.1000/maize"}))
	// PubMed knows no DOI here, so only the title can tell it is the same paper.
	if result := syncRecords(t, store, "pubmed", harvested("777", Paper{Title: "Root cell types in maize.", PMID: "777"})); result.PapersAdded != 0 {
		t.Fatalf("title match added a paper: %+v", result)
	}
	// Europe PMC has another title and only the PMID the title match recorded.
	if result := syncRecords(t, store, "europe-pmc", harvested("MED:777", Paper{Title: "Cell types of maize roots", PMID: "777"})); result.PapersAdded != 0 {
		t.Fatalf("PMID match added a paper: %+v", result)
	}
	papers, err := store.ListPapers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(papers) != 1 || papers[0].DOI != "10.1000/maize" || papers[0].PMID != "777" {
		t.Errorf("papers = %+v", papers)
	}
}

func TestSyncKeepsDistinctPapersWithTheSameTitle(t *testing.T) {
	store := openTestLibrary(t)
	early, late := 2010, 2020
	records := []harvestRecord{
		harvested("a", Paper{Title: "Plant single-cell genomics", DOI: "10.1101/preprint", PublicationStatus: "preprint"}),
		harvested("b", Paper{Title: "Plant single-cell genomics", DOI: "10.1000/journal", PublicationStatus: "published"}),
		harvested("c", Paper{Title: "Editorial", PMID: "1", Year: &early}),
		harvested("d", Paper{Title: "Editorial", PMID: "2", Year: &early}),
		harvested("e", Paper{Title: "Commentary", DOI: "10.1000/old", Year: &early}),
		harvested("f", Paper{Title: "Commentary", PMID: "3", Year: &late}),
	}
	if first := syncRecords(t, store, "crossref", records...); first.PapersAdded != 6 {
		t.Errorf("first run = %+v", first)
	}
	if second := syncRecords(t, store, "crossref", records...); second.PapersAdded != 0 || second.Duplicates != 6 {
		t.Errorf("second run = %+v", second)
	}
}

func TestSyncSkipsMalformedDOI(t *testing.T) {
	store := openTestLibrary(t)
	result := syncRecords(t, store, "pubmed", harvested("9", Paper{Title: "A paper", DOI: "10.1016/j.cell. 2020", PMID: "9"}))
	if result.PapersAdded != 1 || result.Papers[0].DOI != "" || result.Papers[0].PMID != "9" {
		t.Errorf("result = %+v", result)
	}
}

func TestSyncKeepsWhatAPersonChanged(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	record := harvested("12345", Paper{Title: "A raw source title", DOI: "10.1000/plant.1", PMID: "12345"})
	first := syncRecords(t, store, "pubmed", record)
	before := first.Papers[0]

	after := before
	after.Title, after.DOI, after.Notes = "The title a person corrected", "", "checked by hand"
	after.Screening, after.ScreeningReason = "excluded", "off topic"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &before, After: after}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("SavePaperEdits() = %v, %v", conflicts, err)
	}
	reviewed, err := store.GetPaper(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}

	// The record no longer matches by title or DOI; only its link to the paper does.
	again := syncRecords(t, store, "pubmed", record)
	if again.PapersAdded != 0 || again.Duplicates != 1 || len(again.Papers) != 1 {
		t.Fatalf("resync = %+v", again)
	}
	// A new source finds the paper by its corrected title, with a DOI the person
	// removed from it.
	other := syncRecords(t, store, "crossref", harvested("10.1000/other", Paper{Title: "The title a person corrected.", DOI: "10.1000/other"}))
	if other.PapersAdded != 0 {
		t.Fatalf("other source = %+v", other)
	}

	current, err := store.GetPaper(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if current.Title != reviewed.Title || current.DOI != "" || current.PMID != reviewed.PMID || current.Notes != reviewed.Notes ||
		current.Screening != "excluded" || current.ScreeningReason != "off topic" ||
		current.ReviewedAt != reviewed.ReviewedAt || current.UpdatedAt != reviewed.UpdatedAt {
		t.Errorf("sync changed reviewed paper:\n got %+v\nwant %+v", current, reviewed)
	}
}

func TestSyncDoesNotRestoreARemovedPaper(t *testing.T) {
	store := openTestLibrary(t)
	record := harvested("12345", Paper{Title: "Removed on purpose", PMID: "12345"})
	syncRecords(t, store, "pubmed", record)
	if _, err := store.db.Exec("DELETE FROM papers"); err != nil {
		t.Fatal(err)
	}
	if result := syncRecords(t, store, "pubmed", record); result.PapersAdded != 0 || result.Duplicates != 1 || len(result.Papers) != 0 {
		t.Errorf("resync = %+v", result)
	}
}

func TestRepositoryRecordsCountAsSkippedWhenSeenAgain(t *testing.T) {
	store := openTestLibrary(t)
	record := harvestRecord{ExternalID: "GSE1", Title: "A series", Payload: map[string]string{"title": "A series"}}
	if first := syncRecords(t, store, "sequence-repositories", record); first.RecordsNew != 1 || first.Duplicates != 0 {
		t.Errorf("first run = %+v", first)
	}
	if second := syncRecords(t, store, "sequence-repositories", record); second.RecordsNew != 0 || second.Duplicates != 1 {
		t.Errorf("second run = %+v", second)
	}
}

func TestSourceAdditionsAwaitReview(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	result := syncRecords(t, store, "pubmed",
		harvested("1", Paper{Title: "Decided later", PMID: "1"}),
		harvested("2", Paper{Title: "Checked without deciding", PMID: "2"}))
	if len(result.Papers) != 2 {
		t.Fatalf("papers = %+v", result.Papers)
	}
	for _, paper := range result.Papers {
		if !paper.AwaitingReview || paper.Screening != "pending" {
			t.Errorf("harvested paper = %+v", paper)
		}
	}
	decided, checked := result.Papers[0], result.Papers[1]
	if decided.Title != "Decided later" {
		decided, checked = checked, decided
	}

	// An agent's edit leaves the paper in review; a person's decision ends it.
	if _, err := store.UpdatePaper(ctx, decided.ID, PaperPatch{Notes: stringPointer("agent note")}); err != nil {
		t.Fatal(err)
	}
	if paper, err := store.GetPaper(ctx, decided.ID); err != nil || !paper.AwaitingReview {
		t.Fatalf("after agent edit = %+v, %v", paper, err)
	}
	before, err := store.GetPaper(ctx, decided.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := before
	after.Screening = "included"
	after.AwaitingReview = true // The window never sets this; the database owns it.
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &before, After: after}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("save decision = %v, %v", conflicts, err)
	}
	if paper, err := store.GetPaper(ctx, decided.ID); err != nil || paper.AwaitingReview || paper.ReviewedAt == "" {
		t.Errorf("after decision = %+v, %v", paper, err)
	}

	if err := store.MarkPaperReviewed(ctx, checked.ID); err != nil {
		t.Fatal(err)
	}
	if paper, err := store.GetPaper(ctx, checked.ID); err != nil || paper.AwaitingReview || paper.Screening != "pending" {
		t.Errorf("after marking reviewed = %+v, %v", paper, err)
	}
	if err := store.MarkPaperReviewed(ctx, "missing"); err == nil {
		t.Error("marking a missing paper succeeded")
	}

	// A person's own paper never enters review.
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{After: Paper{ID: "P-own", Title: "Added by hand", AwaitingReview: true}}}); err != nil {
		t.Fatal(err)
	}
	if paper, err := store.GetPaper(ctx, "P-own"); err != nil || paper.AwaitingReview {
		t.Errorf("hand-added paper = %+v, %v", paper, err)
	}
}

func TestRepositoryRecordsBecomeDatasetsAwaitingReview(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	series := harvestRecord{
		ExternalID: "GSE1", Title: "Root atlas", Payload: map[string]string{"title": "Root atlas"},
		Dataset: repositoryDataset("geo", "study", "GSE1", "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=GSE1",
			"Root atlas", "Single-cell RNA-seq of roots", []string{"Arabidopsis thaliana", " "}),
	}
	untitled := harvestRecord{
		ExternalID: "SRX2", Payload: map[string]string{},
		Dataset: repositoryDataset("sra", "experiment", "SRX2", "", "", "", []string{""}),
	}
	first := syncRecords(t, store, "sequence-repositories", series, untitled)
	if first.DatasetsAdded != 2 || first.Duplicates != 0 || first.Status.LastRun.DatasetsAdded != 2 {
		t.Fatalf("first run = %+v", first)
	}
	datasets, err := store.ListDatasets(ctx)
	if err != nil || len(datasets) != 2 {
		t.Fatalf("datasets = %+v, %v", datasets, err)
	}
	var atlas Dataset
	for _, dataset := range datasets {
		if !dataset.AwaitingReview || dataset.Status != "pending" || len(dataset.PaperLinks) != 0 {
			t.Errorf("harvested dataset = %+v", dataset)
		}
		if dataset.Accession == "GSE1" {
			atlas = dataset
		} else if dataset.Title != "SRX2" || len(dataset.Species) != 0 {
			t.Errorf("untitled dataset = %+v", dataset)
		}
	}
	if atlas.Title != "Root atlas" || atlas.Repository != "geo" || len(atlas.Species) != 1 ||
		atlas.Repositories[0].RecordKind != "study" || atlas.Repositories[0].URL == "" {
		t.Errorf("GEO dataset = %+v", atlas)
	}

	// Seen again, or already held under the accession, a record adds nothing.
	again := syncRecords(t, store, "sequence-repositories", series)
	if again.DatasetsAdded != 0 || again.Duplicates != 1 {
		t.Errorf("second run = %+v", again)
	}
	if _, err := store.AddDataset(ctx, DatasetInput{Title: "Leaf", Repositories: []RepositoryInput{{Repository: "ena", Accession: "PRJEB3"}}}); err != nil {
		t.Fatal(err)
	}
	known := harvestRecord{ExternalID: "PRJEB3", Payload: map[string]string{},
		Dataset: repositoryDataset("ena", "study", "PRJEB3", "", "Leaf study", "", nil)}
	if run := syncRecords(t, store, "sequence-repositories", known); run.DatasetsAdded != 0 || run.Duplicates != 1 {
		t.Errorf("known accession run = %+v", run)
	}
	if automation, err := store.SourceAutomation(ctx); err != nil || automation.Added != 0 {
		t.Errorf("automation = %+v, %v", automation, err)
	}

	if err := store.MarkDatasetReviewed(ctx, atlas.ID); err != nil {
		t.Fatal(err)
	}
	if dataset, err := store.GetDataset(ctx, atlas.ID); err != nil || dataset.AwaitingReview {
		t.Errorf("after marking reviewed = %+v, %v", dataset, err)
	}
	other := datasets[0]
	if other.ID == atlas.ID {
		other = datasets[1]
	}
	after := other
	after.Status = "identified"
	if _, err := store.SaveDatasetEdits(ctx, []DatasetEdit{{Before: &other, After: after}}); err != nil {
		t.Fatal(err)
	}
	if dataset, err := store.GetDataset(ctx, other.ID); err != nil || dataset.AwaitingReview || dataset.Status != "identified" {
		t.Errorf("after status change = %+v, %v", dataset, err)
	}
}

func TestOnlyOneUpdateRunsAtATime(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	finished := make(chan error, 1)
	go func() {
		_, err := store.syncSource(ctx, "pubmed", harvesterFunc(func(context.Context, SourceConfig) ([]harvestRecord, error) {
			close(entered)
			<-release
			return nil, nil
		}))
		finished <- err
	}()
	<-entered
	for _, id := range []string{"pubmed", "crossref"} {
		if _, err := store.syncSource(ctx, id, fakeHarvester{}); !errors.Is(err, ErrSyncBusy) {
			t.Errorf("%s during a run = %v, want ErrSyncBusy", id, err)
		}
	}
	close(release)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	if _, err := store.syncSource(ctx, "crossref", fakeHarvester{}); err != nil {
		t.Errorf("after the run = %v", err)
	}

	// A run left open by a process that died must not block updates for good.
	long := time.Now().UTC().Add(-2 * sourceRunLimit).Format(time.RFC3339Nano)
	if _, err := store.db.Exec("INSERT INTO harvest_runs(id, source_id, status, started_at) VALUES('HR-dead', 'pubmed', 'running', ?)", long); err != nil {
		t.Fatal(err)
	}
	if _, err := store.syncSource(ctx, "crossref", fakeHarvester{}); err != nil {
		t.Errorf("with a dead run = %v", err)
	}
	if err := store.runDueSources(ctx, fakeHarvester{}, func() {}); err != nil {
		t.Fatal(err)
	}
	var status, message string
	if err := store.db.QueryRow("SELECT status, error_message FROM harvest_runs WHERE id = 'HR-dead'").Scan(&status, &message); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || message == "" {
		t.Errorf("dead run = %s, %q", status, message)
	}
}

func TestSourceAutomationRunsWhatIsDue(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	calls := []string{}
	failing := ""
	harvester := harvesterFunc(func(_ context.Context, config SourceConfig) ([]harvestRecord, error) {
		calls = append(calls, config.ID)
		if config.ID == failing {
			return nil, errors.New("remote unavailable")
		}
		if config.ID == "sequence-repositories" {
			return []harvestRecord{{ExternalID: "GSE1", Title: "A series", Payload: map[string]string{"title": "A series"},
				Dataset: repositoryDataset("geo", "study", "GSE1", "", "A series", "", nil)}}, nil
		}
		return []harvestRecord{harvested(config.ID, Paper{Title: "Paper from " + config.ID, Source: config.ID})}, nil
	})
	run := func(t *testing.T) []string {
		t.Helper()
		calls = calls[:0]
		if err := store.runDueSources(ctx, harvester, func() {}); err != nil {
			t.Fatal(err)
		}
		return append([]string{}, calls...)
	}
	age := func(t *testing.T, by time.Duration) {
		t.Helper()
		rows, err := store.db.Query("SELECT id, started_at FROM harvest_runs")
		if err != nil {
			t.Fatal(err)
		}
		runs := map[string]time.Time{}
		for rows.Next() {
			var id, started string
			if err := rows.Scan(&id, &started); err != nil {
				t.Fatal(err)
			}
			if runs[id], err = time.Parse(time.RFC3339Nano, started); err != nil {
				t.Fatal(err)
			}
		}
		_ = rows.Close()
		for id, started := range runs {
			if _, err := store.db.Exec("UPDATE harvest_runs SET started_at = ? WHERE id = ?", started.Add(-by).Format(time.RFC3339Nano), id); err != nil {
				t.Fatal(err)
			}
		}
	}

	if automation, err := store.SourceAutomation(ctx); err != nil || automation.Enabled || automation.IntervalHours != 24 || automation.NextRunAt != "" {
		t.Fatalf("defaults = %+v, %v", automation, err)
	}
	if got := run(t); len(got) != 0 {
		t.Fatalf("disabled automation ran %v", got)
	}
	if _, err := store.SaveSourceAutomation(ctx, true, 0); err == nil {
		t.Error("interval of 0 hours succeeded")
	}
	if _, err := store.SaveSourceAutomation(ctx, true, 721); err == nil {
		t.Error("interval above 720 hours succeeded")
	}

	automation, err := store.SaveSourceAutomation(ctx, true, 48)
	if err != nil {
		t.Fatal(err)
	}
	if automation.NextRunAt == "" {
		t.Errorf("sources that never ran have no next run: %+v", automation)
	}
	if got := run(t); len(got) != 5 {
		t.Fatalf("first run = %v, want the 5 enabled connectors", got)
	}
	automation, err = store.SourceAutomation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if automation.Added != 5 || automation.Skipped != 0 || automation.LastRunAt == "" || automation.Running || len(automation.Failures) != 0 {
		t.Errorf("after first run = %+v", automation)
	}
	next, err := time.Parse(time.RFC3339Nano, automation.NextRunAt)
	if err != nil || next.Before(time.Now().Add(47*time.Hour)) {
		t.Errorf("next run = %q, %v", automation.NextRunAt, err)
	}
	if got := run(t); len(got) != 0 {
		t.Fatalf("nothing due, ran %v", got)
	}

	// Settings and schedule survive a restart, and what was missed runs once.
	reopened, err := Open(store.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	restored, err := reopened.SourceAutomation(ctx)
	if err != nil || !restored.Enabled || restored.IntervalHours != 48 {
		t.Fatalf("after reopening = %+v, %v", restored, err)
	}
	age(t, 49*time.Hour)
	failing = "crossref"
	if got := run(t); len(got) != 5 {
		t.Fatalf("catch-up = %v", got)
	}
	automation, err = store.SourceAutomation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(automation.Failures) != 1 || automation.Failures[0].Source != "Crossref" || automation.Failures[0].Message != "remote unavailable" {
		t.Errorf("failures = %+v", automation.Failures)
	}
	if automation.Added != 0 || automation.Skipped != 4 {
		t.Errorf("rerun summary = %+v", automation)
	}
	if next, err := time.Parse(time.RFC3339Nano, automation.NextRunAt); err != nil || next.After(time.Now().Add(sourceRetryDelay)) {
		t.Errorf("failed source retries at %q, %v", automation.NextRunAt, err)
	}

	// Only the failure comes back early.
	failing = ""
	age(t, 61*time.Minute)
	if got := run(t); len(got) != 1 || got[0] != "crossref" {
		t.Fatalf("retry = %v", got)
	}

	// A disabled source and switched-off automation both stay quiet.
	age(t, 49*time.Hour)
	if _, err := store.SaveSourceConfig(ctx, SourceConfig{ID: "pubmed", Query: "plant", MaxResults: 5}); err != nil {
		t.Fatal(err)
	}
	if got := run(t); len(got) != 4 {
		t.Fatalf("with one source disabled = %v", got)
	}
	age(t, 49*time.Hour)
	if _, err := store.SaveSourceAutomation(ctx, false, 48); err != nil {
		t.Fatal(err)
	}
	if got := run(t); len(got) != 0 {
		t.Fatalf("switched off, ran %v", got)
	}
}

func TestSwitchingAutomationOffStopsACycle(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	if _, err := store.SaveSourceAutomation(ctx, true, 24); err != nil {
		t.Fatal(err)
	}
	calls := 0
	harvester := harvesterFunc(func(context.Context, SourceConfig) ([]harvestRecord, error) {
		calls++
		_, err := store.SaveSourceAutomation(ctx, false, 24)
		return nil, err
	})
	if err := store.runDueSources(ctx, harvester, func() {}); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("harvested %d sources after switching off", calls)
	}
}

func TestBackgroundUpdatesCatchUpAndStop(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	if _, err := store.SaveSourceAutomation(ctx, true, 24); err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{}, 1)
	harvester := harvesterFunc(func(ctx context.Context, _ SourceConfig) ([]harvestRecord, error) {
		entered <- struct{}{}
		<-ctx.Done() // A remote request that is still waiting when the app quits.
		return nil, ctx.Err()
	})
	changes := make(chan struct{}, 16)
	stop := store.startSourceUpdates(harvester, time.Hour, func() { changes <- struct{}{} })
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("a source that never ran was not picked up at startup")
	}
	<-changes
	automation, err := store.SourceAutomation(ctx)
	if err != nil || !automation.Running {
		t.Errorf("while running = %+v, %v", automation, err)
	}
	stop()
	status, err := store.SourceStatus(ctx, "pubmed")
	if err != nil {
		t.Fatal(err)
	}
	if status.LastRun == nil || status.LastRun.Status != "failed" || status.LastRun.ErrorMessage != "应用退出，本次更新已中断" {
		t.Errorf("interrupted run = %+v", status.LastRun)
	}
}

func TestOpenRekeysExistingPapers(t *testing.T) {
	store := openTestLibrary(t)
	ctx := context.Background()
	for _, paper := range []Paper{
		{ID: "A", Title: "First paper.", DOI: "10.1000/a", PMID: "111"},
		{ID: "B", Title: "Second paper", DOI: "10.1000/b", PMID: "222"},
		{ID: "C", Title: "Third paper"},
	} {
		if _, err := store.AddPaper(ctx, paper); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.db.Exec(`
		UPDATE paper_curation SET screening_status = 'included', screening_method = 'human',
			screening_reviewed_at = '2026-01-01T00:00:00Z' WHERE paper_id = 'C';
		UPDATE paper_identifiers SET value = 'PMID:222' WHERE value = '222';
		UPDATE paper_identifiers SET value = '10.1000/b.' WHERE value = '10.1000/b';
		INSERT INTO paper_identifiers(paper_id, namespace, value) VALUES('C', 'pmid', 'PMID:111');
		DROP INDEX idx_papers_title_key;
		ALTER TABLE papers DROP COLUMN title_key;
		ALTER TABLE harvest_runs DROP COLUMN papers_added;
		ALTER TABLE harvest_runs DROP COLUMN duplicates;
		DROP TABLE source_automation;
		DELETE FROM schema_migrations WHERE version = 8;
		PRAGMA user_version = 7;`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(store.path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	papers, err := store.ListPapers(ctx)
	if err != nil || len(papers) != 3 {
		t.Fatalf("papers after upgrade = %d, %v", len(papers), err)
	}
	second, err := store.GetPaper(ctx, "B")
	if err != nil || second.PMID != "222" || second.DOI != "10.1000/b" {
		t.Errorf("B = %+v, %v", second, err)
	}
	// C held A's PMID in another spelling. It keeps its review and stays editable.
	third, err := store.GetPaper(ctx, "C")
	if err != nil || third.PMID != "" || third.ReviewedAt == "" {
		t.Errorf("C = %+v, %v", third, err)
	}
	var setAside string
	if err := store.db.QueryRow("SELECT value FROM paper_identifiers WHERE paper_id = 'C' AND namespace = 'other'").Scan(&setAside); err != nil || setAside != "PMID:111" {
		t.Errorf("set aside = %q, %v", setAside, err)
	}
	edited := third
	edited.Notes = "still editable"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &third, After: edited}}); err != nil || len(conflicts) != 0 {
		t.Errorf("edit after upgrade = %v, %v", conflicts, err)
	}
	if result := syncRecords(t, store, "crossref", harvested("x", Paper{Title: "first paper", PMID: "111"})); result.PapersAdded != 0 {
		t.Errorf("title and PMID no longer match: %+v", result)
	}
	if automation, err := store.SourceAutomation(ctx); err != nil || automation.Enabled {
		t.Errorf("automation after upgrade = %+v, %v", automation, err)
	}
	var rekeyed int
	if err := store.db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version = 8").Scan(&rekeyed); err != nil || rekeyed != 1 {
		t.Errorf("rekey records = %d, %v", rekeyed, err)
	}
}

func TestPubMedXMLParsesNestedTextAndIdentifiers(t *testing.T) {
	input := `<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>42</PMID><Article>
		<ArticleTitle>Single-cell <i>Arabidopsis</i> atlas</ArticleTitle>
		<Abstract><AbstractText Label="BACKGROUND">A &amp; B.</AbstractText></Abstract>
		<AuthorList><Author><LastName>Li</LastName><ForeName>Wei</ForeName></Author></AuthorList>
		<Journal><Title>Plant Journal</Title><JournalIssue><PubDate><Year>2025</Year></PubDate></JournalIssue></Journal>
		<PublicationTypeList><PublicationType>Journal Article</PublicationType></PublicationTypeList>
		</Article></MedlineCitation><PubmedData><ArticleIdList><ArticleId IdType="doi">10.1000/TEST</ArticleId></ArticleIdList></PubmedData>
		</PubmedArticle></PubmedArticleSet>`
	var parsed pubmedArticleSet
	if err := xml.Unmarshal([]byte(input), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Articles) != 1 {
		t.Fatalf("articles = %d", len(parsed.Articles))
	}
	article := parsed.Articles[0]
	if got := article.Citation.Article.Title.String(); got != "Single-cell Arabidopsis atlas" {
		t.Errorf("title = %q", got)
	}
	if got := stripMarkup(article.Citation.Article.Abstract.Sections[0].Inner); got != "A & B." {
		t.Errorf("abstract = %q", got)
	}
	if article.Identifiers[0].Value != "10.1000/TEST" {
		t.Errorf("DOI = %q", article.Identifiers[0].Value)
	}
}

func TestRemoteConnectorsLive(t *testing.T) {
	if os.Getenv("BIBLIOCELL_LIVE_TEST") != "1" {
		t.Skip("set BIBLIOCELL_LIVE_TEST=1 to query public source APIs")
	}
	configs := []SourceConfig{}
	for _, id := range []string{"pubmed", "europe-pmc", "crossref", "biorxiv", "sequence-repositories"} {
		definition, _ := sourceDefinitionByID(id)
		config := definition.defaults
		config.MaxResults = 1
		if id != "sequence-repositories" {
			configs = append(configs, config)
			continue
		}
		for _, provider := range []string{"geo", "sra", "ena"} {
			configured := config
			configured.Provider = provider
			if provider == "sra" {
				configured.Query = `("single cell"[All Fields] OR "single-cell"[All Fields] OR "single nucleus"[All Fields]) AND (plant[All Fields] OR Arabidopsis[Organism])`
			}
			if provider == "ena" {
				configured.Query = `description="*single*" AND tax_tree(33090)`
			}
			configs = append(configs, configured)
		}
	}
	harvester := newRemoteHarvester(noCredentials)
	for _, config := range configs {
		name := config.ID
		if config.Provider != "" {
			name += "/" + config.Provider
		}
		t.Run(name, func(t *testing.T) {
			records, err := harvester.Harvest(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) == 0 {
				t.Log("query succeeded and returned no matching records")
				return
			}
			if records[0].ExternalID == "" || records[0].Title == "" && records[0].Paper == nil {
				t.Errorf("record lacks display metadata: %+v", records[0])
			}
			t.Logf("received %d record(s), first external ID %s", len(records), records[0].ExternalID)
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestHarvesterSendsCredentials(t *testing.T) {
	credentials := SourceCredentials{NCBIAPIKey: "ncbi-secret", ContactEmail: "me@example.org"}
	harvester := newRemoteHarvester(func() SourceCredentials { return credentials })
	requests := []*url.URL{}
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requests = append(requests, request.URL)
		body := `{"esearchresult":{"idlist":["1"]},"result":{}}`
		if strings.Contains(request.URL.Path, "efetch") {
			body = `<PubmedArticleSet></PubmedArticleSet>`
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	ctx := context.Background()
	for _, config := range []SourceConfig{
		{ID: "pubmed", Query: "plant", MaxResults: 1},
		{ID: "sequence-repositories", Provider: "geo", Query: "plant", MaxResults: 1},
		{ID: "sequence-repositories", Provider: "sra", Query: "plant", MaxResults: 1},
	} {
		if _, err := harvester.Harvest(ctx, config); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 6 {
		t.Fatalf("made %d NCBI requests, want 6", len(requests))
	}
	for _, request := range requests {
		if request.Query().Get("api_key") != credentials.NCBIAPIKey {
			t.Errorf("%s sent no API key", request.Path)
		}
	}

	requests = requests[:0]
	if _, err := harvester.Harvest(ctx, SourceConfig{ID: "crossref", Query: "plant", MaxResults: 1}); err != nil {
		t.Fatal(err)
	}
	if got := requests[0].Query().Get("mailto"); got != credentials.ContactEmail {
		t.Errorf("Crossref mailto = %q", got)
	}

	requests = requests[:0]
	if _, err := harvester.Harvest(ctx, SourceConfig{ID: "europe-pmc", Query: "plant", MaxResults: 1}); err != nil {
		t.Fatal(err)
	}
	if query := requests[0].RawQuery; strings.Contains(query, "secret") || strings.Contains(query, "example.org") {
		t.Errorf("Europe PMC received another service's credentials: %s", query)
	}

	// The error ends up in harvest_runs, shown in the app and readable over SQL.
	harvester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	_, err := harvester.Harvest(ctx, SourceConfig{ID: "pubmed", Query: "plant", MaxResults: 1})
	if err == nil || strings.Contains(err.Error(), credentials.NCBIAPIKey) {
		t.Errorf("failure exposes the key or was lost: %v", err)
	}
	harvester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		body := `{"error":"API key invalid","api-key":"ncbi-secret"}`
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	_, err = harvester.Harvest(ctx, SourceConfig{ID: "pubmed", Query: "plant", MaxResults: 1})
	if err == nil || strings.Contains(err.Error(), credentials.NCBIAPIKey) {
		t.Errorf("rejected key is repeated or was lost: %v", err)
	}
}

func noCredentials() SourceCredentials { return SourceCredentials{} }

func pdfResponse(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func TestFetchFulltext(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	paper, err := store.AddPaper(ctx, Paper{Title: "Open paper", DOI: "10.1000/open", Source: "test"})
	if err != nil {
		t.Fatal(err)
	}
	bare, err := store.AddPaper(ctx, Paper{Title: "No identifiers", Source: "test"})
	if err != nil {
		t.Fatal(err)
	}

	email := ""
	unpaywallBody := `{"oa_locations":[{"url_for_pdf":null,"url_for_landing_page":"https://example.org/page"},` +
		`{"url_for_pdf":"https://blocked.example/a.pdf","license":"cc-by"},` +
		`{"url_for_pdf":"http://plain.example/a.pdf"},` +
		`{"url_for_pdf":"https://publisher.example/a.pdf","license":"cc-by"}]}`
	harvester := newRemoteHarvester(func() SourceCredentials { return SourceCredentials{ContactEmail: email} })
	requested := []string{}
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requested = append(requested, request.URL.Host)
		switch request.URL.Host {
		case "www.ebi.ac.uk":
			return pdfResponse(`{"resultList":{"result":[{"pmcid":"PMC1"}]}}`), nil
		case "pmc-oa-opendata.s3.amazonaws.com":
			return pdfResponse(`<ListBucketResult><Contents><Key>PMC1.1/PMC1.1.json</Key></Contents></ListBucketResult>`), nil
		case "api.unpaywall.org":
			if request.URL.Query().Get("email") != email || request.URL.Path != "/v2/10.1000/open" {
				t.Errorf("Unpaywall request = %s", request.URL)
			}
			return pdfResponse(unpaywallBody), nil
		case "blocked.example":
			return pdfResponse("<html>Just a moment...</html>"), nil
		case "plain.example":
			if request.URL.Scheme != "https" {
				t.Errorf("requested %s", request.URL)
			}
			return &http.Response{StatusCode: 429, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "publisher.example":
			return pdfResponse("%PDF-1.7 body"), nil
		}
		t.Errorf("unexpected request to %s", request.URL)
		return nil, errors.New("unexpected request")
	})

	if _, err := store.fetchFulltext(ctx, bare.ID, harvester); !errors.Is(err, ErrNoFulltext) {
		t.Errorf("paper without identifiers: %v", err)
	}
	if _, err := store.fetchFulltext(ctx, paper.ID, harvester); !errors.Is(err, ErrNoFulltext) || !strings.Contains(err.Error(), "联系邮箱") {
		t.Errorf("without email: %v", err)
	}

	email = "me@example.org"
	result, err := store.fetchFulltext(ctx, paper.ID, harvester)
	if err != nil {
		t.Fatal(err)
	}
	if result.Source != "Unpaywall" || result.URL != "https://publisher.example/a.pdf" || result.File.Type != "fulltext" {
		t.Errorf("result = %+v", result)
	}
	var link, license, status string
	if err := store.db.QueryRowContext(ctx, "SELECT url, license, access_status FROM resources WHERE id = ?", result.File.ID).
		Scan(&link, &license, &status); err != nil {
		t.Fatal(err)
	}
	if link != result.URL || license != "cc-by" || status != "available" {
		t.Errorf("provenance = %s %s %s", link, license, status)
	}
	stored, err := store.GetPaper(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Fulltext != "stored" {
		t.Errorf("paper fulltext = %q", stored.Fulltext)
	}

	requested = requested[:0]
	again, err := store.fetchFulltext(ctx, paper.ID, harvester)
	if err != nil || again.Source != "" || again.File.ID != result.File.ID || len(requested) != 0 {
		t.Errorf("second fetch = %+v, %v, requests %v", again, err, requested)
	}
}

func TestPMCCandidatePicksLatestPDF(t *testing.T) {
	harvester := newRemoteHarvester(noCredentials)
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "www.ebi.ac.uk" {
			return pdfResponse(`{"resultList":{"result":[{"pmcid":"PMC7","license":"cc by"}]}}`), nil
		}
		if got := request.URL.Query().Get("prefix"); got != "PMC7." {
			t.Errorf("prefix = %q", got)
		}
		return pdfResponse(`<ListBucketResult><Contents><Key>PMC7.10/PMC7.10.pdf</Key></Contents>` +
			`<Contents><Key>PMC7.2/PMC7.2.pdf</Key></Contents><Contents><Key>PMC7.2/PMC7.2.xml</Key></Contents></ListBucketResult>`), nil
	})
	candidate, err := harvester.pmcCandidate(context.Background(), Paper{PMID: "123"})
	if err != nil {
		t.Fatal(err)
	}
	if candidate == nil || !strings.HasSuffix(candidate.url, "PMC7.10/PMC7.10.pdf") || candidate.license != "cc by" {
		t.Errorf("candidate = %+v", candidate)
	}
}

func stringPointer(value string) *string { return &value }
