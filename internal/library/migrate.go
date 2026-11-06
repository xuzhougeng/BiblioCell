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
	{
		Version:     15,
		Description: "按原文与提示词缓存翻译结果",
		SQL: `CREATE TABLE IF NOT EXISTS translation_cache (
    cache_key TEXT PRIMARY KEY NOT NULL,
    translated_text TEXT NOT NULL CHECK (length(trim(translated_text)) > 0),
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`,
	},
	{
		Version:     16,
		Description: "AI 建议记录所属的判断任务与采纳落点（收录、文章类型或标签）",
		// A database brought up from before v14 already has these columns from
		// schema.sql, so each one is added only when missing.
		Run: func(tx *sql.Tx) error {
			return addColumns(tx, "screening_suggestions", []columnSpec{
				{"task_id", "TEXT NOT NULL DEFAULT ''"},
				{"task_name", "TEXT NOT NULL DEFAULT ''"},
				{"target_kind", "TEXT NOT NULL DEFAULT 'screening' CHECK (target_kind IN ('screening','articleType','tag'))"},
				{"target_value", "TEXT NOT NULL DEFAULT ''"},
			})
		},
	},
	{
		Version:     17,
		Description: "缓存全文文件逐页提取的文本，供全文检索与原文定位",
		SQL: `CREATE TABLE IF NOT EXISTS fulltext_text (
    resource_id TEXT PRIMARY KEY NOT NULL REFERENCES resources(id) ON DELETE CASCADE,
    -- 文件大小与修改时间一起作为缓存键：文件被替换后重新提取。
    file_size INTEGER NOT NULL CHECK (file_size >= 0),
    file_mtime TEXT NOT NULL,
    -- 每页一项的 JSON 数组；提取失败时为空数组并记录 error，避免每次检索都重读坏文件。
    pages TEXT NOT NULL DEFAULT '[]',
    error TEXT NOT NULL DEFAULT '',
    extracted_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`,
	},
	{
		Version:     18,
		Description: "合并重复文献：记录合并掉的文献标识，来源更新与导入不再把它加回来",
		SQL: `CREATE TABLE IF NOT EXISTS merged_papers (
    merged_id TEXT PRIMARY KEY NOT NULL,
    -- 保留的文献；它被删除时这些记录随之删除。
    paper_id TEXT NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    doi TEXT NOT NULL DEFAULT '',
    pmid TEXT NOT NULL DEFAULT '',
    publication_state TEXT NOT NULL DEFAULT 'unknown',
    reason TEXT NOT NULL DEFAULT '',
    -- 由谁合并：desktop、MCP 或 CLI。
    merged_via TEXT NOT NULL DEFAULT '',
    merged_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE INDEX IF NOT EXISTS idx_merged_papers_paper ON merged_papers(paper_id);
CREATE INDEX IF NOT EXISTS idx_merged_papers_doi ON merged_papers(doi) WHERE doi <> '';
CREATE INDEX IF NOT EXISTS idx_merged_papers_pmid ON merged_papers(pmid) WHERE pmid <> '';`,
	},
	{
		Version:     19,
		Description: "收录状态新增“无关”：标无关的文献留在库中只为去重，附件清理后来源更新与导入不再加回",
		Run:         rebuildPaperCuration,
	},
	{Version: 20, Description: "数据集资源清单、样本与参考版本、存储位置", SQL: datasetAssetsDDL},
	{Version: 21, Description: "文献与数据集回收站，保留去重标识与可恢复关联", SQL: trashDDL + `DROP VIEW IF EXISTS v_work_queue; DROP VIEW IF EXISTS v_paper_overview; DROP VIEW IF EXISTS v_dataset_overview;`},
	{Version: 22, Description: "所有人工收录判断的理由均为可选，相关文献保留备查", Run: optionalScreeningReasons},
}

