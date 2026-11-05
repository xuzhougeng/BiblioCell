package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const datasetAssetsDDL = `CREATE TABLE IF NOT EXISTS dataset_workspaces (
 dataset_id TEXT PRIMARY KEY NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
 allocated INTEGER NOT NULL DEFAULT 0 CHECK (allocated IN (0,1)),
 revision INTEGER NOT NULL DEFAULT 0,
 plan_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(plan_json))
);
CREATE TABLE IF NOT EXISTS dataset_assets (
 id TEXT PRIMARY KEY NOT NULL,
 dataset_id TEXT NOT NULL REFERENCES datasets(id) ON DELETE CASCADE,
 source_url TEXT NOT NULL DEFAULT '',
 payload TEXT NOT NULL CHECK (json_valid(payload))
);
CREATE INDEX IF NOT EXISTS idx_dataset_assets_dataset ON dataset_assets(dataset_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_dataset_assets_url ON dataset_assets(dataset_id, source_url) WHERE source_url <> '';`

// DatasetAsset describes one published file, not a claim that its contents are analysis-ready.
type DatasetAsset struct {
	ID           string   `json:"id"`
	DatasetID    string   `json:"datasetId"`
	Title        string   `json:"title"`
	Kind         string   `json:"kind"`
	URL          string   `json:"url"`
	SampleIDs    []string `json:"sampleIds"`
	MatrixKind   string   `json:"matrixKind"`
	ReferenceKey string   `json:"referenceKey"`
	Notes        string   `json:"notes"`
	LocalPath    string   `json:"localPath"`
	SizeBytes    int64    `json:"sizeBytes"`
	SHA256       string   `json:"sha256"`
	CheckedAt    string   `json:"checkedAt"`
	LocalState   string   `json:"localState"`
}

type DatasetSample struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Species   string `json:"species"`
	Tissue    string `json:"tissue"`
	Condition string `json:"condition"`
	Replicate string `json:"replicate"`
	Notes     string `json:"notes"`
}

// Matching belongs to this dataset's relationship with a versioned reference.
type DatasetReference struct {
	Key        string `json:"key"`
	Species    string `json:"species"`
	Assembly   string `json:"assembly"`
	Annotation string `json:"annotation"`
	Status     string `json:"status"`
	Evidence   string `json:"evidence"`
}

type DatasetPlan struct {
	Samples    []DatasetSample    `json:"samples"`
	References []DatasetReference `json:"references"`
	Notes      string             `json:"notes"`
}

type DatasetManifest struct {
	DatasetID string         `json:"datasetId"`
	Directory string         `json:"directory"`
	Allocated bool           `json:"allocated"`
	Revision  int            `json:"revision"`
	Plan      DatasetPlan    `json:"plan"`
	Assets    []DatasetAsset `json:"assets"`
}

var datasetFolderID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,160}$`)

func datasetDirectory(id string) string {
	if !datasetFolderID.MatchString(id) {
		sum := sha256.Sum256([]byte(id))
		id = hex.EncodeToString(sum[:])
	}
	return "datasets/" + id
}

func emptyManifest(id string) DatasetManifest {
	return DatasetManifest{DatasetID: id, Directory: datasetDirectory(id), Plan: DatasetPlan{Samples: []DatasetSample{}, References: []DatasetReference{}}, Assets: []DatasetAsset{}}
}

// ListDatasetManifests uses two database scans, with no network or disk hashing.
func (s *Store) ListDatasetManifests(ctx context.Context) ([]DatasetManifest, error) {
	return s.listDatasetManifests(ctx, "")
}

func (s *Store) listDatasetManifests(ctx context.Context, datasetID string) ([]DatasetManifest, error) {
	result := []DatasetManifest{}
	index := map[string]int{}
	rows, err := s.db.QueryContext(ctx, `SELECT d.id, coalesce(w.allocated,0), coalesce(w.revision,0), coalesce(w.plan_json,'{}') FROM datasets d LEFT JOIN dataset_workspaces w ON w.dataset_id=d.id WHERE d.id NOT IN (SELECT dataset_id FROM trashed_datasets) AND (?='' OR d.id=?) ORDER BY d.id`, datasetID, datasetID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, raw string
		var allocated bool
		var revision int
		if err = rows.Scan(&id, &allocated, &revision, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		m := emptyManifest(id)
		m.Allocated = allocated
		m.Revision = revision
		if err = json.Unmarshal([]byte(raw), &m.Plan); err != nil {
			rows.Close()
			return nil, err
		}
		if m.Plan.Samples == nil {
			m.Plan.Samples = []DatasetSample{}
		}
		if m.Plan.References == nil {
			m.Plan.References = []DatasetReference{}
		}
		index[id] = len(result)
		result = append(result, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT dataset_id,payload FROM dataset_assets WHERE (?='' OR dataset_id=?) ORDER BY id`, datasetID, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		var a DatasetAsset
		if err = rows.Scan(&id, &raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &a); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			result[i].Assets = append(result[i].Assets, a)
		}
	}
	return result, rows.Err()
}

