package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"bibliocell/internal/library"
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

	var stderr bytes.Buffer
	if code := runCLI([]string{"-data-dir", dataDir, "settings", "set", "-font-size", "99"}, strings.NewReader(""), &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("invalid font size accepted")
	}
	if isCLICommand("myapp://open?x=1") || !isCLICommand("papers") {
		t.Fatal("deep links must reach the window, commands must not")
	}
}
