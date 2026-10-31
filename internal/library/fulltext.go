package library

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	unpaywallURL     = "https://api.unpaywall.org/v2/"
	pmcOpenDataURL   = "https://pmc-oa-opendata.s3.amazonaws.com/"
	maxFulltextBytes = 200 << 20
	fulltextTimeout  = 3 * time.Minute
)

// ErrNoFulltext means no open copy could be downloaded.
var ErrNoFulltext = errors.New("没有找到可下载的开放全文")

// FulltextResult says where a stored full text came from. Source is empty when
// the paper already had one and nothing was fetched.
type FulltextResult struct {
	File    PaperFile `json:"file"`
	Source  string    `json:"source"`
	URL     string    `json:"url"`
	License string    `json:"license"`
}

// fulltextCandidate is one place that offers a PDF, most trusted first.
type fulltextCandidate struct {
	source  string
	url     string
	license string
}

// FetchFulltext stores an open-access PDF for a paper. It uses only services that
// publish PDFs for download: a site that answers with a bot check counts as
// failed, and is never worked around.
func (s *Store) FetchFulltext(ctx context.Context, paperID string, credentials func() SourceCredentials) (FulltextResult, error) {
	return s.fetchFulltext(ctx, paperID, newRemoteHarvester(credentials))
}

func (s *Store) fetchFulltext(ctx context.Context, paperID string, harvester *remoteHarvester) (FulltextResult, error) {
	paper, err := s.GetPaper(ctx, paperID)
	if err != nil {
		return FulltextResult{}, err
	}
	files, err := s.ListPaperFiles(ctx)
	if err != nil {
		return FulltextResult{}, err
	}
	for _, file := range files {
		if file.PaperID == paperID && file.Type == "fulltext" {
			return FulltextResult{File: file}, nil
		}
	}
	if paper.DOI == "" && paper.PMID == "" {
		return FulltextResult{}, fmt.Errorf("%w：这篇文献没有 DOI 或 PMID", ErrNoFulltext)
	}
	candidates, notes := harvester.fulltextCandidates(ctx, paper)
	for _, candidate := range candidates {
		file, err := s.storeFulltext(ctx, paperID, candidate, harvester)
		if err == nil {
			return FulltextResult{File: file, Source: candidate.source, URL: candidate.url, License: candidate.license}, nil
		}
		if ctx.Err() != nil {
			return FulltextResult{}, ctx.Err()
		}
		notes = append(notes, candidate.source+"："+err.Error())
	}
	if len(notes) == 0 {
		return FulltextResult{}, ErrNoFulltext
	}
	return FulltextResult{}, fmt.Errorf("%w（%s）", ErrNoFulltext, strings.Join(notes, "；"))
}

func (s *Store) storeFulltext(ctx context.Context, paperID string, candidate fulltextCandidate, harvester *remoteHarvester) (PaperFile, error) {
	pdf, err := harvester.downloadPDF(ctx, candidate.url)
	if err != nil {
		return PaperFile{}, err
	}
	defer os.Remove(pdf.Name())
	defer pdf.Close()
	file, err := s.importPaperFile(ctx, paperID, pdf, candidate.source+" 开放全文.pdf", "fulltext")
	if err != nil {
		return PaperFile{}, err
	}
	if _, err := s.db.ExecContext(ctx, `
		UPDATE resources SET url = ?, license = nullif(?, ''), access_status = 'available', last_checked_at = ?
		WHERE id = ?`, candidate.url, candidate.license, time.Now().UTC().Format(time.RFC3339Nano), file.ID); err != nil {
		return PaperFile{}, fmt.Errorf("record full text source: %w", err)
	}
	return file, nil
}

// fulltextCandidates asks each service in turn. A failed lookup becomes a note,
// because the next service may still have the paper.
func (h *remoteHarvester) fulltextCandidates(ctx context.Context, paper Paper) (candidates []fulltextCandidate, notes []string) {
	pmc, err := h.pmcCandidate(ctx, paper)
	if err != nil {
		notes = append(notes, "PMC："+err.Error())
	} else if pmc != nil {
		candidates = append(candidates, *pmc)
	}
	if paper.DOI == "" {
		return candidates, notes
	}
	if h.credentials().ContactEmail == "" {
		return candidates, append(notes, "Unpaywall：未设置联系邮箱，已跳过，可在 Crossref 来源配置中填写")
	}
	found, landing, err := h.unpaywallCandidates(ctx, paper.DOI)
	if err != nil {
		return candidates, append(notes, "Unpaywall："+err.Error())
	}
	if len(found) == 0 && landing != "" {
		notes = append(notes, "Unpaywall：只有网页版本 "+landing)
	}
	return append(candidates, found...), notes
}

var pmcPDFKey = regexp.MustCompile(`^PMC\d+\.(\d+)/PMC\d+\.\d+\.pdf$`)