func (s *Store) DatasetManifest(ctx context.Context, id string) (DatasetManifest, error) {
	// Fetching the manifest also verifies the dataset still exists.
	all, err := s.listDatasetManifests(ctx, id)
	if err != nil {
		return DatasetManifest{}, err
	}
	for _, m := range all {
		if m.DatasetID == id {
			return m, nil
		}
	}
	return DatasetManifest{}, ErrDatasetNotFound
}

func validateDatasetPlan(p DatasetPlan) error {
	samples := map[string]bool{}
	refs := map[string]bool{}
	for _, v := range p.Samples {
		if strings.TrimSpace(v.ID) == "" || samples[v.ID] {
			return errors.New("sample IDs must be nonempty and unique")
		}
		samples[v.ID] = true
	}
	for _, v := range p.References {
		if strings.TrimSpace(v.Key) == "" || refs[v.Key] {
			return errors.New("reference keys must be nonempty and unique")
		}
		refs[v.Key] = true
		if v.Status != "candidate" && v.Status != "matched" && v.Status != "unresolved" {
			return errors.New("invalid reference matching status")
		}
		if v.Status == "matched" && (strings.TrimSpace(v.Assembly) == "" || strings.TrimSpace(v.Annotation) == "" || strings.TrimSpace(v.Evidence) == "") {
			return errors.New("matched references require assembly, annotation and matching evidence")
		}
	}
	raw, _ := json.Marshal(p)
	if len(raw) > 4<<20 {
		return errors.New("dataset plan is too large")
	}
	return nil
}

func (s *Store) SaveDatasetPlan(ctx context.Context, id string, revision int, p DatasetPlan) error {
	s.datasetWorkspaceMu.Lock()
	defer s.datasetWorkspaceMu.Unlock()
	if p.Samples == nil {
		p.Samples = []DatasetSample{}
	}
	if p.References == nil {
		p.References = []DatasetReference{}
	}
	if err := validateDatasetPlan(p); err != nil {
		return err
	}
	m, err := s.DatasetManifest(ctx, id)
	if err != nil {
		return err
	}
	samples := map[string]bool{}
	refs := map[string]bool{}
	for _, v := range p.Samples {
		samples[v.ID] = true
	}
	for _, v := range p.References {
		refs[v.Key] = true
	}
	for _, a := range m.Assets {
		for _, sid := range a.SampleIDs {
			if !samples[sid] {
				return fmt.Errorf("sample %s is still used by a file", sid)
			}
		}
		if a.ReferenceKey != "" && !refs[a.ReferenceKey] {
			return fmt.Errorf("reference %s is still used by a file", a.ReferenceKey)
		}
	}
	raw, _ := json.Marshal(p)
	// Allocation may have created a revision-zero workspace already.
	_, err = s.db.ExecContext(ctx, `INSERT INTO dataset_workspaces(dataset_id) VALUES (?) ON CONFLICT DO NOTHING`, id)
	if err != nil {
		return err
	}
	r, err := s.db.ExecContext(ctx, `UPDATE dataset_workspaces SET plan_json=?,revision=revision+1 WHERE dataset_id=? AND revision=?`, string(raw), id, revision)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return errors.New("dataset plan changed; reload before saving")
	}
	return nil
}

