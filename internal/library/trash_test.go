package library

import (
	"context"
	"testing"
)

func TestTrashDedupAndRestoreAcrossSources(t *testing.T) {
	ctx := context.Background()
	s := openTestLibrary(t)
	paper, err := s.AddPaper(ctx, Paper{Title: "Root atlas for trash", DOI: "10.1000/trash", PMID: "1234567", Notes: "keep notes"})
	if err != nil {
		t.Fatal(err)
	}
	input := DatasetInput{Title: "Root dataset", Repositories: []RepositoryInput{{Repository: "geo", Accession: "GSE987"}}, Papers: []PaperLinkInput{{PaperID: paper.ID, EvidenceText: "reported in paper"}}}
	added, err := s.AddDataset(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	id := added.Dataset.ID
	if err := s.DeletePaper(ctx, paper.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteDataset(ctx, id); err != nil {
		t.Fatal(err)
	}
	entries, err := s.ListTrash(ctx)
	if err != nil || len(entries) != 2 {
		t.Fatalf("trash: %+v %v", entries, err)
	}
	for _, e := range entries {
		if e.DeletedAt == "" || e.Identifier == "" {
			t.Fatalf("missing trash metadata: %+v", e)
		}
	}
	papers, _ := s.ListPapers(ctx)
	datasets, _ := s.ListDatasets(ctx)
	if len(papers) != 0 || len(datasets) != 0 {
		t.Fatal("trash visible in active lists")
	}
	status, err := s.Status()
	if err != nil || status.PaperCount != 0 || status.DatasetCount != 0 {
		t.Fatalf("status: %+v %v", status, err)
	}
	// These source records have never been seen. Dedup must use retained identity,
	// rather than relying only on a source's previous sightings.
	for _, offered := range []Paper{{Title: "Changed title", DOI: paper.DOI}, {Title: "Another title", PMID: paper.PMID}, {Title: paper.Title}} {
		run := syncRecords(t, s, "pubmed", harvested(offered.Title, offered))
		if run.PapersAdded != 0 || run.Duplicates != 1 {
			t.Fatalf("source resurrected paper: %+v", run)
		}
	}
	record := harvestRecord{ExternalID: "GSE987", Payload: map[string]string{"accession": "GSE987"}, Dataset: repositoryDataset("geo", "study", "GSE987", "", "Changed dataset title", "", nil)}
	if run := syncRecords(t, s, "sequence-repositories", record); run.DatasetsAdded != 0 || run.Duplicates != 1 {
		t.Fatalf("source resurrected dataset: %+v", run)
	}
	imported, err := s.ImportPapers(ctx, []Paper{{Title: "Imported title", DOI: paper.DOI}}, "", false)
	if err != nil || imported.Count != 0 || len(imported.Skipped) != 1 {
		t.Fatalf("import: %+v %v", imported, err)
	}
	if _, err := s.AddDataset(ctx, input); err == nil {
		t.Fatal("manual add accepted a trashed accession")
	}
	stale := paper
	stale.Notes = "stale edit"
	if _, err := s.SavePaperEdits(ctx, []PaperEdit{{Before: &paper, After: stale}}); err == nil {
		t.Fatal("accepted stale edit")
	}
	// A new UI-generated ID cannot bypass the recycle bin either.
	stale.ID = "P-new-id"
	if _, err := s.SavePaperEdits(ctx, []PaperEdit{{After: stale}}); err == nil {
		t.Fatal("new ID bypassed paper tombstone")
	}
	staleDataset := added.Dataset
	staleDataset.ID = "DS-new-id"
	if _, err := s.SaveDatasetEdits(ctx, []DatasetEdit{{After: staleDataset}}); err == nil {
		t.Fatal("new ID bypassed dataset tombstone")
	}
	if err := s.RestoreTrash(ctx, "dataset", id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPaper(ctx, paper.ID); err == nil {
		t.Fatal("dataset restore also restored paper")
	}
	if err := s.RestoreTrash(ctx, "paper", paper.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := s.GetPaper(ctx, paper.ID)
	if err != nil || restored.Notes != paper.Notes {
		t.Fatalf("restored paper: %+v %v", restored, err)
	}
	ds, err := s.GetDataset(ctx, id)
	if err != nil || len(ds.PaperLinks) != 1 || ds.PaperLinks[0].PaperID != paper.ID {
		t.Fatalf("restored links: %+v %v", ds, err)
	}
	if entries, err := s.ListTrash(ctx); err != nil || len(entries) != 0 {
		t.Fatalf("trash after restore: %+v %v", entries, err)
	}
	if err := s.RestoreTrash(ctx, "paper", paper.ID); err == nil {
		t.Fatal("restored twice")
	}
	if err := s.RestoreTrash(ctx, "invalid", id); err == nil {
		t.Fatal("accepted invalid kind")
	}
}
