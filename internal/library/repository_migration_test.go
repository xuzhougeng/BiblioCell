package library

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func TestSTOmicsRepositoryRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if err := saveDatasets(ctx, store, []Dataset{{ID: "DS-stomics", Title: "Spatial dataset", Repositories: []DatasetRepository{{
		Repository: "stomics", Accession: "ST-test", URL: "https://db.cngb.org/stomics/", Verified: true,
	}}}}); err != nil {
		t.Fatal(err)
	}
	// The agent/MCP input uses the same repository type and still needs review.
	result, err := store.AddDataset(ctx, DatasetInput{Title: "Agent spatial dataset", Repositories: []RepositoryInput{{
		Repository: "Plant Archive", Accession: "ST-agent", URL: "https://example.org/plant-archive/",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Created || len(result.Dataset.Repositories) != 1 || result.Dataset.Repositories[0].Verified {
		t.Fatalf("unexpected agent result: %+v", result)
	}
	if result.Dataset.Repositories[0].Repository != "plant archive" {
		t.Fatalf("custom repository not saved: %+v", result.Dataset.Repositories)
	}
	duplicate, err := store.AddDataset(ctx, DatasetInput{Title: "Same accession", Repositories: []RepositoryInput{{Repository: "PLANT ARCHIVE", Accession: "ST-agent"}}})
	if err != nil || duplicate.Created || duplicate.Dataset.ID != result.Dataset.ID {
		t.Fatalf("custom repository deduplication failed: %+v, %v", duplicate, err)
	}
	datasets, err := store.ListDatasets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range datasets {
		if d.ID == "DS-stomics" && len(d.Repositories) == 1 {
			r := d.Repositories[0]
			if r.Repository != "stomics" || r.Accession != "ST-test" || r.URL != "https://db.cngb.org/stomics/" || !r.Verified {
				t.Fatalf("STOmics record changed: %+v", r)
			}
			return
		}
	}
	t.Fatal("saved STOmics dataset not found")
}

func TestOpenMigratesRepositoryConstraintAndPreservesAssociations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.sqlite")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.ReplaceAll(schemaSQL, "length(trim(repository)) BETWEEN 1 AND 100 AND repository = trim(repository)", "repository IN ('geo','sra','ena','gsa','bioproject','biosample','biostudies','zenodo','figshare','dryad','other')")
	if _, err := db.Exec(legacy); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		PRAGMA user_version = 11;
		DELETE FROM schema_migrations WHERE version >= 12;
		INSERT INTO datasets(id, title) VALUES ('DS-one', 'One'), ('DS-two', 'Two');
		INSERT INTO repository_records(id, repository, accession, record_kind, url, public_status, metadata_json, first_seen_at, last_checked_at)
		VALUES ('RR-shared', 'geo', 'GSE123', 'study', 'https://example.org/data', 'public', '{"keep":true}', '2026-01-01', '2026-02-01');
		INSERT INTO dataset_repositories(dataset_id, repository_record_id, mapping_scope, evidence_url, evidence_text, verified)
		VALUES ('DS-one', 'RR-shared', 'exact', 'https://example.org/paper', 'Evidence one', 1),
		       ('DS-two', 'RR-shared', 'subset', 'https://example.org/paper', 'Evidence two', 0);`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec("INSERT INTO repository_records(id, repository, accession) VALUES ('RR-before', 'stomics', 'ST-test')"); err == nil {
		db.Close()
		t.Fatal("fixture must reject STOmics before migration")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		store, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer store.Close()
			var preserved int
			err := store.db.QueryRow(`SELECT count(*) FROM repository_records r JOIN dataset_repositories dr ON dr.repository_record_id = r.id
				WHERE r.id = 'RR-shared' AND r.repository = 'geo' AND r.accession = 'GSE123' AND r.record_kind = 'study'
				AND r.url = 'https://example.org/data' AND r.public_status = 'public' AND r.metadata_json = '{"keep":true}'
				AND r.first_seen_at = '2026-01-01' AND r.last_checked_at = '2026-02-01' AND dr.evidence_url = 'https://example.org/paper'
				AND ((dr.dataset_id = 'DS-one' AND dr.mapping_scope = 'exact' AND dr.evidence_text = 'Evidence one' AND dr.verified = 1)
				OR (dr.dataset_id = 'DS-two' AND dr.mapping_scope = 'subset' AND dr.evidence_text = 'Evidence two' AND dr.verified = 0))`).Scan(&preserved)
			if err != nil || preserved != 2 {
				t.Fatalf("reopen %d: preserved associations = %d, error = %v", i, preserved, err)
			}
			var enabled, version int
			if err := store.db.QueryRow("PRAGMA foreign_keys").Scan(&enabled); err != nil || enabled != 1 {
				t.Fatalf("foreign keys = %d, error = %v", enabled, err)
			}
			if err := store.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
				t.Fatalf("schema version = %d, error = %v", version, err)
			}
			if i == 0 {
				if err := saveDatasets(context.Background(), store, []Dataset{{ID: "DS-new", Title: "New", Modalities: []string{"custom-seq"}, Platforms: []string{"Custom platform"}, Repositories: []DatasetRepository{{Repository: "custom archive", Accession: "ST-test"}}}}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := store.db.Exec("INSERT INTO repository_records(id, repository, accession) VALUES ('RR-invalid', '', 'X1')"); err == nil {
				t.Fatal("repository constraint was lost")
			}
		}()
	}
}
