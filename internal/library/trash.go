package library

import (
	"context"
	"fmt"
)

const trashDDL = `CREATE TABLE IF NOT EXISTS trashed_papers (
 paper_id TEXT PRIMARY KEY NOT NULL REFERENCES papers(id) ON DELETE CASCADE,
 deleted_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
CREATE TABLE IF NOT EXISTS trashed_datasets (
 dataset_id TEXT PRIMARY KEY NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
 deleted_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);`

// TrashEntry identifies a recoverable record. Its data and associations stay in
// their original tables, so every import path still sees its deduplication keys.
type TrashEntry struct {
	Kind       string `json:"kind"`
	ID         string `json:"id"`
	Title      string `json:"title"`
	Identifier string `json:"identifier"`
	DeletedAt  string `json:"deletedAt"`
}

func (s *Store) ListTrash(ctx context.Context) ([]TrashEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
 SELECT 'paper', p.id, p.title,
 coalesce((SELECT value FROM paper_identifiers WHERE paper_id=p.id ORDER BY namespace LIMIT 1), ''), t.deleted_at
 FROM trashed_papers t JOIN papers p ON p.id=t.paper_id
 UNION ALL
 SELECT 'dataset', d.id, d.title,
 coalesce((SELECT rr.repository || ' ' || rr.accession FROM dataset_repositories dr JOIN repository_records rr ON rr.id=dr.repository_record_id WHERE dr.dataset_id=d.id ORDER BY dr.rowid LIMIT 1), ''), t.deleted_at
 FROM trashed_datasets t JOIN datasets d ON d.id=t.dataset_id
 ORDER BY 5 DESC, 1, 2`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []TrashEntry{}
	for rows.Next() {
		var entry TrashEntry
		if err := rows.Scan(&entry.Kind, &entry.ID, &entry.Title, &entry.Identifier, &entry.DeletedAt); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (s *Store) RestoreTrash(ctx context.Context, kind, id string) error {
	var query string
	switch kind {
	case "paper":
		query = "DELETE FROM trashed_papers WHERE paper_id = ?"
	case "dataset":
		query = "DELETE FROM trashed_datasets WHERE dataset_id = ?"
	default:
		return fmt.Errorf("invalid trash kind: %s", kind)
	}
	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("record not in recycle bin: %s", id)
	}
	return nil
}

func isTrashed(ctx context.Context, reader queryer, kind, id string) (bool, error) {
	query := "SELECT paper_id FROM trashed_papers WHERE paper_id = ?"
	if kind == "dataset" {
		query = "SELECT dataset_id FROM trashed_datasets WHERE dataset_id = ?"
	}
	rows, err := reader.QueryContext(ctx, query, id)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	found := rows.Next()
	return found, rows.Err()
}

func rejectTrashed(ctx context.Context, reader queryer, kind, id string) error {
	if id == "" {
		return nil
	}
	trashed, err := isTrashed(ctx, reader, kind, id)
	if err != nil {
		return err
	}
	if trashed {
		return fmt.Errorf("%s %s is in the recycle bin; restore it from Settings", kind, id)
	}
	return nil
}
