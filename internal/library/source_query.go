package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"
)

// QueryChange is one edit of a source's query, kept so a person can see who
// changed it, why, and restore what it replaced.
type QueryChange struct {
	ID               string `json:"id"`
	SourceID         string `json:"sourceId"`
	Version          string `json:"version"`
	Query            string `json:"query"`
	Provider         string `json:"provider"`
	PreviousQuery    string `json:"previousQuery"`
	PreviousProvider string `json:"previousProvider"`
	ChangedBy        string `json:"changedBy"`
	Reason           string `json:"reason"`
	ChangedAt        string `json:"changedAt"`
}

// querySyntax tells an agent how each connector reads its query.
var querySyntax = map[string]string{
	"pubmed":                `PubMed search syntax with field tags, e.g. ("single cell"[Title/Abstract]) AND Arabidopsis[Title/Abstract]; newest first`,
	"europe-pmc":            `Europe PMC search syntax, e.g. TITLE_ABS:"single cell" AND TITLE_ABS:plant; ranked by relevance`,
	"crossref":              "Plain bibliographic keywords; Crossref ranks by relevance and has no boolean operators",
	"biorxiv":               "Space-separated plain words, every word must appear in title, abstract or category; scans only the last 30 days",
	"sequence-repositories": `GEO and SRA take NCBI Entrez syntax (GEO: add gse[ETYP] for series); ENA takes Portal API syntax for read_study, e.g. description="*single*" AND tax_tree(33090)`,
}

