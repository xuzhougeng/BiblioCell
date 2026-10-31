package library

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func TestDatasetRoundTripAndAssociationPreservation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, id := range []string{"P-one", "P-two"} {
		if _, err := store.AddPaper(ctx, Paper{ID: id, Title: id}); err != nil {
			t.Fatal(err)
		}
	}

	datasets := []Dataset{
		{
			ID: "DS-one", Title: "First dataset", Notes: "personal note",
			Species: []SpeciesEntry{{Name: "Arabidopsis thaliana", Label: "拟南芥"}, {Name: "Oryza sativa", Label: "水稻"}},
			Organs:  []string{"root", "leaf"}, Modalities: []string{"scRNA", "scATAC-seq"},
			Platforms: []string{"10x Genomics Chromium", "BD Rhapsody"},
			Status:    "verified", Availability: "public", MatrixAvailability: "restricted",
			AnnotationAvailability: "unknown",
			Repositories: []DatasetRepository{{
				Repository: "GEO", Accession: "GSE100", MappingScope: "exact",
				EvidenceText: "keep repository evidence", Verified: true,
			}},
			PaperLinks: []DatasetPaperLink{
				{PaperID: "P-one", Role: "generated", EvidenceText: "keep link evidence", Verified: true},
				{PaperID: "P-two", Role: "reused"},
			},
		},
		{
			ID: "DS-two", Title: "Second dataset", Status: "identified",
			Repositories: []DatasetRepository{{Repository: "geo", Accession: "GSE100"}},
		},
	}
	if err := store.SaveDatasets(ctx, datasets); err != nil {
		t.Fatalf("SaveDatasets() error = %v", err)
	}

	saved, err := store.ListDatasets(ctx)
	if err != nil {
		t.Fatalf("ListDatasets() error = %v", err)
	}
	if len(saved) != 2 {
		t.Fatalf("dataset count = %d, want 2", len(saved))
	}
	byID := map[string]Dataset{}
	for _, dataset := range saved {
		byID[dataset.ID] = dataset
	}
	first := byID["DS-one"]
	if len(first.Species) != 2 || len(first.Organs) != 2 || len(first.Modalities) != 2 || len(first.Platforms) != 2 {
		t.Errorf("multi-value fields did not round trip: %+v", first)
	}
	if first.Modalities[0] != "scRNA-seq" || first.Availability != "public" || first.CurationLevel != "files_verified" {
		t.Errorf("canonical mappings = %+v", first)
	}
	if first.Notes != "personal note" || len(first.PaperLinks) != 2 || !first.PaperLinks[0].Verified {
		t.Errorf("notes or paper links not preserved: %+v", first)
	}
	if len(first.Repositories) != 1 || first.Repository != "geo" || first.Accession != "GSE100" {
		t.Errorf("repository mapping = %+v", first.Repositories)
	}
	// The deprecated single-value columns are no longer written.
	var tissue, technology sql.NullString
	var modality string
	if err := store.db.QueryRow("SELECT tissue_text, modality, technology FROM datasets WHERE id = 'DS-one'").
		Scan(&tissue, &modality, &technology); err != nil {
		t.Fatal(err)
	}
	if tissue.Valid || modality != "unknown" || technology.Valid {
		t.Errorf("deprecated columns were written: %v / %q / %v", tissue, modality, technology)
	}

	// Changing a shared accession must not mutate the other dataset's record.
	first.Repositories[0].Accession = "GSE200"
	first.CurationLevel = "reprocessed"
	if err := store.SaveDatasets(ctx, []Dataset{first}); err != nil {
		t.Fatalf("SaveDatasets() shared edit error = %v", err)
	}
	saved, err = store.ListDatasets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byID = map[string]Dataset{}
	for _, dataset := range saved {
		byID[dataset.ID] = dataset
	}
	if got := byID["DS-two"].Accession; got != "GSE100" {
		t.Errorf("shared repository was mutated: %q", got)
	}
	first = byID["DS-one"]
	if first.Accession != "GSE200" || first.CurationLevel != "reprocessed" {
		t.Errorf("edited dataset = %+v", first)
	}
	if !first.Repositories[0].Verified || first.Repositories[0].EvidenceText != "keep repository evidence" ||
		!first.PaperLinks[0].Verified || first.PaperLinks[0].EvidenceText != "keep link evidence" {
		t.Errorf("association metadata was reset: %+v", first)
	}

	// Associations inserted outside the frontend survive a stale full-state save.
	if _, err := store.db.Exec(`
		INSERT INTO repository_records(id, repository, accession) VALUES('RR-external', 'sra', 'SRR100');
		INSERT INTO dataset_repositories(dataset_id, repository_record_id, verified, evidence_text)
		VALUES('DS-one', 'RR-external', 1, 'external repository');
		INSERT INTO paper_datasets(paper_id, dataset_id, role, verified, evidence_text)
		VALUES('P-two', 'DS-one', 'benchmarked', 1, 'external link')`); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveDatasets(ctx, []Dataset{first}); err != nil {
		t.Fatalf("SaveDatasets() stale save error = %v", err)
	}
	var externalRepository, externalLink int
	if err := store.db.QueryRow("SELECT count(*) FROM dataset_repositories WHERE dataset_id = 'DS-one' AND repository_record_id = 'RR-external' AND verified = 1").Scan(&externalRepository); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT count(*) FROM paper_datasets WHERE dataset_id = 'DS-one' AND role = 'benchmarked' AND verified = 1").Scan(&externalLink); err != nil {
		t.Fatal(err)
	}
	if externalRepository != 1 || externalLink != 1 {
		t.Errorf("external associations were deleted: repository=%d link=%d", externalRepository, externalLink)
	}
	if got := byID["DS-two"].Title; got != "Second dataset" {
		t.Errorf("omitting a dataset deleted or changed it: %q", got)
	}
}

