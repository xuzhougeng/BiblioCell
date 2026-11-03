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
	"testing"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

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
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"verdict\":\"included\",\"reason\":\"Plant cell evidence\"}"}}]}`)
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
