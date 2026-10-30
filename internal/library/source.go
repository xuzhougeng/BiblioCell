package library

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// SourceConfig is the user-editable configuration for one remote connector.
type SourceConfig struct {
	ID         string `json:"id"`
	Enabled    bool   `json:"enabled"`
	Query      string `json:"query"`
	MaxResults int    `json:"maxResults"`
	Provider   string `json:"provider"`
}

// HarvestRun describes the most recent synchronization attempt.
type HarvestRun struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	StartedAt      string `json:"startedAt"`
	FinishedAt     string `json:"finishedAt"`
	RecordsSeen    int    `json:"recordsSeen"`
	RecordsNew     int    `json:"recordsNew"`
	RecordsUpdated int    `json:"recordsUpdated"`
	ErrorMessage   string `json:"errorMessage"`
}

// SourceRecord is a compact view of an external record retained in SQLite.
type SourceRecord struct {
	ExternalID string `json:"externalId"`
	Title      string `json:"title"`
	URL        string `json:"url"`
	PaperID    string `json:"paperId"`
	LastSeenAt string `json:"lastSeenAt"`
}

// SourceStatus combines connector metadata, saved configuration, and run state.
type SourceStatus struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Kind          string         `json:"kind"`
	Description   string         `json:"description"`
	HomeURL       string         `json:"homeUrl"`
	Supported     bool           `json:"supported"`
	Enabled       bool           `json:"enabled"`
	Query         string         `json:"query"`
	MaxResults    int            `json:"maxResults"`
	Provider      string         `json:"provider"`
	RecordCount   int            `json:"recordCount"`
	LastSuccessAt string         `json:"lastSuccessAt"`
	LastRun       *HarvestRun    `json:"lastRun"`
	RecentRecords []SourceRecord `json:"recentRecords"`
}

// SyncResult reports exactly what a completed manual synchronization changed.
type SyncResult struct {
	SourceID       string       `json:"sourceId"`
	RecordsSeen    int          `json:"recordsSeen"`
	RecordsNew     int          `json:"recordsNew"`
	RecordsUpdated int          `json:"recordsUpdated"`
	PapersAdded    int          `json:"papersAdded"`
	Duplicates     int          `json:"duplicates"`
	Status         SourceStatus `json:"status"`
	Papers         []Paper      `json:"papers"`
}

type sourceDefinition struct {
	id          string
	name        string
	kind        string
	description string
	homeURL     string
	supported   bool
	defaults    SourceConfig
}

var sourceDefinitions = []sourceDefinition{
	{
		id: "pubmed", name: "PubMed", kind: "literature", supported: true,
		description: "通过 NCBI E-utilities 检索论文与摘要，保留 PMID 和 DOI。",
		homeURL:     "https://pubmed.ncbi.nlm.nih.gov/",
		defaults: SourceConfig{ID: "pubmed", Enabled: true, MaxResults: 20,
			Query: `("single cell"[Title/Abstract] OR "single-cell"[Title/Abstract] OR "single nucleus"[Title/Abstract]) AND (plant[Title/Abstract] OR Arabidopsis[Title/Abstract])`},
	},
	{
		id: "europe-pmc", name: "Europe PMC", kind: "literature", supported: true,
		description: "检索 Europe PMC 核心元数据，保存开放全文线索所需的标识符。",
		homeURL:     "https://europepmc.org/",
		defaults: SourceConfig{ID: "europe-pmc", Enabled: true, MaxResults: 20,
			Query: `(TITLE_ABS:"single cell" OR TITLE_ABS:"single-cell" OR TITLE_ABS:"single nucleus") AND (TITLE_ABS:plant OR TITLE_ABS:Arabidopsis)`},
	},
	{
		id: "crossref", name: "Crossref", kind: "literature", supported: true,
		description: "按书目信息检索 Crossref，保存 DOI、作者和出版信息。",
		homeURL:     "https://www.crossref.org/",
		defaults: SourceConfig{ID: "crossref", Enabled: true, MaxResults: 20,
			Query: "plant single cell RNA sequencing"},
	},
	{
		id: "biorxiv", name: "bioRxiv", kind: "literature", supported: true,
		description: "扫描最近 30 天的 bioRxiv 预印本，在本地按题目、摘要和分类匹配关键词。",
		homeURL:     "https://www.biorxiv.org/",
		defaults: SourceConfig{ID: "biorxiv", Enabled: true, MaxResults: 20,
			Query: "single cell plant"},
	},
	{
		id: "sequence-repositories", name: "GEO / SRA / ENA", kind: "repository", supported: true,
		description: "检索 GEO、SRA 或 ENA，保存 accession 和官方页面线索，不自动关联论文。",
		homeURL:     "https://www.ebi.ac.uk/ena/browser/home",
		defaults: SourceConfig{ID: "sequence-repositories", Enabled: true, MaxResults: 20, Provider: "geo",
			Query: `("single cell"[All Fields] OR "single-cell"[All Fields] OR "single nucleus"[All Fields]) AND (plant[All Fields] OR Arabidopsis[Organism]) AND gse[ETYP]`},
	},
	{
		id: "cncb-gsa", name: "CNCB GSA", kind: "repository", supported: false,
		description: "站点会要求浏览器安全检查，当前仅保留官方入口，不绕过验证自动抓取。",
		homeURL:     "https://ngdc.cncb.ac.cn/gsa/",
		defaults:    SourceConfig{ID: "cncb-gsa", Enabled: false, MaxResults: 20},
	},
}

