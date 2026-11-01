package library

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var testMCPToken = strings.Repeat("t", 64)

type bearerTransport struct {
	base http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+testMCPToken)
	return t.base.RoundTrip(clone)
}

func authenticatedHTTPClient() *http.Client {
	return &http.Client{Transport: bearerTransport{base: http.DefaultTransport}}
}

func TestMCPAgentCanOnlyPropose(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken, noCredentials)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(ctx)
	client := mcp.NewClient(&mcp.Implementation{Name: "bibliocell-propose-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.Endpoint, HTTPClient: authenticatedHTTPClient()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	rejected := func(name string, arguments map[string]any) bool {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: arguments})
		return err != nil || result.IsError
	}
	if !rejected("add_paper", map[string]any{"title": "Self approved", "screening": "included"}) {
		t.Fatal("add_paper accepted a screening decision")
	}
	proposed, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "add_paper", Arguments: map[string]any{
		"title": "Suggested", "proposedScreening": "excluded", "proposedReason": "spatial only",
	}})
	if err != nil || proposed.IsError {
		t.Fatalf("add_paper with a proposal: %v %+v", err, proposed)
	}
	var paper Paper
	data, _ := json.Marshal(proposed.StructuredContent)
	if err := json.Unmarshal(data, &paper); err != nil {
		t.Fatal(err)
	}
	if paper.Screening != "pending" || paper.ProposedScreening != "excluded" || paper.ReviewedAt != "" {
		t.Fatalf("proposal result = %+v", paper)
	}
	if !rejected("update_paper", map[string]any{"id": paper.ID, "patch": map[string]any{"screening": "included"}}) {
		t.Fatal("update_paper accepted a screening decision")
	}
	if !rejected("add_paper", map[string]any{"title": "No reason", "proposedScreening": "excluded"}) {
		t.Fatal("exclusion proposal without a reason accepted")
	}

	reviewed := paper
	reviewed.Screening, reviewed.ScreeningReason = "included", "checked the methods section"
	if conflicts, err := store.SavePaperEdits(ctx, []PaperEdit{{Before: &paper, After: reviewed}}); err != nil || len(conflicts) != 0 {
		t.Fatalf("human decision: %v %v", conflicts, err)
	}
	if !rejected("update_paper", map[string]any{"id": paper.ID, "patch": map[string]any{"journal": "Changed after review"}}) {
		t.Fatal("update_paper edited a reviewed paper")
	}
	if rejected("update_paper", map[string]any{"id": paper.ID, "patch": map[string]any{"proposedScreening": "uncertain"}}) {
		t.Fatal("update_paper refused a proposal on a reviewed paper")
	}
	if got, err := store.GetPaper(ctx, paper.ID); err != nil || got.Screening != "included" || got.Journal != "" || got.ProposedScreening != "uncertain" {
		t.Fatalf("reviewed paper = %+v, %v", got, err)
	}
}

