package library

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// DatasetRepository preserves the normalized repository record and the
// dataset-specific mapping metadata. Repository/accession identify the shared
// external record; verified and evidence belong to this dataset's association.
type DatasetRepository struct {
	ID           string `json:"id"`
	Repository   string `json:"repository"`
	Accession    string `json:"accession"`
	RecordKind   string `json:"recordKind"`
	URL          string `json:"url"`
	PublicStatus string `json:"publicStatus"`
	MappingScope string `json:"mappingScope"`
	EvidenceURL  string `json:"evidenceUrl"`
	EvidenceText string `json:"evidenceText"`
	Verified     bool   `json:"verified"`
}

// DatasetPaperLink describes one role a paper plays in relation to a dataset.
type DatasetPaperLink struct {
	PaperID      string `json:"paperId"`
	Role         string `json:"role"`
	EvidenceURL  string `json:"evidenceUrl"`
	EvidenceText string `json:"evidenceText"`
	Verified     bool   `json:"verified"`
}

// Dataset is the document-shaped representation used by the desktop frontend.
// CurationLevel retains the detailed database state when Status is only a
// three-state display value. Repositories and PaperLinks retain metadata that
// the current UI does not edit so saves do not erase it.
type Dataset struct {
	ID                     string              `json:"id"`
	Title                  string              `json:"title"`
	Description            string              `json:"description"`
	Notes                  string              `json:"notes"`
	Species                []SpeciesEntry      `json:"species"`
	Organs                 []string            `json:"organs"`
	Modalities             []string            `json:"modalities"`
	Platforms              []string            `json:"platforms"`
	Repositories           []DatasetRepository `json:"repositories"`
	Repository             string              `json:"repository"`
	Accession              string              `json:"accession"`
	Status                 string              `json:"status"`
	CurationLevel          string              `json:"curationLevel"`
	Availability           string              `json:"availability"`
	MatrixAvailability     string              `json:"matrixAvailability"`
	AnnotationAvailability string              `json:"annotationAvailability"`
	CellsReported          *int                `json:"cellsReported"`
	CellCountUnit          string              `json:"cellCountUnit"`
	CellCountScope         string              `json:"cellCountScope"`
	PaperLinks             []DatasetPaperLink  `json:"paperLinks"`
	AddedAt                string              `json:"addedAt"`
	UpdatedAt              string              `json:"updatedAt"`
	IsDemo                 bool                `json:"isDemo"`
}

