package library

import (
	"context"
	"testing"
)

func TestPaperAddEditAndPreserveMCPRecords(t *testing.T) {
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()
	year := 2026

	added, err := store.AddPaper(ctx, Paper{
		Title: "A test paper", DOI: "https://doi.org/10.1000/Test", Year: &year,
		Authors: "Researcher A; Researcher B",
		Species: []SpeciesEntry{
			{Name: "Arabidopsis thaliana", Label: "拟南芥"},
			{Name: "Oryza sativa", Label: "水稻"},
		},
		Organs:       []string{"root", "leaf"},
		Modalities:   []string{"scRNA-seq", "snRNA-seq"},
		Platforms:    []string{"10x Genomics Chromium", "BD Rhapsody"},
		ArticleTypes: []string{"original", "review"},
		Source:       "MCP",
	})
	if err != nil {
		t.Fatalf("AddPaper() error = %v", err)
	}
	if added.ID == "" || added.DOI != "10.1000/test" || added.IsDemo {
		t.Errorf("unexpected added paper: %+v", added)
	}

	papers, err := store.ListPapers(ctx)
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if len(papers) != 1 {
		t.Fatalf("paper count = %d, want 1", len(papers))
	}
	saved := papers[0]
	if saved.Authors != "Researcher A; Researcher B" {
		t.Errorf("Authors = %q", saved.Authors)
	}
	if len(saved.Species) != 2 || saved.Species[0] != (SpeciesEntry{Name: "Arabidopsis thaliana", Label: "拟南芥"}) ||
		saved.Species[1] != (SpeciesEntry{Name: "Oryza sativa", Label: "水稻"}) {
		t.Errorf("Species = %+v", saved.Species)
	}
	if len(saved.Organs) != 2 || saved.Organs[0] != "root" || saved.Organs[1] != "leaf" {
		t.Errorf("Organs = %+v", saved.Organs)
	}
	if len(saved.Modalities) != 2 || saved.Modalities[0] != "scRNA-seq" || saved.Modalities[1] != "snRNA-seq" {
		t.Errorf("Modalities = %+v", saved.Modalities)
	}
	if len(saved.Platforms) != 2 || saved.Platforms[0] != "10x Genomics Chromium" || saved.Platforms[1] != "BD Rhapsody" {
		t.Errorf("Platforms = %+v", saved.Platforms)
	}
	if len(saved.ArticleTypes) != 2 || saved.ArticleTypes[0] != "原创研究" || saved.ArticleTypes[1] != "综述" {
		t.Errorf("ArticleTypes = %+v", saved.ArticleTypes)
	}

	// The legacy single-value columns keep carrying the first entry so
	// hand-written SQL against the published schema stays meaningful.
	var speciesText, speciesLabel, articleType string
	if err := store.db.QueryRow("SELECT species_text, species_label, article_type FROM papers WHERE id = ?", saved.ID).
		Scan(&speciesText, &speciesLabel, &articleType); err != nil {
		t.Fatalf("read legacy columns: %v", err)
	}
	if speciesText != "Arabidopsis thaliana" || speciesLabel != "拟南芥" || articleType != "original" {
		t.Errorf("legacy columns = %q / %q / %q", speciesText, speciesLabel, articleType)
	}

	papers[0].Notes = "keep this note"
	papers[0].Tags = []string{"root", "priority"}
	papers[0].Starred = true
	if err := store.SavePapers(ctx, papers); err != nil {
		t.Fatalf("SavePapers() error = %v", err)
	}
	if _, err := store.AddPaper(ctx, Paper{Title: "Added outside the UI", Source: "MCP"}); err != nil {
		t.Fatalf("AddPaper() outside UI error = %v", err)
	}
	if err := store.SavePapers(ctx, papers); err != nil {
		t.Fatalf("SavePapers() after MCP insert error = %v", err)
	}

	papers, err = store.ListPapers(ctx)
	if err != nil {
		t.Fatalf("ListPapers() after edits error = %v", err)
	}
	if len(papers) != 2 {
		t.Fatalf("paper count = %d, want 2; MCP record must be preserved", len(papers))
	}
	var edited Paper
	for _, paper := range papers {
		if paper.ID == added.ID {
			edited = paper
		}
	}
	if edited.Notes != "keep this note" || !edited.Starred || len(edited.Tags) != 2 {
		t.Errorf("paper edits were not preserved: %+v", edited)
	}

	if _, err := store.AddPaper(ctx, Paper{Title: "Duplicate", DOI: "10.1000/test"}); err == nil {
		t.Fatal("AddPaper() with duplicate DOI succeeded, want error")
	}
}

func TestPaperRejectsInvalidArticleType(t *testing.T) {
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	if _, err := store.AddPaper(context.Background(), Paper{
		Title:        "Bad type",
		ArticleTypes: []string{"mystery"},
	}); err == nil {
		t.Fatal("AddPaper() with an unknown article type succeeded, want error")
	}
}

func TestUpdatePaperReplacesMultiValueLists(t *testing.T) {
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx := context.Background()

	added, err := store.AddPaper(ctx, Paper{
		Title: "Patch me", Platforms: []string{"10x Genomics Chromium"},
		Organs: []string{"root"}, ArticleTypes: []string{"original"},
	})
	if err != nil {
		t.Fatalf("AddPaper() error = %v", err)
	}
	patched, err := store.UpdatePaper(ctx, added.ID, PaperPatch{
		Platforms:    &[]string{"BD Rhapsody", "Microwell-seq"},
		Organs:       &[]string{"root", "leaf"},
		ArticleTypes: &[]string{"review"},
	})
	if err != nil {
		t.Fatalf("UpdatePaper() error = %v", err)
	}
	if len(patched.Platforms) != 2 || patched.Platforms[0] != "BD Rhapsody" || patched.Platforms[1] != "Microwell-seq" {
		t.Errorf("patched Platforms = %+v", patched.Platforms)
	}
	if len(patched.Organs) != 2 || patched.Organs[1] != "leaf" {
		t.Errorf("patched Organs = %+v", patched.Organs)
	}
	if len(patched.Species) != 0 {
		t.Errorf("patched Species = %+v, want untouched empty list", patched.Species)
	}
	// The canonical patch value round-trips as its Chinese display label.
	reread, err := store.GetPaper(ctx, added.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	if len(reread.ArticleTypes) != 1 || reread.ArticleTypes[0] != "综述" {
		t.Errorf("reread ArticleTypes = %+v", reread.ArticleTypes)
	}
}
