package library

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrDatasetNotFound indicates that the requested real dataset does not exist.
	ErrDatasetNotFound = errors.New("dataset not found")
	// ErrDatasetReviewed indicates an agent edit of a dataset a person verified.
	ErrDatasetReviewed = errors.New("dataset was verified by a person; an agent can no longer change it")
)

// RepositoryInput is what an agent may state about a repository record. Whether
// it really belongs to the dataset stays for a person to verify.
type RepositoryInput struct {
	Repository   string `json:"repository" jsonschema:"geo, sra, ena, gsa, bioproject, biosample, biostudies, zenodo, figshare, dryad, or other"`
	Accession    string `json:"accession" jsonschema:"Repository accession such as GSE123456"`
	RecordKind   string `json:"recordKind,omitempty" jsonschema:"unknown, project, study, sample, experiment, run, analysis, collection, or other"`
	URL          string `json:"url,omitempty"`
	PublicStatus string `json:"publicStatus,omitempty" jsonschema:"unknown, public, embargoed, restricted, or withdrawn"`
	MappingScope string `json:"mappingScope,omitempty" jsonschema:"unknown, exact, subset, overlap, or related"`
	EvidenceURL  string `json:"evidenceUrl,omitempty" jsonschema:"Where the paper or repository states this accession"`
	EvidenceText string `json:"evidenceText,omitempty" jsonschema:"Short quote a person can check"`
}

func (r RepositoryInput) record() DatasetRepository {
	return DatasetRepository{
		Repository: r.Repository, Accession: r.Accession, RecordKind: r.RecordKind, URL: r.URL,
		PublicStatus: r.PublicStatus, MappingScope: r.MappingScope,
		EvidenceURL: r.EvidenceURL, EvidenceText: r.EvidenceText,
	}
}

// PaperLinkInput is an agent's claim that a paper relates to a dataset.
type PaperLinkInput struct {
	PaperID      string `json:"paperId" jsonschema:"Stable ID of the paper"`
	Role         string `json:"role,omitempty" jsonschema:"generated, reused, reprocessed, benchmarked, or unknown"`
	EvidenceURL  string `json:"evidenceUrl,omitempty" jsonschema:"Where the paper states this relation; this or evidenceText is required"`
	EvidenceText string `json:"evidenceText,omitempty" jsonschema:"Short quote a person can check; this or evidenceUrl is required"`
}

// DatasetInput is what an agent may state about a new dataset.
type DatasetInput struct {
	Title                  string            `json:"title" jsonschema:"Dataset title; required"`
	Description            string            `json:"description,omitempty"`
	Species                []SpeciesEntry    `json:"species,omitempty"`
	Organs                 []string          `json:"organs,omitempty"`
	Modalities             []string          `json:"modalities,omitempty"`
	Platforms              []string          `json:"platforms,omitempty"`
	Repositories           []RepositoryInput `json:"repositories,omitempty"`
	Papers                 []PaperLinkInput  `json:"papers,omitempty" jsonschema:"Papers to link; each needs evidence"`
	Status                 string            `json:"status,omitempty" jsonschema:"pending or identified; only a person can verify a dataset"`
	Availability           string            `json:"availability,omitempty" jsonschema:"unknown, public, unavailable, embargoed, or restricted"`
	MatrixAvailability     string            `json:"matrixAvailability,omitempty"`
	AnnotationAvailability string            `json:"annotationAvailability,omitempty"`
	CellsReported          *int              `json:"cellsReported,omitempty"`
	CellCountUnit          string            `json:"cellCountUnit,omitempty" jsonschema:"cells, nuclei, observations, or not_specified; required with cellsReported"`
	CellCountScope         string            `json:"cellCountScope,omitempty" jsonschema:"What the count covers; required with cellsReported"`
	BatchID                string            `json:"batchId,omitempty" jsonschema:"Import batch to join, so undo_import removes this dataset with it"`
}

// DatasetResult says whether AddDataset created the dataset or found it.
type DatasetResult struct {
	Dataset Dataset `json:"dataset"`
	Created bool    `json:"created"`
}