func TestMCPAddAndListPapers(t *testing.T) {
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken, noCredentials)
	if err != nil {
		t.Fatalf("startMCP() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer func() { _ = server.Close(context.Background()) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "bibliocell-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.Endpoint, DisableStandaloneSSE: true, HTTPClient: authenticatedHTTPClient(),
	}, nil)
	if err != nil {
		t.Fatalf("MCP Connect() error = %v", err)
	}
	defer session.Close()

	addResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "add_paper",
		Arguments: map[string]any{
			"title": "Paper added through MCP", "doi": "10.1000/mcp",
			"species": []map[string]any{{"name": "Oryza sativa", "label": "水稻"}},
			"organs":  []string{"root", "leaf"}, "year": 2026,
			"platforms":    []string{"10x Genomics Chromium", "BD Rhapsody"},
			"modalities":   []string{"scRNA-seq"},
			"articleTypes": []string{"original", "review"},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(add_paper) error = %v", err)
	}
	if addResult.IsError {
		t.Fatalf("add_paper returned tool error: %+v", addResult.Content)
	}
	encoded, err := json.Marshal(addResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal add_paper output: %v", err)
	}
	var added Paper
	if err := json.Unmarshal(encoded, &added); err != nil {
		t.Fatalf("decode add_paper output: %v", err)
	}
	if added.ID == "" || added.Title != "Paper added through MCP" || added.Source != "MCP" {
		t.Errorf("unexpected add_paper output: %+v", added)
	}
	if len(added.Species) != 1 || added.Species[0].Name != "Oryza sativa" || added.Species[0].Label != "水稻" {
		t.Errorf("add_paper species = %+v", added.Species)
	}
	if len(added.Platforms) != 2 || len(added.Organs) != 2 {
		t.Errorf("add_paper multi-value fields = %+v / %+v", added.Platforms, added.Organs)
	}

	updateResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "update_paper", Arguments: map[string]any{
			"id": added.ID,
			"patch": map[string]any{
				"journal": "Updated journal", "starred": true, "tags": []string{"updated"},
				"platforms": []string{"BD Rhapsody"},
			},
		},
	})
	if err != nil || updateResult.IsError {
		t.Fatalf("update_paper failed: err=%v result=%+v", err, updateResult)
	}
	encoded, err = json.Marshal(updateResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal update_paper output: %v", err)
	}
	var updated Paper
	if err := json.Unmarshal(encoded, &updated); err != nil {
		t.Fatalf("decode update_paper output: %v", err)
	}
	if updated.Journal != "Updated journal" || !updated.Starred || len(updated.Tags) != 1 ||
		updated.DOI != added.DOI || updated.Year == nil || *updated.Year != 2026 {
		t.Errorf("partial update did not preserve metadata: %+v", updated)
	}
	if len(updated.Platforms) != 1 || updated.Platforms[0] != "BD Rhapsody" {
		t.Errorf("update_paper platforms = %+v", updated.Platforms)
	}
	if len(updated.Species) != 1 || updated.Species[0].Name != "Oryza sativa" {
		t.Errorf("update_paper dropped species: %+v", updated.Species)
	}

	listResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "list_papers", Arguments: map[string]any{"query": "Oryza", "limit": 10},
	})
	if err != nil {
		t.Fatalf("CallTool(list_papers) error = %v", err)
	}
	if listResult.IsError {
		t.Fatalf("list_papers returned tool error: %+v", listResult.Content)
	}
	encoded, err = json.Marshal(listResult.StructuredContent)
	if err != nil {
		t.Fatalf("marshal list_papers output: %v", err)
	}
	var listed listPapersOutput
	if err := json.Unmarshal(encoded, &listed); err != nil {
		t.Fatalf("decode list_papers output: %v", err)
	}
	if listed.Total != 1 || len(listed.Papers) != 1 || listed.Papers[0].ID != added.ID {
		t.Errorf("unexpected list_papers output: %+v", listed)
	}
}

func TestMCPSchemaResourcesAndAtomicImport(t *testing.T) {
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken, noCredentials)
	if err != nil {
		t.Fatalf("startMCP() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	defer func() { _ = server.Close(context.Background()) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "bibliocell-schema-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.Endpoint, DisableStandaloneSSE: true, HTTPClient: authenticatedHTTPClient(),
	}, nil)
	if err != nil {
		t.Fatalf("MCP Connect() error = %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools() error = %v", err)
	}
	var foundImport bool
	for _, tool := range tools.Tools {
		if tool.Name == "import_papers" {
			foundImport = tool.InputSchema != nil && tool.OutputSchema != nil
		}
	}
	if !foundImport {
		t.Error("import_papers does not expose input and output JSON Schemas")
	}

	paperResource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: paperSchemaURI})
	if err != nil {
		t.Fatalf("ReadResource(paper) error = %v", err)
	}
	if len(paperResource.Contents) != 1 || !strings.Contains(paperResource.Contents[0].Text, `"title"`) {
		t.Errorf("paper Schema resource is incomplete: %+v", paperResource.Contents)
	}
	sqlResource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: databaseSchemaURI})
	if err != nil {
		t.Fatalf("ReadResource(SQL) error = %v", err)
	}
	if len(sqlResource.Contents) != 1 || !strings.Contains(sqlResource.Contents[0].Text, "CREATE TABLE IF NOT EXISTS papers") {
		t.Errorf("SQL Schema resource is incomplete")
	}

	dryRun, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "import_papers", Arguments: map[string]any{
			"dryRun": true, "papers": []map[string]any{{"title": "Validation only"}},
		},
	})
	if err != nil || dryRun.IsError {
		t.Fatalf("dryRun import failed: err=%v result=%+v", err, dryRun)
	}
	status, err := store.Status()
	if err != nil || status.PaperCount != 0 {
		t.Fatalf("dryRun wrote data: status=%+v err=%v", status, err)
	}

	imported, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "import_papers", Arguments: map[string]any{
			"papers": []map[string]any{{"title": "Imported", "doi": "10.1000/import"}},
		},
	})
	if err != nil || imported.IsError {
		t.Fatalf("real import failed: err=%v result=%+v", err, imported)
	}
	failed, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "import_papers", Arguments: map[string]any{
			"papers": []map[string]any{
				{"title": "Must roll back"},
				{"title": "Invalid", "doi": "not-a-doi"},
			},
		},
	})
	if err != nil {
		t.Fatalf("CallTool(invalid import) protocol error = %v", err)
	}
	if !failed.IsError {
		t.Fatal("import with an invalid DOI succeeded, want tool error")
	}
	papers, err := store.ListPapers(ctx)
	if err != nil {
		t.Fatalf("ListPapers() error = %v", err)
	}
	if len(papers) != 1 || papers[0].Title != "Imported" {
		t.Errorf("failed batch was not rolled back: %+v", papers)
	}
}