// paperCurationDDL is paper_curation as schema.sql defines it, for the rebuild
// below. SQLite cannot change a CHECK constraint in place.
const paperCurationDDL = `CREATE TABLE IF NOT EXISTS paper_curation (
    paper_id TEXT PRIMARY KEY NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
    -- excluded 表示相关、保留备查；irrelevant 兼容旧版无关标记。
    -- 新的“不相关”操作使用回收站，保留可恢复的附件、笔记与关联。
    screening_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (screening_status IN ('pending','included','excluded','uncertain','irrelevant')),
    screening_reason TEXT,
    screening_method TEXT NOT NULL DEFAULT 'unreviewed'
        CHECK (screening_method IN ('unreviewed','rule','model','human')),
    screening_reviewed_at TEXT,
    -- screening_status 只记录人的决定；agent 的判断只能写入 proposed_*，等人确认。
    proposed_screening TEXT NOT NULL DEFAULT ''
        CHECK (proposed_screening IN ('','included','excluded','uncertain')),
    proposed_reason TEXT NOT NULL DEFAULT '',
    -- 来源更新新增的文献进入待核查，人做出收录判断或标记核查完成后移出。
    awaiting_review INTEGER NOT NULL DEFAULT 0 CHECK (awaiting_review IN (0,1)),
    reading_status TEXT NOT NULL DEFAULT 'unread'
        CHECK (reading_status IN ('unread','skimmed','read')),
    priority TEXT NOT NULL DEFAULT 'normal'
        CHECK (priority IN ('low','normal','high')),
    starred INTEGER NOT NULL DEFAULT 0 CHECK (starred IN (0,1)),
    tags_json TEXT NOT NULL DEFAULT '[]'
        CHECK (json_valid(tags_json) AND json_type(tags_json) = 'array'),
    -- NULL 表示使用来源值；[] / 空字符串可显式覆盖成空。采集程序不得写这些列。
    organs_override_json TEXT CHECK (organs_override_json IS NULL OR
        (json_valid(organs_override_json) AND json_type(organs_override_json) = 'array')),
    modalities_override_json TEXT CHECK (modalities_override_json IS NULL OR
        (json_valid(modalities_override_json) AND json_type(modalities_override_json) = 'array')),
    conditions_override_text TEXT,
    notes TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
)`

// paperCurationColumns lists the columns copied across the rebuild, by name:
// the order of an old table must not matter.
const paperCurationColumns = `paper_id, screening_status, screening_reason, screening_method, screening_reviewed_at,
	proposed_screening, proposed_reason, awaiting_review, reading_status, priority, starred, tags_json,
	organs_override_json, modalities_override_json, conditions_override_text, notes, updated_at`

// rebuildPaperCuration recreates paper_curation so screening_status admits
// 'irrelevant'. The views and trigger over it are dropped first, because
// renaming a table checks that every view still parses; schema.sql recreates
// them after the migration. A table that already admits the value (built by a
// newer schema.sql on the way here) is left alone, as is a pre-release database
// that has no table yet.
func rebuildPaperCuration(tx *sql.Tx) error {
	var ddl string
	err := tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'paper_curation'").Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // A pre-release database without the table: schema.sql creates it afterwards.
	}
	if err != nil {
		return fmt.Errorf("read paper_curation: %w", err)
	}
	if strings.Contains(ddl, "'irrelevant'") {
		return nil
	}
	return replacePaperCuration(tx)
}

// optionalScreeningReasons also upgrades v19-v21 databases whose table already
// accepts irrelevant, but still requires a reason for related papers.
func optionalScreeningReasons(tx *sql.Tx) error {
	var ddl string
	err := tx.QueryRow("SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'paper_curation'").Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read paper_curation: %w", err)
	}
	if !strings.Contains(ddl, "screening_status <> 'excluded'") {
		return nil
	}
	return replacePaperCuration(tx)
}

func replacePaperCuration(tx *sql.Tx) error {
	steps := []string{
		"DROP VIEW IF EXISTS v_work_queue",
		"DROP VIEW IF EXISTS v_paper_overview",
		"DROP TRIGGER IF EXISTS tr_paper_curation_init",
		"ALTER TABLE paper_curation RENAME TO paper_curation_old",
		paperCurationDDL,
		"INSERT INTO paper_curation(" + paperCurationColumns + ") SELECT " + paperCurationColumns + " FROM paper_curation_old",
		"DROP TABLE paper_curation_old",
	}
	for _, step := range steps {
		if _, err := tx.Exec(step); err != nil {
			return fmt.Errorf("rebuild paper_curation: %w", err)
		}
	}
	return nil
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

// columnSpec is one column a migration adds when the table lacks it.
type columnSpec struct {
	name string
	ddl  string
}

// addColumns adds each missing column, so a migration stays correct for a
// database whose table was created by a newer schema.sql on the way here.
func addColumns(tx *sql.Tx, table string, columns []columnSpec) error {
	rows, err := tx.Query("PRAGMA table_info(" + table + ")")
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
	if err := rows.Close(); err != nil {
		return fmt.Errorf("read %s columns: %w", table, err)
	}
	for _, column := range columns {
		if existing[column.name] {
			continue
		}
		if _, err := tx.Exec("ALTER TABLE " + table + " ADD COLUMN " + column.name + " " + column.ddl); err != nil {
			return fmt.Errorf("add %s.%s: %w", table, column.name, err)
		}
	}
	return nil
}
