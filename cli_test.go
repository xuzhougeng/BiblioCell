package main

import (
	"bytes"
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

// run executes one CLI invocation against dataDir and decodes its JSON output.
func run(t *testing.T, dataDir, stdin string, target any, args ...string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runCLI(append([]string{"-data-dir", dataDir}, args...), strings.NewReader(stdin), &stdout, &stderr)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, stderr.String())
	}
	if target != nil {
		if err := json.Unmarshal(stdout.Bytes(), target); err != nil {
			t.Fatalf("%v output %q: %v", args, stdout.String(), err)
		}
	}
}

func TestCLIRoundTrip(t *testing.T) {
	dataDir := t.TempDir()

	var added library.Paper
	run(t, dataDir, `{"title":"CLI paper","doi":"10.1000/cli","tags":["cli"]}`, &added, "papers", "add")
	if added.ID == "" || added.Source != "CLI" {
		t.Fatalf("unexpected added paper: %+v", added)
	}

	var found []library.Paper
	run(t, dataDir, "", &found, "papers", "list", "-q", "cli")
	if len(found) != 1 || found[0].ID != added.ID {
		t.Fatalf("list -q cli = %+v", found)
	}
	run(t, dataDir, "", &found, "papers", "list", "-q", "nothing-matches")
	if len(found) != 0 {
		t.Fatalf("list with no match = %+v", found)
	}

	var updated library.Paper
	run(t, dataDir, `{"journal":"Cell"}`, &updated, "papers", "update", added.ID)
	if updated.Journal != "Cell" || updated.DOI != "10.1000/cli" {
		t.Fatalf("patch lost fields: %+v", updated)
	}

	var rows []map[string]any
	run(t, dataDir, "", &rows, "sql", "select journal from papers")
	if len(rows) != 1 || rows[0]["journal"] != "Cell" {
		t.Fatalf("sql rows = %+v", rows)
	}

	var config map[string]any
	run(t, dataDir, "", &config, "settings", "set", "-font-size", "18")
	if config["fontSize"] != float64(18) {
		t.Fatalf("settings = %+v", config)
	}

	// Agents reach the same store as MCP: the CLI cannot make a decision either.
	var proposed library.Paper
	run(t, dataDir, `{"proposedScreening":"included"}`, &proposed, "papers", "update", added.ID)
	if proposed.Screening != "pending" || proposed.ProposedScreening != "included" {
		t.Fatalf("proposal changed the decision: %+v", proposed)
	}
	for _, attempt := range [][]string{
		{`{"title":"Self approved","screening":"included"}`, "papers", "add"},
		{`{"screening":"included"}`, "papers", "update", added.ID},
	} {
		if code := runCLI(append([]string{"-data-dir", dataDir}, attempt[1:]...), strings.NewReader(attempt[0]), &bytes.Buffer{}, &bytes.Buffer{}); code == 0 {
			t.Fatalf("%v decided screening through the CLI", attempt)
		}
	}

	var stderr bytes.Buffer
	if code := runCLI([]string{"-data-dir", dataDir, "settings", "set", "-font-size", "99"}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("invalid font size accepted")
	}
	if isCLICommand("myapp://open?x=1") || !isCLICommand("papers") {
		t.Fatal("deep links must reach the window, commands must not")
	}
}