func sourceDefinitionByID(id string) (sourceDefinition, bool) {
	for _, definition := range sourceDefinitions {
		if definition.id == id {
			return definition, true
		}
	}
	return sourceDefinition{}, false
}

func validateSourceConfig(config SourceConfig) (SourceConfig, error) {
	definition, ok := sourceDefinitionByID(strings.TrimSpace(config.ID))
	if !ok {
		return SourceConfig{}, fmt.Errorf("unknown source %q", config.ID)
	}
	config.ID = definition.id
	config.Query = strings.TrimSpace(config.Query)
	config.Provider = strings.TrimSpace(config.Provider)
	if definition.supported && config.Query == "" {
		return SourceConfig{}, errors.New("source query is required")
	}
	if len(config.Query) > 4000 {
		return SourceConfig{}, errors.New("source query cannot exceed 4000 bytes")
	}
	if config.MaxResults < 1 || config.MaxResults > 100 {
		return SourceConfig{}, errors.New("maximum results must be between 1 and 100")
	}
	if definition.id == "sequence-repositories" {
		if !oneOf(config.Provider, "geo", "sra", "ena") {
			return SourceConfig{}, errors.New("repository provider must be geo, sra, or ena")
		}
	} else {
		config.Provider = ""
	}
	return config, nil
}

func (s *Store) ensureSourceStates(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin source initialization: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, definition := range sourceDefinitions {
		config, _ := json.Marshal(definition.defaults)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO source_states(source_id, name, source_kind, base_url, enabled, query_text, config_json)
			VALUES(?, ?, ?, ?, ?, ?, ?) ON CONFLICT(source_id) DO NOTHING`,
			definition.id, definition.name, definition.kind, definition.homeURL,
			boolInt(definition.defaults.Enabled), definition.defaults.Query, config); err != nil {
			return fmt.Errorf("initialize source %s: %w", definition.id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit source initialization: %w", err)
	}
	return nil
}

// SaveSourceConfig validates and persists one connector in the real database.
func (s *Store) SaveSourceConfig(ctx context.Context, config SourceConfig) (SourceStatus, error) {
	if s == nil || s.db == nil {
		return SourceStatus{}, errors.New("library database is not ready")
	}
	config, err := validateSourceConfig(config)
	if err != nil {
		return SourceStatus{}, err
	}
	definition, _ := sourceDefinitionByID(config.ID)
	encoded, _ := json.Marshal(config)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO source_states(source_id, name, source_kind, base_url, enabled, query_text, config_json)
		VALUES(?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(source_id) DO UPDATE SET enabled = excluded.enabled,
			query_text = excluded.query_text, config_json = excluded.config_json`,
		config.ID, definition.name, definition.kind, definition.homeURL,
		boolInt(config.Enabled), config.Query, encoded)
	if err != nil {
		return SourceStatus{}, fmt.Errorf("save source configuration: %w", err)
	}
	return s.SourceStatus(ctx, config.ID)
}