func (s *Store) PrepareDatasetDirectory(ctx context.Context, id string) (string, error) {
	if _, err := s.DatasetManifest(ctx, id); err != nil {
		return "", err
	}
	root, err := os.OpenRoot(filepath.Dir(s.path))
	if err != nil {
		return "", err
	}
	defer root.Close()
	dir := datasetDirectory(id)
	for _, sub := range []string{"source", "original", "standardized", "metadata"} {
		if err = root.MkdirAll(dir+"/"+sub, 0755); err != nil {
			return "", err
		}
	}
	if err = root.MkdirAll("references", 0755); err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO dataset_workspaces(dataset_id,allocated) VALUES (?,1) ON CONFLICT(dataset_id) DO UPDATE SET allocated=1`, id)
	return dir, err
}

func validateAsset(a DatasetAsset) error {
	if strings.TrimSpace(a.Title) == "" || len(a.Title) > 2000 {
		return errors.New("file title is required (maximum 2000 characters)")
	}
	switch a.Kind {
	case "matrix", "cell_metadata", "feature_metadata", "pep", "cds", "gene_map", "raw_reads", "other":
	default:
		return errors.New("invalid dataset file kind")
	}
	switch a.MatrixKind {
	case "unknown", "counts", "normalized":
	default:
		return errors.New("invalid matrix value type")
	}
	if a.URL != "" {
		u, err := url.Parse(a.URL)
		if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http" && u.Scheme != "ftp") || u.User != nil {
			return errors.New("file URL must be a public HTTP(S) or FTP URL without credentials")
		}
	}
	if a.URL == "" && a.LocalPath == "" {
		return errors.New("provide a file URL or a path relative to the storage folder")
	}
	if a.LocalPath != "" && (!filepath.IsLocal(filepath.FromSlash(a.LocalPath)) || strings.Contains(a.LocalPath, ":")) {
		return errors.New("local path must stay inside the storage folder")
	}
	if len(a.Notes) > 50000 || len(a.URL) > 8000 {
		return errors.New("file metadata is too large")
	}
	return nil
}

func (s *Store) SaveDatasetAsset(ctx context.Context, a DatasetAsset) (DatasetAsset, error) {
	s.datasetWorkspaceMu.Lock()
	defer s.datasetWorkspaceMu.Unlock()
	a.Title = strings.TrimSpace(a.Title)
	a.URL = strings.TrimSpace(a.URL)
	a.LocalPath = strings.ReplaceAll(strings.TrimSpace(a.LocalPath), `\`, "/")
	if a.MatrixKind == "" {
		a.MatrixKind = "unknown"
	}
	if a.SampleIDs == nil {
		a.SampleIDs = []string{}
	}
	if err := validateAsset(a); err != nil {
		return a, err
	}
	m, err := s.DatasetManifest(ctx, a.DatasetID)
	if err != nil {
		return a, err
	}
	for _, sid := range a.SampleIDs {
		found := false
		for _, v := range m.Plan.Samples {
			if v.ID == sid {
				found = true
			}
		}
		if !found {
			return a, fmt.Errorf("unknown sample %s", sid)
		}
	}
	if a.ReferenceKey != "" {
		found := false
		for _, v := range m.Plan.References {
			if v.Key == a.ReferenceKey {
				found = true
			}
		}
		if !found {
			return a, errors.New("unknown reference key")
		}
	}
	// A client cannot set a successful verification result. Preserve it only for an unchanged path.
	a.LocalState = "none"
	a.SizeBytes = 0
	a.SHA256 = ""
	a.CheckedAt = ""
	if a.LocalPath != "" {
		a.LocalState = "registered"
	}
	var original []byte
	if a.ID != "" {
		found := false
		for _, old := range m.Assets {
			if old.ID == a.ID {
				found = true
				original, _ = json.Marshal(old)
				if old.LocalPath == a.LocalPath {
					a.LocalState = old.LocalState
					a.SizeBytes = old.SizeBytes
					a.SHA256 = old.SHA256
					a.CheckedAt = old.CheckedAt
				}
			}
		}
		if !found {
			return a, errors.New("dataset file not found")
		}
	} else {
		a.ID, err = newLocalID("DA")
		if err != nil {
			return a, err
		}
	}
	raw, _ := json.Marshal(a)
	if original != nil {
		result, updateErr := s.db.ExecContext(ctx, `UPDATE dataset_assets SET source_url=?,payload=? WHERE id=? AND dataset_id=? AND payload=?`, a.URL, string(raw), a.ID, a.DatasetID, string(original))
		if updateErr != nil {
			return a, updateErr
		}
		n, _ := result.RowsAffected()
		if n == 0 {
			return a, errors.New("file record changed while saving; reload")
		}
		return a, nil
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO dataset_assets(id,dataset_id,source_url,payload) VALUES (?,?,?,?)`, a.ID, a.DatasetID, a.URL, string(raw))
	return a, err
}

