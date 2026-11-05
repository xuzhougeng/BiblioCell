package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

func TestSourceCheckLeavesAProposalOnHarvestedPapers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Messages) < 2 || !strings.Contains(body.Messages[1].Content, "only roots") {
			t.Error("the check prompt did not reach the model")
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"{\"verdict\":\"excluded\",\"reason\":\"Leaf tissue only\",\"excerpt\":\"\",\"confidence\":0.9}"}}]}`)
	}))
	defer server.Close()
	dir := t.TempDir()
	config := settings.Config{StoragePath: dir, FontSize: 15, MCPToken: strings.Repeat("a", 64), BatchScreenConcurrency: 2,
		Models:      []settings.ModelProfile{{ID: "m", Name: "M", Config: agent.Config{Protocol: "openai", Model: "m", BaseURL: server.URL}}},
		SourceCheck: settings.SourceCheck{Enabled: true, Prompt: "only roots"}}
	data, _ := json.Marshal(config)
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
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
	withAbstract, err := store.AddPaper(ctx, library.Paper{Title: "Leaf atlas", Abstract: "Leaf cells"})
	if err != nil {
		t.Fatal(err)
	}
	titleOnly, err := store.AddPaper(ctx, library.Paper{Title: "No abstract"})
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{store: store, settings: preferences}
	service.checkHarvested("pubmed", []string{withAbstract.ID, titleOnly.ID})
	deadline := time.Now().Add(10 * time.Second)
	for service.checks.count() > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := service.checks.count(); n != 0 {
		t.Fatalf("checks still pending: %d", n)
	}
	checked, err := store.GetPaper(ctx, withAbstract.ID)
	if err != nil {
		t.Fatal(err)
	}
	if checked.ProposedScreening != "excluded" || checked.ProposedReason != sourceCheckReason+"Leaf tissue only" || checked.Screening != "pending" {
		t.Errorf("checked paper = %q %q %q", checked.ProposedScreening, checked.ProposedReason, checked.Screening)
	}
	skipped, err := store.GetPaper(ctx, titleOnly.ID)
	if err != nil || skipped.ProposedScreening != "" {
		t.Errorf("title-only paper = %+v, %v", skipped.ProposedScreening, err)
	}

	// Switched off, nothing is asked.
	if err := preferences.Save(settings.Config{StoragePath: dir, FontSize: 15, MCPToken: config.MCPToken, Models: config.Models}); err != nil {
		t.Fatal(err)
	}
	service.checkHarvested("pubmed", []string{titleOnly.ID})
	if n := service.checks.count(); n != 0 {
		t.Errorf("disabled check started: %d", n)
	}
}

func TestSourceCheckNeedsAPromptWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	config := settings.Config{StoragePath: dir, FontSize: 15, SourceCheck: settings.SourceCheck{Enabled: true, Prompt: " "}}
	if err := settings.Validate(config); err == nil {
		t.Fatal("an enabled check without a prompt was accepted")
	}
}
