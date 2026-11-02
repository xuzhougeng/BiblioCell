package library

import "context"

// MarkPaperReviewed takes a paper a source update added out of review without a
// screening decision. It is offered to the desktop window only.
func (s *Store) MarkPaperReviewed(ctx context.Context, id string) error {
	return updateOne(ctx, s.db, "paper",
		"UPDATE paper_curation SET awaiting_review = 0 WHERE paper_id = ?", id)
}

// MarkDatasetReviewed takes a dataset a source update added out of review
// without changing its status. It is offered to the desktop window only.
func (s *Store) MarkDatasetReviewed(ctx context.Context, id string) error {
	return updateOne(ctx, s.db, "dataset",
		"UPDATE datasets SET awaiting_review = 0 WHERE id = ?", id)
}
