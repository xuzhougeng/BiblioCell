package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeletePaperPreservesFilesAndLinksForRestore(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	store, paper := fileFixture(t, storage)
	source := writeSource(t, "original.pdf", "pdf content")
	file, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.AddPaper(ctx, Paper{Title: "Keep this paper"})
	if err != nil {
		t.Fatal(err)
	}
	dataset, err := store.AddDataset(ctx, DatasetInput{Title: "Keep this dataset", Papers: []PaperLinkInput{{PaperID: paper.ID, EvidenceText: "linked"}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePaper(ctx, paper.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetPaper(ctx, paper.ID); err == nil {
		t.Fatal("deleted paper remains")
	}
	if _, err := store.GetPaper(ctx, other.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(storage, filepath.FromSlash(file.LocalPath))); err != nil {
		t.Fatalf("stored copy lost: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("original removed: %v", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM datasets WHERE id = ?", dataset.Dataset.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dataset removed: %d, %v", count, err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM paper_datasets WHERE paper_id = ?", paper.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("link lost: %d, %v", count, err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM resources WHERE id = ?", file.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("resource lost: %d, %v", count, err)
	}
	if err := store.DeletePaper(ctx, paper.ID); err == nil {
		t.Fatal("missing paper deletion should fail")
	}
	if files, err := store.ListPaperFiles(ctx); err != nil || len(files) != 0 {
		t.Fatalf("trashed files visible: %+v %v", files, err)
	}
	if err := store.RestoreTrash(ctx, "paper", paper.ID); err != nil {
		t.Fatal(err)
	}
	if restored, err := store.GetPaper(ctx, paper.ID); err != nil || restored.Title != paper.Title {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	if files, err := store.ListPaperFiles(ctx); err != nil || len(files) != 1 {
		t.Fatalf("restored files: %+v %v", files, err)
	}
}

func TestDeleteDatasetKeepsPapersAndStaysDeletedAfterSync(t *testing.T) {
	ctx := context.Background()
	store := openTestLibrary(t)
	record := harvestRecord{ExternalID: "GSE9", Payload: map[string]string{"accession": "GSE9"},
		Dataset: repositoryDataset("geo", "study", "GSE9", "", "Root atlas", "", nil)}
	if run := syncRecords(t, store, "sequence-repositories", record); run.DatasetsAdded != 1 {
		t.Fatalf("first sync = %+v", run)
	}
	datasets, err := store.ListDatasets(ctx)
	if err != nil || len(datasets) != 1 {
		t.Fatalf("datasets = %d, %v", len(datasets), err)
	}
	paper, err := store.AddPaper(ctx, Paper{Title: "Root paper"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("INSERT INTO paper_datasets(paper_id, dataset_id, role) VALUES(?, ?, 'generated')", paper.ID, datasets[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteDataset(ctx, datasets[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetDataset(ctx, datasets[0].ID); err == nil {
		t.Fatal("deleted dataset remains")
	}
	if _, err := store.GetPaper(ctx, paper.ID); err != nil {
		t.Fatalf("paper removed with its dataset: %v", err)
	}
	if run := syncRecords(t, store, "sequence-repositories", record); run.DatasetsAdded != 0 || run.Duplicates != 1 {
		t.Errorf("sync brought the deleted dataset back: %+v", run)
	}
	if err := store.DeleteDataset(ctx, datasets[0].ID); err == nil {
		t.Fatal("missing dataset deletion should fail")
	}
}

func TestPapersHarvestedHookHearsNewPapersOnly(t *testing.T) {
	store := openTestLibrary(t)
	var heard [][]string
	store.OnPapersHarvested(func(sourceID string, ids []string) {
		if sourceID != "pubmed" {
			t.Errorf("source = %q", sourceID)
		}
		heard = append(heard, ids)
	})
	record := harvested("1", Paper{Title: "A new paper"})
	first := syncRecords(t, store, "pubmed", record)
	syncRecords(t, store, "pubmed", record)
	if len(heard) != 1 || len(heard[0]) != 1 || heard[0][0] != first.Papers[0].ID {
		t.Errorf("heard = %v", heard)
	}
}

func TestDismissPaperKeepsRecordForDedupAndDropsStoredFiles(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	store, paper := fileFixture(t, storage)
	if _, err := store.db.Exec("UPDATE paper_curation SET awaiting_review = 1, proposed_screening = 'included', proposed_reason = 'looks relevant', notes = 'keep this note' WHERE paper_id = ?", paper.ID); err != nil {
		t.Fatal(err)
	}
	file, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "original.pdf", "pdf content"), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DismissPaper(ctx, paper.ID); err != nil {
		t.Fatal(err)
	}
	kept, err := store.GetPaper(ctx, paper.ID)
	if err != nil {
		t.Fatalf("dismissed paper is gone: %v", err)
	}
	if kept.Screening != "irrelevant" || kept.ReviewedAt == "" || kept.AwaitingReview || kept.ProposedScreening != "" || kept.Notes != "keep this note" {
		t.Fatalf("dismissed paper = %+v", kept)
	}
	if kept.Fulltext == "stored" {
		t.Fatalf("full text still counts as stored: %q", kept.Fulltext)
	}
	if _, err := os.Stat(filepath.Join(storage, filepath.FromSlash(file.LocalPath))); !os.IsNotExist(err) {
		t.Fatalf("stored copy remains: %v", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM resources WHERE id = ?", file.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("resource remains: %d, %v", count, err)
	}

	// The record is what keeps the paper from coming back.
	again, err := store.ImportPapers(ctx, []Paper{{Title: paper.Title}}, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if again.Count != 0 || len(again.Skipped) != 1 || again.Skipped[0].ExistingID != paper.ID {
		t.Fatalf("re-import = %+v", again)
	}

	// Listings for agents leave it out unless they ask for it; the desktop reads everything.
	papers, err := store.ListPapers(ctx)
	if err != nil || len(papers) != 1 {
		t.Fatalf("ListPapers = %d, %v", len(papers), err)
	}
	if shown, total := FilterPapers(papers, PaperFilter{}); total != 0 || len(shown) != 0 {
		t.Fatalf("default filter shows the irrelevant paper: %d", total)
	}
	if shown, total := FilterPapers(papers, PaperFilter{Screening: "irrelevant"}); total != 1 || len(shown) != 1 {
		t.Fatalf("explicit filter hides the irrelevant paper: %d", total)
	}
	if err := store.DismissPaper(ctx, "P-missing"); err == nil {
		t.Fatal("dismissing a missing paper should fail")
	}
}