// AddDataset creates a dataset for an agent and links its papers. A dataset the
// library already holds, matched by accession or else by title, is kept as it is
// and only gains the new links, so rerunning is safe.
func (s *Store) AddDataset(ctx context.Context, input DatasetInput) (DatasetResult, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return DatasetResult{}, fmt.Errorf("begin dataset add: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	id, err := existingDatasetID(ctx, tx, input)
	if err != nil {
		return DatasetResult{}, err
	}
	created := id == ""
	if created {
		if id, err = newLocalID("DS"); err != nil {
			return DatasetResult{}, err
		}
		batchID := strings.TrimSpace(input.BatchID)
		if batchID == "" {
			if batchID, err = newLocalID("B"); err != nil {
				return DatasetResult{}, err
			}
		}
		dataset := Dataset{
			ID: id, Title: input.Title, Description: input.Description, Species: input.Species,
			Organs: input.Organs, Modalities: input.Modalities, Platforms: input.Platforms,
			Status: input.Status, Availability: input.Availability,
			MatrixAvailability: input.MatrixAvailability, AnnotationAvailability: input.AnnotationAvailability,
			CellsReported: input.CellsReported, CellCountUnit: input.CellCountUnit,
			CellCountScope: input.CellCountScope, BatchID: batchID,
		}
		for _, repository := range input.Repositories {
			dataset.Repositories = append(dataset.Repositories, repository.record())
		}
		if err := saveDataset(ctx, tx, &dataset); err != nil {
			return DatasetResult{}, err
		}
	}
	for _, link := range input.Papers {
		if err := linkDataset(ctx, tx, id, link); err != nil {
			return DatasetResult{}, err
		}
	}
	dataset, err := readDataset(ctx, tx, id)
	if err != nil {
		return DatasetResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DatasetResult{}, fmt.Errorf("commit dataset add: %w", err)
	}
	return DatasetResult{Dataset: dataset, Created: created}, nil
}

func existingDatasetID(ctx context.Context, tx *sql.Tx, input DatasetInput) (string, error) {
	var id string
	for _, repository := range input.Repositories {
		err := tx.QueryRowContext(ctx, `
			SELECT dr.dataset_id FROM dataset_repositories dr
			JOIN repository_records rr ON rr.id = dr.repository_record_id
			WHERE rr.repository = ? AND rr.accession = ? LIMIT 1`,
			strings.ToLower(strings.TrimSpace(repository.Repository)), strings.TrimSpace(repository.Accession)).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("check dataset accession: %w", err)
		}
	}
	if len(input.Repositories) > 0 {
		return "", nil
	}
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM datasets WHERE lower(trim(title)) = lower(trim(?)) LIMIT 1", input.Title).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("check dataset title: %w", err)
	}
	return id, nil
}

// DatasetPatch contains an agent's edits. Nil fields remain unchanged; a non-nil
// slice replaces the list, except AddRepositories, which only adds.
type DatasetPatch struct {
	Title                  *string           `json:"title,omitempty"`
	Description            *string           `json:"description,omitempty"`
	Species                *[]SpeciesEntry   `json:"species,omitempty"`
	Organs                 *[]string         `json:"organs,omitempty"`
	Modalities             *[]string         `json:"modalities,omitempty"`
	Platforms              *[]string         `json:"platforms,omitempty"`
	AddRepositories        []RepositoryInput `json:"addRepositories,omitempty" jsonschema:"Repository records to add; known accessions are left as they are"`
	Status                 *string           `json:"status,omitempty" jsonschema:"pending or identified; only a person can verify a dataset"`
	Availability           *string           `json:"availability,omitempty"`
	MatrixAvailability     *string           `json:"matrixAvailability,omitempty"`
	AnnotationAvailability *string           `json:"annotationAvailability,omitempty"`
	CellsReported          *int              `json:"cellsReported,omitempty"`
	CellCountUnit          *string           `json:"cellCountUnit,omitempty"`
	CellCountScope         *string           `json:"cellCountScope,omitempty"`
}

