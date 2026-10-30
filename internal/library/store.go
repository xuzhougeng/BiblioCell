// Package library provides the local persistence for BiblioCell.
package library

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

const SchemaVersion = 5

//go:embed schema.sql
var schemaSQL string

// columnMigration describes a column introduced after the original schema.
// Open checks actual columns instead of trusting user_version, so a database
// whose version stamp outran its DDL is healed too.
type columnMigration struct {
	table    string
	column   string
	ddl      string
	backfill string
}

var columnMigrations = []columnMigration{
	{table: "papers", column: "species_text", ddl: "TEXT"},
	{table: "papers", column: "species_label", ddl: "TEXT"},
	{table: "papers", column: "source", ddl: "TEXT"},
	{
		table: "papers", column: "species_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(species_json) AND json_type(species_json) = 'array')",
		backfill: `UPDATE papers SET species_json = json_array(json_object('name', species_text, 'label', coalesce(species_label, '')))` +
			` WHERE species_json = '[]' AND species_text IS NOT NULL AND trim(species_text) <> ''`,
	},
	{
		table: "papers", column: "platforms_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(platforms_json) AND json_type(platforms_json) = 'array')",
	},
	{
		table: "papers", column: "article_types_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(article_types_json) AND json_type(article_types_json) = 'array')",
		backfill: `UPDATE papers SET article_types_json = json_array(article_type)` +
			` WHERE article_types_json = '[]' AND article_type IS NOT NULL AND article_type <> 'unknown'`,
	},
	{table: "datasets", column: "notes", ddl: "TEXT NOT NULL DEFAULT ''"}, {
		table: "datasets", column: "species_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(species_json) AND json_type(species_json) = 'array')",
		backfill: `UPDATE datasets SET species_json = (
			SELECT json_group_array(json_object('name', t.scientific_name, 'label', coalesce(t.common_name, '')))
			FROM dataset_taxa dt JOIN taxa t ON t.taxid = dt.taxid WHERE dt.dataset_id = datasets.id
		) WHERE species_json = '[]'`,
	},
	{
		table: "datasets", column: "organs_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(organs_json) AND json_type(organs_json) = 'array')",
		backfill: `UPDATE datasets SET organs_json = json_array(tissue_text)` +
			` WHERE organs_json = '[]' AND tissue_text IS NOT NULL AND trim(tissue_text) <> ''`,
	},
	{
		table: "datasets", column: "modalities_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(modalities_json) AND json_type(modalities_json) = 'array')",
		backfill: `UPDATE datasets SET modalities_json = CASE modality
			WHEN 'scRNA' THEN json_array('scRNA-seq') WHEN 'snRNA' THEN json_array('snRNA-seq')
			WHEN 'scATAC' THEN json_array('scATAC-seq') WHEN 'snATAC' THEN json_array('snATAC-seq')
			WHEN 'scDNA' THEN json_array('scDNA-seq') WHEN 'scMethyl' THEN json_array('scMethyl-seq')
			WHEN 'longread' THEN json_array('long-read') WHEN 'unknown' THEN '[]'
			ELSE json_array(modality) END WHERE modalities_json = '[]'`,
	},
	{
		table: "datasets", column: "platforms_json",
		ddl: "TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(platforms_json) AND json_type(platforms_json) = 'array')",
		backfill: `UPDATE datasets SET platforms_json = json_array(technology)` +
			` WHERE platforms_json = '[]' AND technology IS NOT NULL AND trim(technology) <> ''`,
	},
	{
		table: "source_states", column: "config_json",
		ddl: "TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json))",
	},
}

// healColumns adds missing columns to existing tables and backfills JSON
// values from legacy fields. Fresh databases are left entirely to schema.sql.
func healColumns(db *sql.DB) error {
	for _, table := range []string{"papers", "datasets", "source_states"} {
		rows, err := db.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			return fmt.Errorf("read %s columns: %w", table, err)
		}
		existing := map[string]bool{}
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				_ = rows.Close()
				return fmt.Errorf("read %s columns: %w", table, err)
			}
			existing[name] = true
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read %s columns: %w", table, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("close %s columns: %w", table, err)
		}
		if len(existing) == 0 {
			continue
		}
		for _, migration := range columnMigrations {
			if migration.table != table || existing[migration.column] {
				continue
			}
			if _, err := db.Exec("ALTER TABLE " + table + " ADD COLUMN " + migration.column + " " + migration.ddl); err != nil {
				return fmt.Errorf("migrate %s column %s: %w", table, migration.column, err)
			}
			if migration.backfill != "" {
				if _, err := db.Exec(migration.backfill); err != nil {
					return fmt.Errorf("backfill %s column %s: %w", table, migration.column, err)
				}
			}
		}
	}
	return nil
}

// Store owns the application's SQLite database.
type Store struct {
	db           *sql.DB
	path         string
	sourceSyncMu sync.Mutex
}

// Status summarizes the initialized local library.
type Status struct {
	DatabasePath  string `json:"databasePath"`
	SchemaVersion int    `json:"schemaVersion"`
	TableCount    int    `json:"tableCount"`
	PaperCount    int    `json:"paperCount"`
	DatasetCount  int    `json:"datasetCount"`
	Integrity     string `json:"integrity"`
}

// Open initializes a library database without deleting existing records.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("database path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	// A single connection keeps PRAGMA foreign_keys active for every operation.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	initialized := false
	defer func() {
		if !initialized {
			_ = db.Close()
		}
	}()

	if _, err := db.Exec("PRAGMA busy_timeout = 5000; PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("configure database: %w", err)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return nil, fmt.Errorf("read schema version: %w", err)
	}
	if version < 0 || version > SchemaVersion {
		return nil, fmt.Errorf("unsupported database schema version %d", version)
	}
	if err := healColumns(db); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return nil, fmt.Errorf("initialize schema: %w", err)
	}
	store := &Store{db: db, path: path}
	if err := store.ensureSourceStates(context.Background()); err != nil {
		return nil, err
	}

	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return nil, fmt.Errorf("check foreign keys: %w", err)
	}
	if rows.Next() {
		_ = rows.Close()
		return nil, errors.New("database contains foreign key violations")
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("check foreign keys: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("finish foreign key check: %w", err)
	}

	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return nil, fmt.Errorf("check database integrity: %w", err)
	}
	if integrity != "ok" {
		return nil, fmt.Errorf("database integrity check failed: %s", integrity)
	}

	initialized = true
	return store, nil
}

// Status returns current database metadata and record counts.
func (s *Store) Status() (Status, error) {
	if s == nil || s.db == nil {
		return Status{}, errors.New("library database is not ready")
	}
	status := Status{DatabasePath: s.path, SchemaVersion: SchemaVersion}
	queries := []struct {
		query  string
		target *int
	}{
		{"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'", &status.TableCount},
		{"SELECT count(*) FROM papers", &status.PaperCount},
		{"SELECT count(*) FROM datasets", &status.DatasetCount},
	}
	for _, query := range queries {
		if err := s.db.QueryRow(query.query).Scan(query.target); err != nil {
			return Status{}, fmt.Errorf("read database status: %w", err)
		}
	}
	if err := s.db.QueryRow("PRAGMA integrity_check").Scan(&status.Integrity); err != nil {
		return Status{}, fmt.Errorf("read database status: %w", err)
	}
	return status, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
