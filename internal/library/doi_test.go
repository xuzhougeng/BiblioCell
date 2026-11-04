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

func TestLookupPMIDAddsCrossrefThroughTheDOI(t *testing.T) {
	harvester := newRemoteHarvester(noCredentials)
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Host {
		case "www.ebi.ac.uk":
			if q := request.URL.Query().Get("query"); q != "EXT_ID:31234567 AND SRC:MED" {
				t.Errorf("Europe PMC query = %q", q)
			}
			return pdfResponse(`{"resultList":{"result":[{"id":"31234567","source":"MED","pmid":"31234567","doi":"10.1038/S41477-019-0001-X",` +
				`"title":"Leaf atlas","authorString":"Li A, Wang B.","pubYear":"2019","abstractText":"Leaves.",` +
				`"journalInfo":{"journal":{"title":"Nat Plants"}}}]}}`), nil
		case "api.crossref.org":
			if request.URL.EscapedPath() != "/works/10.1038/s41477-019-0001-x" {
				t.Errorf("Crossref request = %s", request.URL)
			}
			return pdfResponse(`{"message":{"DOI":"10.1038/s41477-019-0001-x","title":["Leaf atlas"],"type":"journal-article",` +
				`"container-title":["Nature Plants"],"published":{"date-parts":[[2019,8]]},"author":[{"given":"Ada","family":"Li"}]}}`), nil
		}
		return nil, errors.New("unexpected request")
	})
	paper, err := harvester.lookupPMID(context.Background(), "https://pubmed.ncbi.nlm.nih.gov/31234567/")
	if err != nil {
		t.Fatal(err)
	}
	if paper.PMID != "31234567" || paper.DOI != "10.1038/s41477-019-0001-x" || paper.Journal != "Nature Plants" ||
		paper.Authors != "Ada Li" || paper.Abstract != "Leaves." || paper.Year == nil || *paper.Year != 2019 || paper.PublicationStatus != "published" {
		t.Errorf("paper = %+v", paper)
	}
}

func TestLookupPMIDWithoutDOIUsesEuropePMC(t *testing.T) {
	harvester := newRemoteHarvester(noCredentials)
	harvester.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "www.ebi.ac.uk" {
			t.Errorf("unexpected request %s", request.URL)
			return nil, errors.New("unexpected request")
		}
		return pdfResponse(`{"resultList":{"result":[{"id":"100","source":"MED","pmid":"100","title":"Old paper",` +
			`"authorString":"Smith J.","pubYear":"1975","journalInfo":{"journal":{"title":"J Biol"}}}]}}`), nil
	})
	paper, err := harvester.lookupPMID(context.Background(), "PMID: 100")
	if err != nil {
		t.Fatal(err)
	}
	if paper.PMID != "100" || paper.DOI != "" || paper.Journal != "J Biol" || paper.Year == nil || *paper.Year != 1975 {
		t.Errorf("paper = %+v", paper)
	}
}

func TestLookupPMIDReportsMissingRecord(t *testing.T) {
	harvester := newRemoteHarvester(noCredentials)
	harvester.client.Transport = roundTripFunc(func(*http.Request) (*http.Response, error) {
		return pdfResponse(`{"resultList":{"result":[{"pmid":"999","title":"Some other paper"}]}}`), nil
	})
	if _, err := harvester.lookupPMID(context.Background(), "12345"); !errors.Is(err, ErrPMIDNotFound) {
		t.Errorf("missing PMID: %v", err)
	}
	if _, err := harvester.lookupPMID(context.Background(), "10.1000/x"); err == nil || errors.Is(err, ErrPMIDNotFound) {
		t.Errorf("malformed PMID: %v", err)
	}
}

func TestPMIDPatternTellsIdentifiersApart(t *testing.T) {
	for value, isPMID := range map[string]bool{
		"31234567": true, "PMID: 31234567": true, "https://pubmed.ncbi.nlm.nih.gov/31234567/": true,
		"10.1038/s41477-019-0001-x": false, "https://doi.org/10.1038/x": false, "0123": false,
	} {
		if got := pmidPattern.MatchString(normalizePMID(value)); got != isPMID {
			t.Errorf("%q read as PMID = %v, want %v", value, got, isPMID)
		}
	}
}
