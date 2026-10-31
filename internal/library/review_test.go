package library

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func reviewFixture(t *testing.T) (*Store, Paper) {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	paper, err := store.AddPaper(context.Background(), Paper{Title: "Reviewed paper", Authors: "A;B", Source: "MCP"})
	if err != nil {
		t.Fatal(err)
	}
	return store, paper
}

func curation(t *testing.T, store *Store, id string) (status, method string, reviewed sql.NullString) {
	t.Helper()
	if err := store.db.QueryRow("SELECT screening_status, screening_method, screening_reviewed_at FROM paper_curation WHERE paper_id = ?", id).
		Scan(&status, &method, &reviewed); err != nil {
		t.Fatal(err)
	}
	return status, method, reviewed
}

func TestSavePaperEditsKeepsWhatAgentsWrote(t *testing.T) {
	ctx := context.Background()
	store, seen := reviewFixture(t)
	other, err := store.AddPaper(ctx, Paper{Title: "Untouched paper", Source: "MCP"})
	if err != nil {
		t.Fatal(err)
	}

	journal, organs, notes := "Agent Journal", []string{"root", "stele"}, "agent extraction"
	if _, err := store.UpdatePaper(ctx, seen.ID, PaperPatch{Journal: &journal, Organs: &organs, Notes: &notes}); err != nil {
		t.Fatal(err)
	}
	edited := seen
	edited.Starred = true
	conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &seen, After: edited}})
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("SavePaperEdits() = %v, %v", conflicts, err)
	}
	got, err := store.GetPaper(ctx, seen.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Journal != journal || len(got.Organs) != 2 || got.Notes != notes || !got.Starred {
		t.Fatalf("merged paper = %+v", got)
	}
	if again, err := store.GetPaper(ctx, other.ID); err != nil || again.UpdatedAt != other.UpdatedAt {
		t.Errorf("a save rewrote a paper nobody edited: %q, want %q, %v", again.UpdatedAt, other.UpdatedAt, err)
	}

	unchanged := got
	conflicts, err = store.SavePaperEdits(ctx, []PaperEdit{{Before: &got, After: unchanged}})
	if err != nil || len(conflicts) != 0 {
		t.Fatalf("no-op save = %v, %v", conflicts, err)
	}
	if again, err := store.GetPaper(ctx, seen.ID); err != nil || again.UpdatedAt != got.UpdatedAt {
		t.Errorf("a no-op save bumped updated_at: %v", err)
	}
}

func TestSavePaperEditsReportsSameFieldConflict(t *testing.T) {
	ctx := context.Background()
	store, seen := reviewFixture(t)
	notes := "written by the agent"
	if _, err := store.UpdatePaper(ctx, seen.ID, PaperPatch{Notes: &notes}); err != nil {
		t.Fatal(err)
	}

	edited := seen
	edited.Notes, edited.Starred = "written by the person", true
	conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &seen, After: edited}})
	if err != nil || len(conflicts) != 1 || conflicts[0] != seen.Title {
		t.Fatalf("SavePaperEdits() = %v, %v", conflicts, err)
	}
	got, err := store.GetPaper(ctx, seen.ID)
	if err != nil || got.Notes != notes || got.Starred {
		t.Fatalf("a conflicting edit was partly saved: %+v, %v", got, err)
	}

	// Both sides writing the same value is agreement, not a conflict.
	edited.Notes = notes
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &seen, After: edited}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("matching edit = %v, %v", conflicts, err)
	}

	// Stored formatting differs from what was typed; a later edit must not read that as a conflict.
	seenAuthors, err := store.GetPaper(ctx, seen.ID)
	if err != nil {
		t.Fatal(err)
	}
	typed := seenAuthors
	typed.Authors = "C;D"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &seenAuthors, After: typed}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("first authors edit = %v, %v", conflicts, err)
	}
	retyped := typed
	retyped.Authors = "E;F"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &typed, After: retyped}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("second authors edit = %v, %v", conflicts, err)
	}
}

