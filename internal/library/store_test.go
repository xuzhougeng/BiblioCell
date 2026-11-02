package library

import (
	"database/sql"
	"path/filepath"
	"strconv"
	"testing"

	_ "modernc.org/sqlite"
)

// legacyPapersDDL is the v1 papers table: no species_text/species_label/
// source columns and none of the v3 JSON list columns.
const legacyPapersDDL = `
CREATE TABLE papers (
	id TEXT PRIMARY KEY NOT NULL,
	title TEXT NOT NULL,
	abstract TEXT,
	authors_json TEXT NOT NULL DEFAULT '[]',
	journal TEXT,
	organs_json TEXT NOT NULL DEFAULT '[]',
	modalities_json TEXT NOT NULL DEFAULT '[]',
	conditions_text TEXT,
	online_date TEXT,
	issue_year INTEGER,
	publication_state TEXT NOT NULL DEFAULT 'unknown',
	article_type TEXT NOT NULL DEFAULT 'unknown',
	is_retracted INTEGER NOT NULL DEFAULT 0,
	canonical_url TEXT,
	fulltext_checked_at TEXT,
	data_checked_at TEXT,
	first_seen_at TEXT NOT NULL DEFAULT '2026-01-01T00:00:00Z',
	source_updated_at TEXT,
	updated_at TEXT NOT NULL DEFAULT '2026-01-01T00:00:00Z'
);`

func TestOpenInitializesAndPreservesDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data", "bibliocell.sqlite")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	status, err := store.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", status.SchemaVersion, SchemaVersion)
	}
	if status.TableCount != 22 {
		t.Errorf("TableCount = %d, want 22", status.TableCount)
	}
	if status.PaperCount != 0 || status.DatasetCount != 0 {
		t.Errorf("new database contains records: %+v", status)
	}
	if status.Integrity != "ok" {
		t.Errorf("Integrity = %q, want ok", status.Integrity)
	}

	var foreignKeys int
	if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Errorf("foreign_keys = %d, want 1", foreignKeys)
	}
	if _, err := store.db.Exec("INSERT INTO papers(id, title) VALUES('paper-1', 'Test paper')"); err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatalf("reopen database: %v", err)
	}
	defer store.Close()
	status, err = store.Status()
	if err != nil {
		t.Fatalf("Status() after reopening error = %v", err)
	}
	if status.PaperCount != 1 {
		t.Errorf("PaperCount after reopening = %d, want 1", status.PaperCount)
	}
}

// createLegacyDatabase writes a papers table that predates schema v3 and
// stamps user_version, reproducing both the honest v2 database and the
// damaged one whose version stamp outran its real columns.
func createLegacyDatabase(t *testing.T, path string, v2Columns bool, userVersion int) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(legacyPapersDDL); err != nil {
		t.Fatalf("create legacy papers table: %v", err)
	}
	if v2Columns {
		for _, column := range []string{"species_text", "species_label", "source"} {
			if _, err := db.Exec("ALTER TABLE papers ADD COLUMN " + column + " TEXT"); err != nil {
				t.Fatalf("add legacy column %s: %v", column, err)
			}
		}
		if _, err := db.Exec(`INSERT INTO papers(id, title, article_type, species_text, species_label)
			VALUES('v2-paper', 'Proper v2 paper', 'original', 'Oryza sativa', '水稻')`); err != nil {
			t.Fatalf("insert v2 paper: %v", err)
		}
	} else if _, err := db.Exec(`INSERT INTO papers(id, title, article_type)
		VALUES('v1-paper', 'Damaged stamped-v2 paper', 'review')`); err != nil {
		t.Fatalf("insert v1 paper: %v", err)
	}
	if _, err := db.Exec("PRAGMA user_version = " + strconv.Itoa(userVersion)); err != nil {
		t.Fatalf("stamp user_version: %v", err)
	}
}

func TestOpenHealsDamagedV2Database(t *testing.T) {
	// user_version says 2, but the papers table never received the v2 columns.
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	createLegacyDatabase(t, path, false, 2)

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() on damaged database error = %v", err)
	}
	defer store.Close()

	status, err := store.Status()
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", status.SchemaVersion, SchemaVersion)
	}
	var articleTypes string
	if err := store.db.QueryRow("SELECT article_types_json FROM papers WHERE id = 'v1-paper'").Scan(&articleTypes); err != nil {
		t.Fatalf("read article_types_json: %v", err)
	}
	if articleTypes != `["review"]` {
		t.Errorf("article_types_json = %s, want [\"review\"]", articleTypes)
	}
}

func TestOpenMigratesProperV2Database(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	createLegacyDatabase(t, path, true, 2)

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var species, articleTypes string
	if err := store.db.QueryRow("SELECT species_json, article_types_json FROM papers WHERE id = 'v2-paper'").
		Scan(&species, &articleTypes); err != nil {
		t.Fatalf("read migrated columns: %v", err)
	}
	if species != `[{"name":"Oryza sativa","label":"水稻"}]` {
		t.Errorf("species_json = %s", species)
	}
	if articleTypes != `["original"]` {
		t.Errorf("article_types_json = %s, want [\"original\"]", articleTypes)
	}

	// Reopening is stable and keeps the backfilled values.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened database: %v", err)
	}
}
