package library

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// buildDatabase creates a database from a frozen schema file in testdata and
// runs extra statements against it, the way a released build would have left it.
func buildDatabase(t *testing.T, path, fixture string, statements ...string) {
	t.Helper()
	ddl, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(ddl)); err != nil {
		t.Fatalf("build %s: %v", fixture, err)
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("seed %s: %v\n%s", fixture, err, statement)
		}
	}
}

func userVersion(t *testing.T, path string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		t.Fatal(err)
	}
	return version
}

func columnNames(t *testing.T, db *sql.DB, table string) []string {
	t.Helper()
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	names := []string{}
	for rows.Next() {
		var cid, notNull, pk int
		var name, kind string
		var dflt any
		if err := rows.Scan(&cid, &name, &kind, &notNull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return names
}

// withMigrations swaps the migration list for the test and restores it after.
func withMigrations(t *testing.T, list []Migration) {
	t.Helper()
	previous, previousVersion := migrations, SchemaVersion
	migrations, SchemaVersion = list, baselineVersion+len(list)
	t.Cleanup(func() { migrations, SchemaVersion = previous, previousVersion })
}

// v010Seed is a small library as v0.1.0 would have written it: a paper with
// curation, identifiers and an attachment, a dataset with a GEO record.
var v010Seed = []string{
	`INSERT INTO papers(id, title, title_key, journal, issue_year, species_json, organs_json, modalities_json, platforms_json, article_types_json, source)
	 VALUES ('P-old', 'Single-cell atlas of the barley root', 'singlecellatlasofthebarleyroot', 'Plant Cell', 2025,
	         '[{"name":"Hordeum vulgare","label":"大麦"}]', '["root"]', '["scRNA-seq"]', '["10x Genomics Chromium"]', '["original"]', 'PubMed')`,
	`INSERT INTO paper_identifiers(paper_id, namespace, value) VALUES ('P-old', 'doi', '10.1000/barley'), ('P-old', 'pmid', '12345678')`,
	// tr_paper_curation_init created the curation row with the paper.
	`UPDATE paper_curation SET screening_status = 'included', screening_reason = 'Root single-cell data', screening_method = 'human',
	 notes = 'Keep for the atlas comparison', tags_json = '["atlas"]' WHERE paper_id = 'P-old'`,
	`INSERT INTO datasets(id, title, species_json, modalities_json) VALUES ('DS-old', 'Barley root scRNA-seq', '[{"name":"Hordeum vulgare","label":"大麦"}]', '["scRNA-seq"]')`,
	`INSERT INTO repository_records(id, repository, accession) VALUES ('RR-old', 'geo', 'GSE100001')`,
	`INSERT INTO dataset_repositories(dataset_id, repository_record_id, mapping_scope) VALUES ('DS-old', 'RR-old', 'exact')`,
	`INSERT INTO paper_datasets(paper_id, dataset_id, role, evidence_text) VALUES ('P-old', 'DS-old', 'generated', 'Data availability statement')`,
	`INSERT INTO resources(id, resource_type, title, local_path, local_state) VALUES ('R-old', 'fulltext', 'barley.pdf', 'papers/barley.pdf', 'stored')`,
	`INSERT INTO paper_resources(paper_id, resource_id) VALUES ('P-old', 'R-old')`,
}

func TestOpenUpgradesV010Database(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bibliocell.sqlite")
	buildDatabase(t, path, "schema_v11.sql", v010Seed...)
	if got := userVersion(t, path); got != 11 {
		t.Fatalf("fixture user_version = %d, want 11 (the v0.1.0 release)", got)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() on a v0.1.0 database: %v", err)
	}
	defer store.Close()

	if store.Upgraded == nil || store.Upgraded.From != 11 || store.Upgraded.To != SchemaVersion {
		t.Fatalf("Upgraded = %+v, want 11 -> %d", store.Upgraded, SchemaVersion)
	}
	if store.Upgraded.Backup != filepath.Join(dir, "bibliocell.sqlite.bak-v11") {
		t.Errorf("backup path = %s", store.Upgraded.Backup)
	}
	// The backup is the untouched v0.1.0 file: same version, same paper.
	if got := userVersion(t, store.Upgraded.Backup); got != 11 {
		t.Errorf("backup user_version = %d, want 11", got)
	}
	backup, err := sql.Open("sqlite", store.Upgraded.Backup)
	if err != nil {
		t.Fatal(err)
	}
	var backedUp int
	if err := backup.QueryRow("SELECT count(*) FROM papers WHERE id = 'P-old'").Scan(&backedUp); err != nil || backedUp != 1 {
		t.Errorf("backup papers = %d, error = %v", backedUp, err)
	}
	backup.Close()

	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatalf("user_version = %d, error = %v, want %d", version, err, SchemaVersion)
	}
	var recorded int
	if err := store.db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version IN (12, 13)").Scan(&recorded); err != nil || recorded != 2 {
		t.Errorf("recorded migrations 12 and 13 = %d, error = %v", recorded, err)
	}

	// Every seeded row survived, with its curation, links and attachment.
	var title, journal, reason, notes, accession, role, localPath string
	var year int
	if err := store.db.QueryRow(`SELECT p.title, p.journal, p.issue_year, c.screening_reason, c.notes, r.accession, pd.role, res.local_path
		FROM papers p JOIN paper_curation c ON c.paper_id = p.id
		JOIN paper_datasets pd ON pd.paper_id = p.id JOIN dataset_repositories dr ON dr.dataset_id = pd.dataset_id
		JOIN repository_records r ON r.id = dr.repository_record_id JOIN paper_resources pr ON pr.paper_id = p.id JOIN resources res ON res.id = pr.resource_id
		WHERE p.id = 'P-old'`).Scan(&title, &journal, &year, &reason, &notes, &accession, &role, &localPath); err != nil {
		t.Fatalf("read upgraded paper: %v", err)
	}
	if title != "Single-cell atlas of the barley root" || journal != "Plant Cell" || year != 2025 || reason != "Root single-cell data" ||
		notes != "Keep for the atlas comparison" || accession != "GSE100001" || role != "generated" || localPath != "papers/barley.pdf" {
		t.Errorf("upgraded paper lost data: %q %q %d %q %q %q %q %q", title, journal, year, reason, notes, accession, role, localPath)
	}
	var doi, pmid int
	if err := store.db.QueryRow("SELECT count(*) FROM paper_identifiers WHERE paper_id = 'P-old' AND namespace = 'doi' AND value = '10.1000/barley'").Scan(&doi); err != nil || doi != 1 {
		t.Errorf("doi rows = %d, error = %v", doi, err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM paper_identifiers WHERE paper_id = 'P-old' AND namespace = 'pmid' AND value = '12345678'").Scan(&pmid); err != nil || pmid != 1 {
		t.Errorf("pmid rows = %d, error = %v", pmid, err)
	}

	// Version 13's custom repository names are accepted now; 11 rejected them.
	if _, err := store.db.Exec("INSERT INTO repository_records(id, repository, accession) VALUES ('RR-st', 'stomics', 'STT0000001')"); err != nil {
		t.Errorf("STOmics record after upgrade: %v", err)
	}

	// The upgraded database has exactly the columns a fresh one has.
	fresh, err := Open(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if diff := schemaDiff(t, fresh.db, store.db); diff != "" {
		t.Errorf("upgraded v0.1.0 schema differs from a fresh one:\n%s", diff)
	}

	// The app's own reads work on the upgraded library.
	status, err := store.Status()
	if err != nil || status.PaperCount != 1 || status.DatasetCount != 1 {
		t.Errorf("Status = %+v, error = %v", status, err)
	}

	// A second open finds a current database and touches nothing.
	store.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if again.Upgraded != nil {
		t.Errorf("second Open reported an upgrade: %+v", again.Upgraded)
	}
}

// schemaSignature lists every table, view and index with their columns, so two
// databases can be compared regardless of how each arrived at its shape.
func schemaSignature(t *testing.T, db *sql.DB) map[string][]string {
	t.Helper()
	rows, err := db.Query("SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY type, name")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	signature := map[string][]string{}
	type object struct{ kind, name string }
	objects := []object{}
	for rows.Next() {
		var o object
		if err := rows.Scan(&o.kind, &o.name); err != nil {
			t.Fatal(err)
		}
		objects = append(objects, o)
	}
	for _, o := range objects {
		var columns []string
		switch o.kind {
		case "table", "view":
			info, err := db.Query("PRAGMA table_info(" + o.name + ")")
			if err != nil {
				t.Fatal(err)
			}
			for info.Next() {
				var cid, notNull, pk int
				var name, kind string
				var dflt sql.NullString
				if err := info.Scan(&cid, &name, &kind, &notNull, &dflt, &pk); err != nil {
					t.Fatal(err)
				}
				columns = append(columns, fmt.Sprintf("%s %s notnull=%d default=%s pk=%d", name, kind, notNull, dflt.String, pk))
			}
			info.Close()
		case "index":
			info, err := db.Query("PRAGMA index_info(" + o.name + ")")
			if err != nil {
				t.Fatal(err)
			}
			for info.Next() {
				var seq, cid int
				var name sql.NullString
				if err := info.Scan(&seq, &cid, &name); err != nil {
					t.Fatal(err)
				}
				columns = append(columns, name.String)
			}
			info.Close()
		}
		signature[o.kind+" "+o.name] = columns
	}
	return signature
}

func schemaDiff(t *testing.T, want, got *sql.DB) string {
	t.Helper()
	a, b := schemaSignature(t, want), schemaSignature(t, got)
	var diff []string
	for name, columns := range a {
		if other, ok := b[name]; !ok {
			diff = append(diff, "missing "+name)
		} else if !reflect.DeepEqual(columns, other) {
			diff = append(diff, fmt.Sprintf("%s differs:\n  fresh:    %v\n  migrated: %v", name, columns, other))
		}
	}
	for name := range b {
		if _, ok := a[name]; !ok {
			diff = append(diff, "extra "+name)
		}
	}
	return strings.Join(diff, "\n")
}

// TestMigrationsMatchSchema is the guard for adding a migration: the baseline
// plus every migration must produce what schema.sql builds in one go, and
// schema.sql must carry the same version number.
func TestMigrationsMatchSchema(t *testing.T) {
	pragma := regexp.MustCompile(`PRAGMA user_version = (\d+);`).FindStringSubmatch(schemaSQL)
	if pragma == nil {
		t.Fatal("schema.sql has no PRAGMA user_version")
	}
	if stamped, _ := strconv.Atoi(pragma[1]); stamped != SchemaVersion {
		t.Errorf("schema.sql stamps version %d, but the migration list ends at %d: update both", stamped, SchemaVersion)
	}
	if !strings.Contains(schemaSQL, fmt.Sprintf("schema v%d\n", SchemaVersion)) {
		t.Errorf("schema.sql header does not say v%d", SchemaVersion)
	}
	for i, m := range migrations {
		if m.Version != baselineVersion+i+1 {
			t.Errorf("migration %d is out of order; want %d", m.Version, baselineVersion+i+1)
		}
	}

	fresh, err := Open(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()

	path := filepath.Join(t.TempDir(), "baseline.sqlite")
	buildDatabase(t, path, "schema_v13.sql")
	if got := userVersion(t, path); got != baselineVersion {
		t.Fatalf("baseline fixture user_version = %d, want %d", got, baselineVersion)
	}
	migrated, err := Open(path)
	if err != nil {
		t.Fatalf("Open() on the baseline: %v", err)
	}
	defer migrated.Close()
	if diff := schemaDiff(t, fresh.db, migrated.db); diff != "" {
		t.Errorf("baseline + migrations differs from schema.sql:\n%s", diff)
	}
	versions := func(db *sql.DB) []int {
		rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []int
		for rows.Next() {
			var v int
			if err := rows.Scan(&v); err != nil {
				t.Fatal(err)
			}
			out = append(out, v)
		}
		return out
	}
	if a, b := versions(fresh.db), versions(migrated.db); !reflect.DeepEqual(a, b) {
		t.Errorf("recorded versions differ: fresh %v, migrated %v", a, b)
	}
	if len(migrations) > 0 && (migrated.Upgraded == nil || migrated.Upgraded.From != baselineVersion) {
		t.Errorf("Upgraded = %+v", migrated.Upgraded)
	}
}

func TestMigrationsRunInOrderAndRecordVersions(t *testing.T) {
	withMigrations(t, []Migration{
		{Version: 14, Description: "test column", SQL: "ALTER TABLE papers ADD COLUMN mig_test TEXT NOT NULL DEFAULT ''"},
		{Version: 15, Description: "test backfill", Run: func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE papers SET mig_test = 'filled'")
			return err
		}},
	})
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	buildDatabase(t, path, "schema_v13.sql", "INSERT INTO papers(id, title) VALUES ('P-1', 'Kept')")

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if store.Upgraded == nil || store.Upgraded.From != 13 || store.Upgraded.To != 15 {
		t.Fatalf("Upgraded = %+v", store.Upgraded)
	}
	var version int
	if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 15 {
		t.Errorf("user_version = %d, error = %v", version, err)
	}
	var filled string
	if err := store.db.QueryRow("SELECT mig_test FROM papers WHERE id = 'P-1'").Scan(&filled); err != nil || filled != "filled" {
		t.Errorf("mig_test = %q, error = %v", filled, err)
	}
	var descriptions []string
	rows, err := store.db.Query("SELECT description FROM schema_migrations WHERE version >= 14 ORDER BY version")
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var d string
		_ = rows.Scan(&d)
		descriptions = append(descriptions, d)
	}
	rows.Close()
	if !reflect.DeepEqual(descriptions, []string{"test column", "test backfill"}) {
		t.Errorf("recorded descriptions = %v", descriptions)
	}
	if _, err := os.Stat(backupPath(path, 13)); err != nil {
		t.Errorf("backup before migrating: %v", err)
	}

	// A fresh database gets the rows too, even though no step ran on it.
	fresh, err := Open(filepath.Join(t.TempDir(), "fresh.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	var count int
	if err := fresh.db.QueryRow("SELECT count(*) FROM schema_migrations WHERE version IN (14, 15)").Scan(&count); err != nil || count != 2 {
		t.Errorf("fresh database recorded %d of 2 migrations, error = %v", count, err)
	}
	if err := fresh.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 15 {
		t.Errorf("fresh user_version = %d, error = %v", version, err)
	}
}

func TestFailedMigrationRollsBackAndKeepsBackup(t *testing.T) {
	withMigrations(t, []Migration{
		{Version: 14, Description: "adds a column", SQL: "ALTER TABLE papers ADD COLUMN mig_ok TEXT"},
		{Version: 15, Description: "breaks halfway", SQL: "ALTER TABLE papers ADD COLUMN mig_half TEXT; INSERT INTO no_such_table VALUES (1)"},
	})
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	buildDatabase(t, path, "schema_v13.sql", "INSERT INTO papers(id, title) VALUES ('P-1', 'Kept')")

	_, err := Open(path)
	if err == nil {
		t.Fatal("Open() succeeded despite a broken migration")
	}
	backup := backupPath(path, 13)
	for _, want := range []string{"migration 15", "breaks halfway", "left at version 14", backup} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	// The file holds the last completed step and nothing of the failed one.
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if got := userVersion(t, path); got != 14 {
		t.Errorf("user_version after failure = %d, want 14", got)
	}
	columns := strings.Join(columnNames(t, db, "papers"), ",")
	if !strings.Contains(columns, "mig_ok") || strings.Contains(columns, "mig_half") {
		t.Errorf("papers columns after failure: %s", columns)
	}
	var kept int
	if err := db.QueryRow("SELECT count(*) FROM papers WHERE id = 'P-1'").Scan(&kept); err != nil || kept != 1 {
		t.Errorf("paper rows = %d, error = %v", kept, err)
	}
	// The backup is the pre-upgrade file, ready to copy back.
	if got := userVersion(t, backup); got != 13 {
		t.Errorf("backup user_version = %d, want 13", got)
	}

	// Once the step is fixed the database continues from where it stopped.
	migrations[1].SQL = "ALTER TABLE papers ADD COLUMN mig_half TEXT"
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() after fixing the migration: %v", err)
	}
	defer store.Close()
	if store.Upgraded == nil || store.Upgraded.From != 14 || store.Upgraded.Backup != backupPath(path, 14) {
		t.Errorf("Upgraded after retry = %+v", store.Upgraded)
	}
}

func TestOpenRefusesNewerDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	buildDatabase(t, path, "schema_v13.sql", "PRAGMA user_version = 99")
	_, err := Open(path)
	if err == nil || !strings.Contains(err.Error(), "newer than this build") {
		t.Fatalf("Open() on a newer database: %v", err)
	}
	if _, err := os.Stat(backupPath(path, 99)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a refused open must not write a backup")
	}
}

func TestInspectReportsVersionWithoutChangingTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	info, err := Inspect(path)
	if err != nil || info.State != "missing" || info.Exists {
		t.Fatalf("Inspect(missing) = %+v, %v", info, err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Inspect created the database")
	}

	buildDatabase(t, path, "schema_v11.sql", v010Seed...)
	info, err = Inspect(path)
	if err != nil || info.State != "outdated" || info.Version != 11 || info.AppVersion != SchemaVersion {
		t.Fatalf("Inspect(v0.1.0) = %+v, %v", info, err)
	}
	if len(info.Pending) == 0 || info.Pending[0].Version != baselineVersion {
		t.Errorf("pending for v11 = %+v", info.Pending)
	}
	// Ten rows: schema.sql wrote 1-7 and 9-11, rekeyPapers adds 8 on the first Open.
	if len(info.Applied) != 10 {
		t.Errorf("applied rows for v11 = %d", len(info.Applied))
	}
	if got := userVersion(t, path); got != 11 {
		t.Errorf("Inspect changed user_version to %d", got)
	}

	upgrade, err := Migrate(path)
	if err != nil || upgrade == nil || upgrade.From != 11 || upgrade.To != SchemaVersion {
		t.Fatalf("Migrate() = %+v, %v", upgrade, err)
	}
	info, err = Inspect(path)
	if err != nil || info.State != "current" || info.Version != SchemaVersion || len(info.Pending) != 0 {
		t.Fatalf("Inspect(after) = %+v, %v", info, err)
	}
	if upgrade, err := Migrate(path); err != nil || upgrade != nil {
		t.Errorf("second Migrate() = %+v, %v", upgrade, err)
	}
}
