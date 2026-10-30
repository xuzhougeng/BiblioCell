package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// SpeciesEntry pairs a scientific species name with its display label.
type SpeciesEntry struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// Paper is the document-shaped representation used by the desktop frontend.
// Species, organs, modalities, platforms, and article types are multi-valued:
// one paper can cover several of each, and an empty array means "not curated".
type Paper struct {
	ID                string         `json:"id"`
	Title             string         `json:"title"`
	DOI               string         `json:"doi"`
	PMID              string         `json:"pmid"`
	Authors           string         `json:"authors"`
	Year              *int           `json:"year"`
	Journal           string         `json:"journal"`
	PublicationStatus string         `json:"publicationStatus"`
	ArticleTypes      []string       `json:"articleTypes"`
	Species           []SpeciesEntry `json:"species"`
	Organs            []string       `json:"organs"`
	Modalities        []string       `json:"modalities"`
	Platforms         []string       `json:"platforms"`
	Abstract          string         `json:"abstract"`
	Notes             string         `json:"notes"`
	Tags              []string       `json:"tags"`
	Screening         string         `json:"screening"`
	ScreeningReason   string         `json:"screeningReason"`
	Reading           string         `json:"reading"`
	Fulltext          string         `json:"fulltext"`
	Starred           bool           `json:"starred"`
	Source            string         `json:"source"`
	AddedAt           string         `json:"addedAt"`
	UpdatedAt         string         `json:"updatedAt"`
	IsDemo            bool           `json:"isDemo"`
}

// PaperPatch contains optional metadata edits. Nil fields remain unchanged;
// a non-nil slice replaces the whole list, so an empty array clears the value.
type PaperPatch struct {
	Title             *string         `json:"title,omitempty" jsonschema:"New non-empty paper title"`
	DOI               *string         `json:"doi,omitempty" jsonschema:"New DOI; an empty string clears it"`
	PMID              *string         `json:"pmid,omitempty" jsonschema:"New PMID; an empty string clears it"`
	Authors           *string         `json:"authors,omitempty" jsonschema:"Authors separated by semicolons"`
	Year              *int            `json:"year,omitempty" jsonschema:"New publication year from 1800 to 2200"`
	ClearYear         bool            `json:"clearYear,omitempty" jsonschema:"Set the publication year to unknown; overrides year"`
	Journal           *string         `json:"journal,omitempty"`
	PublicationStatus *string         `json:"publicationStatus,omitempty" jsonschema:"unknown, preprint, published, or unpublished"`
	ArticleTypes      *[]string       `json:"articleTypes,omitempty" jsonschema:"Replace all article types; each item is original, method, review, other, or a Chinese display label"`
	Species           *[]SpeciesEntry `json:"species,omitempty" jsonschema:"Replace all species entries of name plus optional display label"`
	Organs            *[]string       `json:"organs,omitempty" jsonschema:"Replace all organs or tissues"`
	Modalities        *[]string       `json:"modalities,omitempty" jsonschema:"Replace all modalities such as scRNA-seq, snRNA-seq, or scATAC-seq"`
	Platforms         *[]string       `json:"platforms,omitempty" jsonschema:"Replace all sequencing platforms such as 10x Genomics Chromium or BD Rhapsody"`
	Abstract          *string         `json:"abstract,omitempty"`
	Notes             *string         `json:"notes,omitempty"`
	Tags              *[]string       `json:"tags,omitempty" jsonschema:"Replace all personal tags"`
	Screening         *string         `json:"screening,omitempty" jsonschema:"pending, included, excluded, or uncertain"`
	ScreeningReason   *string         `json:"screeningReason,omitempty" jsonschema:"Required when changing screening to excluded"`
	Reading           *string         `json:"reading,omitempty" jsonschema:"unread, skimmed, or read"`
	Starred           *bool           `json:"starred,omitempty"`
}

// ErrPaperNotFound indicates that the requested real paper does not exist.
var ErrPaperNotFound = errors.New("paper not found")

var doiPattern = regexp.MustCompile(`^10\.\d{4,9}/\S+$`)

// paperSelect is the shared projection for ListPapers and GetPaper; callers
// append their own WHERE / ORDER BY clause.
const paperSelect = `
		SELECT id, title, coalesce(abstract, ''), authors_json, coalesce(journal, ''),
		       coalesce(issue_year, 0), publication_state, article_types_json,
		       coalesce(species_json, '[]'),
		       organs_json, modalities_json, coalesce(platforms_json, '[]'), coalesce(source, ''),
		       first_seen_at, updated_at, screening_status, coalesce(screening_reason, ''),
		       reading_status, starred, tags_json, notes, fulltext_status,
		       coalesce((SELECT value FROM paper_identifiers WHERE paper_id = id AND namespace = 'doi'), ''),
		       coalesce((SELECT value FROM paper_identifiers WHERE paper_id = id AND namespace = 'pmid'), '')
		FROM v_paper_overview`