// pmcCandidate finds a paper's PDF in the PMC Open Access Subset, which NCBI
// publishes as an open S3 bucket. Europe PMC only supplies the PMCID.
func (h *remoteHarvester) pmcCandidate(ctx context.Context, paper Paper) (*fulltextCandidate, error) {
	query := `DOI:"` + paper.DOI + `"`
	if paper.DOI == "" {
		query = "EXT_ID:" + paper.PMID + " AND SRC:MED"
	}
	params := url.Values{"query": {query}, "format": {"json"}, "resultType": {"core"}, "pageSize": {"1"}}
	var search struct {
		Results struct {
			Items []struct {
				PMCID   string `json:"pmcid"`
				License string `json:"license"`
			} `json:"result"`
		} `json:"resultList"`
	}
	if err := h.getJSON(ctx, europePMCURL+"?"+params.Encode(), &search); err != nil {
		return nil, err
	}
	if len(search.Results.Items) == 0 || search.Results.Items[0].PMCID == "" {
		return nil, nil
	}
	item := search.Results.Items[0]
	body, err := h.get(ctx, pmcOpenDataURL+"?"+url.Values{"list-type": {"2"}, "prefix": {item.PMCID + "."}}.Encode())
	if err != nil {
		return nil, err
	}
	var listing struct {
		Keys []string `xml:"Contents>Key"`
	}
	if err := xml.Unmarshal(body, &listing); err != nil {
		return nil, fmt.Errorf("decode PMC listing: %w", err)
	}
	latest, version := "", 0
	for _, key := range listing.Keys {
		match := pmcPDFKey.FindStringSubmatch(key)
		if match == nil {
			continue
		}
		if number, err := strconv.Atoi(match[1]); err == nil && number > version {
			latest, version = key, number
		}
	}
	if latest == "" {
		return nil, nil
	}
	return &fulltextCandidate{source: "PMC", url: pmcOpenDataURL + latest, license: item.License}, nil
}

// unpaywallCandidates lists every location with a PDF: the best one often has
// only a landing page, returned in case nothing else turns up.
func (h *remoteHarvester) unpaywallCandidates(ctx context.Context, doi string) ([]fulltextCandidate, string, error) {
	endpoint := unpaywallURL + strings.ReplaceAll(url.PathEscape(doi), "%2F", "/") + "?" +
		url.Values{"email": {h.credentials().ContactEmail}}.Encode()
	var response struct {
		Locations []struct {
			PDF     string `json:"url_for_pdf"`
			License string `json:"license"`
			Landing string `json:"url_for_landing_page"`
		} `json:"oa_locations"`
	}
	if err := h.getJSON(ctx, endpoint, &response); err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, "", nil // Unpaywall does not know this DOI.
		}
		return nil, "", err
	}
	candidates, landing := []fulltextCandidate{}, ""
	for _, location := range response.Locations {
		if location.PDF != "" {
			candidates = append(candidates, fulltextCandidate{source: "Unpaywall", url: location.PDF, license: location.License})
		}
		if landing == "" {
			landing = location.Landing
		}
	}
	return candidates, landing, nil
}

// downloadPDF saves a link to a temporary file for the caller to remove. Links
// come from third parties, so only https is requested and the bytes must be a
// PDF: sites often answer with a login or bot-check page and status 200.
// ponytail: https-only, not an address filter; block private ranges if these
// links ever come from somewhere less curated than Unpaywall and PMC.
func (h *remoteHarvester) downloadPDF(ctx context.Context, link string) (*os.File, error) {
	parsed, err := url.Parse(link)
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("不是网页链接")
	}
	parsed.Scheme = "https" // Repositories often list an http address for an https site.
	client := *h.client
	client.Timeout = fulltextTimeout
	client.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" || len(via) >= 10 {
			return errors.New("重定向到了不安全的地址")
		}
		return nil
	}
	request, err := h.newRequest(ctx, parsed.String())
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/pdf")
	response, err := client.Do(request)
	if err != nil {
		return nil, plainRequestError(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	temporary, err := os.CreateTemp("", "bibliocell-*.pdf")
	if err != nil {
		return nil, err
	}
	keep := false
	defer func() {
		if !keep {
			temporary.Close()
			os.Remove(temporary.Name())
		}
	}()
	size, err := io.Copy(temporary, io.LimitReader(response.Body, maxFulltextBytes+1))
	if err != nil {
		return nil, plainRequestError(err)
	}
	if size > maxFulltextBytes {
		return nil, errors.New("文件超过 200 MiB")
	}
	magic := make([]byte, 5)
	if _, err := temporary.ReadAt(magic, 0); err != nil || string(magic) != "%PDF-" {
		return nil, errors.New("返回的不是 PDF")
	}
	if _, err := temporary.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return temporary, nil
}