// ListDatasets returns real datasets and all normalized associations.
func (s *Store) ListDatasets(ctx context.Context) ([]Dataset, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, title, coalesce(description, ''), notes, species_json, organs_json,
		       modalities_json, platforms_json, curation_level, raw_availability,
		       matrix_availability, annotation_availability, n_cells_reported,
		       coalesce(cell_count_unit, ''), coalesce(cell_count_scope, ''),
		       first_seen_at, updated_at
		FROM datasets ORDER BY first_seen_at DESC, id`)
	if err != nil {
		return nil, fmt.Errorf("list datasets: %w", err)
	}
	datasets := []Dataset{}
	indexes := map[string]int{}
	for rows.Next() {
		var d Dataset
		var species, organs, modalities, platforms string
		var cells sql.NullInt64
		if err := rows.Scan(
			&d.ID, &d.Title, &d.Description, &d.Notes, &species, &organs,
			&modalities, &platforms, &d.CurationLevel, &d.Availability,
			&d.MatrixAvailability, &d.AnnotationAvailability, &cells,
			&d.CellCountUnit, &d.CellCountScope, &d.AddedAt, &d.UpdatedAt,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("read dataset: %w", err)
		}
		d.Species = speciesEntries(species)
		d.Organs = jsonStrings(organs)
		d.Modalities = jsonStrings(modalities)
		d.Platforms = jsonStrings(platforms)
		d.Repositories = []DatasetRepository{}
		d.PaperLinks = []DatasetPaperLink{}
		d.Status = datasetStatus(d.CurationLevel)
		d.Availability = displayAvailability(d.Availability)
		d.MatrixAvailability = displayAvailability(d.MatrixAvailability)
		d.AnnotationAvailability = displayAvailability(d.AnnotationAvailability)
		if cells.Valid {
			value := int(cells.Int64)
			d.CellsReported = &value
		}
		d.IsDemo = false
		indexes[d.ID] = len(datasets)
		datasets = append(datasets, d)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("list datasets: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close datasets: %w", err)
	}

	repositoryRows, err := s.db.QueryContext(ctx, `
		SELECT dr.dataset_id, rr.id, rr.repository, rr.accession, rr.record_kind,
		       coalesce(rr.url, ''), rr.public_status, dr.mapping_scope,
		       coalesce(dr.evidence_url, ''), coalesce(dr.evidence_text, ''), dr.verified
		FROM dataset_repositories dr
		JOIN repository_records rr ON rr.id = dr.repository_record_id
		ORDER BY dr.dataset_id, dr.rowid`)
	if err != nil {
		return nil, fmt.Errorf("list dataset repositories: %w", err)
	}
	for repositoryRows.Next() {
		var datasetID string
		var repository DatasetRepository
		var verified int
		if err := repositoryRows.Scan(
			&datasetID, &repository.ID, &repository.Repository, &repository.Accession,
			&repository.RecordKind, &repository.URL, &repository.PublicStatus,
			&repository.MappingScope, &repository.EvidenceURL, &repository.EvidenceText, &verified,
		); err != nil {
			repositoryRows.Close()
			return nil, fmt.Errorf("read dataset repository: %w", err)
		}
		repository.Verified = verified == 1
		if index, ok := indexes[datasetID]; ok {
			datasets[index].Repositories = append(datasets[index].Repositories, repository)
		}
	}
	if err := repositoryRows.Err(); err != nil {
		repositoryRows.Close()
		return nil, fmt.Errorf("list dataset repositories: %w", err)
	}
	if err := repositoryRows.Close(); err != nil {
		return nil, fmt.Errorf("close dataset repositories: %w", err)
	}

	linkRows, err := s.db.QueryContext(ctx, `
		SELECT dataset_id, paper_id, role, coalesce(evidence_url, ''),
		       coalesce(evidence_text, ''), verified
		FROM paper_datasets ORDER BY dataset_id, rowid`)
	if err != nil {
		return nil, fmt.Errorf("list dataset paper links: %w", err)
	}
	for linkRows.Next() {
		var datasetID string
		var link DatasetPaperLink
		var verified int
		if err := linkRows.Scan(&datasetID, &link.PaperID, &link.Role, &link.EvidenceURL, &link.EvidenceText, &verified); err != nil {
			linkRows.Close()
			return nil, fmt.Errorf("read dataset paper link: %w", err)
		}
		link.Verified = verified == 1
		if index, ok := indexes[datasetID]; ok {
			datasets[index].PaperLinks = append(datasets[index].PaperLinks, link)
		}
	}
	if err := linkRows.Err(); err != nil {
		linkRows.Close()
		return nil, fmt.Errorf("list dataset paper links: %w", err)
	}
	if err := linkRows.Close(); err != nil {
		return nil, fmt.Errorf("close dataset paper links: %w", err)
	}

	for i := range datasets {
		if len(datasets[i].Repositories) > 0 {
			datasets[i].Repository = datasets[i].Repositories[0].Repository
			datasets[i].Accession = datasets[i].Repositories[0].Accession
		}
	}
	return datasets, nil
}

// SaveDatasets upserts datasets without deleting records or associations added
// elsewhere. Each call is atomic across all supplied datasets.
func (s *Store) SaveDatasets(ctx context.Context, datasets []Dataset) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dataset save: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	seen := make(map[string]bool, len(datasets))
	for i := range datasets {
		if seen[datasets[i].ID] {
			return fmt.Errorf("duplicate dataset ID %q", datasets[i].ID)
		}
		seen[datasets[i].ID] = true
		if err := saveDataset(ctx, tx, &datasets[i]); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("save datasets: %w", err)
	}
	return nil
}

func saveDataset(ctx context.Context, tx *sql.Tx, d *Dataset) error {
	if err := normalizeDataset(d); err != nil {
		return err
	}
	// The legacy single-value columns (tissue_text, modality, technology) are
	// no longer written; the *_json arrays are authoritative.
	var cells any
	if d.CellsReported != nil {
		cells = *d.CellsReported
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO datasets(
			id, title, description, notes, species_json, organs_json, modalities_json,
			platforms_json, n_cells_reported,
			cell_count_unit, cell_count_scope, raw_availability, matrix_availability,
			annotation_availability, curation_level, first_seen_at, updated_at
		) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			title = excluded.title, description = excluded.description, notes = excluded.notes,
			species_json = excluded.species_json, organs_json = excluded.organs_json,
			modalities_json = excluded.modalities_json, platforms_json = excluded.platforms_json,
			n_cells_reported = excluded.n_cells_reported,
			cell_count_unit = excluded.cell_count_unit, cell_count_scope = excluded.cell_count_scope,
			raw_availability = excluded.raw_availability,
			matrix_availability = excluded.matrix_availability,
			annotation_availability = excluded.annotation_availability,
			curation_level = excluded.curation_level, updated_at = excluded.updated_at`,
		d.ID, d.Title, nullableString(d.Description), d.Notes, speciesJSONArray(d.Species),
		jsonArray(d.Organs), jsonArray(d.Modalities), jsonArray(d.Platforms), cells,
		nullableString(d.CellCountUnit), nullableString(d.CellCountScope),
		d.Availability, d.MatrixAvailability, d.AnnotationAvailability,
		d.CurationLevel, d.AddedAt, d.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save dataset %s: %w", d.ID, err)
	}
	if err := saveDatasetRepositories(ctx, tx, d); err != nil {
		return err
	}
	for _, link := range d.PaperLinks {
		link.PaperID = strings.TrimSpace(link.PaperID)
		link.Role = strings.TrimSpace(link.Role)
		if link.PaperID == "" || !oneOf(link.Role, "generated", "reused", "reprocessed", "benchmarked", "unknown") {
			return fmt.Errorf("invalid paper link for dataset %s", d.ID)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO paper_datasets(paper_id, dataset_id, role, evidence_url, evidence_text, verified)
			VALUES(?, ?, ?, ?, ?, ?)
			ON CONFLICT(paper_id, dataset_id, role) DO UPDATE SET
				evidence_url = excluded.evidence_url, evidence_text = excluded.evidence_text,
				verified = excluded.verified`,
			link.PaperID, d.ID, link.Role, nullableString(link.EvidenceURL),
			nullableString(link.EvidenceText), boolInt(link.Verified)); err != nil {
			return fmt.Errorf("save paper link for dataset %s: %w", d.ID, err)
		}
	}
	return nil
}

func normalizeDataset(d *Dataset) error {
	d.ID = strings.TrimSpace(d.ID)
	d.Title = strings.TrimSpace(d.Title)
	d.Description = strings.TrimSpace(d.Description)
	if d.ID == "" || d.Title == "" {
		return errors.New("dataset ID and title are required")
	}
	d.Species = cleanSpecies(d.Species)
	d.Organs = cleanStrings(d.Organs)
	d.Modalities = canonicalDatasetModalities(d.Modalities)
	d.Platforms = cleanStrings(d.Platforms)
	if d.Status == "" {
		d.Status = datasetStatus(d.CurationLevel)
	}
	if !oneOf(d.Status, "pending", "identified", "verified") {
		return fmt.Errorf("invalid dataset status %q", d.Status)
	}
	if d.CurationLevel == "" || d.Status != datasetStatus(d.CurationLevel) {
		d.CurationLevel = datasetCurationLevel(d.Status)
	}
	if !oneOf(d.CurationLevel, "indexed", "samples_checked", "files_verified", "matrix_standardized", "reprocessed") {
		return fmt.Errorf("invalid dataset curation level %q", d.CurationLevel)
	}
	var err error
	if d.Availability, err = databaseAvailability(d.Availability); err != nil {
		return err
	}
	if d.MatrixAvailability, err = databaseAvailability(d.MatrixAvailability); err != nil {
		return err
	}
	if d.AnnotationAvailability, err = databaseAvailability(d.AnnotationAvailability); err != nil {
		return err
	}
	if d.CellsReported != nil {
		if *d.CellsReported < 0 {
			return errors.New("dataset cell count cannot be negative")
		}
		if !oneOf(d.CellCountUnit, "cells", "nuclei", "observations", "not_specified") || strings.TrimSpace(d.CellCountScope) == "" {
			return errors.New("dataset cell count requires a valid unit and scope")
		}
	}
	if len(d.Repositories) == 0 && (strings.TrimSpace(d.Repository) != "" || strings.TrimSpace(d.Accession) != "") {
		d.Repositories = []DatasetRepository{{Repository: d.Repository, Accession: d.Accession}}
	}
	if d.Status == "verified" {
		hasAccession := false
		for _, repository := range d.Repositories {
			if strings.TrimSpace(repository.Repository) != "" && strings.TrimSpace(repository.Accession) != "" {
				hasAccession = true
				break
			}
		}
		if !hasAccession {
			return errors.New("a verified dataset needs a repository accession")
		}
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if d.AddedAt == "" {
		d.AddedAt = now
	}
	d.UpdatedAt = now
	d.IsDemo = false
	return nil
}

func saveDatasetRepositories(ctx context.Context, tx *sql.Tx, d *Dataset) error {
	for _, repository := range d.Repositories {
		repository.ID = strings.TrimSpace(repository.ID)
		repository.Repository = strings.ToLower(strings.TrimSpace(repository.Repository))
		repository.Accession = strings.TrimSpace(repository.Accession)
		if repository.ID != "" && repository.Repository == "" && repository.Accession == "" {
			if _, err := tx.ExecContext(ctx,
				"DELETE FROM dataset_repositories WHERE dataset_id = ? AND repository_record_id = ?",
				d.ID, repository.ID); err != nil {
				return fmt.Errorf("unlink dataset repository: %w", err)
			}
			continue
		}
		if repository.Repository == "" || repository.Accession == "" {
			return fmt.Errorf("dataset %s repository and accession must be provided together", d.ID)
		}
		if !oneOf(repository.Repository, "geo", "sra", "ena", "gsa", "bioproject", "biosample", "biostudies", "zenodo", "figshare", "dryad", "other") {
			return fmt.Errorf("invalid repository %q", repository.Repository)
		}
		if repository.RecordKind == "" {
			repository.RecordKind = "unknown"
		}
		if repository.PublicStatus == "" {
			repository.PublicStatus = "unknown"
		}
		if repository.MappingScope == "" {
			repository.MappingScope = "unknown"
		}
		if !oneOf(repository.RecordKind, "unknown", "project", "study", "sample", "experiment", "run", "analysis", "collection", "other") ||
			!oneOf(repository.PublicStatus, "unknown", "public", "embargoed", "restricted", "withdrawn") ||
			!oneOf(repository.MappingScope, "unknown", "exact", "subset", "overlap", "related") {
			return fmt.Errorf("invalid repository metadata for dataset %s", d.ID)
		}

		recordID, err := saveRepositoryRecord(ctx, tx, d.ID, repository)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO dataset_repositories(
				dataset_id, repository_record_id, mapping_scope, evidence_url, evidence_text, verified
			) VALUES(?, ?, ?, ?, ?, ?)
			ON CONFLICT(dataset_id, repository_record_id) DO UPDATE SET
				mapping_scope = excluded.mapping_scope, evidence_url = excluded.evidence_url,
				evidence_text = excluded.evidence_text, verified = excluded.verified`,
			d.ID, recordID, repository.MappingScope, nullableString(repository.EvidenceURL),
			nullableString(repository.EvidenceText), boolInt(repository.Verified)); err != nil {
			return fmt.Errorf("save dataset repository association: %w", err)
		}
	}
	return nil
}

