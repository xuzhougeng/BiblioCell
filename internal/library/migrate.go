package library

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Schema versions live in SQLite's user_version and in the schema_migrations
// table (one row per applied version, with a description).
//
// baselineVersion is the schema that schema.sql builds for a fresh database in
// one go. Databases older than that (v0.1.0 shipped user_version 11) are first
// brought up to the baseline by the legacy repair steps in store.go, which
// inspect the actual DDL. Everything after the baseline is a numbered entry in
// migrations below, applied in order inside its own transaction.
//
// To add a migration:
//  1. Append an entry to migrations with Version = the previous one + 1.
//  2. Update schema.sql so a fresh database has the same result, and bump its
//     "PRAGMA user_version" to the new number.
//  3. TestMigrationsMatchSchema fails until both agree.
const baselineVersion = 13

// Migration is one schema step. SQL runs first (several statements are fine),
// then Run, both inside a transaction that also records the version. A view
// that needs new columns is dropped here and recreated by schema.sql, which
// runs after every migration.
type Migration struct {
	Version     int
	Description string
	SQL         string
	Run         func(tx *sql.Tx) error
}

// migrations lists every step after the baseline, in order and without gaps.
// Keep entries forever: a database may be several versions behind.
var migrations = []Migration{
	{
		Version:     14,
		Description: "AI 筛选建议记录：模型配置、结论与置信度，用于和人工判定对照",
		SQL: `CREATE TABLE IF NOT EXISTS screening_suggestions (
    id TEXT PRIMARY KEY NOT NULL,
    paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    requirement TEXT NOT NULL,
    -- 设置中的模型配置：profile 名称与模型 id 都存下来，配置改名或删除后仍能对照。
    profile_id TEXT NOT NULL DEFAULT '',
    profile_name TEXT NOT NULL DEFAULT '',
    model TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT '',
    verdict TEXT NOT NULL CHECK (verdict IN ('included','excluded','uncertain')),
    reason TEXT NOT NULL DEFAULT '',
    -- 模型读的是全文还是摘要。
    basis TEXT NOT NULL DEFAULT '',
    -- 模型引用的原文片段及其所在 PDF 页码（0 表示未定位到）。
    excerpt TEXT NOT NULL DEFAULT '',
    page INTEGER NOT NULL DEFAULT 0 CHECK (page >= 0),
    confidence REAL CHECK (confidence IS NULL OR (confidence >= 0 AND confidence <= 1)),
    -- 同一次批量筛选或重跑共用一个 run_id；逐篇筛选时为空。
    run_id TEXT NOT NULL DEFAULT '',
    kind TEXT NOT NULL DEFAULT 'screen' CHECK (kind IN ('screen','batch','rerun')),
    input_tokens INTEGER NOT NULL DEFAULT 0,
    output_tokens INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_screening_suggestions_paper ON screening_suggestions(paper_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_screening_suggestions_run ON screening_suggestions(run_id, created_at DESC);`,
	},
}

// SchemaVersion is the schema this build writes: the baseline plus every
// migration. Databases with a higher user_version come from a newer build and
// are refused.
var SchemaVersion = baselineVersion + len(migrations)

// Upgrade reports what Open did to an outdated database.
type Upgrade struct {
	From   int    `json:"from"`
	To     int    `json:"to"`
	Backup string `json:"backup"`
}

// MigrationInfo describes a migration for the command line.
type MigrationInfo struct {
	Version     int    `json:"version"`
	Description string `json:"description"`
	AppliedAt   string `json:"appliedAt,omitempty"`
}

// VersionInfo is what `db version` prints: where the file stands against this
// build, without changing it.
type VersionInfo struct {
	DatabasePath string          `json:"databasePath"`
	Exists       bool            `json:"exists"`
	Version      int             `json:"version"`
	AppVersion   int             `json:"appVersion"`
	State        string          `json:"state"` // current, outdated, newer or missing
	Applied      []MigrationInfo `json:"applied"`
	Pending      []MigrationInfo `json:"pending"`
}

func init() {
	for i, m := range migrations {
		if m.Version != baselineVersion+i+1 {
			panic(fmt.Sprintf("library: migration %d is out of order; want %d", m.Version, baselineVersion+i+1))
		}
		if m.Description == "" || (m.SQL == "" && m.Run == nil) {
			panic(fmt.Sprintf("library: migration %d needs a description and a step", m.Version))
		}
	}
}

