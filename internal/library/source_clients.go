package library

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ncbiEUtilsURL  = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/"
	europePMCURL   = "https://www.ebi.ac.uk/europepmc/webservices/rest/search"
	crossrefURL    = "https://api.crossref.org/works"
	bioRxivURL     = "https://api.biorxiv.org/details/biorxiv/"
	maxRemoteBytes = 16 << 20
)

// SourceCredentials raise the rate limits of the sources that offer it.
type SourceCredentials struct {
	NCBIAPIKey   string
	ContactEmail string
}

type remoteHarvester struct {
	client      *http.Client
	credentials func() SourceCredentials
}

// newRemoteHarvester reads credentials at each request, so an edit applies to
// the next sync without a restart.
func newRemoteHarvester(credentials func() SourceCredentials) *remoteHarvester {
	return &remoteHarvester{client: &http.Client{Timeout: 60 * time.Second}, credentials: credentials}
}

func (h *remoteHarvester) Harvest(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	switch config.ID {
	case "pubmed":
		return h.harvestPubMed(ctx, config)
	case "europe-pmc":
		return h.harvestEuropePMC(ctx, config)
	case "crossref":
		return h.harvestCrossref(ctx, config)
	case "biorxiv":
		return h.harvestBioRxiv(ctx, config)
	case "sequence-repositories":
		return h.harvestRepository(ctx, config)
	default:
		return nil, fmt.Errorf("source %s has no automatic connector", config.ID)
	}
}

func (h *remoteHarvester) newRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("create remote request: %w", err)
	}
	req.Header.Set("User-Agent", "BiblioCell/0.1 (personal research library; manual metadata sync)")
	return req, nil
}

// plainRequestError drops the URL: it can carry a key, and this message is saved
// with the run.
func plainRequestError(err error) error {
	var requestError *url.Error
	if errors.As(err, &requestError) {
		return requestError.Err
	}
	return err
}

