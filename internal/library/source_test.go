package library

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
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
			return []harvestRecord{{ExternalID: "GSE1", Title: "A series", Payload: map[string]string{"title": "A series"}}}, nil
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
	harvester := newRemoteHarvester()
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

func stringPointer(value string) *string { return &value }
