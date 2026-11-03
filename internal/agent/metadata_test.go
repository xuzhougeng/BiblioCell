package agent

import (
	"context"
	"strings"
	"testing"
)

func TestExtractMetadataRequiresFulltextAndChecksQuotes(t *testing.T) {
	provider := answering(`{"claims":[]}`)
	if _, err := ExtractMetadata(context.Background(), provider, "Title only", " "); err == nil || len(provider.requests) != 0 {
		t.Fatal("missing full text must fail without calling the model")
	}
	provider = answering("```json\n" + `{"claims":[
		{"field":"platforms","values":["10x Genomics Chromium"],"excerpt":"Libraries were prepared using 10x Genomics Chromium."},
		{"field":"species","species":[{"name":"Arabidopsis thaliana","label":"拟南芥"}],"excerpt":"We sampled Arabidopsis thaliana roots."},
		{"field":"organs","values":["leaf"],"excerpt":"A fabricated quotation not present in this paper."},
		{"field":"source","values":["CLI"],"excerpt":"We sampled Arabidopsis thaliana roots."},
		{"field":"dataset","repository":"GEO","accession":"GSE123","role":"generated","excerpt":"Data are available in GEO under GSE123."},
		{"field":"dataset","repository":"geo","accession":"GSE999","excerpt":"Data are available in GEO under GSE123."}
	]}` + "\n```")
	result, err := ExtractMetadata(context.Background(), provider, "Root atlas", "We sampled Arabidopsis thaliana roots.\nLibraries were prepared using\n10x Genomics Chromium.\nData are available in GEO under GSE123.")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Claims) != 3 || result.Omitted != 3 || result.Truncated {
		t.Fatalf("unexpected extraction: %+v", result)
	}
	if result.Claims[2].Repository != "geo" || result.Claims[2].Accession != "GSE123" {
		t.Fatalf("dataset: %+v", result.Claims[2])
	}
	if len(provider.requests[0].Tools) != 0 {
		t.Fatal("extraction must not have write tools")
	}
}

func TestExtractMetadataKeepsDataAvailabilityWhenTruncated(t *testing.T) {
	provider := answering(`{"claims":[{"field":"dataset","repository":"geo","accession":"GSE123","role":"unknown","excerpt":"Data are available under GSE123."}]}`)
	result, err := ExtractMetadata(context.Background(), provider, "Atlas", "BEGIN METHODS\n"+strings.Repeat("x", maxFulltextRunes+100)+"\nData are available under GSE123.")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Claims) != 1 {
		t.Fatalf("truncation: %+v", result)
	}
	prompt := provider.requests[0].Messages[0].Content
	if !strings.Contains(prompt, "BEGIN METHODS") || !strings.HasSuffix(prompt, "Data are available under GSE123.") {
		t.Fatal("lost the start or end of the paper")
	}
}

func TestExtractMetadataRejectsMalformedResponse(t *testing.T) {
	for _, text := range []string{"not JSON", `{"claims":"wrong"}`, `{"claims":[`} {
		if _, err := ExtractMetadata(context.Background(), answering(text), "Atlas", "A full text document."); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}
