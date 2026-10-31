package library

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const enaSearchURL = "https://www.ebi.ac.uk/ena/portal/api/search"

func (h *remoteHarvester) harvestRepository(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	switch config.Provider {
	case "geo":
		return h.harvestGEO(ctx, config)
	case "sra":
		return h.harvestSRA(ctx, config)
	case "ena":
		return h.harvestENA(ctx, config)
	default:
		return nil, fmt.Errorf("unsupported repository provider %q", config.Provider)
	}
}

func (h *remoteHarvester) harvestGEO(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	ids, err := h.searchNCBI(ctx, "gds", config.Query, config.MaxResults)
	if err != nil || len(ids) == 0 {
		return []harvestRecord{}, err
	}
	params := url.Values{"db": {"gds"}, "id": {strings.Join(ids, ",")}, "retmode": {"json"}}
	var response struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := h.getJSON(ctx, h.ncbiURL("esummary.fcgi", params), &response); err != nil {
		return nil, fmt.Errorf("fetch GEO metadata: %w", err)
	}
	records := make([]harvestRecord, 0, len(ids))
	for _, id := range ids {
		raw, ok := response.Result[id]
		if !ok {
			continue
		}
		var item struct {
			Accession  string `json:"accession"`
			Title      string `json:"title"`
			Summary    string `json:"summary"`
			Taxon      string `json:"taxon"`
			EntryType  string `json:"entrytype"`
			GDSType    string `json:"gdstype"`
			BioProject string `json:"bioproject"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decode GEO record %s: %w", id, err)
		}
		if item.Accession == "" {
			continue
		}
		records = append(records, harvestRecord{
			ExternalID: item.Accession, Title: item.Title,
			URL:     "https://www.ncbi.nlm.nih.gov/geo/query/acc.cgi?acc=" + url.QueryEscape(item.Accession),
			Payload: raw,
		})
	}
	return records, nil
}

func (h *remoteHarvester) harvestSRA(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	ids, err := h.searchNCBI(ctx, "sra", config.Query, config.MaxResults)
	if err != nil || len(ids) == 0 {
		return []harvestRecord{}, err
	}
	params := url.Values{"db": {"sra"}, "id": {strings.Join(ids, ",")}, "retmode": {"json"}}
	var response struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := h.getJSON(ctx, h.ncbiURL("esummary.fcgi", params), &response); err != nil {
		return nil, fmt.Errorf("fetch SRA metadata: %w", err)
	}
	records := make([]harvestRecord, 0, len(ids))
	for _, id := range ids {
		raw, ok := response.Result[id]
		if !ok {
			continue
		}
		var item struct {
			ExpXML string `json:"expxml"`
			Runs   string `json:"runs"`
		}
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decode SRA record %s: %w", id, err)
		}
		var detail struct {
			Summary struct {
				Title string `xml:"Title"`
			} `xml:"Summary"`
			Experiment struct {
				Accession string `xml:"acc,attr"`
				Status    string `xml:"status,attr"`
			} `xml:"Experiment"`
			Study struct {
				Accession string `xml:"acc,attr"`
				Name      string `xml:"name,attr"`
			} `xml:"Study"`
			Organism struct {
				Name string `xml:"ScientificName,attr"`
			} `xml:"Organism"`
			BioProject string `xml:"Bioproject"`
		}
		if err := xml.Unmarshal([]byte("<SRA>"+item.ExpXML+"</SRA>"), &detail); err != nil {
			return nil, fmt.Errorf("decode SRA experiment %s: %w", id, err)
		}
		var runs struct {
			Items []struct {
				Accession string `xml:"acc,attr"`
				Public    string `xml:"is_public,attr"`
			} `xml:"Run"`
		}
		if err := xml.Unmarshal([]byte("<Runs>"+item.Runs+"</Runs>"), &runs); err != nil {
			return nil, fmt.Errorf("decode SRA runs %s: %w", id, err)
		}
		externalID := detail.Experiment.Accession
		if externalID == "" {
			externalID = detail.Study.Accession
		}
		if externalID == "" {
			continue
		}
		title := strings.TrimSpace(detail.Summary.Title)
		if title == "" {
			title = strings.TrimSpace(detail.Study.Name)
		}
		runAccessions := make([]string, 0, len(runs.Items))
		for _, run := range runs.Items {
			if run.Accession != "" {
				runAccessions = append(runAccessions, run.Accession)
			}
		}
		payload := map[string]any{
			"title": title, "experiment": externalID, "study": detail.Study.Accession,
			"organism": detail.Organism.Name, "bioproject": detail.BioProject, "runs": runAccessions,
		}
		records = append(records, harvestRecord{
			ExternalID: externalID, Title: title,
			URL: "https://www.ncbi.nlm.nih.gov/sra/" + url.PathEscape(externalID), Payload: payload,
		})
	}
	return records, nil
}

func (h *remoteHarvester) harvestENA(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	// read_study can return one row per run. Fetch extra rows, then collapse to
	// unique study accessions before applying the user's result limit.
	limit := config.MaxResults * 5
	params := url.Values{
		"result": {"read_study"}, "query": {config.Query},
		"fields": {"study_accession,study_title,scientific_name,first_public"},
		"format": {"json"}, "limit": {strconv.Itoa(limit)},
	}
	var items []struct {
		Accession string `json:"study_accession"`
		Title     string `json:"study_title"`
		Species   string `json:"scientific_name"`
		Public    string `json:"first_public"`
	}
	if err := h.getJSON(ctx, enaSearchURL+"?"+params.Encode(), &items); err != nil {
		return nil, fmt.Errorf("search ENA: %w", err)
	}
	records := make([]harvestRecord, 0, config.MaxResults)
	seen := map[string]bool{}
	for _, item := range items {
		if item.Accession == "" || seen[item.Accession] {
			continue
		}
		seen[item.Accession] = true
		records = append(records, harvestRecord{
			ExternalID: item.Accession, Title: item.Title,
			URL: "https://www.ebi.ac.uk/ena/browser/view/" + url.PathEscape(item.Accession), Payload: item,
		})
		if len(records) >= config.MaxResults {
			break
		}
	}
	return records, nil
}
