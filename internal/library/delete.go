package library

import (
	"context"
	"fmt"
	"os"
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