// UpdateDataset applies an agent's patch until a person verifies the dataset.
func (s *Store) UpdateDataset(ctx context.Context, id string, patch DatasetPatch) (Dataset, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Dataset{}, fmt.Errorf("begin dataset update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	dataset, err := readDataset(ctx, tx, strings.TrimSpace(id))
	if err != nil {
		return Dataset{}, err
	}
	if dataset.ReviewedAt != "" {
		return Dataset{}, ErrDatasetReviewed
	}
	applyDatasetPatch(&dataset, patch)
	if err := saveDataset(ctx, tx, &dataset); err != nil {
		return Dataset{}, err
	}
	updated, err := readDataset(ctx, tx, dataset.ID)
	if err != nil {
		return Dataset{}, err
	}
	if err := tx.Commit(); err != nil {
		return Dataset{}, fmt.Errorf("commit dataset update: %w", err)
	}
	return updated, nil
}

func applyDatasetPatch(d *Dataset, patch DatasetPatch) {
	set := func(target *string, value *string) {
		if value != nil {
			*target = *value
		}
	}
	set(&d.Title, patch.Title)
	set(&d.Description, patch.Description)
	set(&d.Status, patch.Status)
	set(&d.Availability, patch.Availability)
	set(&d.MatrixAvailability, patch.MatrixAvailability)
	set(&d.AnnotationAvailability, patch.AnnotationAvailability)
	set(&d.CellCountUnit, patch.CellCountUnit)
	set(&d.CellCountScope, patch.CellCountScope)
	if patch.Species != nil {
		d.Species = *patch.Species
	}
	if patch.Organs != nil {
		d.Organs = *patch.Organs
	}
	if patch.Modalities != nil {
		d.Modalities = *patch.Modalities
	}
	if patch.Platforms != nil {
		d.Platforms = *patch.Platforms
	}
	if patch.CellsReported != nil {
		d.CellsReported = patch.CellsReported
	}
	for _, repository := range patch.AddRepositories {
		known := false
		for _, existing := range d.Repositories {
			known = known || (strings.EqualFold(existing.Repository, strings.TrimSpace(repository.Repository)) &&
				existing.Accession == strings.TrimSpace(repository.Accession))
		}
		if !known {
			d.Repositories = append(d.Repositories, repository.record())
		}
	}
}

// LinkDataset records an agent's claim that a paper relates to a dataset.
func (s *Store) LinkDataset(ctx context.Context, datasetID string, link PaperLinkInput) (Dataset, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Dataset{}, fmt.Errorf("begin dataset link: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	datasetID = strings.TrimSpace(datasetID)
	if _, err := readDataset(ctx, tx, datasetID); err != nil {
		return Dataset{}, err
	}
	if err := linkDataset(ctx, tx, datasetID, link); err != nil {
		return Dataset{}, err
	}
	dataset, err := readDataset(ctx, tx, datasetID)
	if err != nil {
		return Dataset{}, err
	}
	if err := tx.Commit(); err != nil {
		return Dataset{}, fmt.Errorf("commit dataset link: %w", err)
	}
	return dataset, nil
}

// linkDataset upserts an unverified link. A link a person verified stays as it is.
func linkDataset(ctx context.Context, tx *sql.Tx, datasetID string, link PaperLinkInput) error {
	paperID, role := strings.TrimSpace(link.PaperID), strings.TrimSpace(link.Role)
	if role == "" {
		role = "unknown"
	}
	if !oneOf(role, "generated", "reused", "reprocessed", "benchmarked", "unknown") {
		return fmt.Errorf("invalid paper link role %q", role)
	}
	if strings.TrimSpace(link.EvidenceURL) == "" && strings.TrimSpace(link.EvidenceText) == "" {
		return errors.New("a paper link needs evidenceUrl or evidenceText for the person checking it")
	}
	if _, err := readPaper(ctx, tx, paperID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO paper_datasets(paper_id, dataset_id, role, evidence_url, evidence_text, verified)
		VALUES(?, ?, ?, ?, ?, 0)
		ON CONFLICT(paper_id, dataset_id, role) DO UPDATE SET
			evidence_url = excluded.evidence_url, evidence_text = excluded.evidence_text
		WHERE verified = 0`,
		paperID, datasetID, role, nullableString(link.EvidenceURL), nullableString(link.EvidenceText)); err != nil {
		return fmt.Errorf("link paper %s to dataset %s: %w", paperID, datasetID, err)
	}
	return nil
}

// VerifyDatasetLink records a person's check of a paper-dataset link.
func (s *Store) VerifyDatasetLink(ctx context.Context, paperID, datasetID, role string, verified bool) error {
	return updateOne(ctx, s.db, "paper link",
		"UPDATE paper_datasets SET verified = ? WHERE paper_id = ? AND dataset_id = ? AND role = ?",
		boolInt(verified), paperID, datasetID, role)
}

// VerifyDatasetRepository records a person's check of a repository record.
func (s *Store) VerifyDatasetRepository(ctx context.Context, datasetID, recordID string, verified bool) error {
	return updateOne(ctx, s.db, "dataset repository",
		"UPDATE dataset_repositories SET verified = ? WHERE dataset_id = ? AND repository_record_id = ?",
		boolInt(verified), datasetID, recordID)
}

func updateOne(ctx context.Context, db *sql.DB, what, statement string, args ...any) error {
	result, err := db.ExecContext(ctx, statement, args...)
	if err != nil {
		return fmt.Errorf("update %s: %w", what, err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("update %s: %w", what, err)
	} else if count == 0 {
		return fmt.Errorf("%s not found", what)
	}
	return nil
}

// DatasetFilter narrows a dataset listing for MCP and the CLI.
type DatasetFilter struct {
	Query  string `json:"query,omitempty" jsonschema:"Optional text matched against title, accession, species, organs, modalities, and platforms"`
	Status string `json:"status,omitempty" jsonschema:"Only datasets with this status: pending, identified, or verified"`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum datasets to return, from 1 to 500; defaults to 100"`
	Offset int    `json:"offset,omitempty" jsonschema:"Matching datasets to skip; use it with limit to page through total"`
}

// FilterDatasets returns the requested page of matching datasets and how many
// match in all. A zero limit means every match after offset.
func FilterDatasets(datasets []Dataset, filter DatasetFilter) ([]Dataset, int) {
	query := strings.ToLower(strings.TrimSpace(filter.Query))
	matches := make([]Dataset, 0, len(datasets))
	for _, d := range datasets {
		if filter.Status != "" && d.Status != filter.Status {
			continue
		}
		texts := []string{d.Title, strings.Join(d.Organs, " "), strings.Join(d.Modalities, " "), strings.Join(d.Platforms, " ")}
		for _, entry := range d.Species {
			texts = append(texts, entry.Name, entry.Label)
		}
		for _, repository := range d.Repositories {
			texts = append(texts, repository.Repository, repository.Accession)
		}
		if query == "" || strings.Contains(strings.ToLower(strings.Join(texts, " ")), query) {
			matches = append(matches, d)
		}
	}
	return page(matches, filter.Limit, filter.Offset), len(matches)
}
