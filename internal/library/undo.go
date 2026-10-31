package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrBatchNotFound indicates that no paper or dataset carries the batch ID.
var ErrBatchNotFound = errors.New("import batch not found")

// UndoResult reports what UndoImport removed. Kept lists the records a person
// has already worked on, which stay.
type UndoResult struct {
	BatchID         string   `json:"batchId"`
	PapersRemoved   int      `json:"papersRemoved"`
	DatasetsRemoved int      `json:"datasetsRemoved"`
	Kept            []string `json:"kept"`
}

const batchPapers = `
	SELECT p.id, c.screening_reviewed_at IS NOT NULL OR c.starred = 1 OR c.reading_status <> 'unread'
	    OR EXISTS (SELECT 1 FROM paper_datasets l WHERE l.paper_id = p.id AND l.verified = 1)
	    OR EXISTS (SELECT 1 FROM field_evidence e WHERE e.target_paper_id = p.id AND e.review_status <> 'unreviewed')
	FROM papers p JOIN paper_curation c ON c.paper_id = p.id WHERE p.import_batch_id = ?`

const batchDatasets = `
	SELECT d.id, d.reviewed_at IS NOT NULL
	    OR EXISTS (SELECT 1 FROM paper_datasets l WHERE l.dataset_id = d.id AND l.verified = 1)
	    OR EXISTS (SELECT 1 FROM dataset_repositories r WHERE r.dataset_id = d.id AND r.verified = 1)
	    OR EXISTS (SELECT 1 FROM field_evidence e WHERE e.target_dataset_id = d.id AND e.review_status <> 'unreviewed')
	FROM datasets d WHERE d.import_batch_id = ?`

// UndoImport removes what one agent batch created, with the copies of its files
// in the storage folder. Anything a person has reviewed, starred, read, or
// verified stays and is listed in Kept.
func (s *Store) UndoImport(ctx context.Context, batchID string) (UndoResult, error) {
	batchID = strings.TrimSpace(batchID)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return UndoResult{}, fmt.Errorf("begin undo: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result := UndoResult{BatchID: batchID, Kept: []string{}}
	papers, kept, err := batchRecords(ctx, tx, batchPapers, batchID)
	if err != nil {
		return UndoResult{}, err
	}
	result.Kept = append(result.Kept, kept...)
	datasets, kept, err := batchRecords(ctx, tx, batchDatasets, batchID)
	if err != nil {
		return UndoResult{}, err
	}
	result.Kept = append(result.Kept, kept...)
	if len(papers)+len(datasets)+len(result.Kept) == 0 {
		return UndoResult{}, fmt.Errorf("%w: %s", ErrBatchNotFound, batchID)
	}

	stored := []string{}
	for _, id := range papers {
		files, err := s.deletePaperFiles(ctx, tx, id)
		if err != nil {
			return UndoResult{}, err
		}
		stored = append(stored, files...)
		if _, err := tx.ExecContext(ctx, "DELETE FROM papers WHERE id = ?", id); err != nil {
			return UndoResult{}, fmt.Errorf("remove paper %s: %w", id, err)
		}
	}
	for _, id := range datasets {
		if _, err := tx.ExecContext(ctx, "DELETE FROM datasets WHERE id = ?", id); err != nil {
			return UndoResult{}, fmt.Errorf("remove dataset %s: %w", id, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return UndoResult{}, fmt.Errorf("commit undo: %w", err)
	}
	for _, path := range stored {
		_ = os.Remove(path) // A leftover copy is only unused disk space.
	}
	result.PapersRemoved, result.DatasetsRemoved = len(papers), len(datasets)
	return result, nil
}

// batchRecords splits a batch's records into those free to remove and those a
// person has touched. The query selects an ID and whether it was touched.
func batchRecords(ctx context.Context, tx *sql.Tx, query, batchID string) (free, touched []string, err error) {
	rows, err := tx.QueryContext(ctx, query, batchID)
	if err != nil {
		return nil, nil, fmt.Errorf("read batch: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var worked bool
		if err := rows.Scan(&id, &worked); err != nil {
			return nil, nil, fmt.Errorf("read batch: %w", err)
		}
		if worked {
			touched = append(touched, id)
		} else {
			free = append(free, id)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, errors.Join(errors.New("read batch"), err)
	}
	return free, touched, nil
}