func sourceConfigFromRow(id string, enabled int, query string, encoded string) SourceConfig {
	definition, _ := sourceDefinitionByID(id)
	config := definition.defaults
	_ = json.Unmarshal([]byte(encoded), &config)
	config.ID = id
	config.Enabled = enabled == 1
	config.Query = strings.TrimSpace(query)
	if config.MaxResults < 1 || config.MaxResults > 100 {
		config.MaxResults = definition.defaults.MaxResults
	}
	if definition.id == "sequence-repositories" && !oneOf(config.Provider, "geo", "sra", "ena") {
		config.Provider = definition.defaults.Provider
	}
	if definition.id != "sequence-repositories" {
		config.Provider = ""
	}
	return config
}

// ListSources returns all connector configurations and their latest local state.
func (s *Store) ListSources(ctx context.Context) ([]SourceStatus, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("library database is not ready")
	}
	if err := s.ensureSourceStates(ctx); err != nil {
		return nil, err
	}
	statuses := make([]SourceStatus, 0, len(sourceDefinitions))
	for _, definition := range sourceDefinitions {
		status, err := s.SourceStatus(ctx, definition.id)
		if err != nil {
			return nil, err
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// SourceStatus reads one connector without contacting its remote service.
func (s *Store) SourceStatus(ctx context.Context, id string) (SourceStatus, error) {
	definition, ok := sourceDefinitionByID(id)
	if !ok {
		return SourceStatus{}, fmt.Errorf("unknown source %q", id)
	}
	status := SourceStatus{
		ID: definition.id, Name: definition.name, Kind: definition.kind,
		Description: definition.description, HomeURL: definition.homeURL,
		Supported: definition.supported, RecentRecords: []SourceRecord{},
	}
	var enabled int
	var encoded string
	err := s.db.QueryRowContext(ctx, `
		SELECT enabled, coalesce(query_text, ''), coalesce(config_json, '{}'),
		       coalesce(last_success_at, ''),
		       (SELECT count(DISTINCT external_id) FROM source_records WHERE source_id = source_states.source_id)
		FROM source_states WHERE source_id = ?`, id).
		Scan(&enabled, &status.Query, &encoded, &status.LastSuccessAt, &status.RecordCount)
	if errors.Is(err, sql.ErrNoRows) {
		config := definition.defaults
		status.Enabled, status.Query, status.MaxResults, status.Provider = config.Enabled, config.Query, config.MaxResults, config.Provider
		return status, nil
	}
	if err != nil {
		return SourceStatus{}, fmt.Errorf("read source %s: %w", id, err)
	}
	config := sourceConfigFromRow(id, enabled, status.Query, encoded)
	status.Enabled, status.MaxResults, status.Provider = config.Enabled, config.MaxResults, config.Provider

	var run HarvestRun
	err = s.db.QueryRowContext(ctx, `
		SELECT id, status, started_at, coalesce(finished_at, ''), records_seen,
		       records_new, records_updated, coalesce(error_message, '')
		FROM harvest_runs WHERE source_id = ? ORDER BY started_at DESC, id DESC LIMIT 1`, id).
		Scan(&run.ID, &run.Status, &run.StartedAt, &run.FinishedAt, &run.RecordsSeen,
			&run.RecordsNew, &run.RecordsUpdated, &run.ErrorMessage)
	if err == nil {
		status.LastRun = &run
	} else if !errors.Is(err, sql.ErrNoRows) {
		return SourceStatus{}, fmt.Errorf("read source run %s: %w", id, err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT external_id,
		       coalesce(nullif(json_extract(payload_json, '$.title'), ''),
		                nullif(json_extract(payload_json, '$.study_title'), ''), external_id),
		       coalesce(source_url, ''), coalesce(paper_id, ''), last_seen_at
		FROM source_records WHERE source_id = ?
		GROUP BY external_id ORDER BY last_seen_at DESC, external_id LIMIT 8`, id)
	if err != nil {
		return SourceStatus{}, fmt.Errorf("read source records %s: %w", id, err)
	}
	defer rows.Close()
	for rows.Next() {
		var record SourceRecord
		if err := rows.Scan(&record.ExternalID, &record.Title, &record.URL, &record.PaperID, &record.LastSeenAt); err != nil {
			return SourceStatus{}, fmt.Errorf("read source record %s: %w", id, err)
		}
		status.RecentRecords = append(status.RecentRecords, record)
	}
	if err := rows.Err(); err != nil {
		return SourceStatus{}, fmt.Errorf("read source records %s: %w", id, err)
	}
	return status, nil
}

type harvestRecord struct {
	ExternalID string
	Title      string
	URL        string
	Paper      *Paper
	Payload    any
}

type sourceHarvester interface {
	Harvest(context.Context, SourceConfig) ([]harvestRecord, error)
}

// SyncSource performs one manual fetch and atomically commits its local records.
func (s *Store) SyncSource(ctx context.Context, id string) (SyncResult, error) {
	return s.syncSource(ctx, id, newRemoteHarvester())
}

func (s *Store) syncSource(ctx context.Context, id string, harvester sourceHarvester) (SyncResult, error) {
	if s == nil || s.db == nil {
		return SyncResult{}, errors.New("library database is not ready")
	}
	definition, ok := sourceDefinitionByID(id)
	if !ok {
		return SyncResult{}, fmt.Errorf("unknown source %q", id)
	}
	if !definition.supported {
		return SyncResult{}, errors.New("this source only offers a manual official link")
	}
	s.sourceSyncMu.Lock()
	defer s.sourceSyncMu.Unlock()

	status, err := s.SourceStatus(ctx, id)
	if err != nil {
		return SyncResult{}, err
	}
	config := SourceConfig{ID: id, Enabled: status.Enabled, Query: status.Query, MaxResults: status.MaxResults, Provider: status.Provider}
	config, err = validateSourceConfig(config)
	if err != nil {
		return SyncResult{}, err
	}
	if !config.Enabled {
		return SyncResult{}, errors.New("enable and save this source before synchronizing")
	}

	runID, err := newLocalID("HR")
	if err != nil {
		return SyncResult{}, err
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO harvest_runs(id, source_id, status, query_text, started_at)
		VALUES(?, ?, 'running', ?, ?)`, runID, id, config.Query, startedAt); err != nil {
		return SyncResult{}, fmt.Errorf("start source run: %w", err)
	}

	records, harvestErr := harvester.Harvest(ctx, config)
	if harvestErr != nil {
		s.failSourceRun(runID, harvestErr)
		return SyncResult{}, harvestErr
	}
	result, err := s.commitSourceRecords(ctx, id, runID, config.Query, records)
	if err != nil {
		s.failSourceRun(runID, err)
		return SyncResult{}, err
	}
	result.SourceID = id
	result.Status, err = s.SourceStatus(ctx, id)
	if err != nil {
		return SyncResult{}, err
	}
	result.Papers, err = s.ListPapers(ctx)
	if err != nil {
		return SyncResult{}, err
	}
	return result, nil
}

func (s *Store) failSourceRun(runID string, runErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	message := runErr.Error()
	if len(message) > 4000 {
		message = message[:4000]
	}
	_, _ = s.db.ExecContext(ctx, `
		UPDATE harvest_runs SET status = 'failed', finished_at = ?, error_message = ?
		WHERE id = ? AND status = 'running'`, time.Now().UTC().Format(time.RFC3339Nano), message, runID)
}

func (s *Store) commitSourceRecords(ctx context.Context, sourceID, runID, query string, records []harvestRecord) (SyncResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return SyncResult{}, fmt.Errorf("begin source commit: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result := SyncResult{RecordsSeen: len(records)}

	for i, record := range records {
		if strings.TrimSpace(record.ExternalID) == "" {
			return SyncResult{}, fmt.Errorf("source record %d has no external identifier", i)
		}
		payload, err := json.Marshal(record.Payload)
		if err != nil {
			return SyncResult{}, fmt.Errorf("encode source record %s: %w", record.ExternalID, err)
		}
		if !json.Valid(payload) {
			return SyncResult{}, fmt.Errorf("source record %s has invalid JSON", record.ExternalID)
		}
		hash := sha256.Sum256(payload)
		paperID := ""
		if record.Paper != nil {
			paper := *record.Paper
			normalizePaper(&paper)
			paperID, err = existingPaperID(ctx, tx, paper)
			if err != nil {
				return SyncResult{}, err
			}
			if paperID == "" {
				paperID, err = newPaperID()
				if err != nil {
					return SyncResult{}, err
				}
				paper.ID = paperID
				if err := savePaper(ctx, tx, &paper); err != nil {
					return SyncResult{}, fmt.Errorf("save harvested paper %s: %w", record.ExternalID, err)
				}
				result.PapersAdded++
			} else {
				result.Duplicates++
			}
		}

		var previous int
		if err := tx.QueryRowContext(ctx,
			"SELECT count(*) FROM source_records WHERE source_id = ? AND external_id = ?",
			sourceID, record.ExternalID).Scan(&previous); err != nil {
			return SyncResult{}, fmt.Errorf("check source record %s: %w", record.ExternalID, err)
		}
		recordID, err := newLocalID("SR")
		if err != nil {
			return SyncResult{}, err
		}
		insert, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO source_records(
				id, source_id, external_id, harvest_run_id, payload_sha256, payload_json,
				source_url, first_seen_at, last_seen_at, paper_id)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			recordID, sourceID, record.ExternalID, runID, hex.EncodeToString(hash[:]), payload,
			record.URL, now, now, nullableString(paperID))
		if err != nil {
			return SyncResult{}, fmt.Errorf("save source record %s: %w", record.ExternalID, err)
		}
		inserted, err := insert.RowsAffected()
		if err != nil {
			return SyncResult{}, err
		}
		if previous == 0 {
			result.RecordsNew++
		} else if inserted == 1 {
			result.RecordsUpdated++
		}
		if inserted == 0 {
			if err := tx.QueryRowContext(ctx, `
				SELECT id FROM source_records WHERE source_id = ? AND external_id = ? AND payload_sha256 = ?`,
				sourceID, record.ExternalID, hex.EncodeToString(hash[:])).Scan(&recordID); err != nil {
				return SyncResult{}, fmt.Errorf("locate source record %s: %w", record.ExternalID, err)
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE source_records SET harvest_run_id = ?, last_seen_at = ?, paper_id = coalesce(?, paper_id)
				WHERE id = ?`, runID, now, nullableString(paperID), recordID); err != nil {
				return SyncResult{}, fmt.Errorf("refresh source record %s: %w", record.ExternalID, err)
			}
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE harvest_runs SET status = 'succeeded', finished_at = ?, records_seen = ?,
			records_new = ?, records_updated = ? WHERE id = ? AND status = 'running'`,
		now, result.RecordsSeen, result.RecordsNew, result.RecordsUpdated, runID); err != nil {
		return SyncResult{}, fmt.Errorf("finish source run: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE source_states SET committed_watermark = ?, last_successful_run_id = ?, last_success_at = ?
		WHERE source_id = ?`, now, runID, now, sourceID); err != nil {
		return SyncResult{}, fmt.Errorf("save source checkpoint: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return SyncResult{}, fmt.Errorf("commit source run: %w", err)
	}
	return result, nil
}

func existingPaperID(ctx context.Context, tx *sql.Tx, paper Paper) (string, error) {
	if paper.DOI != "" {
		var id string
		err := tx.QueryRowContext(ctx,
			"SELECT paper_id FROM paper_identifiers WHERE namespace = 'doi' AND value = ?", paper.DOI).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("check harvested DOI: %w", err)
		}
	}
	if paper.PMID != "" {
		var id string
		err := tx.QueryRowContext(ctx,
			"SELECT paper_id FROM paper_identifiers WHERE namespace = 'pmid' AND value = ?", paper.PMID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("check harvested PMID: %w", err)
		}
	}
	var id string
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM papers WHERE lower(trim(title)) = lower(trim(?)) LIMIT 1", paper.Title).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("check harvested title: %w", err)
	}
	return "", nil
}

func newLocalID(prefix string) (string, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate local identifier: %w", err)
	}
	return prefix + "-" + hex.EncodeToString(suffix[:]), nil
}
