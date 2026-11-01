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