func TestScreeningRecordsThePersonsDecision(t *testing.T) {
	ctx := context.Background()
	store, seen := reviewFixture(t)
	if status, method, reviewed := curation(t, store, seen.ID); status != "pending" || method != "unreviewed" || reviewed.Valid {
		t.Fatalf("agent import = %s %s %v", status, method, reviewed)
	}
	proposal := "included"
	proposed, err := store.UpdatePaper(ctx, seen.ID, PaperPatch{ProposedScreening: &proposal})
	if err != nil {
		t.Fatal(err)
	}
	if status, method, _ := curation(t, store, seen.ID); status != "pending" || method != "unreviewed" {
		t.Fatalf("a proposal changed the decision: %s %s", status, method)
	}

	decided := proposed
	decided.Screening = "included"
	decided.ReviewedAt = "2000-01-01" // The window cannot choose this time.
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &proposed, After: decided}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetPaper(ctx, seen.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status, method, reviewed := curation(t, store, seen.ID); status != "included" || method != "human" || !reviewed.Valid || reviewed.String == "2000-01-01" {
		t.Fatalf("human decision = %s %s %v", status, method, reviewed)
	}
	if got.ReviewedAt == "" || got.ProposedScreening != "" {
		t.Fatalf("decision did not settle the proposal: %+v", got)
	}

	undone := got
	undone.Screening = "pending"
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &got, After: undone}}); err != nil {
		t.Fatal(err)
	}
	if status, method, reviewed := curation(t, store, seen.ID); status != "pending" || method != "unreviewed" || reviewed.Valid {
		t.Fatalf("reset = %s %s %v", status, method, reviewed)
	}
	journal := "Editable again"
	if _, err := store.UpdatePaper(ctx, seen.ID, PaperPatch{Journal: &journal}); err != nil {
		t.Errorf("reset paper stayed locked: %v", err)
	}

	created := Paper{ID: "P-human", Title: "Typed by a person", Screening: "uncertain"}
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{After: created}}); err != nil {
		t.Fatal(err)
	}
	if status, method, reviewed := curation(t, store, "P-human"); status != "uncertain" || method != "human" || !reviewed.Valid {
		t.Fatalf("new paper = %s %s %v", status, method, reviewed)
	}
}

func TestAgentWritesCannotCarryADecision(t *testing.T) {
	ctx := context.Background()
	store, _ := reviewFixture(t)
	for _, paper := range []Paper{
		{Title: "Claims a decision", Screening: "included"},
		{Title: "Claims a review", Screening: "excluded", ScreeningReason: "x", ReviewedAt: "2026-01-01"},
	} {
		if _, err := store.AddPaper(ctx, paper); err == nil {
			t.Errorf("AddPaper(%+v) succeeded", paper)
		}
	}
	if imported, err := store.AddPaper(ctx, Paper{Title: "Forged stamp", ReviewedAt: "2026-01-01"}); err != nil || imported.ReviewedAt != "" {
		t.Errorf("import kept a review stamp: %+v, %v", imported, err)
	}
	invalid := "pending"
	seen, _ := store.ListPapers(ctx)
	if _, err := store.UpdatePaper(ctx, seen[0].ID, PaperPatch{ProposedScreening: &invalid}); err == nil {
		t.Error("pending accepted as a proposal")
	}
}

func TestReviewBackfillSeparatesAgentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Rebuild the v5 shape: no proposal columns, every row labelled human.
	if _, err := store.db.Exec(`
		INSERT INTO papers(id, title, source, first_seen_at, updated_at) VALUES
			('agent', 'Agent decided', 'MCP', '2026-01-01', '2026-01-01'),
			('cli', 'CLI excluded', 'CLI', '2026-01-01', '2026-01-01'),
			('person', 'Person decided', '手动添加', '2026-01-01', '2026-01-01'),
			('queued', 'Still pending', 'PubMed', '2026-01-01', '2026-01-01');
		UPDATE paper_curation SET screening_method = 'human', updated_at = '2026-01-02';
		UPDATE paper_curation SET screening_status = 'included' WHERE paper_id IN ('agent', 'person');
		UPDATE paper_curation SET screening_status = 'excluded', screening_reason = 'off topic' WHERE paper_id = 'cli';
		DROP VIEW v_work_queue;
		DROP VIEW v_paper_overview;
		ALTER TABLE paper_curation DROP COLUMN proposed_screening;
		ALTER TABLE paper_curation DROP COLUMN proposed_reason;
		PRAGMA user_version = 5;`); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("Open() v5 database: %v", err)
	}
	defer store.Close()
	for _, want := range []struct{ id, status, method, proposed, reason string }{
		{"agent", "pending", "unreviewed", "included", ""},
		{"cli", "pending", "unreviewed", "excluded", "off topic"},
		{"person", "included", "human", "", ""},
		{"queued", "pending", "unreviewed", "", ""},
	} {
		paper, err := store.GetPaper(context.Background(), want.id)
		if err != nil {
			t.Fatal(err)
		}
		status, method, reviewed := curation(t, store, want.id)
		if status != want.status || method != want.method || paper.ProposedScreening != want.proposed || paper.ProposedReason != want.reason {
			t.Errorf("%s = %s %s, proposal %q %q", want.id, status, method, paper.ProposedScreening, paper.ProposedReason)
		}
		if reviewed.Valid != (want.method == "human") {
			t.Errorf("%s reviewed at %v", want.id, reviewed)
		}
	}
	if status, err := store.Status(); err != nil || status.SchemaVersion != SchemaVersion {
		t.Errorf("Status() = %+v, %v", status, err)
	}
}
