package library

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type countingHarvester struct {
	fakeHarvester
	count int
}

func (h countingHarvester) Count(context.Context, SourceConfig) (int, error) { return h.count, nil }

func TestQueryChangesAreVersionedAndAttributed(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	before, err := store.SourceStatus(ctx, "crossref")
	if err != nil || before.QueryVersion != "1" || before.QueryChange != nil {
		t.Fatalf("initial status = %+v, %v", before, err)
	}

	// Toggling a source without touching its query is not a query change.
	if _, err := store.SaveSourceConfig(ctx, SourceConfig{ID: "crossref", Enabled: false, Query: before.Query, MaxResults: 20}); err != nil {
		t.Fatal(err)
	}
	if history, _ := store.SourceQueryHistory(ctx, "crossref"); len(history) != 0 {
		t.Fatalf("history after toggle = %+v", history)
	}
	human, err := store.SaveSourceConfig(ctx, SourceConfig{ID: "crossref", Enabled: false, Query: "plant scRNA-seq", MaxResults: 20})
	if err != nil || human.QueryVersion != "2" || human.QueryChange == nil || human.QueryChange.ChangedBy != "human" {
		t.Fatalf("human change = %+v, %v", human, err)
	}

	if _, err := store.UpdateSourceQuery(ctx, QueryUpdate{SourceID: "crossref", Query: "plant single nucleus"}); err == nil {
		t.Fatal("an agent change without a reason was accepted")
	}
	if _, err := store.UpdateSourceQuery(ctx, QueryUpdate{SourceID: "crossref", Query: "plant scRNA-seq", Reason: "same"}); err == nil {
		t.Fatal("an unchanged query was accepted")
	}
	agent, err := store.UpdateSourceQuery(ctx, QueryUpdate{SourceID: "crossref", Query: "plant single nucleus", Reason: "9 of 10 included vs 4 of 10"})
	if err != nil {
		t.Fatal(err)
	}
	if agent.Enabled || agent.Query != "plant single nucleus" || agent.QueryVersion != "3" {
		t.Fatalf("agent change = %+v", agent)
	}
	change := agent.QueryChange
	if change == nil || change.ChangedBy != "agent" || change.PreviousQuery != "plant scRNA-seq" || change.Reason != "9 of 10 included vs 4 of 10" {
		t.Fatalf("recorded change = %+v", change)
	}
	history, err := store.SourceQueryHistory(ctx, "crossref")
	if err != nil || len(history) != 2 || history[0].Version != "3" || history[1].Version != "2" {
		t.Fatalf("history = %+v, %v", history, err)
	}

	// A run records the query version it searched with.
	if _, err := store.SaveSourceConfig(ctx, SourceConfig{ID: "crossref", Enabled: true, Query: agent.Query, MaxResults: 20}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.syncSource(ctx, "crossref", fakeHarvester{}); err != nil {
		t.Fatal(err)
	}
	var version string
	if err := store.db.QueryRow("SELECT query_version FROM harvest_runs WHERE source_id = 'crossref'").Scan(&version); err != nil || version != "3" {
		t.Fatalf("run query version = %q, %v", version, err)
	}
}

func TestTrySourceQueryEvaluatesWithoutWriting(t *testing.T) {
	ctx := context.Background()
	store, _ := agentStore(t)
	included, err := store.AddPaper(ctx, Paper{Title: "Root atlas", DOI: "10.1000/root"})
	if err != nil {
		t.Fatal(err)
	}
	decidePaper(t, store, included, "included")
	excluded, err := store.AddPaper(ctx, Paper{Title: "Mouse brain", PMID: "222"})
	if err != nil {
		t.Fatal(err)
	}
	decided := excluded
	decided.Screening, decided.ScreeningReason = "excluded", "not a plant"
	if _, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &excluded, After: decided}}); err != nil {
		t.Fatal(err)
	}
	before, _ := store.Status()

	harvester := countingHarvester{count: 1234, fakeHarvester: fakeHarvester{records: []harvestRecord{
		{ExternalID: "10.1000/root", Title: "Root atlas", Paper: &Paper{Title: "Root atlas", DOI: "https://doi.org/10.1000/ROOT"}},
		{ExternalID: "222", Title: "Mouse brain", Paper: &Paper{Title: "Mouse brain", PMID: "222"}},
		{ExternalID: "10.1000/new", Title: "Leaf nuclei", Paper: &Paper{Title: "Leaf nuclei", DOI: "10.1000/new", Abstract: "snRNA-seq of leaves"}},
	}}}
	trial, err := store.trySourceQuery(ctx, QueryTrialInput{SourceID: "crossref", Query: "plant nuclei", Limit: 5}, harvester)
	if err != nil {
		t.Fatal(err)
	}
	if trial.Query != "plant nuclei" || trial.HitCount == nil || *trial.HitCount != 1234 {
		t.Fatalf("trial = %+v", trial)
	}
	if trial.Fetched != 3 || trial.New != 1 || trial.InLibrary != 2 || trial.Included != 1 || trial.Excluded != 1 {
		t.Fatalf("trial counts = %+v", trial)
	}
	if trial.Records[0].PaperID != included.ID || trial.Records[0].Decision != "included" || trial.Records[2].Abstract != "snRNA-seq of leaves" {
		t.Fatalf("trial records = %+v", trial.Records)
	}

	after, _ := store.Status()
	var runs, records int
	_ = store.db.QueryRow("SELECT count(*) FROM harvest_runs").Scan(&runs)
	_ = store.db.QueryRow("SELECT count(*) FROM source_records").Scan(&records)
	if after.PaperCount != before.PaperCount || runs != 0 || records != 0 {
		t.Fatalf("trial wrote: papers %d→%d, runs %d, records %d", before.PaperCount, after.PaperCount, runs, records)
	}
	if status, _ := store.SourceStatus(ctx, "crossref"); status.Query == "plant nuclei" {
		t.Fatal("trial saved its query")
	}

	// Without a query the saved one is tried, for a baseline.
	baseline, err := store.trySourceQuery(ctx, QueryTrialInput{SourceID: "crossref"}, fakeHarvester{})
	if err != nil || baseline.Query != "plant single cell RNA sequencing" || baseline.HitCount != nil {
		t.Fatalf("baseline = %+v, %v", baseline, err)
	}
	if _, err := store.trySourceQuery(ctx, QueryTrialInput{SourceID: "cncb-gsa"}, fakeHarvester{}); err == nil {
		t.Fatal("trial of a manual-only source succeeded")
	}
}

func TestMCPUpdatesSourceQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, _ := agentStore(t)
	local, err := ConnectLocalMCP(ctx, store, noCredentials)
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	call := func(name string, arguments map[string]any, target any) *mcp.CallToolResult {
		t.Helper()
		result, err := local.Session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if target != nil && !result.IsError {
			data, _ := json.Marshal(result.StructuredContent)
			if err := json.Unmarshal(data, target); err != nil {
				t.Fatal(err)
			}
		}
		return result
	}

	var sources struct {
		Sources []struct {
			ID          string `json:"id"`
			Query       string `json:"query"`
			QuerySyntax string `json:"querySyntax"`
		} `json:"sources"`
	}
	call("list_sources", nil, &sources)
	if len(sources.Sources) != 6 || sources.Sources[0].ID != "pubmed" || sources.Sources[0].QuerySyntax == "" || sources.Sources[0].Query == "" {
		t.Fatalf("list_sources = %+v", sources)
	}
	if result := call("update_source_query", map[string]any{"sourceId": "pubmed", "query": "plant[tiab]"}, nil); !result.IsError {
		t.Fatal("update_source_query without a reason succeeded")
	}
	var status SourceStatus
	call("update_source_query", map[string]any{"sourceId": "pubmed", "query": `"single nucleus"[tiab] AND plant[tiab]`, "reason": "fewer animal studies"}, &status)
	if status.QueryChange == nil || status.QueryChange.ChangedBy != "agent" {
		t.Fatalf("update_source_query = %+v", status)
	}
	var history queryHistoryOutput
	call("source_query_history", map[string]any{"sourceId": "pubmed"}, &history)
	if len(history.Changes) != 1 || history.Changes[0].Reason != "fewer animal studies" {
		t.Fatalf("source_query_history = %+v", history)
	}
}
