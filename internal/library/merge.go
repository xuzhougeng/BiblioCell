package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// ErrMergeConflict means both papers carry a person's screening decision and
// the decisions differ; a person settles that before the papers are merged.
var ErrMergeConflict = errors.New("both papers have a person's screening decision and they differ; settle it in the desktop app before merging")

// MergeInput names the paper to keep and the duplicate folded into it, such as
// a journal article and the preprint it grew from.
type MergeInput struct {
	KeepID  string `json:"keepId" jsonschema:"Paper that stays, usually the published version"`
	MergeID string `json:"mergeId" jsonschema:"Duplicate that is folded into keepId and removed, usually the preprint"`
	Reason  string `json:"reason" jsonschema:"Why these are the same work, e.g. Crossref is-preprint-of or matching title and authors; required"`
	// Via records who merged: desktop, MCP, or CLI.
	Via string `json:"-"`
}

// MergedPaper is what remains of a merged-away paper: enough to recognize it
// when a source or an import offers it again.
type MergedPaper struct {
	MergedID          string `json:"mergedId"`
	PaperID           string `json:"paperId"`
	Title             string `json:"title"`
	DOI               string `json:"doi"`
	PMID              string `json:"pmid"`
	PublicationStatus string `json:"publicationStatus"`
	Reason            string `json:"reason"`
	MergedVia         string `json:"mergedVia"`
	MergedAt          string `json:"mergedAt"`
}

// MergeResult is the kept paper after the merge and what moved onto it.
type MergeResult struct {
	Paper  Paper       `json:"paper"`
	Merged MergedPaper `json:"merged"`
	// ScreeningFrom says where the kept paper's screening decision came from:
	// "kept", "merged" (carried over from the duplicate), or "none".
	ScreeningFrom string      `json:"screeningFrom"`
	Moved         MergeCounts `json:"moved"`
}

// MergeCounts counts the records re-attached to the kept paper.
type MergeCounts struct {
	Files         int `json:"files"`
	DatasetLinks  int `json:"datasetLinks"`
	Evidence      int `json:"evidence"`
	Suggestions   int `json:"suggestions"`
	SourceRecords int `json:"sourceRecords"`
}

