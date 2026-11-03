package library

import (
	"context"
	"testing"
)

func TestNormalizeJudgeTask(t *testing.T) {
	for _, preset := range JudgePresets() {
		if _, err := NormalizeJudgeTask(preset); err != nil {
			t.Errorf("preset %s: %v", preset.ID, err)
		}
	}
	task, err := NormalizeJudgeTask(JudgeTask{Question: " 是否用了 10x 平台 ", Target: JudgeTarget{Value: " 10x "}})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "custom" || task.Basis != "abstract" || task.Target.Kind != "tag" || task.Target.Value != "10x" || task.Name != "是否用了 10x 平台" {
		t.Fatalf("normalized = %+v", task)
	}
	if _, err := NormalizeJudgeTask(JudgeTask{Question: "q", Target: JudgeTarget{Kind: "tag"}}); err == nil {
		t.Error("a tag task without a tag was accepted")
	}
	if _, err := NormalizeJudgeTask(JudgeTask{Target: JudgeTarget{Kind: "articleType", Value: "review"}}); err == nil {
		t.Error("an article type task without a question was accepted")
	}
	if _, err := NormalizeJudgeTask(JudgeTask{Question: "q", Target: JudgeTarget{Kind: "articleType", Value: "poem"}}); err == nil {
		t.Error("an unknown article type was accepted")
	}
	if task, err := NormalizeJudgeTask(JudgeTask{Question: "q", Target: JudgeTarget{Kind: "articleType", Value: "综述"}}); err != nil || task.Target.Value != "review" {
		t.Errorf("display label not canonicalised: %+v, %v", task, err)
	}
}

func TestAdoptJudgementsMarksOnlyYesAnswers(t *testing.T) {
	store, _ := reviewFixture(t)
	ctx := context.Background()
	review, err := store.AddPaper(ctx, Paper{Title: "A review", Abstract: "We review"})
	if err != nil {
		t.Fatal(err)
	}
	original, err := store.AddPaper(ctx, Paper{Title: "An atlas", Abstract: "We sequence", ArticleTypes: []string{"original"}, Tags: []string{"kept"}})
	if err != nil {
		t.Fatal(err)
	}
	// A person's decision must not block adoption: adopting is their act too.
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &review, After: func() Paper { p := review; p.Screening = "included"; return p }()}}); err != nil {
		t.Fatal(err)
	}
	yes, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: review.ID, Requirement: "review?", Verdict: "included", TaskID: "review", TargetKind: "articleType", TargetValue: "review"})
	if err != nil {
		t.Fatal(err)
	}
	no, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: original.ID, Requirement: "review?", Verdict: "excluded", TaskID: "review", TargetKind: "articleType", TargetValue: "review"})
	if err != nil {
		t.Fatal(err)
	}
	tag, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: original.ID, Requirement: "sc?", Verdict: "included", TaskID: "custom", TargetKind: "tag", TargetValue: "单细胞"})
	if err != nil {
		t.Fatal(err)
	}
	screen, err := store.AddScreeningSuggestion(ctx, ScreeningSuggestion{PaperID: original.ID, Requirement: "plants", Verdict: "included"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.AdoptJudgements(ctx, []string{yes.ID, no.ID, tag.ID, tag.ID, screen.ID, "SG-missing"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Adopted != 2 || result.Skipped != 3 {
		t.Fatalf("result = %+v", result)
	}
	got, _ := store.GetPaper(ctx, review.ID)
	if len(got.ArticleTypes) != 1 || got.ArticleTypes[0] != "综述" || got.Screening != "included" {
		t.Fatalf("review paper = %+v", got)
	}
	got, _ = store.GetPaper(ctx, original.ID)
	if len(got.ArticleTypes) != 1 || len(got.Tags) != 2 || got.Tags[1] != "单细胞" {
		t.Fatalf("original paper = %+v", got)
	}
	// Adopting again changes nothing.
	if result, err := store.AdoptJudgements(ctx, []string{yes.ID, tag.ID}); err != nil || result.Adopted != 0 || result.Skipped != 2 {
		t.Fatalf("second adoption = %+v, %v", result, err)
	}
}
