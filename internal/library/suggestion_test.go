package library

import (
	"context"
	"testing"
)

func TestScreeningSuggestionsAreRecordedAndCompared(t *testing.T) {
	ctx := context.Background()
	store, paper := reviewFixture(t)
	other, err := store.AddPaper(ctx, Paper{Title: "Other paper", Abstract: "roots"})
	if err != nil {
		t.Fatal(err)
	}
	confidence := 0.9
	first, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: paper.ID, Requirement: "plant roots", ProfileID: "fast", ProfileName: "Fast", Model: "m-fast",
		Protocol: "openai", Verdict: "excluded", Reason: "not plants", Basis: "abstract", Confidence: &confidence, Kind: "batch", RunID: "run1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == "" || first.CreatedAt == "" || first.Confidence == nil || *first.Confidence != 0.9 || first.Kind != "batch" {
		t.Fatalf("saved suggestion = %+v", first)
	}
	if _, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: paper.ID, Requirement: "x", Verdict: "maybe"}); err == nil {
		t.Fatal("invalid verdict accepted")
	}
	if _, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: "P-missing", Requirement: "x", Verdict: "included"}); err == nil {
		t.Fatal("unknown paper accepted")
	}
	if _, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: other.ID, Requirement: "plant roots", ProfileID: "fast", ProfileName: "Fast", Model: "m-fast", Verdict: "included", Kind: "batch", RunID: "run1"}); err != nil {
		t.Fatal(err)
	}

	// Recording a suggestion never touches the paper's own decision.
	if status, method, reviewed := curation(t, store, paper.ID); status != "pending" || method != "unreviewed" || reviewed.Valid {
		t.Fatalf("suggestion changed curation: %s %s %v", status, method, reviewed)
	}
	report, err := store.CalibrationReport(ctx, CalibrationFilter{})
	if err != nil || report.Total != 2 || report.Compared != 0 || report.Pending != 2 {
		t.Fatalf("report before decisions = %+v, %v", report, err)
	}

	// An agent proposal is no decision; a person's decision is.
	seen, _ := store.GetPaper(ctx, paper.ID)
	proposed := seen
	proposed.ProposedScreening, proposed.ProposedReason = "excluded", "agent says so"
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &seen, After: proposed}}); err != nil {
		t.Fatal(err)
	}
	report, err = store.CalibrationReport(ctx, CalibrationFilter{})
	if err != nil || report.Compared != 0 {
		t.Fatalf("agent proposal counted as a decision: %+v, %v", report, err)
	}
	seen, _ = store.GetPaper(ctx, paper.ID)
	decided := seen
	decided.Screening, decided.ScreeningReason = "included", ""
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &seen, After: decided}}); err != nil {
		t.Fatal(err)
	}
	report, err = store.CalibrationReport(ctx, CalibrationFilter{})
	if err != nil || report.Total != 2 || report.Compared != 1 || report.Agreed != 0 || report.Pending != 1 {
		t.Fatalf("report after a decision = %+v, %v", report, err)
	}
	if len(report.Mismatches) != 1 || report.Mismatches[0].PaperID != paper.ID || report.Mismatches[0].Decision != "included" || report.Mismatches[0].Title != "Reviewed paper" || report.Mismatches[0].DecidedAt == "" {
		t.Fatalf("mismatches = %+v", report.Mismatches)
	}
	if len(report.Runs) != 1 || report.Runs[0].RunID != "run1" || report.Runs[0].Total != 2 || report.Runs[0].Compared != 1 {
		t.Fatalf("runs = %+v", report.Runs)
	}

	// A rerun with another model is a new record beside the old one.
	if _, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: paper.ID, Requirement: "plant roots", ProfileID: "big", ProfileName: "Big", Model: "m-big", Verdict: "included", Kind: "rerun", RunID: "run2"}); err != nil {
		t.Fatal(err)
	}
	list, err := store.ListScreeningSuggestions(ctx, paper.ID)
	if err != nil || len(list) != 2 || list[0].RunID != "run2" || list[1].ID != first.ID {
		t.Fatalf("list = %+v, %v", list, err)
	}
	report, _ = store.CalibrationReport(ctx, CalibrationFilter{})
	if report.Compared != 2 || report.Agreed != 1 || len(report.ByModel) != 2 || len(report.Models) != 2 {
		t.Fatalf("report with two models = %+v", report)
	}
	byRun, _ := store.CalibrationReport(ctx, CalibrationFilter{RunID: "run2"})
	if byRun.Total != 1 || byRun.Agreed != 1 || byRun.Rate != 1 {
		t.Fatalf("run2 report = %+v", byRun)
	}

	// Deleting the paper takes its suggestions with it.
	if err := store.DeletePaper(ctx, paper.ID); err != nil {
		t.Fatal(err)
	}
	if list, err := store.ListScreeningSuggestions(ctx, ""); err != nil || len(list) != 1 || list[0].PaperID != other.ID {
		t.Fatalf("suggestions after delete = %+v, %v", list, err)
	}
}

func TestApplyPaperMetadataRecordsEvidencePageAndExtractor(t *testing.T) {
	ctx := context.Background()
	store, before, proposal := metadataFixture(t)
	proposal.Extractor = "fulltext-metadata:Fast/m-fast"
	if err := store.ApplyPaperMetadata(ctx, before, proposal); err != nil {
		t.Fatal(err)
	}
	evidence, err := store.ListEvidence(ctx, before.ID, "")
	if err != nil || len(evidence) != 2 {
		t.Fatalf("evidence = %+v, %v", evidence, err)
	}
	for _, e := range evidence {
		// A text file is one page, and the page is read back from the location.
		if e.Location != "page 1" || e.Page != 1 || e.Extractor != "fulltext-metadata:Fast/m-fast" {
			t.Fatalf("evidence without page or extractor: %+v", e)
		}
	}
	_, pages, err := store.PaperFulltextPages(ctx, before.ID)
	if err != nil || len(pages) != 1 {
		t.Fatalf("pages = %d, %v", len(pages), err)
	}
}
