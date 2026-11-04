package library

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"testing"
)

func mergeStore(t *testing.T) (*Store, context.Context) {
	t.Helper()
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store, context.Background()
}

func decide(t *testing.T, store *Store, paper Paper, screening, reason string) Paper {
	t.Helper()
	after := paper
	after.Screening, after.ScreeningReason = screening, reason
	if conflicts, err := store.SavePaperEdits(context.Background(), []PaperEdit{{Before: &paper, After: after}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("decide %s: %v %v", paper.ID, conflicts, err)
	}
	decided, err := store.GetPaper(context.Background(), paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	return decided
}

func TestMergePapersFoldsPreprintIntoPublished(t *testing.T) {
	store, ctx := mergeStore(t)
	year := 2024
	preprint, err := store.AddPaper(ctx, Paper{
		Title: "Root atlas", DOI: "10.1101/2024.01.01.000001", Journal: "bioRxiv", PublicationStatus: "preprint", Year: &year,
		Abstract: "Preprint abstract.", Organs: []string{"root"}, Tags: []string{"atlas"}, Notes: "Read the methods.", Starred: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	preprint = decide(t, store, preprint, "included", "")
	published, err := store.AddPaper(ctx, Paper{
		Title: "A root atlas", DOI: "10.1038/s41477-025-00001-x", Journal: "Nature Plants", PublicationStatus: "published",
		Tags: []string{"plants"}, Notes: "Journal version.", AwaitingReview: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`INSERT INTO resources(id, resource_type, title, url) VALUES('R1', 'fulltext', 'preprint.pdf', 'https://example.org/p.pdf')`,
		`INSERT INTO paper_resources(paper_id, resource_id) VALUES('` + preprint.ID + `', 'R1')`,
		`INSERT INTO datasets(id, title) VALUES('D1', 'Root data'), ('D2', 'Other data')`,
		`INSERT INTO paper_datasets(paper_id, dataset_id, role, verified) VALUES('` + preprint.ID + `', 'D1', 'generated', 1), ('` + preprint.ID + `', 'D2', 'reused', 0), ('` + published.ID + `', 'D1', 'generated', 0)`,
		`INSERT INTO field_evidence(id, target_paper_id, field_name, source_url, extraction_method) VALUES('E1', '` + preprint.ID + `', 'organs', 'https://example.org', 'model')`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}

	for _, bad := range []MergeInput{
		{KeepID: published.ID, MergeID: published.ID, Reason: "self"},
		{KeepID: published.ID, MergeID: preprint.ID},
		{KeepID: published.ID, MergeID: "P-missing", Reason: "gone"},
	} {
		if _, err := store.MergePapers(ctx, bad); err == nil {
			t.Errorf("MergePapers(%+v) succeeded", bad)
		}
	}

	result, err := store.MergePapers(ctx, MergeInput{KeepID: published.ID, MergeID: preprint.ID, Reason: "Crossref is-preprint-of", Via: "CLI"})
	if err != nil {
		t.Fatal(err)
	}
	kept := result.Paper
	if kept.DOI != published.DOI || kept.Journal != "Nature Plants" || kept.PublicationStatus != "published" || kept.Title != "A root atlas" {
		t.Errorf("kept bibliographic record changed: %+v", kept)
	}
	if kept.Screening != "included" || kept.ReviewedAt != preprint.ReviewedAt || kept.AwaitingReview || result.ScreeningFrom != "merged" {
		t.Errorf("screening not carried over: %+v from %s", kept, result.ScreeningFrom)
	}
	if !kept.Starred || !slices.Equal(kept.Tags, []string{"plants", "atlas"}) || kept.Notes != "Journal version.\n\nRead the methods." ||
		kept.Abstract != "Preprint abstract." || !slices.Equal(kept.Organs, []string{"root"}) {
		t.Errorf("curation not combined: %+v", kept)
	}
	if result.Moved.Files != 1 || result.Moved.DatasetLinks != 1 || result.Moved.Evidence != 1 {
		t.Errorf("moved = %+v", result.Moved)
	}
	if _, err := store.GetPaper(ctx, preprint.ID); !errors.Is(err, ErrPaperNotFound) {
		t.Errorf("preprint still there: %v", err)
	}
	var links, verified, files int
	store.db.QueryRow(`SELECT count(*), sum(verified) FROM paper_datasets WHERE paper_id = ?`, published.ID).Scan(&links, &verified)
	store.db.QueryRow(`SELECT count(*) FROM paper_resources WHERE paper_id = ?`, published.ID).Scan(&files)
	if links != 2 || verified != 1 || files != 1 {
		t.Errorf("links = %d (verified %d), files = %d", links, verified, files)
	}
	merged, err := store.ListMergedPapers(ctx, published.ID)
	if err != nil || len(merged) != 1 || merged[0].DOI != preprint.DOI || merged[0].MergedVia != "CLI" || merged[0].Reason != "Crossref is-preprint-of" {
		t.Fatalf("merged = %+v, %v", merged, err)
	}

	// The preprint never comes back: imports find the kept paper by its DOI or old ID.
	again, err := store.ImportPapers(ctx, []Paper{
		{Title: "Root atlas", DOI: "https://doi.org/10.1101/2024.01.01.000001"},
		{ID: preprint.ID, Title: "Root atlas, renamed"},
	}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Count != 0 || len(again.Skipped) != 2 || again.Skipped[0].ExistingID != published.ID || again.Skipped[1].ExistingID != published.ID {
		t.Errorf("re-import = %+v", again)
	}
}

func TestMergePapersRefusesDifferentDecisions(t *testing.T) {
	store, ctx := mergeStore(t)
	a, _ := store.AddPaper(ctx, Paper{Title: "Same work", DOI: "10.1101/2024.02.02.000002", PublicationStatus: "preprint"})
	b, _ := store.AddPaper(ctx, Paper{Title: "Same work", DOI: "10.1000/journal.2", PublicationStatus: "published"})
	decide(t, store, a, "excluded", "spatial only")
	decide(t, store, b, "included", "")
	if _, err := store.MergePapers(ctx, MergeInput{KeepID: b.ID, MergeID: a.ID, Reason: "same title"}); !errors.Is(err, ErrMergeConflict) {
		t.Fatalf("err = %v, want ErrMergeConflict", err)
	}
	if _, err := store.GetPaper(ctx, a.ID); err != nil {
		t.Errorf("refused merge removed the duplicate: %v", err)
	}
}

func TestMergedRecordDoesNotLendIdentifiers(t *testing.T) {
	store, ctx := mergeStore(t)
	pre, _ := store.AddPaper(ctx, Paper{Title: "Leaf atlas", DOI: "10.1101/2024.03.03.000003", PMID: "38000001", PublicationStatus: "preprint"})
	pub, _ := store.AddPaper(ctx, Paper{Title: "Leaf atlas", DOI: "10.1000/leaf", PublicationStatus: "published"})
	if _, err := store.MergePapers(ctx, MergeInput{KeepID: pub.ID, MergeID: pre.ID, Reason: "same title and authors"}); err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// A source offering the preprint again finds the kept paper but leaves its identifiers alone.
	offered := Paper{Title: "Leaf atlas", DOI: pre.DOI, PMID: pre.PMID}
	id, err := existingPaperID(ctx, tx, offered)
	if err != nil || id != pub.ID {
		t.Fatalf("existingPaperID = %q, %v", id, err)
	}
	if err := fillPaperIdentifiers(ctx, tx, id, offered); err != nil {
		t.Fatal(err)
	}
	if kept, err := readPaper(ctx, tx, pub.ID); err != nil || kept.PMID != "" || kept.DOI != "10.1000/leaf" {
		t.Errorf("kept paper took the preprint's identifiers: %+v %v", kept, err)
	}
}

func TestFindDuplicatesSuggestsThePublishedVersion(t *testing.T) {
	store, ctx := mergeStore(t)
	pre, _ := store.AddPaper(ctx, Paper{Title: "Single-cell <i>Arabidopsis</i> roots", DOI: "10.1101/2024.04.04.000004", PublicationStatus: "preprint"})
	pub, _ := store.AddPaper(ctx, Paper{Title: "Single-cell Arabidopsis roots.", DOI: "10.1000/roots", PublicationStatus: "published"})
	store.AddPaper(ctx, Paper{Title: "Unrelated"})
	result, err := store.FindDuplicates(ctx, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Groups) != 1 || len(result.Groups[0].Papers) != 2 || result.Groups[0].SuggestedKeepID != pub.ID || result.Groups[0].Papers[0].ID != pre.ID {
		t.Errorf("duplicates = %+v", result)
	}
}

func TestFindPublishedVersionsReadsCrossrefRelation(t *testing.T) {
	store, ctx := mergeStore(t)
	withJournal, _ := store.AddPaper(ctx, Paper{Title: "Shoot atlas", DOI: "10.1101/2024.05.05.000005", PublicationStatus: "preprint"})
	store.AddPaper(ctx, Paper{Title: "Shoot atlas, revised title", DOI: "10.1000/shoot", PublicationStatus: "published"})
	store.AddPaper(ctx, Paper{Title: "Still a preprint", DOI: "10.1101/2024.06.06.000006", PublicationStatus: "preprint"})
	store.AddPaper(ctx, Paper{Title: "Journal only", DOI: "10.1000/other", PublicationStatus: "published"})
	harvester := newRemoteHarvester(noCredentials)
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.EscapedPath() {
		case "/works/10.1101/2024.05.05.000005":
			return pdfResponse(`{"message":{"DOI":"10.1101/2024.05.05.000005","title":["Shoot atlas"],"relation":{"is-preprint-of":[{"id":"10.1000/SHOOT","id-type":"doi"}]}}}`), nil
		case "/works/10.1101/2024.06.06.000006":
			return pdfResponse(`{"message":{"DOI":"10.1101/2024.06.06.000006","title":["Still a preprint"]}}`), nil
		}
		t.Errorf("unexpected request %s", request.URL)
		return nil, errors.New("unexpected request")
	})
	result, err := store.findPublishedVersions(ctx, 20, 0, harvester)
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 2 || result.Checked != 2 || len(result.Errors) != 0 || len(result.Published) != 1 {
		t.Fatalf("result = %+v", result)
	}
	found := result.Published[0]
	if found.Preprint.ID != withJournal.ID || found.PublishedDOI != "10.1000/shoot" || found.PublishedPaperID == "" {
		t.Errorf("published version = %+v", found)
	}
}