func (h *remoteHarvester) get(ctx context.Context, endpoint string) ([]byte, error) {
	req, err := h.newRequest(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json, application/xml, text/xml;q=0.9, */*;q=0.1")
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("contact remote source: %w", plainRequestError(err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read remote response: %w", err)
	}
	if len(body) > maxRemoteBytes {
		return nil, errors.New("remote response exceeds 16 MiB; reduce the result limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet := strings.Join(strings.Fields(string(body)), " ")
		// NCBI repeats a rejected key in its error body.
		if key := h.credentials().NCBIAPIKey; key != "" {
			snippet = strings.ReplaceAll(snippet, key, "[hidden]")
		}
		if len(snippet) > 240 {
			snippet = snippet[:240]
		}
		return nil, fmt.Errorf("remote source returned HTTP %d: %s", resp.StatusCode, snippet)
	}
	return body, nil
}

func (h *remoteHarvester) getJSON(ctx context.Context, endpoint string, target any) error {
	body, err := h.get(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode remote JSON: %w", err)
	}
	return nil
}

// ncbiURL builds an E-utilities request, with the API key when there is one.
func (h *remoteHarvester) ncbiURL(tool string, params url.Values) string {
	if key := h.credentials().NCBIAPIKey; key != "" {
		params.Set("api_key", key)
	}
	return ncbiEUtilsURL + tool + "?" + params.Encode()
}

type eSearchResponse struct {
	Result struct {
		IDs []string `json:"idlist"`
	} `json:"esearchresult"`
}

func (h *remoteHarvester) searchNCBI(ctx context.Context, database, query string, limit int) ([]string, error) {
	params := url.Values{
		"db": {database}, "term": {query}, "retmode": {"json"},
		"retmax": {strconv.Itoa(limit)}, "sort": {"pub date"},
	}
	var response eSearchResponse
	if err := h.getJSON(ctx, h.ncbiURL("esearch.fcgi", params), &response); err != nil {
		return nil, fmt.Errorf("search NCBI %s: %w", database, err)
	}
	return response.Result.IDs, nil
}

type markedText struct {
	Inner string `xml:",innerxml"`
}

func (t markedText) String() string { return stripMarkup(t.Inner) }

type pubmedArticleSet struct {
	Articles []struct {
		Citation struct {
			PMID    string `xml:"PMID"`
			Article struct {
				Title    markedText `xml:"ArticleTitle"`
				Abstract struct {
					Sections []struct {
						Label string `xml:"Label,attr"`
						Inner string `xml:",innerxml"`
					} `xml:"AbstractText"`
				} `xml:"Abstract"`
				Authors []struct {
					LastName   string `xml:"LastName"`
					ForeName   string `xml:"ForeName"`
					Collective string `xml:"CollectiveName"`
				} `xml:"AuthorList>Author"`
				Journal struct {
					Title string `xml:"Title"`
					Issue struct {
						PubDate struct {
							Year        string `xml:"Year"`
							MedlineDate string `xml:"MedlineDate"`
						} `xml:"PubDate"`
					} `xml:"JournalIssue"`
				} `xml:"Journal"`
				PublicationTypes []string `xml:"PublicationTypeList>PublicationType"`
			} `xml:"Article"`
		} `xml:"MedlineCitation"`
		Identifiers []struct {
			Type  string `xml:"IdType,attr"`
			Value string `xml:",chardata"`
		} `xml:"PubmedData>ArticleIdList>ArticleId"`
	} `xml:"PubmedArticle"`
}

func (h *remoteHarvester) harvestPubMed(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	ids, err := h.searchNCBI(ctx, "pubmed", config.Query, config.MaxResults)
	if err != nil || len(ids) == 0 {
		return []harvestRecord{}, err
	}
	params := url.Values{"db": {"pubmed"}, "id": {strings.Join(ids, ",")}, "retmode": {"xml"}}
	body, err := h.get(ctx, h.ncbiURL("efetch.fcgi", params))
	if err != nil {
		return nil, fmt.Errorf("fetch PubMed records: %w", err)
	}
	var parsed pubmedArticleSet
	if err := xml.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("decode PubMed XML: %w", err)
	}
	records := make([]harvestRecord, 0, len(parsed.Articles))
	for _, item := range parsed.Articles {
		article := item.Citation.Article
		title := article.Title.String()
		pmid := strings.TrimSpace(item.Citation.PMID)
		if title == "" || pmid == "" {
			continue
		}
		authors := make([]string, 0, len(article.Authors))
		for _, author := range article.Authors {
			name := strings.TrimSpace(author.ForeName + " " + author.LastName)
			if author.Collective != "" {
				name = author.Collective
			}
			if name != "" {
				authors = append(authors, name)
			}
		}
		abstractParts := make([]string, 0, len(article.Abstract.Sections))
		for _, section := range article.Abstract.Sections {
			text := stripMarkup(section.Inner)
			if text == "" {
				continue
			}
			if section.Label != "" {
				text = section.Label + ": " + text
			}
			abstractParts = append(abstractParts, text)
		}
		doi := ""
		for _, identifier := range item.Identifiers {
			if strings.EqualFold(identifier.Type, "doi") {
				doi = identifier.Value
				break
			}
		}
		year := yearFromString(article.Journal.Issue.PubDate.Year + " " + article.Journal.Issue.PubDate.MedlineDate)
		paper := Paper{
			Title: title, DOI: doi, PMID: pmid, Authors: strings.Join(authors, "; "), Year: year,
			Journal: article.Journal.Title, PublicationStatus: "published",
			ArticleTypes: literatureArticleTypes(article.PublicationTypes), Abstract: strings.Join(abstractParts, "\n"),
			Source: "PubMed",
		}
		records = append(records, harvestRecord{
			ExternalID: pmid, Title: title, URL: "https://pubmed.ncbi.nlm.nih.gov/" + url.PathEscape(pmid) + "/",
			Paper: &paper, Payload: paper,
		})
	}
	return records, nil
}

func (h *remoteHarvester) harvestEuropePMC(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	params := url.Values{
		"query": {config.Query}, "format": {"json"}, "resultType": {"core"},
		"pageSize": {strconv.Itoa(config.MaxResults)},
	}
	var response struct {
		Results struct {
			Items []struct {
				ID           string `json:"id"`
				Source       string `json:"source"`
				PMID         string `json:"pmid"`
				DOI          string `json:"doi"`
				Title        string `json:"title"`
				AuthorString string `json:"authorString"`
				Abstract     string `json:"abstractText"`
				PubYear      string `json:"pubYear"`
				PubTypes     struct {
					Values []string `json:"pubType"`
				} `json:"pubTypeList"`
				JournalInfo struct {
					Journal struct {
						Title string `json:"title"`
					} `json:"journal"`
				} `json:"journalInfo"`
			} `json:"result"`
		} `json:"resultList"`
	}
	if err := h.getJSON(ctx, europePMCURL+"?"+params.Encode(), &response); err != nil {
		return nil, fmt.Errorf("search Europe PMC: %w", err)
	}
	records := make([]harvestRecord, 0, len(response.Results.Items))
	for _, item := range response.Results.Items {
		if strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.ID) == "" {
			continue
		}
		status := "published"
		if item.Source == "PPR" {
			status = "preprint"
		}
		paper := Paper{
			Title: stripMarkup(item.Title), DOI: item.DOI, PMID: item.PMID,
			Authors: strings.ReplaceAll(item.AuthorString, ", ", "; "), Year: yearFromString(item.PubYear),
			Journal: item.JournalInfo.Journal.Title, PublicationStatus: status,
			ArticleTypes: literatureArticleTypes(item.PubTypes.Values), Abstract: stripMarkup(item.Abstract),
			Source: "Europe PMC",
		}
		externalID := item.Source + ":" + item.ID
		link := "https://europepmc.org/article/" + url.PathEscape(item.Source) + "/" + url.PathEscape(item.ID)
		records = append(records, harvestRecord{
			ExternalID: externalID, Title: paper.Title, URL: link, Paper: &paper, Payload: item,
		})
	}
	return records, nil
}

func (h *remoteHarvester) harvestCrossref(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	params := url.Values{
		"query.bibliographic": {config.Query}, "rows": {strconv.Itoa(config.MaxResults)},
		"select": {"DOI,title,author,published,container-title,abstract,type,URL"},
	}
	if email := h.credentials().ContactEmail; email != "" {
		params.Set("mailto", email)
	}
	var response struct {
		Message struct {
			Items []struct {
				DOI       string   `json:"DOI"`
				Title     []string `json:"title"`
				Container []string `json:"container-title"`
				Abstract  string   `json:"abstract"`
				Type      string   `json:"type"`
				URL       string   `json:"URL"`
				Published struct {
					DateParts [][]int `json:"date-parts"`
				} `json:"published"`
				Authors []struct {
					Given  string `json:"given"`
					Family string `json:"family"`
				} `json:"author"`
			} `json:"items"`
		} `json:"message"`
	}
	if err := h.getJSON(ctx, crossrefURL+"?"+params.Encode(), &response); err != nil {
		return nil, fmt.Errorf("search Crossref: %w", err)
	}
	records := make([]harvestRecord, 0, len(response.Message.Items))
	for _, item := range response.Message.Items {
		if item.DOI == "" || len(item.Title) == 0 || stripMarkup(item.Title[0]) == "" {
			continue
		}
		authors := make([]string, 0, len(item.Authors))
		for _, author := range item.Authors {
			if name := strings.TrimSpace(author.Given + " " + author.Family); name != "" {
				authors = append(authors, name)
			}
		}
		var year *int
		if len(item.Published.DateParts) > 0 && len(item.Published.DateParts[0]) > 0 {
			value := item.Published.DateParts[0][0]
			year = &value
		}
		journal := ""
		if len(item.Container) > 0 {
			journal = item.Container[0]
		}
		articleTypes := []string{}
		if item.Type == "review-article" {
			articleTypes = append(articleTypes, "综述")
		}
		paper := Paper{
			Title: stripMarkup(item.Title[0]), DOI: item.DOI, Authors: strings.Join(authors, "; "),
			Year: year, Journal: journal, PublicationStatus: "published", ArticleTypes: articleTypes,
			Abstract: stripMarkup(item.Abstract), Source: "Crossref",
		}
		if item.Type == "posted-content" {
			paper.PublicationStatus = "preprint"
		}
		link := item.URL
		if link == "" {
			link = "https://doi.org/" + normalizeDOI(item.DOI)
		}
		records = append(records, harvestRecord{
			ExternalID: normalizeDOI(item.DOI), Title: paper.Title, URL: link, Paper: &paper, Payload: item,
		})
	}
	return records, nil
}

func (h *remoteHarvester) harvestBioRxiv(ctx context.Context, config SourceConfig) ([]harvestRecord, error) {
	terms := strings.Fields(normalizeSearchText(config.Query))
	if len(terms) == 0 {
		return nil, fmt.Errorf("bioRxiv keywords are required")
	}
	end := time.Now().UTC()
	start := end.AddDate(0, 0, -30)
	records := []harvestRecord{}
	seen := map[string]bool{}
	for cursor, pages := 0, 0; pages < 10 && len(records) < config.MaxResults; pages++ {
		endpoint := bioRxivURL + start.Format("2006-01-02") + "/" + end.Format("2006-01-02") + "/" + strconv.Itoa(cursor)
		var response struct {
			Collection []struct {
				Title    string `json:"title"`
				Authors  string `json:"authors"`
				DOI      string `json:"doi"`
				Date     string `json:"date"`
				Version  string `json:"version"`
				Category string `json:"category"`
				Abstract string `json:"abstract"`
			} `json:"collection"`
		}
		if err := h.getJSON(ctx, endpoint, &response); err != nil {
			return nil, fmt.Errorf("scan bioRxiv: %w", err)
		}
		if len(response.Collection) == 0 {
			break
		}
		for _, item := range response.Collection {
			haystack := normalizeSearchText(item.Title + " " + item.Abstract + " " + item.Category)
			matched := true
			for _, term := range terms {
				if !strings.Contains(haystack, term) {
					matched = false
					break
				}
			}
			externalID := item.DOI + "v" + item.Version
			if !matched || item.DOI == "" || item.Title == "" || seen[externalID] {
				continue
			}
			seen[externalID] = true
			paper := Paper{
				Title: item.Title, DOI: item.DOI, Authors: item.Authors, Year: yearFromString(item.Date),
				Journal: "bioRxiv", PublicationStatus: "preprint", ArticleTypes: []string{"原创研究"},
				Abstract: item.Abstract, Source: "bioRxiv",
			}
			records = append(records, harvestRecord{
				ExternalID: externalID, Title: item.Title,
				URL:   "https://www.biorxiv.org/content/" + item.DOI + "v" + url.PathEscape(item.Version),
				Paper: &paper, Payload: item,
			})
			if len(records) >= config.MaxResults {
				break
			}
		}
		cursor += len(response.Collection)
	}
	return records, nil
}

var markupPattern = regexp.MustCompile(`(?s)<[^>]+>`)
var nonWordPattern = regexp.MustCompile(`[^a-z0-9]+`)
var yearPattern = regexp.MustCompile(`(?:18|19|20|21)\d{2}`)

func stripMarkup(value string) string {
	value = markupPattern.ReplaceAllString(value, " ")
	return strings.Join(strings.Fields(html.UnescapeString(value)), " ")
}

func normalizeSearchText(value string) string {
	return strings.TrimSpace(nonWordPattern.ReplaceAllString(strings.ToLower(value), " "))
}

func yearFromString(value string) *int {
	match := yearPattern.FindString(value)
	if match == "" {
		return nil
	}
	year, err := strconv.Atoi(match)
	if err != nil {
		return nil
	}
	return &year
}

func literatureArticleTypes(values []string) []string {
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), "review") {
			return []string{"综述"}
		}
	}
	for _, value := range values {
		if strings.EqualFold(value, "Journal Article") || strings.EqualFold(value, "Preprint") {
			return []string{"原创研究"}
		}
	}
	return []string{}
}