func (s *Store) queryHistory(ctx context.Context, sourceID string, limit int) ([]QueryChange, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source_id, query_version, query_text, provider, previous_query, previous_provider,
		       changed_by, reason, changed_at
		FROM source_query_history WHERE source_id = ?
		ORDER BY changed_at DESC, id DESC LIMIT ?`, sourceID, limit)
	if err != nil {
		return nil, fmt.Errorf("read query history %s: %w", sourceID, err)
	}
	defer rows.Close()
	changes := []QueryChange{}
	for rows.Next() {
		var c QueryChange
		if err := rows.Scan(&c.ID, &c.SourceID, &c.Version, &c.Query, &c.Provider, &c.PreviousQuery,
			&c.PreviousProvider, &c.ChangedBy, &c.Reason, &c.ChangedAt); err != nil {
			return nil, fmt.Errorf("read query history %s: %w", sourceID, err)
		}
		changes = append(changes, c)
	}
	return changes, rows.Err()
}

// SourceQueryHistory lists a source's query changes, newest first.
func (s *Store) SourceQueryHistory(ctx context.Context, sourceID string) ([]QueryChange, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("library database is not ready")
	}
	if _, ok := sourceDefinitionByID(sourceID); !ok {
		return nil, fmt.Errorf("unknown source %q", sourceID)
	}
	return s.queryHistory(ctx, sourceID, 50)
}

// QueryUpdate is an agent's change to a source's query, with its reason.
type QueryUpdate struct {
	SourceID   string `json:"sourceId" jsonschema:"Source to change, from list_sources"`
	Query      string `json:"query" jsonschema:"The new query in the source's own syntax"`
	Provider   string `json:"provider,omitempty" jsonschema:"sequence-repositories only: geo, sra, or ena; keeps the current one when empty"`
	MaxResults int    `json:"maxResults,omitempty" jsonschema:"Records saved per update, 1 to 100; keeps the current limit when 0"`
	Reason     string `json:"reason" jsonschema:"Why the new query is better, citing what try_source_query showed for the old and new query; a person reads this"`
}

// UpdateSourceQuery saves an agent's new query. It keeps the source enabled or
// disabled as a person left it and records the change for them to review.
func (s *Store) UpdateSourceQuery(ctx context.Context, update QueryUpdate) (SourceStatus, error) {
	reason := strings.TrimSpace(update.Reason)
	if reason == "" {
		return SourceStatus{}, errors.New("reason is required: say what the trial showed and why the new query is better")
	}
	if utf8.RuneCountInString(reason) > 2000 {
		return SourceStatus{}, errors.New("reason cannot exceed 2000 characters")
	}
	current, err := s.SourceStatus(ctx, strings.TrimSpace(update.SourceID))
	if err != nil {
		return SourceStatus{}, err
	}
	if !current.Supported {
		return SourceStatus{}, errors.New("this source only offers a manual official link")
	}
	config := SourceConfig{ID: current.ID, Enabled: current.Enabled, Query: update.Query,
		MaxResults: current.MaxResults, Provider: current.Provider}
	if update.MaxResults != 0 {
		config.MaxResults = update.MaxResults
	}
	if strings.TrimSpace(update.Provider) != "" {
		config.Provider = update.Provider
	}
	config, err = validateSourceConfig(config)
	if err != nil {
		return SourceStatus{}, err
	}
	if config.Query == current.Query && config.Provider == current.Provider && config.MaxResults == current.MaxResults {
		return SourceStatus{}, errors.New("nothing changed: the source already uses this query")
	}
	return s.saveSourceConfig(ctx, config, "agent", reason)
}

// QueryTrialInput asks how a query would do without saving anything.
type QueryTrialInput struct {
	SourceID string `json:"sourceId" jsonschema:"Source to search, from list_sources"`
	Query    string `json:"query,omitempty" jsonschema:"Query to try in the source's syntax; the saved query when empty, to get a baseline"`
	Provider string `json:"provider,omitempty" jsonschema:"sequence-repositories only: geo, sra, or ena; the saved one when empty"`
	Limit    int    `json:"limit,omitempty" jsonschema:"Records to fetch, 1 to 100; defaults to the source's maxResults"`
}

// QueryTrial reports what a query fetches and how that compares with what the
// library already holds and what a person decided about it.
type QueryTrial struct {
	SourceID string `json:"sourceId"`
	Query    string `json:"query"`
	Provider string `json:"provider,omitempty"`
	// HitCount is the source's total for the query, nil where the source
	// cannot say (bioRxiv, ENA) or the count request failed.
	HitCount *int `json:"hitCount"`
	Fetched  int  `json:"fetched"`
	// New are records the library does not hold; the rest match a paper or
	// dataset, broken down by a person's decision on it.
	New              int           `json:"new"`
	InLibrary        int           `json:"inLibrary"`
	Included         int           `json:"included"`
	Excluded         int           `json:"excluded"`
	Uncertain        int           `json:"uncertain"`
	Undecided        int           `json:"undecided"`
	AlreadyHarvested int           `json:"alreadyHarvested"`
	Records          []TrialRecord `json:"records"`
}

// TrialRecord is one fetched record, with enough text to judge its relevance.
type TrialRecord struct {
	ExternalID string `json:"externalId"`
	Title      string `json:"title"`
	Year       *int   `json:"year,omitempty"`
	URL        string `json:"url"`
	Abstract   string `json:"abstract,omitempty"`
	// PaperID or DatasetID names the matching library record; Decision is a
	// person's verdict on it: included, excluded, uncertain, or pending.
	PaperID          string `json:"paperId,omitempty"`
	DatasetID        string `json:"datasetId,omitempty"`
	Decision         string `json:"decision,omitempty"`
	AlreadyHarvested bool   `json:"alreadyHarvested,omitempty"`
}

// hitCounter is implemented by harvesters that can report a query's total.
type hitCounter interface {
	Count(context.Context, SourceConfig) (int, error)
}

// TrySourceQuery runs a query against the remote source and evaluates the
// result against the library. It writes nothing: no records, runs, or papers.
func (s *Store) TrySourceQuery(ctx context.Context, input QueryTrialInput, credentials func() SourceCredentials) (QueryTrial, error) {
	return s.trySourceQuery(ctx, input, newRemoteHarvester(credentials))
}

func (s *Store) trySourceQuery(ctx context.Context, input QueryTrialInput, harvester sourceHarvester) (QueryTrial, error) {
	if s == nil || s.db == nil {
		return QueryTrial{}, errors.New("library database is not ready")
	}
	current, err := s.SourceStatus(ctx, strings.TrimSpace(input.SourceID))
	if err != nil {
		return QueryTrial{}, err
	}
	if !current.Supported {
		return QueryTrial{}, errors.New("this source only offers a manual official link")
	}
	config := SourceConfig{ID: current.ID, Enabled: true, Query: current.Query, MaxResults: current.MaxResults, Provider: current.Provider}
	if strings.TrimSpace(input.Query) != "" {
		config.Query = input.Query
	}
	if strings.TrimSpace(input.Provider) != "" {
		config.Provider = input.Provider
	}
	if input.Limit != 0 {
		config.MaxResults = input.Limit
	}
	if config, err = validateSourceConfig(config); err != nil {
		return QueryTrial{}, err
	}
	records, err := harvester.Harvest(ctx, config)
	if err != nil {
		return QueryTrial{}, err
	}
	trial := QueryTrial{SourceID: config.ID, Query: config.Query, Provider: config.Provider, Records: []TrialRecord{}}
	if counter, ok := harvester.(hitCounter); ok {
		if count, err := counter.Count(ctx, config); err == nil && count >= 0 {
			trial.HitCount = &count
		}
	}

	// Only reads; the rollback below discards nothing.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return QueryTrial{}, fmt.Errorf("begin query trial: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, record := range records {
		item := TrialRecord{ExternalID: record.ExternalID, Title: record.Title, URL: record.URL}
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM source_records WHERE source_id = ? AND external_id = ?)",
			config.ID, record.ExternalID).Scan(&item.AlreadyHarvested); err != nil {
			return QueryTrial{}, fmt.Errorf("check source record %s: %w", record.ExternalID, err)
		}
		if record.Paper != nil {
			paper := *record.Paper
			normalizePaper(&paper)
			if !doiPattern.MatchString(paper.DOI) {
				paper.DOI = ""
			}
			item.Year, item.Abstract = paper.Year, clip(paper.Abstract, 400)
			if item.PaperID, err = existingPaperID(ctx, tx, paper); err != nil {
				return QueryTrial{}, err
			}
			if item.PaperID != "" {
				if err := tx.QueryRowContext(ctx,
					"SELECT coalesce((SELECT screening_status FROM paper_curation WHERE paper_id = ?), 'pending')",
					item.PaperID).Scan(&item.Decision); err != nil {
					return QueryTrial{}, fmt.Errorf("read paper decision %s: %w", item.PaperID, err)
				}
			}
		} else if record.Dataset != nil {
			dataset := *record.Dataset
			item.Abstract = clip(dataset.Description, 400)
			inputs := make([]RepositoryInput, 0, len(dataset.Repositories))
			for _, repository := range dataset.Repositories {
				inputs = append(inputs, RepositoryInput{Repository: repository.Repository, Accession: repository.Accession})
			}
			if item.DatasetID, err = existingDatasetID(ctx, tx, DatasetInput{Title: dataset.Title, Repositories: inputs}); err != nil {
				return QueryTrial{}, err
			}
			if item.DatasetID != "" {
				// A person verifying a dataset is what including a paper is.
				var status string
				if err := tx.QueryRowContext(ctx, "SELECT status FROM datasets WHERE id = ?", item.DatasetID).Scan(&status); err != nil {
					return QueryTrial{}, fmt.Errorf("read dataset status %s: %w", item.DatasetID, err)
				}
				item.Decision = map[string]string{"verified": "included"}[status]
				if item.Decision == "" {
					item.Decision = "pending"
				}
			}
		}
		trial.Fetched++
		if item.AlreadyHarvested {
			trial.AlreadyHarvested++
		}
		switch item.Decision {
		case "":
			trial.New++
		case "included":
			trial.Included++
		case "excluded":
			trial.Excluded++
		case "uncertain":
			trial.Uncertain++
		default:
			trial.Undecided++
		}
		trial.Records = append(trial.Records, item)
	}
	trial.InLibrary = trial.Fetched - trial.New
	return trial, nil
}

// clip keeps the first limit runes of a text.
func clip(text string, limit int) string {
	text = strings.TrimSpace(text)
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	return string([]rune(text)[:limit]) + "…"
}

// Count asks the source for a query's total number of matches.
func (h *remoteHarvester) Count(ctx context.Context, config SourceConfig) (int, error) {
	switch {
	case config.ID == "pubmed":
		return h.countNCBI(ctx, "pubmed", config.Query)
	case config.ID == "sequence-repositories" && config.Provider == "geo":
		return h.countNCBI(ctx, "gds", config.Query)
	case config.ID == "sequence-repositories" && config.Provider == "sra":
		return h.countNCBI(ctx, "sra", config.Query)
	case config.ID == "europe-pmc":
		params := url.Values{"query": {config.Query}, "format": {"json"}, "resultType": {"idlist"}, "pageSize": {"1"}}
		var response struct {
			HitCount int `json:"hitCount"`
		}
		if err := h.getJSON(ctx, europePMCURL+"?"+params.Encode(), &response); err != nil {
			return 0, fmt.Errorf("count Europe PMC: %w", err)
		}
		return response.HitCount, nil
	case config.ID == "crossref":
		params := url.Values{"query.bibliographic": {config.Query}, "rows": {"0"}}
		if email := h.credentials().ContactEmail; email != "" {
			params.Set("mailto", email)
		}
		var response struct {
			Message struct {
				Total int `json:"total-results"`
			} `json:"message"`
		}
		if err := h.getJSON(ctx, crossrefURL+"?"+params.Encode(), &response); err != nil {
			return 0, fmt.Errorf("count Crossref: %w", err)
		}
		return response.Message.Total, nil
	default:
		return -1, nil
	}
}

func (h *remoteHarvester) countNCBI(ctx context.Context, database, query string) (int, error) {
	params := url.Values{"db": {database}, "term": {query}, "retmode": {"json"}, "retmax": {"0"}}
	var response struct {
		Result struct {
			Count string `json:"count"`
		} `json:"esearchresult"`
	}
	if err := h.getJSON(ctx, h.ncbiURL("esearch.fcgi", params), &response); err != nil {
		return 0, fmt.Errorf("count NCBI %s: %w", database, err)
	}
	return strconv.Atoi(response.Result.Count)
}
