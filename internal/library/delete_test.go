package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDeletePaperCleansStoredCopyOnly(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(storage, filepath.FromSlash(file.LocalPath))); !os.IsNotExist(err) {
		t.Fatalf("stored copy remains: %v", err)
	}
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("original removed: %v", err)
	}
	var count int
	if err := store.db.QueryRow("SELECT count(*) FROM datasets WHERE id = ?", dataset.Dataset.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("dataset removed: %d, %v", count, err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM paper_datasets WHERE paper_id = ?", paper.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("link remains: %d, %v", count, err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM resources WHERE id = ?", file.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("resource remains: %d, %v", count, err)
	}
	if err := store.DeletePaper(ctx, paper.ID); err == nil {
		t.Fatal("missing paper deletion should fail")
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
