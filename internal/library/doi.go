package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ErrDOINotFound means no source returned metadata for a DOI.
var ErrDOINotFound = errors.New("没有找到这个 DOI 的书目信息")

// LookupDOI fetches bibliographic metadata for a DOI without saving anything:
// Crossref gives the publisher's record, and Europe PMC adds the PMID and the
// abstract Crossref often lacks. The person decides what to keep.
func LookupDOI(ctx context.Context, doi string, credentials func() SourceCredentials) (Paper, error) {
	return newRemoteHarvester(credentials).lookupDOI(ctx, doi)
}

func (h *remoteHarvester) lookupDOI(ctx context.Context, doi string) (Paper, error) {
	doi = normalizeDOI(doi)
	if !doiPattern.MatchString(doi) {
		return Paper{}, fmt.Errorf("invalid DOI %q", doi)
	}
	var notes []string
	paper, err := h.crossrefDOI(ctx, doi)
	if err != nil {
		notes = append(notes, "Crossref："+err.Error())
	}
	extra, err := h.europePMCDOI(ctx, doi)
	if err != nil {
		notes = append(notes, "Europe PMC："+err.Error())
	}
	paper = mergeLookup(paper, extra)
	if paper.Title == "" {
		return Paper{}, lookupMiss(ctx, ErrDOINotFound, notes)
	}
	paper.DOI = doi
	return paper, nil
}

// ErrPMIDNotFound means no source returned metadata for a PMID.
var ErrPMIDNotFound = errors.New("没有找到这个 PMID 的书目信息")

// LookupPMID fetches bibliographic metadata for a PubMed ID without saving
// anything: Europe PMC mirrors PubMed, and when the record carries a DOI,
// Crossref adds the publisher's record the way LookupDOI does.
func LookupPMID(ctx context.Context, pmid string, credentials func() SourceCredentials) (Paper, error) {
	return newRemoteHarvester(credentials).lookupPMID(ctx, pmid)
}

// LookupIdentifier looks up a DOI or a PMID, telling them apart by shape: a
// PMID is all digits, anything else is read as a DOI.
func LookupIdentifier(ctx context.Context, value string, credentials func() SourceCredentials) (Paper, error) {
	if pmidPattern.MatchString(normalizePMID(value)) {
		return LookupPMID(ctx, value, credentials)
	}
	if !doiPattern.MatchString(normalizeDOI(value)) {
		return Paper{}, fmt.Errorf("%q is neither a DOI (10.xxxx/...) nor a PMID", strings.TrimSpace(value))
	}
	return LookupDOI(ctx, value, credentials)
}

var pmidPattern = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

func (h *remoteHarvester) lookupPMID(ctx context.Context, pmid string) (Paper, error) {
	pmid = normalizePMID(pmid)
	if !pmidPattern.MatchString(pmid) {
		return Paper{}, fmt.Errorf("invalid PMID %q", pmid)
	}
	var notes []string
	extra, err := h.europePMCPMID(ctx, pmid)
	if err != nil {
		notes = append(notes, "Europe PMC："+err.Error())
	}
	paper := extra
	if doi := normalizeDOI(extra.DOI); doiPattern.MatchString(doi) {
		publisher, err := h.crossrefDOI(ctx, doi)
		if err != nil {
			notes = append(notes, "Crossref："+err.Error())
		}
		paper = mergeLookup(publisher, extra)
		paper.DOI = doi
	} else {
		paper.DOI = ""
	}
	if paper.Title == "" {
		return Paper{}, lookupMiss(ctx, ErrPMIDNotFound, notes)
	}
	paper.PMID = pmid
	return paper, nil
}

// mergeLookup keeps the publisher's record and fills its blanks from Europe
// PMC, which carries the PMID and the abstract Crossref often lacks.
func mergeLookup(paper, extra Paper) Paper {
	if paper.Title == "" {
		return extra
	}
	fillBlank(&paper.PMID, extra.PMID)
	fillBlank(&paper.Abstract, extra.Abstract)
	fillBlank(&paper.Authors, extra.Authors)
	fillBlank(&paper.Journal, extra.Journal)
	if paper.Year == nil {
		paper.Year = extra.Year
	}
	if len(paper.ArticleTypes) == 0 {
		paper.ArticleTypes = extra.ArticleTypes
	}
	return paper
}

// lookupMiss explains an empty lookup: a cancelled request, the source
// errors that hid the record, or a plain not found.
func lookupMiss(ctx context.Context, notFound error, notes []string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if len(notes) > 0 {
		return fmt.Errorf("%w（%s）", notFound, strings.Join(notes, "；"))
	}
	return notFound
}

func fillBlank(target *string, value string) {
	if strings.TrimSpace(*target) == "" {
		*target = value
	}
}

// crossrefDOI returns an empty paper, not an error, when Crossref has no record.
func (h *remoteHarvester) crossrefDOI(ctx context.Context, doi string) (Paper, error) {
	endpoint := crossrefURL + "/" + strings.ReplaceAll(url.PathEscape(doi), "%2F", "/")
	if email := h.credentials().ContactEmail; email != "" {
		endpoint += "?" + url.Values{"mailto": {email}}.Encode()
	}
	var response struct {
		Message crossrefWork `json:"message"`
	}
	if err := h.getJSON(ctx, endpoint, &response); err != nil {
		if strings.Contains(err.Error(), "HTTP 404") {
			return Paper{}, nil
		}
		return Paper{}, err
	}
	return response.Message.paper(), nil
}

func (h *remoteHarvester) europePMCDOI(ctx context.Context, doi string) (Paper, error) {
	params := url.Values{
		"query": {`DOI:"` + doi + `"`}, "format": {"json"}, "resultType": {"core"}, "pageSize": {"1"},
	}
	var response struct {
		Results struct {
			Items []europePMCResult `json:"result"`
		} `json:"resultList"`
	}
	if err := h.getJSON(ctx, europePMCURL+"?"+params.Encode(), &response); err != nil {
		return Paper{}, err
	}
	for _, item := range response.Results.Items {
		if normalizeDOI(item.DOI) == doi {
			return item.paper(), nil
		}
	}
	return Paper{}, nil
}

func (h *remoteHarvester) europePMCPMID(ctx context.Context, pmid string) (Paper, error) {
	params := url.Values{
		"query": {"EXT_ID:" + pmid + " AND SRC:MED"}, "format": {"json"}, "resultType": {"core"}, "pageSize": {"1"},
	}
	var response struct {
		Results struct {
			Items []europePMCResult `json:"result"`
		} `json:"resultList"`
	}
	if err := h.getJSON(ctx, europePMCURL+"?"+params.Encode(), &response); err != nil {
		return Paper{}, err
	}
	for _, item := range response.Results.Items {
		if item.PMID == pmid {
			return item.paper(), nil
		}
	}
	return Paper{}, nil
}