// saveRepositoryRecord updates an unshared record in place where possible.
// Shared records are never mutated: a changed accession gets a new/existing
// record and only the current dataset's old association is replaced.
func saveRepositoryRecord(ctx context.Context, tx *sql.Tx, datasetID string, repository DatasetRepository) (string, error) {
	if repository.ID != "" {
		var oldRepository, oldAccession string
		err := tx.QueryRowContext(ctx,
			"SELECT repository, accession FROM repository_records WHERE id = ?", repository.ID).
			Scan(&oldRepository, &oldAccession)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("read repository record: %w", err)
		}
		if err == nil && (oldRepository != repository.Repository || oldAccession != repository.Accession) {
			var existingID string
			lookupErr := tx.QueryRowContext(ctx,
				"SELECT id FROM repository_records WHERE repository = ? AND accession = ?",
				repository.Repository, repository.Accession).Scan(&existingID)
			if lookupErr != nil && !errors.Is(lookupErr, sql.ErrNoRows) {
				return "", fmt.Errorf("find repository record: %w", lookupErr)
			}
			var references int
			if err := tx.QueryRowContext(ctx,
				"SELECT count(*) FROM dataset_repositories WHERE repository_record_id = ?", repository.ID).
				Scan(&references); err != nil {
				return "", fmt.Errorf("count repository references: %w", err)
			}
			if lookupErr == nil || references > 1 {
				if _, err := tx.ExecContext(ctx,
					"DELETE FROM dataset_repositories WHERE dataset_id = ? AND repository_record_id = ?",
					datasetID, repository.ID); err != nil {
					return "", fmt.Errorf("replace dataset repository association: %w", err)
				}
				if lookupErr == nil {
					return existingID, nil
				}
				repository.ID = ""
			} else {
				if _, err := tx.ExecContext(ctx,
					"UPDATE repository_records SET repository = ?, accession = ? WHERE id = ?",
					repository.Repository, repository.Accession, repository.ID); err != nil {
					return "", fmt.Errorf("update repository record: %w", err)
				}
				return repository.ID, nil
			}
		} else if err == nil {
			return repository.ID, nil
		}
	}

	var existingID string
	err := tx.QueryRowContext(ctx,
		"SELECT id FROM repository_records WHERE repository = ? AND accession = ?",
		repository.Repository, repository.Accession).Scan(&existingID)
	if err == nil {
		return existingID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("find repository record: %w", err)
	}
	if repository.ID == "" {
		repository.ID, err = newRepositoryRecordID()
		if err != nil {
			return "", err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO repository_records(id, repository, accession, record_kind, url, public_status)
		VALUES(?, ?, ?, ?, ?, ?)`, repository.ID, repository.Repository, repository.Accession,
		repository.RecordKind, nullableString(repository.URL), repository.PublicStatus); err != nil {
		return "", fmt.Errorf("create repository record: %w", err)
	}
	return repository.ID, nil
}

func datasetStatus(level string) string {
	switch level {
	case "samples_checked":
		return "identified"
	case "files_verified", "matrix_standardized", "reprocessed":
		return "verified"
	default:
		return "pending"
	}
}

func datasetCurationLevel(status string) string {
	switch status {
	case "identified":
		return "samples_checked"
	case "verified":
		return "files_verified"
	default:
		return "indexed"
	}
}

func displayAvailability(value string) string {
	if value == "available" {
		return "public"
	}
	return value
}

func databaseAvailability(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "unknown"
	}
	if value == "public" {
		value = "available"
	}
	if !oneOf(value, "unknown", "available", "unavailable", "embargoed", "restricted") {
		return "", fmt.Errorf("invalid dataset availability %q", value)
	}
	return value, nil
}

func canonicalDatasetModalities(values []string) []string {
	values = cleanStrings(values)
	for i, value := range values {
		switch strings.ToLower(value) {
		case "scrna", "scrna-seq":
			values[i] = "scRNA-seq"
		case "snrna", "snrna-seq":
			values[i] = "snRNA-seq"
		case "scatac", "scatac-seq":
			values[i] = "scATAC-seq"
		case "snatac", "snatac-seq":
			values[i] = "snATAC-seq"
		case "scdna", "scdna-seq":
			values[i] = "scDNA-seq"
		case "scmethyl", "scmethyl-seq":
			values[i] = "scMethyl-seq"
		case "multiome":
			values[i] = "Multiome"
		case "longread", "long-read":
			values[i] = "long-read"
		}
	}
	return values
}

func nullableString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func newRepositoryRecordID() (string, error) {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return "", fmt.Errorf("generate repository record ID: %w", err)
	}
	return "RR-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(suffix[:]), nil
}