func (s *Store) VerifyDatasetAsset(ctx context.Context, datasetID, id string) (DatasetAsset, error) {
	m, err := s.DatasetManifest(ctx, datasetID)
	if err != nil {
		return DatasetAsset{}, err
	}
	var a DatasetAsset
	for _, v := range m.Assets {
		if v.ID == id {
			a = v
		}
	}
	if a.ID == "" || a.LocalPath == "" {
		return a, errors.New("register a local path first")
	}
	root, err := os.OpenRoot(filepath.Dir(s.path))
	if err != nil {
		return a, err
	}
	defer root.Close()
	f, checkErr := root.Open(a.LocalPath)
	var size int64
	var digest string
	if checkErr == nil {
		st, e := f.Stat()
		checkErr = e
		if e == nil && !st.Mode().IsRegular() {
			checkErr = errors.New("path is not a regular file")
		}
		if checkErr == nil {
			h := sha256.New()
			size, checkErr = io.Copy(h, &contextReader{ctx: ctx, r: f})
			digest = hex.EncodeToString(h.Sum(nil))
			after, e := f.Stat()
			if checkErr == nil && (e != nil || after.Size() != st.Size() || !after.ModTime().Equal(st.ModTime())) {
				checkErr = errors.New("file changed during verification")
			}
		}
		f.Close()
	}
	previous := a.SHA256
	original, _ := json.Marshal(a)
	a.LocalState = "verified"
	a.SizeBytes = size
	a.SHA256 = digest
	a.CheckedAt = time.Now().UTC().Format(time.RFC3339)
	if checkErr != nil {
		a.LocalState = "missing"
		a.SHA256 = previous
		a.SizeBytes = 0
	} else if previous != "" && previous != digest {
		a.LocalState = "changed"
		a.SHA256 = previous
		checkErr = errors.New("file checksum changed; re-register the file to accept a replacement")
	}
	raw, _ := json.Marshal(a)
	r, err := s.db.ExecContext(ctx, `UPDATE dataset_assets SET payload=? WHERE id=? AND dataset_id=? AND payload=?`, string(raw), id, datasetID, string(original))
	if err != nil {
		return a, err
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return a, errors.New("file record changed during verification; reload")
	}
	return a, checkErr
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r *contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func (s *Store) RemoveDatasetAsset(ctx context.Context, datasetID, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM dataset_assets WHERE id=? AND dataset_id=?`, id, datasetID)
	return err
}

// Export writes a reviewable snapshot. It never infers that an allocated directory contains data.
func (s *Store) ExportDatasetManifest(ctx context.Context, id string) (string, error) {
	dir, err := s.PrepareDatasetDirectory(ctx, id)
	if err != nil {
		return "", err
	}
	m, err := s.DatasetManifest(ctx, id)
	if err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	root, err := os.OpenRoot(filepath.Dir(s.path))
	if err != nil {
		return "", err
	}
	defer root.Close()
	token, err := newLocalID("manifest")
	if err != nil {
		return "", err
	}
	tmp := path.Join(dir, token+".tmp")
	defer root.Remove(tmp)
	f, err := root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return "", err
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	dest := path.Join(dir, "manifest.json")
	return dest, root.Rename(tmp, dest)
}

func (s *Store) DatasetDirectoryPath(ctx context.Context, id string) (string, error) {
	if _, err := s.PrepareDatasetDirectory(ctx, id); err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(s.path), filepath.FromSlash(datasetDirectory(id))), nil
}