// ListPapers returns all real papers, newest first. Demo records are never read.
func (s *Store) ListPapers(ctx context.Context) ([]Paper, error) {
	rows, err := s.db.QueryContext(ctx, paperSelect+` ORDER BY first_seen_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list papers: %w", err)
	}
	defer rows.Close()

	papers := []Paper{}
	for rows.Next() {
		p, err := scanPaper(rows)
		if err != nil {
			return nil, err
		}
		papers = append(papers, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list papers: %w", err)
	}
	return papers, nil
}

// scanPaper reads one paperSelect row into the frontend representation.
func scanPaper(rows *sql.Rows) (Paper, error) {
	var p Paper
	var authors, species, organs, modalities, platforms, articleTypes, tags string
	var year, starred int
	if err := rows.Scan(
		&p.ID, &p.Title, &p.Abstract, &authors, &p.Journal,
		&year, &p.PublicationStatus, &articleTypes,
		&species, &organs, &modalities, &platforms, &p.Source,
		&p.AddedAt, &p.UpdatedAt, &p.Screening, &p.ScreeningReason,
		&p.Reading, &starred, &tags, &p.Notes, &p.Fulltext, &p.DOI, &p.PMID,
	); err != nil {
		return Paper{}, fmt.Errorf("read paper: %w", err)
	}
	if year != 0 {
		p.Year = &year
	}
	p.Authors = strings.Join(jsonStrings(authors), "; ")
	p.Species = speciesEntries(species)
	p.Organs = jsonStrings(organs)
	p.Modalities = jsonStrings(modalities)
	p.Platforms = jsonStrings(platforms)
	p.ArticleTypes = displayArticleTypes(articleTypes)
	p.Tags = jsonStrings(tags)
	p.Starred = starred == 1
	if p.Fulltext != "linked" && p.Fulltext != "stored" {
		p.Fulltext = "missing"
	}
	p.IsDemo = false
	return p, nil
}

// FilterPapers keeps papers whose metadata contains query, case-insensitively,
// and at most limit of them when limit is positive. Shared by MCP and the CLI.
func FilterPapers(papers []Paper, query string, limit int) []Paper {
	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]Paper, 0, len(papers))
	for _, paper := range papers {
		if limit > 0 && len(matches) >= limit {
			break
		}
		speciesTexts := make([]string, 0, len(paper.Species)*2)
		for _, entry := range paper.Species {
			speciesTexts = append(speciesTexts, entry.Name, entry.Label)
		}
		haystack := strings.ToLower(strings.Join([]string{
			paper.Title, paper.DOI, paper.PMID, paper.Authors,
			strings.Join(speciesTexts, " "), strings.Join(paper.Organs, " "),
			strings.Join(paper.Modalities, " "), strings.Join(paper.Platforms, " "),
			strings.Join(paper.ArticleTypes, " "), strings.Join(paper.Tags, " "),
		}, " "))
		if query == "" || strings.Contains(haystack, query) {
			matches = append(matches, paper)
		}
	}
	return matches
}

// SavePapers upserts frontend papers without deleting records added by MCP or another window.
func (s *Store) SavePapers(ctx context.Context, papers []Paper) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin paper save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	seen := make(map[string]bool, len(papers))
	for i := range papers {
		if seen[papers[i].ID] {
			return fmt.Errorf("duplicate paper ID %q", papers[i].ID)
		}
		seen[papers[i].ID] = true
		if err := savePaper(ctx, tx, &papers[i]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save papers: %w", err)
	}
	return nil
}

// ImportResult describes an atomic paper import or validation run.
type ImportResult struct {
	DryRun bool    `json:"dryRun"`
	Count  int     `json:"count"`
	Papers []Paper `json:"papers"`
}

// ImportPapers validates and inserts a batch in one transaction. DryRun rolls it back.
func (s *Store) ImportPapers(ctx context.Context, papers []Paper, dryRun bool) (ImportResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("begin paper import: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result := ImportResult{DryRun: dryRun, Papers: []Paper{}}
	seen := make(map[string]bool, len(papers))
	for i := range papers {
		if papers[i].ID == "" {
			id, err := newPaperID()
			if err != nil {
				return ImportResult{}, err
			}
			papers[i].ID = id
		}
		normalizePaper(&papers[i])
		if seen[papers[i].ID] {
			return ImportResult{}, fmt.Errorf("paper %d: duplicate ID %q", i, papers[i].ID)
		}
		seen[papers[i].ID] = true
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM papers WHERE id = ?)", papers[i].ID).Scan(&exists); err != nil {
			return ImportResult{}, fmt.Errorf("check paper %d ID: %w", i, err)
		}
		if exists == 1 {
			return ImportResult{}, fmt.Errorf("paper %d: ID %s already exists", i, papers[i].ID)
		}
		if err := savePaper(ctx, tx, &papers[i]); err != nil {
			return ImportResult{}, fmt.Errorf("paper %d: %w", i, err)
		}
		result.Papers = append(result.Papers, papers[i])
	}
	result.Count = len(result.Papers)
	if dryRun {
		return result, nil
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit paper import: %w", err)
	}
	return result, nil
}

// AddPaper validates and inserts one real paper.
func (s *Store) AddPaper(ctx context.Context, p Paper) (Paper, error) {
	result, err := s.ImportPapers(ctx, []Paper{p}, false)
	if err != nil {
		return Paper{}, err
	}
	return result.Papers[0], nil
}

// GetPaper returns one real paper by its stable ID.
func (s *Store) GetPaper(ctx context.Context, id string) (Paper, error) {
	id = strings.TrimSpace(id)
	rows, err := s.db.QueryContext(ctx, paperSelect+` WHERE id = ?`, id)
	if err != nil {
		return Paper{}, fmt.Errorf("get paper: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return Paper{}, fmt.Errorf("get paper: %w", err)
		}
		return Paper{}, fmt.Errorf("%w: %s", ErrPaperNotFound, id)
	}
	return scanPaper(rows)
}

// UpdatePaper applies a partial metadata patch without replacing omitted fields.
func (s *Store) UpdatePaper(ctx context.Context, id string, patch PaperPatch) (Paper, error) {
	paper, err := s.GetPaper(ctx, id)
	if err != nil {
		return Paper{}, err
	}
	previousUpdatedAt := paper.UpdatedAt
	applyPaperPatch(&paper, patch)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Paper{}, fmt.Errorf("begin paper update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var currentUpdatedAt string
	err = tx.QueryRowContext(ctx, "SELECT updated_at FROM papers WHERE id = ?", paper.ID).Scan(&currentUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Paper{}, fmt.Errorf("%w: %s", ErrPaperNotFound, paper.ID)
	}
	if err != nil {
		return Paper{}, fmt.Errorf("check paper before update: %w", err)
	}
	if currentUpdatedAt != previousUpdatedAt {
		return Paper{}, errors.New("paper changed during update; call get_paper again and retry the patch")
	}
	if err := savePaper(ctx, tx, &paper); err != nil {
		return Paper{}, fmt.Errorf("update paper %s: %w", paper.ID, err)
	}
	if err := tx.Commit(); err != nil {
		return Paper{}, fmt.Errorf("commit paper update: %w", err)
	}
	return paper, nil
}

func applyPaperPatch(paper *Paper, patch PaperPatch) {
	if patch.Title != nil {
		paper.Title = *patch.Title
	}
	if patch.DOI != nil {
		paper.DOI = *patch.DOI
	}
	if patch.PMID != nil {
		paper.PMID = *patch.PMID
	}
	if patch.Authors != nil {
		paper.Authors = *patch.Authors
	}
	if patch.ClearYear {
		paper.Year = nil
	} else if patch.Year != nil {
		paper.Year = patch.Year
	}
	if patch.Journal != nil {
		paper.Journal = *patch.Journal
	}
	if patch.PublicationStatus != nil {
		paper.PublicationStatus = *patch.PublicationStatus
	}
	if patch.ArticleTypes != nil {
		paper.ArticleTypes = *patch.ArticleTypes
	}
	if patch.Species != nil {
		paper.Species = *patch.Species
	}
	if patch.Organs != nil {
		paper.Organs = *patch.Organs
	}
	if patch.Modalities != nil {
		paper.Modalities = *patch.Modalities
	}
	if patch.Platforms != nil {
		paper.Platforms = *patch.Platforms
	}
	if patch.Abstract != nil {
		paper.Abstract = *patch.Abstract
	}
	if patch.Notes != nil {
		paper.Notes = *patch.Notes
	}
	if patch.Tags != nil {
		paper.Tags = *patch.Tags
	}
	if patch.Screening != nil {
		paper.Screening = *patch.Screening
	}
	if patch.ScreeningReason != nil {
		paper.ScreeningReason = *patch.ScreeningReason
	}
	if patch.Reading != nil {
		paper.Reading = *patch.Reading
	}
	if patch.Starred != nil {
		paper.Starred = *patch.Starred
	}
}

func savePaper(ctx context.Context, tx *sql.Tx, p *Paper) error {
	normalizePaper(p)
	if p.ID == "" || p.Title == "" {
		return errors.New("paper ID and title are required")
	}
	if p.DOI != "" && !doiPattern.MatchString(p.DOI) {
		return fmt.Errorf("invalid DOI %q", p.DOI)
	}
	if p.Year != nil && (*p.Year < 1800 || *p.Year > 2200) {
		return errors.New("paper year must be between 1800 and 2200")
	}
	if !oneOf(p.Screening, "pending", "included", "excluded", "uncertain") {
		return fmt.Errorf("invalid screening status %q", p.Screening)
	}
	if p.Screening == "excluded" && p.ScreeningReason == "" {
		return errors.New("an excluded paper needs a screening reason")
	}
	if !oneOf(p.Reading, "unread", "skimmed", "read") {
		return fmt.Errorf("invalid reading status %q", p.Reading)
	}
	if !oneOf(p.PublicationStatus, "unknown", "preprint", "published", "unpublished") {
		return fmt.Errorf("invalid publication status %q", p.PublicationStatus)
	}
	articleTypes, err := canonicalArticleTypes(p.ArticleTypes)
	if err != nil {
		return err
	}

	if p.DOI != "" {
		var existingID string
		err := tx.QueryRowContext(ctx,
			"SELECT paper_id FROM paper_identifiers WHERE namespace = 'doi' AND value = ? AND paper_id <> ?",
			p.DOI, p.ID).Scan(&existingID)
		if err == nil {
			return fmt.Errorf("DOI %s already belongs to paper %s", p.DOI, existingID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("check DOI: %w", err)
		}
	}

	authors := jsonArray(strings.Split(p.Authors, ";"))
	species := speciesJSONArray(p.Species)
	organs := jsonArray(p.Organs)
	modalities := jsonArray(p.Modalities)
	platforms := jsonArray(p.Platforms)
	tags := jsonArray(p.Tags)
	// The legacy single-value columns (species_text, species_label,
	// article_type) are no longer written; the *_json arrays are authoritative.
	var year any
	if p.Year != nil {
		year = *p.Year
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO papers(
			id, title, abstract, authors_json, journal, organs_json, modalities_json,
			platforms_json, issue_year, publication_state,
			article_types_json, species_json, source, first_seen_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title, abstract = excluded.abstract,
			authors_json = excluded.authors_json, journal = excluded.journal,
			organs_json = excluded.organs_json, modalities_json = excluded.modalities_json,
			platforms_json = excluded.platforms_json, issue_year = excluded.issue_year,
			publication_state = excluded.publication_state,
			article_types_json = excluded.article_types_json,
			species_json = excluded.species_json, source = excluded.source,
			updated_at = excluded.updated_at`,
		p.ID, p.Title, p.Abstract, authors, p.Journal, organs, modalities,
		platforms, year, p.PublicationStatus,
		jsonArray(articleTypes), species, p.Source, p.AddedAt, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save paper %s: %w", p.ID, err)
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO paper_curation(
			paper_id, screening_status, screening_reason, screening_method,
			reading_status, starred, tags_json, notes, updated_at
		) VALUES(?, ?, ?, 'human', ?, ?, ?, ?, ?)
		ON CONFLICT(paper_id) DO UPDATE SET
			screening_status = excluded.screening_status,
			screening_reason = excluded.screening_reason,
			screening_method = 'human', reading_status = excluded.reading_status,
			starred = excluded.starred, tags_json = excluded.tags_json,
			notes = excluded.notes, updated_at = excluded.updated_at`,
		p.ID, p.Screening, p.ScreeningReason, p.Reading, boolInt(p.Starred), tags, p.Notes, p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save paper curation %s: %w", p.ID, err)
	}

	if _, err := tx.ExecContext(ctx, "DELETE FROM paper_identifiers WHERE paper_id = ?", p.ID); err != nil {
		return fmt.Errorf("update paper identifiers %s: %w", p.ID, err)
	}
	for _, identifier := range []struct {
		namespace string
		value     string
	}{{"doi", p.DOI}, {"pmid", p.PMID}} {
		if identifier.value == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO paper_identifiers(paper_id, namespace, value, raw_value) VALUES(?, ?, ?, ?)",
			p.ID, identifier.namespace, identifier.value, identifier.value); err != nil {
			return fmt.Errorf("save paper %s: %w", identifier.namespace, err)
		}
	}
	return nil
}

func normalizePaper(p *Paper) {
	p.ID = strings.TrimSpace(p.ID)
	p.Title = strings.TrimSpace(p.Title)
	p.DOI = normalizeDOI(p.DOI)
	p.PMID = strings.TrimSpace(p.PMID)
	p.Authors = strings.TrimSpace(p.Authors)
	p.Journal = strings.TrimSpace(p.Journal)
	p.Species = cleanSpecies(p.Species)
	p.Organs = cleanStrings(p.Organs)
	p.Modalities = cleanStrings(p.Modalities)
	p.Platforms = cleanStrings(p.Platforms)
	p.ArticleTypes = cleanStrings(p.ArticleTypes)
	p.ScreeningReason = strings.TrimSpace(p.ScreeningReason)
	p.Source = strings.TrimSpace(p.Source)
	if p.Screening == "" {
		p.Screening = "pending"
	}
	if p.Reading == "" {
		p.Reading = "unread"
	}
	if p.PublicationStatus == "" {
		p.PublicationStatus = "unknown"
	}
	if p.Source == "" {
		p.Source = "手动添加"
	}
	if p.Fulltext == "" {
		p.Fulltext = "missing"
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if p.AddedAt == "" {
		p.AddedAt = now
	}
	p.UpdatedAt = now
	p.IsDemo = false
}

func normalizeDOI(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	for _, prefix := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"} {
		value = strings.TrimPrefix(value, prefix)
	}
	return strings.TrimSpace(value)
}

func newPaperID() (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate paper ID: %w", err)
	}
	return fmt.Sprintf("P-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(suffix[:])), nil
}

func jsonArray(values []string) string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	encoded, _ := json.Marshal(clean)
	return string(encoded)
}

func jsonStrings(value string) []string {
	var values []string
	if json.Unmarshal([]byte(value), &values) != nil {
		return []string{}
	}
	return values
}

// cleanStrings trims every entry, drops blanks, and removes duplicates while
// keeping the original order.
func cleanStrings(values []string) []string {
	clean := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !slices.Contains(clean, value) {
			clean = append(clean, value)
		}
	}
	return clean
}

// cleanSpecies trims entries, drops ones without a scientific name, and
// deduplicates by name.
func cleanSpecies(entries []SpeciesEntry) []SpeciesEntry {
	clean := make([]SpeciesEntry, 0, len(entries))
	for _, entry := range entries {
		entry.Name = strings.TrimSpace(entry.Name)
		entry.Label = strings.TrimSpace(entry.Label)
		if entry.Name == "" {
			continue
		}
		duplicate := false
		for _, kept := range clean {
			if kept.Name == entry.Name {
				duplicate = true
				break
			}
		}
		if !duplicate {
			clean = append(clean, entry)
		}
	}
	return clean
}

func speciesJSONArray(entries []SpeciesEntry) string {
	encoded, _ := json.Marshal(cleanSpecies(entries))
	return string(encoded)
}

func speciesEntries(value string) []SpeciesEntry {
	var entries []SpeciesEntry
	if json.Unmarshal([]byte(value), &entries) != nil {
		return []SpeciesEntry{}
	}
	return cleanSpecies(entries)
}

// articleTypeLabels maps every accepted article type spelling, canonical or
// Chinese display, to its canonical database value.
var articleTypeLabels = map[string]string{
	"original": "original", "原创研究": "original",
	"method": "method", "方法研究": "method",
	"review": "review", "综述": "review",
	"other": "other", "相关资料": "other", "其他": "other",
}

var articleTypeDisplay = map[string]string{
	"original": "原创研究", "method": "方法研究", "review": "综述", "other": "相关资料",
}

// canonicalArticleTypes validates a whole article type list and returns the
// canonical values; an empty list is fine and means 待判断.
func canonicalArticleTypes(values []string) ([]string, error) {
	clean := make([]string, 0, len(values))
	for _, value := range cleanStrings(values) {
		canonical, ok := articleTypeLabels[value]
		if !ok {
			return nil, fmt.Errorf("invalid article type %q (use original, method, review, or other)", value)
		}
		if !slices.Contains(clean, canonical) {
			clean = append(clean, canonical)
		}
	}
	return clean, nil
}

// displayArticleTypes turns a stored canonical JSON array into the Chinese
// display labels used by the frontend; unknown values are dropped.
func displayArticleTypes(value string) []string {
	display := []string{}
	for _, canonical := range jsonStrings(value) {
		if label, ok := articleTypeDisplay[canonical]; ok {
			display = append(display, label)
		}
	}
	return display
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
