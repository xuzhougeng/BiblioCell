package library

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"bibliocell/internal/proxy"
)

type DatasetDiscovery struct {
	FilesFound   int    `json:"filesFound"`
	FilesAdded   int    `json:"filesAdded"`
	SamplesFound int    `json:"samplesFound"`
	SourceURL    string `json:"sourceUrl"`
}

var geoSeriesID = regexp.MustCompile(`^GSE[0-9]+$`)

func guessDatasetFileKind(name string) string {
	n := strings.ToLower(name)
	switch {
	case strings.Contains(n, "barcode"), strings.Contains(n, "cell_metadata"), strings.Contains(n, "cell_annotation"):
		return "cell_metadata"
	case strings.Contains(n, "features.tsv"), strings.Contains(n, "genes.tsv"):
		return "feature_metadata"
	case strings.Contains(n, ".mtx"), strings.HasSuffix(n, ".h5ad"), strings.Contains(n, "matrix.h5"):
		return "matrix"
	case strings.Contains(n, ".fastq"), strings.Contains(n, ".fq.gz"):
		return "raw_reads"
	default:
		return "other"
	}
}

// parseGEOSOFT keeps series archives and sample files. An archive is not assumed
// to contain a count matrix, and repeated URLs retain every sample association.
func parseGEOSOFT(r io.Reader, datasetID, accession string) ([]DatasetSample, []DatasetAsset, error) {
	samples := []DatasetSample{}
	assets := []DatasetAsset{}
	byURL := map[string]int{}
	sampleIndex := -1
	seriesFound := false
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 2<<20)
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, " = ")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if key == "^SERIES" {
			seriesFound = value == accession
			sampleIndex = -1
			continue
		}
		if key == "^SAMPLE" {
			samples = append(samples, DatasetSample{ID: value})
			sampleIndex = len(samples) - 1
			continue
		}
		if strings.HasPrefix(key, "^") {
			sampleIndex = -1
		}
		if sampleIndex >= 0 {
			s := &samples[sampleIndex]
			switch key {
			case "!Sample_title":
				s.Title = value
			case "!Sample_organism_ch1":
				s.Species = value
			case "!Sample_source_name_ch1", "!Sample_data_processing":
				if s.Notes != "" {
					s.Notes += "\n"
				}
				s.Notes += strings.TrimPrefix(key, "!Sample_") + ": " + value
			case "!Sample_characteristics_ch1":
				if s.Notes != "" {
					s.Notes += "\n"
				}
				s.Notes += value
				k, v, ok := strings.Cut(value, ":")
				if ok {
					switch strings.ToLower(strings.TrimSpace(k)) {
					case "tissue", "organ":
						s.Tissue = strings.TrimSpace(v)
					case "treatment", "condition":
						s.Condition = strings.TrimSpace(v)
					case "replicate", "biological replicate":
						s.Replicate = strings.TrimSpace(v)
					}
				}
			}
		}
		if !strings.HasPrefix(key, "!Series_supplementary_file") && !strings.HasPrefix(key, "!Sample_supplementary_file") {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" || (u.Scheme != "ftp" && u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		// NCBI FTP paths have equivalent HTTPS endpoints, usable from a browser.
		if u.Scheme == "ftp" && u.Hostname() == "ftp.ncbi.nlm.nih.gov" {
			u.Scheme = "https"
		}
		value = u.String()
		i, exists := byURL[value]
		if !exists {
			name, err := url.PathUnescape(path.Base(u.Path))
			if err != nil {
				name = path.Base(u.Path)
			}
			i = len(assets)
			byURL[value] = i
			assets = append(assets, DatasetAsset{DatasetID: datasetID, Title: name, URL: value, Kind: guessDatasetFileKind(name), MatrixKind: "unknown", LocalState: "none", SampleIDs: []string{}})
		}
		if sampleIndex >= 0 {
			sid := samples[sampleIndex].ID
			found := false
			for _, v := range assets[i].SampleIDs {
				if sid == v {
					found = true
				}
			}
			if !found {
				assets[i].SampleIDs = append(assets[i].SampleIDs, sid)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	if !seriesFound {
		return nil, nil, errors.New("GEO response did not contain the requested series")
	}
	return samples, assets, nil
}

func (s *Store) DiscoverGEOFiles(ctx context.Context, id, accession string) (DatasetDiscovery, error) {
	return s.discoverGEOFiles(ctx, id, accession, proxy.Client(90*time.Second, proxy.Inherit), "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi")
}

func (s *Store) discoverGEOFiles(ctx context.Context, id, accession string, client *http.Client, endpoint string) (DatasetDiscovery, error) {
	result := DatasetDiscovery{}
	accession = strings.ToUpper(strings.TrimSpace(accession))
	if !geoSeriesID.MatchString(accession) {
		return result, errors.New("choose a GEO GSE accession")
	}
	d, err := s.GetDataset(ctx, id)
	if err != nil {
		return result, err
	}
	linked := false
	for _, r := range d.Repositories {
		if strings.EqualFold(r.Repository, "geo") && strings.EqualFold(r.Accession, accession) {
			linked = true
		}
	}
	if !linked {
		return result, errors.New("GSE accession must be linked to this dataset first")
	}
	m, err := s.DatasetManifest(ctx, id)
	if err != nil {
		return result, err
	}
	result.SourceURL = endpoint + "?" + url.Values{"acc": {accession}, "targ": {"all"}, "form": {"text"}, "view": {"full"}}.Encode()
	req, err := http.NewRequestWithContext(ctx, "GET", result.SourceURL, nil)
	if err != nil {
		return result, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("GEO returned HTTP %d", resp.StatusCode)
	}
	const maxSOFT = 64 << 20
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSOFT+1))
	if err != nil {
		return result, err
	}
	if len(data) > maxSOFT {
		return result, errors.New("GEO metadata exceeds 64 MiB; inspect the repository manually")
	}
	samples, assets, err := parseGEOSOFT(strings.NewReader(string(data)), id, accession)
	if err != nil {
		return result, err
	}
	s.datasetWorkspaceMu.Lock()
	defer s.datasetWorkspaceMu.Unlock()
	result.FilesFound = len(assets)
	result.SamplesFound = len(samples)
	known := map[string]bool{}
	for _, v := range m.Plan.Samples {
		known[v.ID] = true
	}
	for _, v := range samples {
		if !known[v.ID] {
			m.Plan.Samples = append(m.Plan.Samples, v)
			known[v.ID] = true
		}
	}
	if err = validateDatasetPlan(m.Plan); err != nil {
		return result, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `INSERT INTO dataset_workspaces(dataset_id) VALUES (?) ON CONFLICT DO NOTHING`, id); err != nil {
		return result, err
	}
	raw, _ := json.Marshal(m.Plan)
	res, err := tx.ExecContext(ctx, `UPDATE dataset_workspaces SET plan_json=?,revision=revision+1 WHERE dataset_id=? AND revision=?`, string(raw), id, m.Revision)
	if err != nil {
		return result, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return result, errors.New("dataset plan changed during discovery; retry")
	}
	for _, a := range assets {
		if err = validateAsset(a); err != nil {
			return result, err
		}
		a.ID, err = newLocalID("DA")
		if err != nil {
			return result, err
		}
		raw, _ = json.Marshal(a)
		res, err = tx.ExecContext(ctx, `INSERT INTO dataset_assets(id,dataset_id,source_url,payload) VALUES (?,?,?,?) ON CONFLICT DO NOTHING`, a.ID, id, a.URL, string(raw))
		if err != nil {
			return result, err
		}
		n, _ = res.RowsAffected()
		result.FilesAdded += int(n)
	}
	return result, tx.Commit()
}