func TestCLIAgentCommands(t *testing.T) {
	dataDir := t.TempDir()
	var imported library.ImportResult
	run(t, dataDir, `[{"title":"CLI batch paper"}]`, &imported, "papers", "import", "-batch", "cli-run")
	if imported.Count != 1 || imported.BatchID != "cli-run" {
		t.Fatalf("import = %+v", imported)
	}
	paperID := imported.Papers[0].ID

	var added library.DatasetResult
	run(t, dataDir, `{"title":"CLI dataset","batchId":"cli-run","repositories":[{"repository":"geo","accession":"GSE5"}]}`, &added, "datasets", "add")
	var linked library.Dataset
	run(t, dataDir, `{"paperId":"`+paperID+`","role":"generated","evidenceText":"we deposited"}`, &linked, "datasets", "link", added.Dataset.ID)
	if len(linked.PaperLinks) != 1 || linked.PaperLinks[0].Verified {
		t.Fatalf("link = %+v", linked)
	}
	var updated library.Dataset
	run(t, dataDir, `{"status":"identified"}`, &updated, "datasets", "update", added.Dataset.ID)
	var listed []library.Dataset
	run(t, dataDir, "", &listed, "datasets", "list", "-status", "identified")
	if len(listed) != 1 {
		t.Fatalf("datasets list = %+v", listed)
	}
	var got library.Dataset
	run(t, dataDir, "", &got, "datasets", "get", added.Dataset.ID)
	if got.Status != "identified" {
		t.Fatalf("datasets get = %+v", got)
	}

	var evidence []library.Evidence
	run(t, dataDir, `[{"paperId":"`+paperID+`","field":"organs","excerpt":"root tips","sourceUrl":"https://example.org"}]`, &evidence, "evidence", "add")
	run(t, dataDir, "", &evidence, "evidence", "list", "-paper", paperID)
	if len(evidence) != 1 || evidence[0].Status != "unreviewed" {
		t.Fatalf("evidence = %+v", evidence)
	}

	var papers []library.Paper
	run(t, dataDir, "", &papers, "papers", "list", "-batch", "cli-run", "-needs-review", "-limit", "1")
	if len(papers) != 1 {
		t.Fatalf("papers list = %+v", papers)
	}
	var files []library.PaperFile
	run(t, dataDir, "", &files, "papers", "files", paperID)
	if len(files) != 0 {
		t.Fatalf("papers files = %+v", files)
	}

	for _, attempt := range [][]string{
		{"datasets", "delete", "x"}, {"datasets", "verify", added.Dataset.ID}, {"evidence", "verify", "x"},
		{"serve", "-addr", "0.0.0.0:0"},
	} {
		if code := runCLI(append([]string{"-data-dir", dataDir}, attempt...), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code == 0 {
			t.Errorf("%v succeeded", attempt)
		}
	}
	if code := runCLI([]string{"-data-dir", dataDir, "datasets", "update", added.Dataset.ID}, strings.NewReader(`{"status":"verified"}`), &bytes.Buffer{}, &bytes.Buffer{}); code == 0 {
		t.Error("the CLI verified a dataset")
	}

	var undone library.UndoResult
	run(t, dataDir, "", &undone, "papers", "undo", "cli-run")
	if undone.PapersRemoved != 1 || undone.DatasetsRemoved != 1 {
		t.Fatalf("undo = %+v", undone)
	}
	if !isCLICommand("serve") || !isCLICommand("evidence") {
		t.Fatal("agent commands must not open a window")
	}
}

func TestStorageFolderKeepsWorkingAfterCopy(t *testing.T) {
	dataDir := t.TempDir()
	original := filepath.Join(t.TempDir(), "library")
	run(t, dataDir, "", nil, "settings", "set", "-storage", original)

	var added library.Paper
	run(t, dataDir, `{"title":"Portable paper","doi":"10.1000/portable"}`, &added, "papers", "add")
	run(t, dataDir, "", nil, "sources", "set", "pubmed", "-query", "plant single cell")
	source := filepath.Join(t.TempDir(), "全文 final.pdf")
	if err := os.WriteFile(source, []byte("pdf content"), 0o600); err != nil {
		t.Fatal(err)
	}
	var attached []library.PaperFile
	run(t, dataDir, "", &attached, "papers", "attach", added.ID, source)
	if len(attached) != 1 || attached[0].Title != "全文 final.pdf" {
		t.Fatalf("attached = %+v", attached)
	}

	// A location saved in the database would break the copy.
	database, err := os.ReadFile(filepath.Join(original, settings.DatabaseFile))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(database, []byte(original)) {
		t.Fatal("database stores the storage folder location")
	}

	copied := filepath.Join(t.TempDir(), "moved", "library")
	if err := os.CopyFS(copied, os.DirFS(original)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(original); err != nil {
		t.Fatal(err)
	}
	run(t, dataDir, "", nil, "settings", "set", "-storage", copied)

	var status library.Status
	run(t, dataDir, "", &status, "status")
	if status.StoragePath != copied || status.DatabasePath != filepath.Join(copied, settings.DatabaseFile) || status.PaperCount != 1 {
		t.Fatalf("status after copy = %+v", status)
	}
	var found library.Paper
	run(t, dataDir, "", &found, "papers", "get", added.ID)
	if found.Title != "Portable paper" {
		t.Fatalf("paper after copy = %+v", found)
	}
	var sources []library.SourceStatus
	run(t, dataDir, "", &sources, "sources", "list")
	kept := false
	for _, source := range sources {
		kept = kept || (source.ID == "pubmed" && source.Query == "plant single cell")
	}
	if !kept {
		t.Fatalf("source configuration lost after copy: %+v", sources)
	}
	if found.Fulltext != "stored" {
		t.Fatalf("fulltext after copy = %q", found.Fulltext)
	}
	if data, err := os.ReadFile(filepath.Join(copied, filepath.FromSlash(attached[0].LocalPath))); err != nil || string(data) != "pdf content" {
		t.Fatalf("paper file after copy = %q, %v", data, err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("copy still touches the old folder: %v", err)
	}
}

func TestCLIDeletePaper(t *testing.T) {
	dataDir := t.TempDir()
	var paper library.Paper
	run(t, dataDir, "{\"title\":\"Delete through CLI\"}", &paper, "papers", "add")
	for _, args := range [][]string{{"papers", "delete"}, {"papers", "delete", paper.ID, "extra"}} {
		if code := runCLI(append([]string{"-data-dir", dataDir}, args...), strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code == 0 {
			t.Fatalf("accepted invalid args: %v", args)
		}
	}
	var output struct {
		ID      string
		Deleted bool
	}
	run(t, dataDir, "", &output, "papers", "delete", paper.ID)
	if output.ID != paper.ID || !output.Deleted {
		t.Fatalf("output: %+v", output)
	}
	var papers []library.Paper
	run(t, dataDir, "", &papers, "papers", "list")
	if len(papers) != 0 {
		t.Fatal("paper remains")
	}
	var stderr bytes.Buffer
	if code := runCLI([]string{"-data-dir", dataDir, "papers", "delete", paper.ID}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 || !strings.Contains(stderr.String(), "paper not found") {
		t.Fatalf("missing paper: %d %s", code, stderr.String())
	}
}

// TestCLIAgentRunsConfiguredModel drives the built-in loop end to end: the
// configured Chat Completions service asks for a tool, the CLI runs it against
// the real database, and the final answer comes back.
func TestCLIAgentRunsConfiguredModel(t *testing.T) {
	dataDir := t.TempDir()
	run(t, dataDir, `{"title":"Agent sees me"}`, nil, "papers", "add")

	var calls int
	var toolOutput string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			Tools []struct {
				Function struct{ Name string } `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "test-model" || len(body.Tools) < 10 {
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if calls == 1 {
			w.Write([]byte(`{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"list_papers","arguments":"{\"query\":\"agent\"}"}}]},"finish_reason":"tool_calls"}]}`))
			return
		}
		toolOutput = body.Messages[len(body.Messages)-1].Content
		w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"库里有 1 篇。"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	var saved settings.Config
	run(t, dataDir, "", &saved, "settings", "set", "-ai-protocol", "openai", "-ai-base-url", server.URL, "-ai-model", "test-model", "-ai-api-key", "k")
	if len(saved.Models) != 1 || saved.Models[0].Config.Model != "test-model" || saved.Models[0].Config.Protocol != agent.ProtocolOpenAI {
		t.Fatalf("saved models = %+v", saved.Models)
	}
	var result agent.Result
	run(t, dataDir, "", &result, "agent", "how", "many", "papers?")
	if result.Text != "库里有 1 篇。" || result.Steps != 2 || result.StopReason != "end_turn" || len(result.Messages) != 4 {
		t.Fatalf("result = %+v", result)
	}
	if !strings.Contains(toolOutput, "Agent sees me") || !strings.Contains(toolOutput, `"total":1`) {
		t.Fatalf("tool output sent to the model = %q", toolOutput)
	}
	if code := runCLI([]string{"-data-dir", t.TempDir(), "agent", "hello"}, strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}); code == 0 {
		t.Error("agent ran without an AI configuration")
	}
}
