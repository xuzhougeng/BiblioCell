package library

import (
	"context"
	"fmt"
	"os"
	"time"
)

// DeletePaper removes one paper and its owned files, preserving datasets.
func (s *Store) DeletePaper(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	paths, err := s.deletePaperFiles(ctx, tx, id)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM papers WHERE id = ?", id)
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
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, path := range paths {
		_ = os.Remove(path)
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

// DeleteDataset removes one dataset with its paper links, species, evidence
// and the resource entries only it held. Papers stay, unlinked. The source
// record that brought it in stays too, so the next update of that source
// skips the accession instead of adding the dataset back.
func (s *Store) DeleteDataset(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	// Resource entries no paper and no other dataset refers to go with it.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM resources WHERE id IN (
			SELECT r.resource_id FROM dataset_resources r WHERE r.dataset_id = ?
			AND NOT EXISTS (SELECT 1 FROM paper_resources p WHERE p.resource_id = r.resource_id)
			AND NOT EXISTS (SELECT 1 FROM dataset_resources o WHERE o.resource_id = r.resource_id AND o.dataset_id <> r.dataset_id))`, id); err != nil {
		return fmt.Errorf("delete dataset resources: %w", err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM datasets WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete dataset: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("dataset not found: %s", id)
	}
	return tx.Commit()
}