func TestMCPReadOnlyWithoutToken(t *testing.T) {
	store, err := Open(t.TempDir() + "/bibliocell.sqlite")
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	original, err := store.AddPaper(ctx, Paper{Title: "Read only", DOI: "10.1000/readonly", Notes: "preserve"})
	if err != nil {
		t.Fatalf("AddPaper() error = %v", err)
	}
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken, noCredentials)
	if err != nil {
		t.Fatalf("startMCP() error = %v", err)
	}
	defer func() { _ = server.Close(context.Background()) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "bibliocell-readonly-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: server.Endpoint, DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("MCP Connect() error = %v", err)
	}
	defer session.Close()

	for _, name := range []string{"describe_library", "library_status", "list_papers"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name})
		if err != nil || result.IsError {
			t.Errorf("unauthenticated read %s failed: err=%v result=%+v", name, err, result)
		}
	}
	getResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "get_paper", Arguments: map[string]any{"id": original.ID},
	})
	if err != nil || getResult.IsError {
		t.Fatalf("unauthenticated get_paper failed: err=%v result=%+v", err, getResult)
	}

	writeCalls := []mcp.CallToolParams{
		{Name: "delete_paper", Arguments: map[string]any{"id": original.ID}},
		{Name: "add_paper", Arguments: map[string]any{"title": "Unauthorized"}},
		{Name: "import_papers", Arguments: map[string]any{"dryRun": true, "papers": []map[string]any{{"title": "Unauthorized"}}}},
		{Name: "update_paper", Arguments: map[string]any{"id": original.ID, "patch": map[string]any{"title": "Changed"}}},
	}
	for _, call := range writeCalls {
		result, err := session.CallTool(ctx, &call)
		if err != nil {
			t.Errorf("CallTool(%s) protocol error = %v", call.Name, err)
		} else if !result.IsError {
			t.Errorf("unauthenticated write %s succeeded", call.Name)
		}
	}
	unchanged, err := store.GetPaper(ctx, original.ID)
	if err != nil {
		t.Fatalf("GetPaper() error = %v", err)
	}
	if unchanged.Title != original.Title || unchanged.Notes != original.Notes {
		t.Errorf("unauthenticated call changed paper: %+v", unchanged)
	}
	status, err := store.Status()
	if err != nil || status.PaperCount != 1 {
		t.Errorf("unauthenticated call added a paper: status=%+v err=%v", status, err)
	}

	request, err := http.NewRequest(http.MethodPost, server.Endpoint, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Authorization", "Bearer wrong-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("request with invalid token error = %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Errorf("invalid token HTTP status = %d, want 401", response.StatusCode)
	}
}

func TestMCPDeletePaper(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	store := openTestLibrary(t)
	paper, err := store.AddPaper(ctx, Paper{Title: "Delete through MCP"})
	if err != nil {
		t.Fatal(err)
	}
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken, noCredentials)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(ctx)
	client := mcp.NewClient(&mcp.Implementation{Name: "delete-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.Endpoint, HTTPClient: authenticatedHTTPClient()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, tool := range tools.Tools {
		if tool.Name == "delete_paper" {
			found = true
		}
	}
	if !found {
		t.Fatal("delete_paper not discoverable")
	}
	call := &mcp.CallToolParams{Name: "delete_paper", Arguments: map[string]any{"id": paper.ID}}
	result, err := session.CallTool(ctx, call)
	if err != nil || result.IsError {
		t.Fatalf("delete: %v %+v", err, result)
	}
	var output deletePaperOutput
	data, _ := json.Marshal(result.StructuredContent)
	if err := json.Unmarshal(data, &output); err != nil || !output.Deleted || output.ID != paper.ID {
		t.Fatalf("output: %+v, %v", output, err)
	}
	if _, err := store.GetPaper(ctx, paper.ID); err == nil {
		t.Fatal("paper remains")
	}
	result, err = session.CallTool(ctx, call)
	if err != nil || !result.IsError {
		t.Fatalf("missing paper: %v %+v", err, result)
	}
}
