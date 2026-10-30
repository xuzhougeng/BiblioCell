package library

import (
	"context"
	"encoding/xml"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeHarvester struct {
	records []harvestRecord
	err     error
}

func (f fakeHarvester) Harvest(context.Context, SourceConfig) ([]harvestRecord, error) {
	return f.records, f.err
}

func TestSourceConfigurationRoundTrip(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	sources, err := store.ListSources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 6 {
		t.Fatalf("source count = %d, want 6", len(sources))
	}
	if sources[0].Query == "" || !sources[0].Enabled || sources[5].Supported {
		t.Errorf("unexpected defaults: %+v / %+v", sources[0], sources[5])
	}

	config := SourceConfig{ID: "sequence-repositories", Enabled: true, Query: `description="*single*" AND tax_tree(33090)`, MaxResults: 7, Provider: "ena"}
	status, err := store.SaveSourceConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != "ena" || status.MaxResults != 7 || status.Query != config.Query {
		t.Errorf("saved source = %+v", status)
	}
	reopened, err := Open(store.path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	status, err = reopened.SourceStatus(context.Background(), config.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status.Provider != "ena" || status.MaxResults != 7 || status.Query != config.Query {
		t.Errorf("reopened source = %+v", status)
	}

	config.MaxResults = 101
	if _, err := store.SaveSourceConfig(context.Background(), config); err == nil {
		t.Fatal("result limit above 100 succeeded")
	}
}

func TestSyncSourceIsIdempotentAndPreservesCuration(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	year := 2026
	paper := Paper{Title: "A plant single-cell study", DOI: "10.1000/plant.1", PMID: "12345", Year: &year, Source: "PubMed"}
	harvester := fakeHarvester{records: []harvestRecord{{
		ExternalID: "12345", Title: paper.Title, URL: "https://pubmed.ncbi.nlm.nih.gov/12345/",
		Paper: &paper, Payload: map[string]string{"title": paper.Title},
	}}}

	first, err := store.syncSource(ctx, "pubmed", harvester)
	if err != nil {
		t.Fatal(err)
	}
	if first.PapersAdded != 1 || first.Duplicates != 0 || first.RecordsNew != 1 || first.Status.LastRun.Status != "succeeded" {
		t.Errorf("first sync = %+v", first)
	}
	paperID := first.Papers[0].ID
	if _, err := store.UpdatePaper(ctx, paperID, PaperPatch{Notes: stringPointer("keep this note"), Reading: stringPointer("read")}); err != nil {
		t.Fatal(err)
	}

	second, err := store.syncSource(ctx, "pubmed", harvester)
	if err != nil {
		t.Fatal(err)
	}
	if second.PapersAdded != 0 || second.Duplicates != 1 || second.RecordsNew != 0 || second.RecordsUpdated != 0 {
		t.Errorf("second sync = %+v", second)
	}
	preserved, err := store.GetPaper(ctx, paperID)
	if err != nil {
		t.Fatal(err)
	}
	if preserved.Notes != "keep this note" || preserved.Reading != "read" {
		t.Errorf("sync replaced curation: %+v", preserved)
	}
	if second.Status.RecordCount != 1 || len(second.Status.RecentRecords) != 1 || second.Status.RecentRecords[0].PaperID != paperID {
		t.Errorf("source records = %+v", second.Status)
	}
}

func TestSyncSourceFailureDoesNotCreateRecords(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "library.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	if _, err := store.syncSource(ctx, "pubmed", fakeHarvester{err: errors.New("remote unavailable")}); err == nil {
		t.Fatal("failed remote sync succeeded")
	}
	status, err := store.SourceStatus(ctx, "pubmed")
	if err != nil {
		t.Fatal(err)
	}
	if status.LastRun == nil || status.LastRun.Status != "failed" || status.RecordCount != 0 || status.LastSuccessAt != "" {
		t.Errorf("failed source state = %+v", status)
	}
	papers, err := store.ListPapers(ctx)
	if err != nil || len(papers) != 0 {
		t.Errorf("papers after failed sync = %d, %v", len(papers), err)
	}
}

func TestPubMedXMLParsesNestedTextAndIdentifiers(t *testing.T) {
	input := `<PubmedArticleSet><PubmedArticle><MedlineCitation><PMID>42</PMID><Article>
		<ArticleTitle>Single-cell <i>Arabidopsis</i> atlas</ArticleTitle>
		<Abstract><AbstractText Label="BACKGROUND">A &amp; B.</AbstractText></Abstract>
		<AuthorList><Author><LastName>Li</LastName><ForeName>Wei</ForeName></Author></AuthorList>
		<Journal><Title>Plant Journal</Title><JournalIssue><PubDate><Year>2025</Year></PubDate></JournalIssue></Journal>
		<PublicationTypeList><PublicationType>Journal Article</PublicationType></PublicationTypeList>
		</Article></MedlineCitation><PubmedData><ArticleIdList><ArticleId IdType="doi">10.1000/TEST</ArticleId></ArticleIdList></PubmedData>
		</PubmedArticle></PubmedArticleSet>`
	var parsed pubmedArticleSet
	if err := xml.Unmarshal([]byte(input), &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Articles) != 1 {
		t.Fatalf("articles = %d", len(parsed.Articles))
	}
	article := parsed.Articles[0]
	if got := article.Citation.Article.Title.String(); got != "Single-cell Arabidopsis atlas" {
		t.Errorf("title = %q", got)
	}
	if got := stripMarkup(article.Citation.Article.Abstract.Sections[0].Inner); got != "A & B." {
		t.Errorf("abstract = %q", got)
	}
	if article.Identifiers[0].Value != "10.1000/TEST" {
		t.Errorf("DOI = %q", article.Identifiers[0].Value)
	}
}

func TestRemoteConnectorsLive(t *testing.T) {
	if os.Getenv("BIBLIOCELL_LIVE_TEST") != "1" {
		t.Skip("set BIBLIOCELL_LIVE_TEST=1 to query public source APIs")
	}
	configs := []SourceConfig{}
	for _, id := range []string{"pubmed", "europe-pmc", "crossref", "biorxiv", "sequence-repositories"} {
		definition, _ := sourceDefinitionByID(id)
		config := definition.defaults
		config.MaxResults = 1
		if id != "sequence-repositories" {
			configs = append(configs, config)
			continue
		}
		for _, provider := range []string{"geo", "sra", "ena"} {
			configured := config
			configured.Provider = provider
			if provider == "sra" {
				configured.Query = `("single cell"[All Fields] OR "single-cell"[All Fields] OR "single nucleus"[All Fields]) AND (plant[All Fields] OR Arabidopsis[Organism])`
			}
			if provider == "ena" {
				configured.Query = `description="*single*" AND tax_tree(33090)`
			}
			configs = append(configs, configured)
		}
	}
	harvester := newRemoteHarvester()
	for _, config := range configs {
		name := config.ID
		if config.Provider != "" {
			name += "/" + config.Provider
		}
		t.Run(name, func(t *testing.T) {
			records, err := harvester.Harvest(context.Background(), config)
			if err != nil {
				t.Fatal(err)
			}
			if len(records) == 0 {
				t.Log("query succeeded and returned no matching records")
				return
			}
			if records[0].ExternalID == "" || records[0].Title == "" && records[0].Paper == nil {
				t.Errorf("record lacks display metadata: %+v", records[0])
			}
			t.Logf("received %d record(s), first external ID %s", len(records), records[0].ExternalID)
		})
	}
}

func stringPointer(value string) *string { return &value }
