package library

import (
	"context"
	"fmt"
	"os"
	"time"
)

// DeletePaper moves a paper to the recycle bin, preserving all files and links.
func (s *Store) DeletePaper(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO trashed_papers(paper_id)
 SELECT id FROM papers WHERE id = ? AND id NOT IN (SELECT paper_id FROM trashed_papers)`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("paper not found: %s", id)
	}
	return nil
}

// DismissPaper marks one paper irrelevant and removes its stored files. The
// record itself stays, with its DOI, PMID and title: that is what keeps a source
// update or an import from bringing the paper back. Notes and the screening
// reason stay too, so the decision can be reversed from the papers table.
func (s *Store) DismissPaper(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	paths, err := s.deletePaperFiles(ctx, tx, id)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.ExecContext(ctx, `
		UPDATE paper_curation SET screening_status = 'irrelevant', screening_method = 'human',
			screening_reviewed_at = ?, proposed_screening = '', proposed_reason = '',
			awaiting_review = 0, updated_at = ?
		WHERE paper_id = ?`, now, now, id)
	if err != nil {
		return fmt.Errorf("dismiss paper: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("paper not found: %s", id)
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, path := range paths {
		_ = os.Remove(path)
	}
	return nil
}

// DeleteDataset hides a dataset while retaining its accession, files and links.
func (s *Store) DeleteDataset(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `INSERT INTO trashed_datasets(dataset_id)
 SELECT id FROM datasets WHERE id = ? AND id NOT IN (SELECT dataset_id FROM trashed_datasets)`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("dataset not found: %s", id)
	}
	return nil
}
