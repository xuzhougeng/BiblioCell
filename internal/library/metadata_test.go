package library

import (
	"context"
	"reflect"
	"testing"
)

func metadataFixture(t *testing.T) (*Store, Paper, MetadataProposal) {
	t.Helper()
	ctx := context.Background()
	store, paper := fileFixture(t, t.TempDir())
	file, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "fulltext.txt", "Libraries were prepared with 10x Genomics Chromium.\nData are available under GSE123."), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	paper, err = store.GetPaper(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	return store, paper, MetadataProposal{PaperID: paper.ID, FileID: file.ID, Claims: []MetadataClaim{
		{Field: "platforms", Values: []string{"10x Genomics Chromium"}, Excerpt: "Libraries were prepared with 10x Genomics Chromium."},
		{Field: "dataset", Repository: "geo", Accession: "GSE123", Role: "generated", Excerpt: "Data are available under GSE123."},
	}}
}

func TestApplyPaperMetadataPreservesCurationAndRecordsEvidence(t *testing.T) {
	ctx := context.Background()
	store, before, proposal := metadataFixture(t)
	curated := before
	curated.Screening, curated.Notes, curated.Source = "included", "My notes", "CLI"
	curated.Species = []SpeciesEntry{{Name: "Arabidopsis thaliana", Label: "拟南芥"}}
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &before, After: curated}}); err != nil {
		t.Fatal(err)
	}
	before, _ = store.GetPaper(ctx, before.ID)
	for i := 0; i < 2; i++ {
		if err := store.ApplyPaperMetadata(ctx, before, proposal); err != nil {
			t.Fatal(err)
		}
	}
	got, err := store.GetPaper(ctx, before.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Platforms, []string{"10x Genomics Chromium"}) || got.Screening != before.Screening || got.ReviewedAt != before.ReviewedAt || got.AddedAt != before.AddedAt || got.Source != "CLI" || got.Notes != "My notes" || !reflect.DeepEqual(got.Species, before.Species) {
		t.Fatalf("unexpected metadata or curation change: %+v", got)
	}
	evidence, err := store.ListEvidence(ctx, before.ID, "")
	if err != nil || len(evidence) != 2 {
		t.Fatalf("evidence = %+v, %v", evidence, err)
	}
	for _, e := range evidence {
		if e.FileID != proposal.FileID || e.Excerpt == "" || e.Status != "unreviewed" {
			t.Fatalf("invalid evidence: %+v", e)
		}
	}
	datasets, err := store.ListDatasets(ctx)
	if err != nil || len(datasets) != 1 {
		t.Fatalf("datasets = %+v, %v", datasets, err)
	}
	d := datasets[0]
	if len(d.PaperLinks) != 1 || d.PaperLinks[0].Verified || d.Repositories[0].Verified || !d.AwaitingReview {
		t.Fatalf("AI claims should need review: %+v", d)
	}
	if err := store.VerifyDatasetLink(ctx, before.ID, d.ID, "generated", true); err != nil {
		t.Fatal(err)
	}
	if err := store.VerifyDatasetRepository(ctx, d.ID, d.Repositories[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if err := store.ApplyPaperMetadata(ctx, before, proposal); err != nil {
		t.Fatal(err)
	}
	datasets, _ = store.ListDatasets(ctx)
	if !datasets[0].PaperLinks[0].Verified || !datasets[0].Repositories[0].Verified {
		t.Fatal("existing verification was reset")
	}
}

func TestApplyPaperMetadataRejectsStaleEditsAndInvalidEvidence(t *testing.T) {
	ctx := context.Background()
	for _, scenario := range []string{"conflict", "quote", "file", "rollback"} {
		t.Run(scenario, func(t *testing.T) {
			store, before, proposal := metadataFixture(t)
			want := before.Platforms
			switch scenario {
			case "conflict":
				changed := before
				changed.Platforms = []string{"Manual platform"}
				want = changed.Platforms
				if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &before, After: changed}}); err != nil {
					t.Fatal(err)
				}
			case "quote":
				proposal.Claims[0].Excerpt = "This quotation was invented."
			case "file":
				proposal.FileID = "another-file"
			case "rollback":
				if _, err := store.db.Exec("CREATE TRIGGER reject_dataset BEFORE INSERT ON datasets BEGIN SELECT RAISE(ABORT, 'test failure'); END"); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.ApplyPaperMetadata(ctx, before, proposal); err == nil {
				t.Fatal("invalid/stale apply succeeded")
			}
			got, err := store.GetPaper(ctx, before.ID)
			if err != nil || !reflect.DeepEqual(got.Platforms, want) {
				t.Fatalf("partial paper write: %+v, %v", got, err)
			}
			evidence, _ := store.ListEvidence(ctx, before.ID, "")
			datasets, _ := store.ListDatasets(ctx)
			if len(evidence) != 0 || len(datasets) != 0 {
				t.Fatal("partial evidence or dataset write")
			}
		})
	}
}
