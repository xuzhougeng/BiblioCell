package library

import (
	"context"
	"errors"
	"fmt"
	"net/url"
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
	if paper.Title == "" {
		paper = extra
	} else {
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
	}
	if paper.Title == "" {
		if ctx.Err() != nil {
			return Paper{}, ctx.Err()
		}
		if len(notes) > 0 {
			return Paper{}, fmt.Errorf("%w（%s）", ErrDOINotFound, strings.Join(notes, "；"))
		}
		return Paper{}, ErrDOINotFound
	}
	paper.DOI = doi
	return paper, nil
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