// pendingMigrations lists the steps a database at version needs.
func pendingMigrations(version int) []MigrationInfo {
	pending := []MigrationInfo{}
	if version < baselineVersion {
		pending = append(pending, MigrationInfo{Version: baselineVersion, Description: "基线：按实际表结构补齐列、视图与仓库名称约束"})
	}
	for _, m := range migrations {
		if m.Version > version {
			pending = append(pending, MigrationInfo{Version: m.Version, Description: m.Description})
		}
	}
	return pending
}

// backupPath is where Open copies the database before touching an outdated
// one: beside it, named after the version being left behind.
func backupPath(path string, version int) string {
	return fmt.Sprintf("%s.bak-v%d", path, version)
}

// backupDatabase writes a consistent copy of the open database with VACUUM
// INTO, which is safe while the file is in use. An older backup of the same
// version is replaced: a failed earlier attempt rolled back, so the file holds
// the same state again.
func backupDatabase(db *sql.DB, path string, version int) (string, error) {
	target := backupPath(path, version)
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("replace old backup %s: %w", target, err)
	}
	if _, err := db.Exec("VACUUM INTO '" + strings.ReplaceAll(target, "'", "''") + "'"); err != nil {
		return "", fmt.Errorf("back up database to %s: %w", target, err)
	}
	return target, nil
}

// applyMigrations runs every migration after version, each in a transaction
// that also records the version, so a failure leaves the database at the last
// completed step. Backup names the copy taken beforehand, for the error.
func applyMigrations(db *sql.DB, version int, backup string) error {
	for _, m := range migrations {
		if m.Version <= version {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %d (%s) failed, database left at version %d, backup at %s: %w", m.Version, m.Description, m.Version-1, backup, err)
		}
	}
	return nil
}

func applyMigration(db *sql.DB, m Migration) (err error) {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if m.SQL != "" {
		if _, err = tx.Exec(m.SQL); err != nil {
			return err
		}
	}
	if m.Run != nil {
		if err = m.Run(tx); err != nil {
			return err
		}
	}
	if _, err = tx.Exec("INSERT OR REPLACE INTO schema_migrations(version, description) VALUES (?, ?)", m.Version, m.Description); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	if _, err = tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.Version)); err != nil {
		return fmt.Errorf("stamp version: %w", err)
	}
	return tx.Commit()
}

// recordMigrations writes the schema_migrations rows for every migration on a
// database that schema.sql built directly, where no step actually ran.
func recordMigrations(db *sql.DB) error {
	for _, m := range migrations {
		if _, err := db.Exec("INSERT OR IGNORE INTO schema_migrations(version, description) VALUES (?, ?)", m.Version, m.Description); err != nil {
			return fmt.Errorf("record migration %d: %w", m.Version, err)
		}
	}
	return nil
}

// readVersion returns user_version and whether the file holds any tables yet.
func readVersion(db *sql.DB) (version int, hasTables bool, err error) {
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return 0, false, fmt.Errorf("read schema version: %w", err)
	}
	var tables int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'").Scan(&tables); err != nil {
		return 0, false, fmt.Errorf("read tables: %w", err)
	}
	return version, tables > 0, nil
}

// Inspect reports a database's version without opening it through Open, so
// nothing is migrated or created. A missing file is reported, not made.
func Inspect(path string) (VersionInfo, error) {
	info := VersionInfo{DatabasePath: path, AppVersion: SchemaVersion, Applied: []MigrationInfo{}, Pending: []MigrationInfo{}}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		info.State = "missing"
		info.Pending = pendingMigrations(-1)
		return info, nil
	} else if err != nil {
		return info, fmt.Errorf("stat database: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return info, fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	version, hasTables, err := readVersion(db)
	if err != nil {
		return info, err
	}
	info.Exists = true
	info.Version = version
	if hasTables {
		rows, err := db.Query("SELECT version, description, applied_at FROM schema_migrations ORDER BY version")
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var m MigrationInfo
				if err := rows.Scan(&m.Version, &m.Description, &m.AppliedAt); err != nil {
					return info, fmt.Errorf("read applied migrations: %w", err)
				}
				info.Applied = append(info.Applied, m)
			}
			if err := rows.Err(); err != nil {
				return info, fmt.Errorf("read applied migrations: %w", err)
			}
		}
	}
	switch {
	case version > SchemaVersion:
		info.State = "newer"
	case version == SchemaVersion:
		info.State = "current"
	default:
		info.State = "outdated"
		info.Pending = pendingMigrations(version)
	}
	return info, nil
}

// Migrate brings the database up to date the way the app does on start and
// reports what changed; nil when it was already current.
func Migrate(path string) (*Upgrade, error) {
	store, err := Open(path)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.Upgraded, nil
}