// MergePapers folds one paper into another in a single transaction. Files,
// dataset links, evidence, AI suggestions, and source records move to the kept
// paper; its bibliographic record stays as it is, and only its blank abstract
// and classification fields are filled from the duplicate. A person's
// screening decision on either paper survives, notes and tags are combined, and
// the duplicate's ID, DOI, and PMID are recorded so a source update or an import
// finds the kept paper instead of adding the duplicate back.
func (s *Store) MergePapers(ctx context.Context, input MergeInput) (MergeResult, error) {
	input.KeepID, input.MergeID = strings.TrimSpace(input.KeepID), strings.TrimSpace(input.MergeID)
	input.Reason = strings.TrimSpace(input.Reason)
	if input.KeepID == "" || input.MergeID == "" {
		return MergeResult{}, errors.New("keepId and mergeId are required")
	}
	if input.KeepID == input.MergeID {
		return MergeResult{}, errors.New("keepId and mergeId must be different papers")
	}
	if input.Reason == "" {
		return MergeResult{}, errors.New("a merge needs a reason saying why the papers are the same work")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MergeResult{}, fmt.Errorf("begin merge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	keep, err := readPaper(ctx, tx, input.KeepID)
	if err != nil {
		return MergeResult{}, err
	}
	dup, err := readPaper(ctx, tx, input.MergeID)
	if err != nil {
		return MergeResult{}, err
	}

	result := MergeResult{ScreeningFrom: "none"}
	switch {
	case keep.ReviewedAt != "" && dup.ReviewedAt != "" && keep.Screening != dup.Screening:
		return MergeResult{}, fmt.Errorf("%w (%s is %s, %s is %s)", ErrMergeConflict, keep.ID, keep.Screening, dup.ID, dup.Screening)
	case keep.ReviewedAt != "":
		result.ScreeningFrom = "kept"
	case dup.ReviewedAt != "":
		keep.Screening, keep.ScreeningReason, keep.ReviewedAt = dup.Screening, dup.ScreeningReason, dup.ReviewedAt
		result.ScreeningFrom = "merged"
	}
	if keep.ProposedScreening == "" {
		keep.ProposedScreening, keep.ProposedReason = dup.ProposedScreening, dup.ProposedReason
	}
	// A person who checked either version has seen this work.
	keep.AwaitingReview = keep.ReviewedAt == "" && keep.AwaitingReview && dup.AwaitingReview
	keep.Starred = keep.Starred || dup.Starred
	if readingRank(dup.Reading) > readingRank(keep.Reading) {
		keep.Reading = dup.Reading
	}
	keep.Tags = cleanStrings(append(append([]string{}, keep.Tags...), dup.Tags...))
	keep.Notes = combineNotes(keep.Notes, dup.Notes)
	fillBlank(&keep.Abstract, dup.Abstract)
	if len(keep.ArticleTypes) == 0 {
		keep.ArticleTypes = dup.ArticleTypes
	}
	if len(keep.Species) == 0 {
		keep.Species = dup.Species
	}
	if len(keep.Organs) == 0 {
		keep.Organs = dup.Organs
	}
	if len(keep.Modalities) == 0 {
		keep.Modalities = dup.Modalities
	}
	if len(keep.Platforms) == 0 {
		keep.Platforms = dup.Platforms
	}
	keep.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := savePaper(ctx, tx, &keep); err != nil {
		return MergeResult{}, fmt.Errorf("save kept paper: %w", err)
	}

	if result.Moved, err = moveRecords(ctx, tx, dup.ID, keep.ID); err != nil {
		return MergeResult{}, err
	}
	result.Merged = MergedPaper{
		MergedID: dup.ID, PaperID: keep.ID, Title: dup.Title, DOI: dup.DOI, PMID: dup.PMID,
		PublicationStatus: dup.PublicationStatus, Reason: input.Reason, MergedVia: input.Via, MergedAt: keep.UpdatedAt,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO merged_papers(merged_id, paper_id, title, doi, pmid, publication_state, reason, merged_via, merged_at)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dup.ID, keep.ID, dup.Title, dup.DOI, dup.PMID, dup.PublicationStatus, input.Reason, input.Via, keep.UpdatedAt); err != nil {
		return MergeResult{}, fmt.Errorf("record merged paper: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM papers WHERE id = ?", dup.ID); err != nil {
		return MergeResult{}, fmt.Errorf("remove merged paper: %w", err)
	}
	if result.Paper, err = readPaper(ctx, tx, keep.ID); err != nil {
		return MergeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return MergeResult{}, fmt.Errorf("commit merge: %w", err)
	}
	return result, nil
}

// moveRecords re-attaches everything that hangs off paper from to paper to.
// Where to already has the same link, the duplicate row is left to be removed
// with from, except that a verified link stays verified.
func moveRecords(ctx context.Context, tx *sql.Tx, from, to string) (MergeCounts, error) {
	var counts MergeCounts
	exec := func(target *int, query string, args ...any) error {
		result, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("merge records: %w", err)
		}
		if target != nil {
			n, _ := result.RowsAffected()
			*target += int(n)
		}
		return nil
	}
	steps := []struct {
		count *int
		query string
	}{
		{nil, `UPDATE paper_datasets SET verified = 1 WHERE paper_id = ?2 AND EXISTS (
			SELECT 1 FROM paper_datasets d WHERE d.paper_id = ?1 AND d.dataset_id = paper_datasets.dataset_id
			AND d.role = paper_datasets.role AND d.verified = 1)`},
		{&counts.DatasetLinks, `UPDATE OR IGNORE paper_datasets SET paper_id = ?2 WHERE paper_id = ?1`},
		{&counts.Files, `UPDATE OR IGNORE paper_resources SET paper_id = ?2 WHERE paper_id = ?1`},
		{&counts.Evidence, `UPDATE field_evidence SET target_paper_id = ?2 WHERE target_paper_id = ?1`},
		{&counts.Suggestions, `UPDATE screening_suggestions SET paper_id = ?2 WHERE paper_id = ?1`},
		{&counts.SourceRecords, `UPDATE source_records SET paper_id = ?2 WHERE paper_id = ?1`},
		{nil, `UPDATE OR IGNORE paper_taxa SET paper_id = ?2 WHERE paper_id = ?1`},
		{nil, `DELETE FROM paper_relations WHERE (source_paper_id = ?1 AND target_paper_id = ?2) OR (source_paper_id = ?2 AND target_paper_id = ?1)`},
		{nil, `UPDATE OR IGNORE paper_relations SET source_paper_id = ?2 WHERE source_paper_id = ?1`},
		{nil, `UPDATE OR IGNORE paper_relations SET target_paper_id = ?2 WHERE target_paper_id = ?1`},
		// Papers merged into the duplicate earlier now point at the kept one.
		{nil, `UPDATE merged_papers SET paper_id = ?2 WHERE paper_id = ?1`},
		{nil, `UPDATE paper_curation SET
			priority = CASE WHEN priority = 'high' OR (SELECT priority FROM paper_curation WHERE paper_id = ?1) = 'high' THEN 'high'
				WHEN priority = 'normal' OR (SELECT priority FROM paper_curation WHERE paper_id = ?1) = 'normal' THEN 'normal' ELSE 'low' END,
			organs_override_json = coalesce(organs_override_json, (SELECT organs_override_json FROM paper_curation WHERE paper_id = ?1)),
			modalities_override_json = coalesce(modalities_override_json, (SELECT modalities_override_json FROM paper_curation WHERE paper_id = ?1)),
			conditions_override_text = coalesce(conditions_override_text, (SELECT conditions_override_text FROM paper_curation WHERE paper_id = ?1))
			WHERE paper_id = ?2`},
	}
	for _, step := range steps {
		if err := exec(step.count, step.query, from, to); err != nil {
			return MergeCounts{}, err
		}
	}
	return counts, nil
}

func readingRank(value string) int {
	return slices.Index([]string{"unread", "skimmed", "read"}, value)
}

func combineNotes(kept, merged string) string {
	kept, merged = strings.TrimSpace(kept), strings.TrimSpace(merged)
	switch {
	case merged == "" || merged == kept || strings.Contains(kept, merged):
		return kept
	case kept == "":
		return merged
	}
	return kept + "\n\n" + merged
}

// mergedPaperID returns the paper a merged-away ID, DOI, or PMID now belongs to.
func mergedPaperID(ctx context.Context, tx *sql.Tx, paper Paper) (string, error) {
	for _, lookup := range [][2]string{{"merged_id", paper.ID}, {"doi", paper.DOI}, {"pmid", paper.PMID}} {
		if lookup[1] == "" {
			continue
		}
		var id string
		err := tx.QueryRowContext(ctx, "SELECT paper_id FROM merged_papers WHERE "+lookup[0]+" = ? LIMIT 1", lookup[1]).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("check merged %s: %w", lookup[0], err)
		}
	}
	return "", nil
}

// ListMergedPapers returns what was merged into a paper, newest first.
func (s *Store) ListMergedPapers(ctx context.Context, paperID string) ([]MergedPaper, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT merged_id, paper_id, title, doi, pmid, publication_state, reason, merged_via, merged_at
		FROM merged_papers WHERE paper_id = ? ORDER BY merged_at DESC, merged_id`, paperID)
	if err != nil {
		return nil, fmt.Errorf("list merged papers: %w", err)
	}
	defer rows.Close()
	merged := []MergedPaper{}
	for rows.Next() {
		var m MergedPaper
		if err := rows.Scan(&m.MergedID, &m.PaperID, &m.Title, &m.DOI, &m.PMID, &m.PublicationStatus, &m.Reason, &m.MergedVia, &m.MergedAt); err != nil {
			return nil, fmt.Errorf("read merged paper: %w", err)
		}
		merged = append(merged, m)
	}
	return merged, rows.Err()
}

// DuplicateCandidate is a paper as needed to decide a merge.
type DuplicateCandidate struct {
	ID                string `json:"id"`
	Title             string `json:"title"`
	DOI               string `json:"doi"`
	PMID              string `json:"pmid"`
	Authors           string `json:"authors"`
	Year              *int   `json:"year"`
	Journal           string `json:"journal"`
	PublicationStatus string `json:"publicationStatus"`
	Screening         string `json:"screening"`
	Reviewed          bool   `json:"reviewed"`
	AddedAt           string `json:"addedAt"`
}

func duplicateCandidate(p Paper) DuplicateCandidate {
	return DuplicateCandidate{
		ID: p.ID, Title: p.Title, DOI: p.DOI, PMID: p.PMID, Authors: p.Authors, Year: p.Year, Journal: p.Journal,
		PublicationStatus: p.PublicationStatus, Screening: p.Screening, Reviewed: p.ReviewedAt != "", AddedAt: p.AddedAt,
	}
}

// DuplicateGroup is a set of papers that look like one work.
type DuplicateGroup struct {
	// SuggestedKeepID prefers a published version, then one a person reviewed,
	// then the earliest added; the others would be merged into it.
	SuggestedKeepID string               `json:"suggestedKeepId"`
	Papers          []DuplicateCandidate `json:"papers"`
}

// DuplicatesResult is one page of duplicate groups.
type DuplicatesResult struct {
	Groups []DuplicateGroup `json:"groups"`
	Total  int              `json:"total"`
}

// FindDuplicates groups papers whose titles match once case, punctuation, and
// markup are ignored, such as a preprint and its journal version. It reads the
// library only; a title that changed on publication needs FindPublishedVersions.
func (s *Store) FindDuplicates(ctx context.Context, limit, offset int) (DuplicatesResult, error) {
	result := DuplicatesResult{Groups: []DuplicateGroup{}}
	const groups = `SELECT title_key FROM papers WHERE title_key <> '' GROUP BY title_key HAVING count(*) > 1`
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM ("+groups+")").Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count duplicates: %w", err)
	}
	keys, err := queryStrings(ctx, s.db, groups+" ORDER BY min(first_seen_at), title_key LIMIT ? OFFSET ?", limit, max(offset, 0))
	if err != nil {
		return result, fmt.Errorf("find duplicates: %w", err)
	}
	for _, key := range keys {
		ids, err := queryStrings(ctx, s.db, "SELECT id FROM papers WHERE title_key = ? ORDER BY first_seen_at, id", key)
		if err != nil {
			return result, fmt.Errorf("read duplicates: %w", err)
		}
		group := DuplicateGroup{Papers: make([]DuplicateCandidate, 0, len(ids))}
		for _, id := range ids {
			paper, err := s.GetPaper(ctx, id)
			if err != nil {
				return result, err
			}
			group.Papers = append(group.Papers, duplicateCandidate(paper))
		}
		group.SuggestedKeepID = suggestKeep(group.Papers)
		result.Groups = append(result.Groups, group)
	}
	return result, nil
}

func suggestKeep(papers []DuplicateCandidate) string {
	best := 0
	score := func(p DuplicateCandidate) int {
		points := 0
		switch p.PublicationStatus {
		case "published":
			points += 4
		case "preprint":
			points -= 4
		}
		if p.Reviewed {
			points += 2
		}
		return points
	}
	for i := range papers {
		if score(papers[i]) > score(papers[best]) {
			best = i // Ties keep the earliest added, which comes first.
		}
	}
	return papers[best].ID
}

func queryStrings(ctx context.Context, db queryer, query string, args ...any) ([]string, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, rows.Err()
}

// PublishedVersion is a library preprint Crossref links to its journal version.
type PublishedVersion struct {
	Preprint     DuplicateCandidate `json:"preprint"`
	PublishedDOI string             `json:"publishedDoi"`
	// PublishedPaperID is the library paper holding PublishedDOI, empty when
	// the journal version is not in the library yet.
	PublishedPaperID string `json:"publishedPaperId,omitempty"`
}

// PublishedVersionsResult is one page of preprints checked against Crossref.
type PublishedVersionsResult struct {
	// Total counts the library's preprints with a DOI; Checked is this page.
	Total     int                `json:"total"`
	Checked   int                `json:"checked"`
	Published []PublishedVersion `json:"published"`
	Errors    []string           `json:"errors"`
}

// FindPublishedVersions asks Crossref, for one page of the library's
// preprints, whether each was published as a journal article
// (relation is-preprint-of), and says whether that article is in the library.
// It writes nothing.
func (s *Store) FindPublishedVersions(ctx context.Context, limit, offset int, credentials func() SourceCredentials) (PublishedVersionsResult, error) {
	return s.findPublishedVersions(ctx, limit, offset, newRemoteHarvester(credentials))
}

func (s *Store) findPublishedVersions(ctx context.Context, limit, offset int, h *remoteHarvester) (PublishedVersionsResult, error) {
	result := PublishedVersionsResult{Published: []PublishedVersion{}, Errors: []string{}}
	const preprints = `FROM papers p JOIN paper_identifiers i ON i.paper_id = p.id AND i.namespace = 'doi'
		WHERE p.publication_state = 'preprint' OR i.value LIKE '10.1101/%'`
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) "+preprints).Scan(&result.Total); err != nil {
		return result, fmt.Errorf("count preprints: %w", err)
	}
	ids, err := queryStrings(ctx, s.db, "SELECT p.id "+preprints+" ORDER BY p.first_seen_at, p.id LIMIT ? OFFSET ?", limit, max(offset, 0))
	if err != nil {
		return result, fmt.Errorf("list preprints: %w", err)
	}
	for _, id := range ids {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		paper, err := s.GetPaper(ctx, id)
		if err != nil {
			return result, err
		}
		result.Checked++
		work, found, err := h.crossrefWork(ctx, paper.DOI)
		if err != nil {
			result.Errors = append(result.Errors, paper.DOI+": "+err.Error())
			continue
		}
		if !found {
			continue
		}
		for _, related := range work.Relation["is-preprint-of"] {
			doi := normalizeDOI(related.ID)
			if !strings.EqualFold(related.IDType, "doi") || !doiPattern.MatchString(doi) || doi == paper.DOI {
				continue
			}
			version := PublishedVersion{Preprint: duplicateCandidate(paper), PublishedDOI: doi}
			var holder string
			err := s.db.QueryRowContext(ctx, "SELECT paper_id FROM paper_identifiers WHERE namespace = 'doi' AND value = ?", doi).Scan(&holder)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return result, fmt.Errorf("check published DOI: %w", err)
			}
			version.PublishedPaperID = holder
			result.Published = append(result.Published, version)
			break
		}
	}
	return result, nil
}