func TestDatasetCellCountAndVerifiedValidation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	cells := 1200
	if err := store.SaveDatasets(context.Background(), []Dataset{{
		ID: "DS-invalid", Title: "Invalid", CellsReported: &cells,
	}}); err == nil {
		t.Fatal("cell count without unit and scope succeeded")
	}
	if err := store.SaveDatasets(context.Background(), []Dataset{{
		ID: "DS-verified", Title: "Verified without accession", Status: "verified",
	}}); err == nil {
		t.Fatal("verified dataset without accession succeeded")
	}
}

const legacyDatasetsDDL = `
CREATE TABLE datasets (
	id TEXT PRIMARY KEY NOT NULL,
	title TEXT NOT NULL,
	description TEXT,
	dataset_kind TEXT NOT NULL DEFAULT 'original',
	modality TEXT NOT NULL DEFAULT 'unknown',
	tissue_text TEXT,
	material_text TEXT,
	developmental_stage TEXT,
	conditions_text TEXT,
	technology TEXT,
	chemistry_version TEXT,
	n_cells_reported INTEGER,
	cell_count_unit TEXT,
	cell_count_scope TEXT,
	biological_replicates INTEGER,
	reference_assembly TEXT,
	reference_annotation TEXT,
	raw_availability TEXT NOT NULL DEFAULT 'unknown',
	matrix_availability TEXT NOT NULL DEFAULT 'unknown',
	annotation_availability TEXT NOT NULL DEFAULT 'unknown',
	availability_checked_at TEXT,
	curation_level TEXT NOT NULL DEFAULT 'indexed',
	first_seen_at TEXT NOT NULL DEFAULT '2026-01-01T00:00:00Z',
	updated_at TEXT NOT NULL DEFAULT '2026-01-01T00:00:00Z'
);
CREATE TABLE taxa (
	taxid INTEGER PRIMARY KEY,
	scientific_name TEXT NOT NULL,
	common_name TEXT,
	lineage_json TEXT NOT NULL DEFAULT '[]',
	updated_at TEXT NOT NULL DEFAULT '2026-01-01T00:00:00Z'
);
CREATE TABLE dataset_taxa (
	dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
	taxid INTEGER NOT NULL REFERENCES taxa(taxid),
	name_as_reported TEXT,
	PRIMARY KEY(dataset_id, taxid)
);`

func TestOpenMigratesV3Datasets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bibliocell.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(legacyDatasetsDDL + `
		INSERT INTO datasets(id, title, tissue_text, modality, technology, chemistry_version)
		VALUES('DS-legacy', 'Legacy dataset', 'root', 'scRNA', '10x Genomics Chromium', 'v3');
		INSERT INTO taxa(taxid, scientific_name, common_name) VALUES(3702, 'Arabidopsis thaliana', '拟南芥');
		INSERT INTO dataset_taxa(dataset_id, taxid) VALUES('DS-legacy', 3702);
		PRAGMA user_version = 3;`); err != nil {
		db.Close()
		t.Fatalf("create v3 database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open() v3 database error = %v", err)
	}
	defer store.Close()
	var species, organs, modalities, platforms, chemistry, notes string
	if err := store.db.QueryRow(`SELECT species_json, organs_json, modalities_json, platforms_json,
		chemistry_version, notes FROM datasets WHERE id = 'DS-legacy'`).
		Scan(&species, &organs, &modalities, &platforms, &chemistry, &notes); err != nil {
		t.Fatal(err)
	}
	if species != `[{"name":"Arabidopsis thaliana","label":"拟南芥"}]` || organs != `["root"]` ||
		modalities != `["scRNA-seq"]` || platforms != `["10x Genomics Chromium"]` {
		t.Errorf("backfill = %s / %s / %s / %s", species, organs, modalities, platforms)
	}
	if chemistry != "v3" || notes != "" {
		t.Errorf("chemistry or notes changed: %q / %q", chemistry, notes)
	}
	if SchemaVersion != 5 {
		t.Errorf("SchemaVersion = %d, want 5", SchemaVersion)
	}
}
