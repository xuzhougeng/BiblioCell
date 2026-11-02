package library

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestLookupDOIMergesCrossrefAndEuropePMC(t *testing.T) {
	harvester := newRemoteHarvester(func() SourceCredentials { return SourceCredentials{ContactEmail: "me@example.org"} })
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "api.crossref.org":
			if request.URL.EscapedPath() != "/works/10.1101/2025.01.02.123456" || request.URL.Query().Get("mailto") != "me@example.org" {
				t.Errorf("Crossref request = %s", request.URL)
			}
			return pdfResponse(`{"message":{"DOI":"10.1101/2025.01.02.123456","title":["Root <i>atlas</i>"],"type":"posted-content",` +
				`"container-title":[],"published":{"date-parts":[[2025,1,2]]},"author":[{"given":"Ada","family":"Li"}]}}`), nil
		case "www.ebi.ac.uk":
			if q := request.URL.Query().Get("query"); q != `DOI:"10.1101/2025.01.02.123456"` {
				t.Errorf("Europe PMC query = %q", q)
			}
			return pdfResponse(`{"resultList":{"result":[{"id":"PPR1","source":"PPR","doi":"10.1101/2025.01.02.123456",` +
				`"title":"Root atlas","abstractText":"Single-nucleus roots.","pmid":"","journalInfo":{"journal":{"title":"bioRxiv"}}}]}}`), nil
		}
		return nil, errors.New("unexpected request")
	})
	paper, err := harvester.lookupDOI(context.Background(), "https://doi.org/10.1101/2025.01.02.123456")
	if err != nil {
		t.Fatal(err)
	}
	if paper.Title != "Root atlas" || paper.Authors != "Ada Li" || paper.Year == nil || *paper.Year != 2025 ||
		paper.PublicationStatus != "preprint" || paper.Abstract != "Single-nucleus roots." || paper.Journal != "bioRxiv" ||
		paper.DOI != "10.1101/2025.01.02.123456" {
		t.Errorf("paper = %+v", paper)
	}
}

func TestLookupDOIReportsMissingRecord(t *testing.T) {
	harvester := newRemoteHarvester(noCredentials)
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host == "api.crossref.org" {
			return &http.Response{StatusCode: 404, Body: pdfResponse("Resource not found.").Body}, nil
		}
		return pdfResponse(`{"resultList":{"result":[]}}`), nil
	})
	if _, err := harvester.lookupDOI(context.Background(), "10.1000/missing"); !errors.Is(err, ErrDOINotFound) {
		t.Errorf("missing DOI: %v", err)
	}
	if _, err := harvester.lookupDOI(context.Background(), "not a doi"); err == nil || errors.Is(err, ErrDOINotFound) {
		t.Errorf("malformed DOI: %v", err)
	}

	harvester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("connection refused")
	})
	_, err := harvester.lookupDOI(context.Background(), "10.1000/offline")
	if !errors.Is(err, ErrDOINotFound) || !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("offline: %v", err)
	}
}
