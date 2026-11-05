package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

func TestExtractPaperMetadataReadsFulltextAndWaitsForApply(t *testing.T) {
	const excerpt = "Libraries were prepared using Stereo-seq."
	called := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called++
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		if request.Model != "chat-model" || len(request.Messages) < 2 || !strings.Contains(request.Messages[len(request.Messages)-1].Content, excerpt) {
			t.Errorf("wrong model or missing full text: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": `{"claims":[{"field":"platforms","values":["Stereo-seq"],"excerpt":"` + excerpt + `"}]}`}, "finish_reason": "stop"}}})
	}))
	defer server.Close()
	dir := t.TempDir()
	store, err := library.Open(filepath.Join(dir, "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	paper, err := store.AddPaper(context.Background(), library.Paper{Title: "Test paper", Abstract: "This abstract must not substitute for full text."})
	if err != nil {
		t.Fatal(err)
	}
	config, err := settings.Open(filepath.Join(dir, "settings.json"), dir, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	value := config.Get()
	value.Models = []settings.ModelProfile{{ID: "chat", Name: "Chat", Config: agent.Config{Protocol: "openai", BaseURL: server.URL, Model: "chat-model"}}}
	if err := config.Save(value); err != nil {
		t.Fatal(err)
	}
	service := &Library{store: store, settings: config}
	if _, err := service.ExtractPaperMetadata(paper.ID); err == nil || called != 0 {
		t.Fatal("missing full text fell back to abstract")
	}
	source := filepath.Join(dir, "source.txt")
	if err := os.WriteFile(source, []byte(excerpt), 0600); err != nil {
		t.Fatal(err)
	}
	file, err := store.ImportPaperFile(context.Background(), paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := service.ExtractPaperMetadata(paper.ID)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.FileID != file.ID || proposal.PaperID != paper.ID || len(proposal.Claims) != 1 || called != 1 {
		t.Fatalf("bad proposal: %+v", proposal)
	}
	before, _ := store.GetPaper(context.Background(), paper.ID)
	if len(before.Platforms) != 0 {
		t.Fatal("extraction wrote without application")
	}
	if err := service.ApplyPaperMetadata(before, proposal); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetPaper(context.Background(), paper.ID)
	if len(got.Platforms) != 1 || got.Platforms[0] != "Stereo-seq" {
		t.Fatalf("not applied: %+v", got)
	}
}
