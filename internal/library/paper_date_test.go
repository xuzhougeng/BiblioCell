package library

import (
	"context"
	"testing"
)

func TestPaperDateCorrectionPersistsAndConflicts(t *testing.T) {
	ctx := context.Background()
	store, before := reviewFixture(t)
	after := before
	after.AddedAt = "2025-01-02T03:04:05.123Z"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &before, After: after}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("correct date: %v, %v", conflicts, err)
	}
	got, err := store.GetPaper(ctx, before.ID)
	if err != nil || got.AddedAt != after.AddedAt {
		t.Fatalf("date did not persist: %q, %v", got.AddedAt, err)
	}
	// An ordinary metadata update must retain the corrected date.
	journal := "Updated journal"
	if _, err := store.UpdatePaper(ctx, before.ID, PaperPatch{Journal: &journal}); err != nil {
		t.Fatal(err)
	}
	stale := before
	stale.AddedAt = "2024-01-02T00:00:00Z"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &before, After: stale}}); err != nil || len(conflicts) != 1 {
		t.Fatalf("expected date conflict: %v, %v", conflicts, err)
	}
	got, err = store.GetPaper(ctx, before.ID)
	if err != nil || got.AddedAt != after.AddedAt || got.Journal != journal {
		t.Fatalf("metadata update lost the date: %+v, %v", got, err)
	}
	invalid := got
	invalid.AddedAt = "invalid"
	invalid.Title = "must not be saved"
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &got, After: invalid}}); err == nil {
		t.Fatal("invalid date accepted")
	}
	again, err := store.GetPaper(ctx, before.ID)
	if err != nil || again.AddedAt != got.AddedAt || again.Title != got.Title {
		t.Fatalf("invalid edit was not rolled back: %+v, %v", again, err)
	}
}
