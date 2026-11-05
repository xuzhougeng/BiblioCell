package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

func TestTranslateTextUsesSavedPrompt(t *testing.T) {
	requests := make(chan string, 32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Content string } `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) < 2 {
			t.Error("invalid translation request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- body.Messages[0].Content
		if body.Messages[1].Content == "empty reply" {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":""}}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Root atlas"}}]}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	preferences, err := settings.Open(filepath.Join(dir, "settings.json"), dir, strings.Repeat("t", 64))
	if err != nil {
		t.Fatal(err)
	}
	config := preferences.Get()
	config.Models = []settings.ModelProfile{{ID: "translation", Name: "Translation", Config: agent.Config{Protocol: "openai", Model: "test-model", BaseURL: server.URL}}}
	config.TranslationPrompt = "Translate into English. Preserve scientific names."
	if err := preferences.Save(config); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "library.sqlite")
	store, err := library.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{settings: preferences, store: store}
	t.Cleanup(func() { service.store.Close() })
	if text, err := service.TranslateText("根图谱"); err != nil || text != "Root atlas" {
		t.Fatalf("TranslateText = %q, %v", text, err)
	}
	if prompt := <-requests; prompt != config.TranslationPrompt {
		t.Fatalf("system prompt = %q", prompt)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = library.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	service = &Library{settings: preferences, store: store}
	if text, err := service.CachedTranslation("根图谱"); err != nil || text != "Root atlas" {
		t.Fatalf("restored cache = %q, %v", text, err)
	}
	if _, err := service.TranslateText("根图谱"); err != nil || len(requests) != 0 {
		t.Fatalf("cache hit called the model: %v", err)
	}
	config.TranslationPrompt = "Translate into French."
	if err := preferences.Save(config); err != nil {
		t.Fatal(err)
	}
	if text, err := service.CachedTranslation("根图谱"); err != nil || text != "" {
		t.Fatalf("changed prompt reused stale text: %q, %v", text, err)
	}
	var group sync.WaitGroup
	for range 12 {
		group.Go(func() {
			if _, err := service.TranslateText("根图谱"); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if len(requests) != 1 {
		t.Fatalf("concurrent translations made %d requests, want 1", len(requests))
	}
	<-requests
	if text, err := service.CachedTranslation("different abstract"); err != nil || text != "" {
		t.Fatalf("changed source reused stale text: %q, %v", text, err)
	}
	for range 2 {
		if _, err := service.TranslateText("empty reply"); err == nil {
			t.Fatal("empty model result succeeded")
		}
	}
	if len(requests) != 2 {
		t.Fatal("failed translations must be retried")
	}
	if text, err := service.CachedTranslation("empty reply"); err != nil || text != "" {
		t.Fatalf("failed translation cached: %q, %v", text, err)
	}
}

func TestScreenPaperUsesSelectedModelWithoutChangingLibrary(t *testing.T) {
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		requests <- body.Model
		if len(body.Messages) < 2 || !strings.Contains(body.Messages[1].Content, "plant cells") {
			t.Error("missing screening requirement")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"verdict\":\"included\",\"reason\":\"Plant cell evidence\",\"excerpt\":\"Plant cell atlas evidence\",\"confidence\":0.8}"}}]}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	config := settings.Config{StoragePath: dir, FontSize: 15, MCPToken: strings.Repeat("a", 64), Models: []settings.ModelProfile{
		{ID: "default", Name: "Default", Config: agent.Config{Protocol: "openai", Model: "default-model", BaseURL: server.URL}},
		{ID: "batch", Name: "Batch", Config: agent.Config{Protocol: "openai", Model: "batch-model", BaseURL: server.URL}},
	}, Tasks: settings.TaskModels{Screen: "default"}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	preferences, err := settings.Open(path, dir, config.MCPToken)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	paper, err := store.AddPaper(ctx, library.Paper{Title: "Root atlas", Abstract: "Plant cell atlas evidence"})
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{store: store, settings: preferences}
	beforePaper, err := store.GetPaper(ctx, paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeSettings := preferences.Get()
	answer, err := service.ScreenPaperWithModel(paper.ID, "plant cells", "batch")
	if err != nil || answer.Verdict != "included" || answer.Basis != "abstract" {
		t.Fatalf("answer = %+v, err = %v", answer, err)
	}
	if got := <-requests; got != "batch-model" {
		t.Fatalf("selected model = %s", got)
	}
	// The suggestion is on record with the model that gave it, for the comparison view.
	if answer.ID == "" || answer.ProfileID != "batch" || answer.ProfileName != "Batch" || answer.Model != "batch-model" || answer.Excerpt != "Plant cell atlas evidence" || answer.Confidence == nil || *answer.Confidence != 0.8 || answer.Kind != "screen" {
		t.Fatalf("suggestion not recorded: %+v", answer)
	}
	if _, err := service.ScreenPaperInRun(paper.ID, "plant cells", "batch", "run-1", "rerun"); err != nil {
		t.Fatal(err)
	}
	<-requests
	recorded, err := service.ListScreeningSuggestions(paper.ID)
	if err != nil || len(recorded) != 2 || recorded[0].RunID != "run-1" || recorded[0].Kind != "rerun" || recorded[1].ID != answer.ID {
		t.Fatalf("recorded = %+v, %v", recorded, err)
	}
	// Both answers came from the same model, so the report keeps the newer one; the run is listed on its own.
	report, err := service.CalibrationReport(library.CalibrationFilter{})
	if err != nil || report.Total != 1 || report.Compared != 0 || report.Pending != 1 || len(report.Runs) != 1 || report.Runs[0].RunID != "run-1" {
		t.Fatalf("report = %+v, %v", report, err)
	}
	if _, err := service.ScreenPaper(paper.ID, "plant cells"); err != nil {
		t.Fatal(err)
	}
	if got := <-requests; got != "default-model" {
		t.Fatalf("default model = %s", got)
	}
	if _, err := service.ScreenPaperWithModel(paper.ID, "plant cells", "deleted"); err == nil {
		t.Fatal("deleted model silently fell back")
	}
	after, err := store.GetPaper(ctx, paper.ID)
	if err != nil || !reflect.DeepEqual(after, beforePaper) {
		t.Fatalf("screening mutated paper: %v", err)
	}
	if !reflect.DeepEqual(beforeSettings, preferences.Get()) {
		t.Fatal("screening changed task bindings")
	}
	empty, err := store.AddPaper(ctx, library.Paper{Title: "Title only"})
	if err != nil {
		t.Fatal(err)
	}
	skipped, err := service.ScreenPaperWithModel(empty.ID, "plant cells", "batch")
	if err != nil || !skipped.Skipped {
		t.Fatalf("empty paper = %+v, %v", skipped, err)
	}
	if len(requests) != 0 {
		t.Fatal("empty paper made a model call")
	}
}

func TestJudgePaperInRunAsksTheQuestionAndRecordsTheTarget(t *testing.T) {
	prompts := make(chan [2]string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) < 2 {
			t.Error("invalid judge request")
			w.WriteHeader(400)
			return
		}
		prompts <- [2]string{body.Messages[0].Content, body.Messages[1].Content}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"verdict\":\"yes\",\"reason\":\"A review\",\"excerpt\":\"We review\",\"confidence\":0.9}"}}]}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	config := settings.Config{StoragePath: dir, FontSize: 15, MCPToken: strings.Repeat("a", 64), Models: []settings.ModelProfile{
		{ID: "default", Name: "Default", Config: agent.Config{Protocol: "openai", Model: "default-model", BaseURL: server.URL}},
	}, Tasks: settings.TaskModels{Screen: "default"}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	preferences, err := settings.Open(path, dir, config.MCPToken)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open(filepath.Join(dir, "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	paper, err := store.AddPaper(ctx, library.Paper{Title: "Root atlases", Abstract: "We review root atlases"})
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{store: store, settings: preferences}

	// A preset is looked up by id: the question and target come from it.
	answer, err := service.JudgePaperInRun(paper.ID, library.JudgeTask{ID: "review"}, "", "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if answer.Verdict != "included" || answer.TaskID != "review" || answer.TargetKind != "articleType" || answer.TargetValue != "review" || answer.Basis != "abstract" || answer.RunID != "run-1" {
		t.Fatalf("answer = %+v", answer)
	}
	got := <-prompts
	if !strings.Contains(got[0], "判断问题") || !strings.Contains(got[1], "## 判断问题") || !strings.Contains(got[1], "综述") || strings.Contains(got[1], "筛选要求") {
		t.Fatalf("prompt = %q", got)
	}
	if after, _ := store.GetPaper(ctx, paper.ID); len(after.ArticleTypes) != 0 {
		t.Fatalf("judging changed the paper: %+v", after.ArticleTypes)
	}

	// A custom task names its own question and tag.
	custom, err := service.JudgePaperInRun(paper.ID, library.JudgeTask{Question: "是否涉及根？", Target: library.JudgeTarget{Kind: "tag", Value: "根"}}, "", "run-2")
	if err != nil {
		t.Fatal(err)
	}
	if custom.TaskID != "custom" || custom.TargetKind != "tag" || custom.TargetValue != "根" || custom.Requirement != "是否涉及根？" {
		t.Fatalf("custom = %+v", custom)
	}
	<-prompts
	if _, err := service.JudgePaperInRun(paper.ID, library.JudgeTask{Question: "q", Target: library.JudgeTarget{Kind: "tag"}}, "", ""); err == nil {
		t.Fatal("a tag task without a tag ran")
	}

	result, err := service.AdoptJudgements([]string{answer.ID, custom.ID})
	if err != nil || result.Adopted != 2 {
		t.Fatalf("adopt = %+v, %v", result, err)
	}
	after, _ := store.GetPaper(ctx, paper.ID)
	if len(after.ArticleTypes) != 1 || after.ArticleTypes[0] != "综述" || len(after.Tags) != 1 || after.Tags[0] != "根" {
		t.Fatalf("adopted paper = %+v", after)
	}
	// The calibration view compares screening suggestions only.
	report, err := service.CalibrationReport(library.CalibrationFilter{})
	if err != nil || report.Total != 0 {
		t.Fatalf("report = %+v, %v", report, err)
	}
}
