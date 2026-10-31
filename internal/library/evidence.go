package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ErrEvidenceNotFound indicates that no evidence has the requested ID.
var ErrEvidenceNotFound = errors.New("evidence not found")

// EvidenceInput is an agent's claim about one field of a paper or dataset, with
// what a person needs to check it.
type EvidenceInput struct {
	PaperID   string `json:"paperId,omitempty" jsonschema:"The paper the claim is about; give this or datasetId"`
	DatasetID string `json:"datasetId,omitempty" jsonschema:"The dataset the claim is about; give this or paperId"`
	Field     string `json:"field" jsonschema:"The paper or dataset JSON field, such as organs or platforms; screening for the basis of a proposal"`
	Value     string `json:"value,omitempty" jsonschema:"The value extracted"`
	SourceURL string `json:"sourceUrl,omitempty" jsonschema:"Where it came from; this or fileId is required"`
	FileID    string `json:"fileId,omitempty" jsonschema:"Attached file it came from, from attach_file or list_files; this or sourceUrl is required"`
	Location  string `json:"location,omitempty" jsonschema:"Where in the source, such as Methods, page 3"`
	Excerpt   string `json:"excerpt" jsonschema:"Short quote from the source that a person can check"`
	Extractor string `json:"extractor,omitempty" jsonschema:"Agent or model that extracted it"`
}

// Evidence is a stored claim. Status is a person's verdict: unreviewed, verified,
// or rejected.
type Evidence struct {
	ID         string `json:"id"`
	PaperID    string `json:"paperId"`
	DatasetID  string `json:"datasetId"`
	Field      string `json:"field"`
	Value      string `json:"value"`
	SourceURL  string `json:"sourceUrl"`
	FileID     string `json:"fileId"`
	Location   string `json:"location"`
	Excerpt    string `json:"excerpt"`
	Extractor  string `json:"extractor"`
	Status     string `json:"status"`
	ObservedAt string `json:"observedAt"`
}

const evidenceSelect = `
	SELECT id, coalesce(target_paper_id, ''), coalesce(target_dataset_id, ''), field_name,
	       coalesce(raw_value, ''), coalesce(source_url, ''), coalesce(source_resource_id, ''),
	       coalesce(source_location, ''), coalesce(excerpt, ''), coalesce(extractor_version, ''),
	       review_status, observed_at
	FROM field_evidence`

func evidenceFields(e *Evidence) []any {
	return []any{&e.ID, &e.PaperID, &e.DatasetID, &e.Field, &e.Value, &e.SourceURL, &e.FileID,
		&e.Location, &e.Excerpt, &e.Extractor, &e.Status, &e.ObservedAt}
}

// AddEvidence stores an agent's claims as unreviewed, all or none. A claim
// already on record is returned as it is, so rerunning is safe and never resets
// a person's verdict.
func (s *Store) AddEvidence(ctx context.Context, inputs []EvidenceInput) ([]Evidence, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin evidence add: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	ids := make([]string, 0, len(inputs))
	for i, input := range inputs {
		id, err := addEvidence(ctx, tx, input)
		if err != nil {
			return nil, fmt.Errorf("evidence %d: %w", i, err)
		}
		ids = append(ids, id)
	}
	evidence := make([]Evidence, 0, len(ids))
	for _, id := range ids {
		var e Evidence
		if err := tx.QueryRowContext(ctx, evidenceSelect+" WHERE id = ?", id).Scan(evidenceFields(&e)...); err != nil {
			return nil, fmt.Errorf("read evidence: %w", err)
		}
		evidence = append(evidence, e)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit evidence add: %w", err)
	}
	return evidence, nil
}

func addEvidence(ctx context.Context, tx *sql.Tx, input EvidenceInput) (string, error) {
	paperID, datasetID := strings.TrimSpace(input.PaperID), strings.TrimSpace(input.DatasetID)
	field, excerpt := strings.TrimSpace(input.Field), strings.TrimSpace(input.Excerpt)
	sourceURL, fileID := strings.TrimSpace(input.SourceURL), strings.TrimSpace(input.FileID)
	if (paperID == "") == (datasetID == "") {
		return "", errors.New("give exactly one of paperId and datasetId")
	}
	if field == "" || excerpt == "" {
		return "", errors.New("field and excerpt are required")
	}
	if sourceURL == "" && fileID == "" {
		return "", errors.New("give sourceUrl or fileId")
	}
	if paperID != "" {
		if _, err := readPaper(ctx, tx, paperID); err != nil {
			return "", err
		}
	} else if _, err := readDataset(ctx, tx, datasetID); err != nil {
		return "", err
	}
	if fileID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM resources WHERE id = ?)", fileID).Scan(&exists); err != nil {
			return "", fmt.Errorf("check evidence file: %w", err)
		}
		if exists == 0 {
			return "", fmt.Errorf("%w: %s", ErrFileNotFound, fileID)
		}
	}

	var existing string
	err := tx.QueryRowContext(ctx, `
		SELECT id FROM field_evidence
		WHERE coalesce(target_paper_id, '') = ? AND coalesce(target_dataset_id, '') = ? AND field_name = ?
		  AND coalesce(raw_value, '') = ? AND coalesce(source_url, '') = ?
		  AND coalesce(source_resource_id, '') = ? AND coalesce(excerpt, '') = ? LIMIT 1`,
		paperID, datasetID, field, input.Value, sourceURL, fileID, excerpt).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("check evidence: %w", err)
	}
	id, err := newLocalID("E")
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO field_evidence(
			id, target_paper_id, target_dataset_id, field_name, raw_value, source_url,
			source_resource_id, source_location, excerpt, extraction_method, extractor_version
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'model', ?)`,
		id, nullableString(paperID), nullableString(datasetID), field, nullableString(input.Value),
		nullableString(sourceURL), nullableString(fileID), nullableString(input.Location), excerpt,
		nullableString(input.Extractor)); err != nil {
		return "", fmt.Errorf("save evidence: %w", err)
	}
	return id, nil
}

// ListEvidence returns the evidence of one paper or dataset, or all of it when
// both IDs are empty, oldest first.
func (s *Store) ListEvidence(ctx context.Context, paperID, datasetID string) ([]Evidence, error) {
	rows, err := s.db.QueryContext(ctx, evidenceSelect+`
		WHERE (? = '' OR target_paper_id = ?) AND (? = '' OR target_dataset_id = ?)
		ORDER BY observed_at, id`, paperID, paperID, datasetID, datasetID)
	if err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	defer rows.Close()
	evidence := []Evidence{}
	for rows.Next() {
		var e Evidence
		if err := rows.Scan(evidenceFields(&e)...); err != nil {
			return nil, fmt.Errorf("read evidence: %w", err)
		}
		evidence = append(evidence, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list evidence: %w", err)
	}
	return evidence, nil
}

// ReviewEvidence records a person's verdict. It is offered to the desktop window
// only; agents read verdicts through ListEvidence.
func (s *Store) ReviewEvidence(ctx context.Context, id, status string) error {
	if !oneOf(status, "unreviewed", "verified", "rejected") {
		return fmt.Errorf("invalid evidence status %q", status)
	}
	result, err := s.db.ExecContext(ctx, "UPDATE field_evidence SET review_status = ? WHERE id = ?", status, id)
	if err != nil {
		return fmt.Errorf("review evidence: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("review evidence: %w", err)
	} else if count == 0 {
		return fmt.Errorf("%w: %s", ErrEvidenceNotFound, id)
	}
	return nil
}
