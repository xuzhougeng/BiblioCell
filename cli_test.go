package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
