package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func assetTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	d, err := s.AddDataset(context.Background(), DatasetInput{Title: "Resource test", Repositories: []RepositoryInput{{Repository: "geo", Accession: "GSE123"}}})
	if err != nil {
		t.Fatal(err)
	}
	return s, d.Dataset.ID
}

func TestDatasetManifestLifecycle(t *testing.T) {
	s, id := assetTestStore(t)
	ctx := context.Background()
	m, err := s.DatasetManifest(ctx, id)
	if err != nil || m.Allocated || len(m.Assets) != 0 {
		t.Fatalf("initial: %+v %v", m, err)
	}
	dir, err := s.PrepareDatasetDirectory(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, sub := range []string{"source", "original", "metadata", "standardized"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(s.path), dir, sub)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = s.PrepareDatasetDirectory(ctx, id); err != nil {
		t.Fatal(err)
	}
	a, err := s.SaveDatasetAsset(ctx, DatasetAsset{DatasetID: id, Title: "counts.mtx", Kind: "matrix", URL: "https://example.org/counts.mtx", LocalState: "verified", SHA256: strings.Repeat("f", 64)})
	if err != nil || a.LocalState != "none" || a.SHA256 != "" {
		t.Fatalf("client forged verification: %+v %v", a, err)
	}
	a.LocalPath = dir + "/original/counts.mtx"
	a, err = s.SaveDatasetAsset(ctx, a)
	if err != nil || a.LocalState != "registered" {
		t.Fatalf("register: %+v %v", a, err)
	}
	if _, err = s.VerifyDatasetAsset(ctx, id, a.ID); err == nil {
		t.Fatal("missing file passed verification")
	}
	file := filepath.Join(filepath.Dir(s.path), filepath.FromSlash(a.LocalPath))
	if err = os.WriteFile(file, []byte("matrix content"), 0644); err != nil {
		t.Fatal(err)
	}
	a, err = s.VerifyDatasetAsset(ctx, id, a.ID)
	if err != nil || a.LocalState != "verified" || len(a.SHA256) != 64 {
		t.Fatalf("verify: %+v %v", a, err)
	}
	if err = os.WriteFile(file, []byte("replacement"), 0644); err != nil {
		t.Fatal(err)
	}
	a, err = s.VerifyDatasetAsset(ctx, id, a.ID)
	if err == nil || a.LocalState != "changed" {
		t.Fatalf("replacement accepted: %+v %v", a, err)
	}
	output, err := s.ExportDatasetManifest(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(s.path), output))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(raw, &m); err != nil || !m.Allocated || len(m.Assets) != 1 || m.Assets[0].LocalState != "changed" {
		t.Fatalf("snapshot: %+v %v", m, err)
	}
	// Re-export replaces the previous snapshot on Windows too.
	if _, err = s.ExportDatasetManifest(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err = s.RemoveDatasetAsset(ctx, id, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(file); err != nil {
		t.Fatal("removing a record removed the original file", err)
	}
}

func TestDatasetAssetValidationAndPlanConflicts(t *testing.T) {
	s, id := assetTestStore(t)
	ctx := context.Background()
	p := DatasetPlan{Samples: []DatasetSample{{ID: "GSM1"}}, References: []DatasetReference{{Key: "ath", Status: "matched", Assembly: "TAIR10", Annotation: "Araport11", Evidence: "Author reference and feature IDs checked"}}}
	if err := s.SaveDatasetPlan(ctx, id, 0, p); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveDatasetPlan(ctx, id, 0, p); err == nil {
		t.Fatal("stale plan overwrote latest")
	}
	p.References[0].Evidence = ""
	if err := s.SaveDatasetPlan(ctx, id, 1, p); err == nil {
		t.Fatal("matched reference has no evidence")
	}
	for _, local := range []string{"../escape", "C:/Windows/test", "/absolute"} {
		if _, err := s.SaveDatasetAsset(ctx, DatasetAsset{DatasetID: id, Title: "bad", Kind: "matrix", LocalPath: local}); err == nil {
			t.Fatalf("accepted %s", local)
		}
	}
	for _, u := range []string{"javascript:alert(1)", "file:///etc/passwd", "https://user:pass@example.org/file"} {
		if _, err := s.SaveDatasetAsset(ctx, DatasetAsset{DatasetID: id, Title: "bad", Kind: "matrix", URL: u}); err == nil {
			t.Fatalf("accepted %s", u)
		}
	}
	a := DatasetAsset{DatasetID: id, Title: "pep.fa", Kind: "pep", URL: "https://example.org/pep.fa", ReferenceKey: "missing"}
	if _, err := s.SaveDatasetAsset(ctx, a); err == nil {
		t.Fatal("unknown reference accepted")
	}
	a.ReferenceKey = "ath"
	a.SampleIDs = []string{"GSM1"}
	saved, err := s.SaveDatasetAsset(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveDatasetAsset(ctx, a); err == nil {
		t.Fatal("duplicate URL accepted")
	}
	p.References = nil
	if err = s.SaveDatasetPlan(ctx, id, 1, p); err == nil {
		t.Fatal("removed referenced reference key")
	}
	if err = s.DeleteDataset(ctx, id); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.db.QueryRow(`SELECT count(*) FROM dataset_assets WHERE id=?`, saved.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("orphaned asset: %d %v", count, err)
	}
}

const geoFixture = `^SERIES = GSE123
!Series_supplementary_file = ftp://ftp.ncbi.nlm.nih.gov/geo/series/GSEnnn/GSE123/suppl/GSE123_RAW.tar
^SAMPLE = GSM1
!Sample_title = root control
!Sample_organism_ch1 = Arabidopsis thaliana
!Sample_characteristics_ch1 = tissue: root
!Sample_characteristics_ch1 = treatment: control
!Sample_data_processing = Reads aligned against TAIR10 with Araport11 annotations
!Sample_supplementary_file_1 = ftp://ftp.ncbi.nlm.nih.gov/shared/matrix.mtx.gz
!Sample_supplementary_file_2 = NONE
^SAMPLE = GSM2
!Sample_title = root treated
!Sample_supplementary_file_1 = ftp://ftp.ncbi.nlm.nih.gov/shared/matrix.mtx.gz
!Sample_supplementary_file_2 = https://example.org/features.tsv.gz
`

func TestGEOFileDiscoveryIdempotentAndPreservesEdits(t *testing.T) {
	s, id := assetTestStore(t)
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("acc") != "GSE123" {
			t.Error("accession absent")
		}
		w.Write([]byte(geoFixture))
	}))
	defer server.Close()
	result, err := s.discoverGEOFiles(ctx, id, "GSE123", server.Client(), server.URL)
	if err != nil || result.FilesAdded != 3 || result.SamplesFound != 2 {
		t.Fatalf("discovery: %+v %v", result, err)
	}
	m, _ := s.DatasetManifest(ctx, id)
	if !strings.Contains(m.Plan.Samples[0].Notes, "TAIR10") {
		t.Fatal("reference information in processing notes was lost")
	}
	if m.Allocated {
		t.Fatal("discovery allocated directories")
	}
	m.Plan.Samples[0].Tissue = "curated root"
	if err = s.SaveDatasetPlan(ctx, id, m.Revision, m.Plan); err != nil {
		t.Fatal(err)
	}
	for _, a := range m.Assets {
		if strings.Contains(a.Title, "RAW.tar") && a.Kind != "other" {
			t.Fatal("archive assumed to be a matrix")
		}
		if a.Kind == "matrix" {
			if len(a.SampleIDs) != 2 || a.MatrixKind != "unknown" || a.LocalState != "none" {
				t.Fatalf("matrix: %+v", a)
			}
			a.Kind = "other"
			if _, err = s.SaveDatasetAsset(ctx, a); err != nil {
				t.Fatal(err)
			}
		}
	}
	result, err = s.discoverGEOFiles(ctx, id, "GSE123", server.Client(), server.URL)
	if err != nil || result.FilesAdded != 0 {
		t.Fatalf("duplicate: %+v %v", result, err)
	}
	m, _ = s.DatasetManifest(ctx, id)
	if m.Plan.Samples[0].Tissue != "curated root" || len(m.Assets) != 3 {
		t.Fatalf("overwrote curation: %+v", m)
	}
	for _, a := range m.Assets {
		if a.Kind == "matrix" {
			t.Fatal("discovery overwrote manual classification")
		}
	}
	if _, err = s.discoverGEOFiles(ctx, id, "GSE456", server.Client(), server.URL); err == nil {
		t.Fatal("unlinked accession accepted")
	}
	if _, _, err = parseGEOSOFT(strings.NewReader("<html>rate limited</html>"), id, "GSE123"); err == nil {
		t.Fatal("HTML error mistaken for empty discovery")
	}
}
