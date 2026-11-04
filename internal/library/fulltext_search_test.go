package library

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// writeTextPDF writes a small PDF with a text layer: one page per entry, each
// line drawn with the standard Helvetica font, so extraction runs on a real
// PDF without a binary fixture in the repository.
func writeTextPDF(t *testing.T, path string, pages ...[]string) {
	t.Helper()
	escape := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
	var objects []string
	kids := make([]string, 0, len(pages))
	for i := range pages {
		kids = append(kids, fmt.Sprintf("%d 0 R", 4+2*i))
	}
	objects = append(objects,
		"<< /Type /Catalog /Pages 2 0 R >>",
		fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(pages)),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	)
	for i, lines := range pages {
		contentObj := 5 + 2*i
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", contentObj))
		var content strings.Builder
		content.WriteString("BT /F1 12 Tf 72 720 Td 16 TL\n")
		for _, line := range lines {
			content.WriteString("(" + escape.Replace(line) + ") Tj T*\n")
		}
		content.WriteString("ET\n")
		objects = append(objects, fmt.Sprintf("<< /Length %d >>\nstream\n%sendstream", content.Len(), content.String()))
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&out, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSearchFulltextFindsPagesAndCachesText(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	store, paper := fileFixture(t, storage)
	other, err := store.AddPaper(ctx, Paper{Title: "A paper without full text"})
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "atlas.pdf")
	writeTextPDF(t, source,
		[]string{"Single-cell atlas of the root", "Abstract: We profiled 12,000 cells from Arabidopsis roots."},
		[]string{"Methods", "Libraries were prepared with 10x Genomics Chromium", "and sequenced on NovaSeq. Data are under GSE123456."},
	)
	file, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}

	result, err := store.SearchFulltext(ctx, FulltextSearchInput{Query: "10x   genomics CHROMIUM"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || len(result.Hits) != 1 {
		t.Fatalf("hits = %+v", result)
	}
	hit := result.Hits[0]
	if hit.PaperID != paper.ID || hit.Page != 2 || hit.Match != "phrase" || hit.FileID != file.ID || hit.Title != paper.Title {
		t.Fatalf("hit = %+v", hit)
	}
	if hit.Cite != "cite:"+paper.ID+"#page=2" {
		t.Fatalf("cite = %q", hit.Cite)
	}
	if !strings.Contains(hit.Snippet, "10x Genomics Chromium") || !strings.Contains(hit.Snippet, "GSE123456") {
		t.Fatalf("snippet = %q", hit.Snippet)
	}
	if result.SearchedPapers != 1 || result.PapersWithoutFulltext != 1 || len(result.Unreadable) != 0 {
		t.Fatalf("counts = %+v", result)
	}
	if result.Query != "10x genomics CHROMIUM" || result.HowToCite == "" {
		t.Fatalf("query/howToCite = %+v", result)
	}

	// Every word present on a page still counts, after the phrase hits.
	result, err = store.SearchFulltext(ctx, FulltextSearchInput{Query: "GSE123456 cells"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("words on different pages matched: %+v", result.Hits)
	}
	result, err = store.SearchFulltext(ctx, FulltextSearchInput{Query: "NovaSeq Methods"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Match != "terms" || result.Hits[0].Page != 2 {
		t.Fatalf("terms hit = %+v", result.Hits)
	}
	// The scope limits which papers are read.
	result, err = store.SearchFulltext(ctx, FulltextSearchInput{Query: "roots", PaperIDs: []string{other.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if result.SearchedPapers != 0 || result.PapersWithoutFulltext != 1 || len(result.Hits) != 0 {
		t.Fatalf("scoped search = %+v", result)
	}
	if _, err := store.SearchFulltext(ctx, FulltextSearchInput{Query: "   "}); err == nil {
		t.Fatal("blank query accepted")
	}

	// The text was cached under the file's size and modification time.
	var cached int
	if err := store.db.QueryRow(`SELECT count(*) FROM fulltext_text WHERE resource_id = ? AND error = ''`, file.ID).Scan(&cached); err != nil || cached != 1 {
		t.Fatalf("cache rows = %d, %v", cached, err)
	}
	// A replaced file is read again; the stale text is not served.
	stored := filepath.Join(storage, filepath.FromSlash(file.LocalPath))
	writeTextPDF(t, stored, []string{"Entirely new content about leaves"})
	if err := os.Chtimes(stored, time.Now(), time.Now().Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	result, err = store.SearchFulltext(ctx, FulltextSearchInput{Query: "chromium"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 0 {
		t.Fatalf("stale cache served: %+v", result.Hits)
	}
	result, err = store.SearchFulltext(ctx, FulltextSearchInput{Query: "about leaves"})
	if err != nil || len(result.Hits) != 1 || result.Hits[0].Page != 1 {
		t.Fatalf("new content: %+v, %v", result, err)
	}
	// Removing the file removes its cached text with it.
	if err := store.RemovePaperFile(ctx, file.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM fulltext_text`).Scan(&cached); err != nil || cached != 0 {
		t.Fatalf("cache rows after removal = %d, %v", cached, err)
	}
}

func TestSearchFulltextRemembersUnreadableFiles(t *testing.T) {
	ctx := context.Background()
	store, paper := fileFixture(t, t.TempDir())
	source := writeSource(t, "scan.pdf", "%PDF-1.4 not really a pdf")
	file, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.SearchFulltext(ctx, FulltextSearchInput{Query: "anything"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Unreadable) != 1 || result.Unreadable[0].FileID != file.ID || result.Unreadable[0].Error == "" || result.SearchedPapers != 0 {
		t.Fatalf("unreadable = %+v", result)
	}
	var message string
	if err := store.db.QueryRow(`SELECT error FROM fulltext_text WHERE resource_id = ?`, file.ID).Scan(&message); err != nil || message == "" {
		t.Fatalf("cached error = %q, %v", message, err)
	}
	if _, err := store.ReadFulltext(ctx, FulltextReadInput{PaperID: paper.ID}); err == nil {
		t.Fatal("read of an unreadable file succeeded")
	}
}

func TestReadFulltextReturnsPagesInRuns(t *testing.T) {
	ctx := context.Background()
	store, paper := fileFixture(t, t.TempDir())
	if _, err := store.ReadFulltext(ctx, FulltextReadInput{PaperID: paper.ID}); err == nil {
		t.Fatal("read without a full text succeeded")
	}
	source := filepath.Join(t.TempDir(), "long.pdf")
	var pages [][]string
	for i := 1; i <= 10; i++ {
		pages = append(pages, []string{fmt.Sprintf("Page %d of the paper.", i)})
	}
	writeTextPDF(t, source, pages...)
	file, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	result, err := store.ReadFulltext(ctx, FulltextReadInput{PaperID: paper.ID})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalPages != 10 || len(result.Pages) != fulltextReadMaxPages || result.Pages[0].Page != 1 || result.Pages[7].Page != 8 || result.FileID != file.ID {
		t.Fatalf("default read = %+v", result)
	}
	if !strings.Contains(result.Pages[2].Text, "Page 3 of the paper.") || result.Pages[2].Cite != CiteLink(paper.ID, 3) {
		t.Fatalf("page 3 = %+v", result.Pages[2])
	}
	result, err = store.ReadFulltext(ctx, FulltextReadInput{PaperID: paper.ID, FromPage: 9, ToPage: 30})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pages) != 2 || result.Pages[0].Page != 9 || result.Pages[1].Page != 10 {
		t.Fatalf("tail read = %+v", result.Pages)
	}
	result, err = store.ReadFulltext(ctx, FulltextReadInput{PaperID: paper.ID, FromPage: 11})
	if err != nil || len(result.Pages) != 0 {
		t.Fatalf("read past the end = %+v, %v", result, err)
	}
	// Screening reads the same cached text.
	_, cached, err := store.PaperFulltextPages(ctx, paper.ID)
	if err != nil || len(cached) != 10 {
		t.Fatalf("PaperFulltextPages = %d pages, %v", len(cached), err)
	}
}

func TestSnippetAround(t *testing.T) {
	text := strings.Repeat("word ", 60) + "The Match is here. " + strings.Repeat("tail ", 60)
	lower := strings.ToLower(text)
	at := strings.Index(lower, "the match")
	snippet := snippetAround(text, lower, at, len("the match"))
	if !strings.HasPrefix(snippet, "…") || !strings.HasSuffix(snippet, "…") || !strings.Contains(snippet, "The Match is here.") {
		t.Fatalf("snippet = %q", snippet)
	}
	if strings.Contains(snippet, "…ord") || strings.Contains(snippet, "tai…") {
		t.Fatalf("snippet cuts inside a word: %q", snippet)
	}
	// Non-ASCII text keeps its original case and characters.
	text = "Vorwort: Die Zellen der Wurzel — İstanbul study. Ende."
	lower = strings.ToLower(text)
	at = strings.Index(lower, "wurzel")
	if snippet := snippetAround(text, lower, at, len("wurzel")); snippet != text {
		t.Fatalf("short snippet = %q", snippet)
	}
	at = strings.Index(lower, "ende")
	if snippet := snippetAround(text, lower, at, len("ende")); !strings.HasSuffix(snippet, "Ende.") {
		t.Fatalf("snippet after a multibyte fold = %q", snippet)
	}
}

func TestMCPFulltextToolsAreReadOnly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, paper := fileFixture(t, t.TempDir())
	source := filepath.Join(t.TempDir(), "atlas.pdf")
	writeTextPDF(t, source, []string{"Protoplasts were isolated from roots and loaded on a 10x Chromium chip."})
	if _, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext"); err != nil {
		t.Fatal(err)
	}
	server, err := StartMCP(store, "127.0.0.1:0", testMCPToken, noCredentials)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close(ctx)
	client := mcp.NewClient(&mcp.Implementation{Name: "bibliocell-fulltext-test", Version: "1"}, nil)
	// No bearer token: the tools read, so they are open to unauthenticated clients.
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.Endpoint}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	decode := func(result *mcp.CallToolResult, into any) {
		t.Helper()
		data, _ := json.Marshal(result.StructuredContent)
		if err := json.Unmarshal(data, into); err != nil {
			t.Fatal(err)
		}
	}
	found, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_fulltext", Arguments: map[string]any{"query": "chromium chip"}})
	if err != nil || found.IsError {
		t.Fatalf("search_fulltext: %v %+v", err, found)
	}
	var search FulltextSearchResult
	decode(found, &search)
	if len(search.Hits) != 1 || search.Hits[0].Cite != CiteLink(paper.ID, 1) || search.HowToCite == "" {
		t.Fatalf("search result = %+v", search)
	}
	read, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "read_fulltext", Arguments: map[string]any{"paperId": paper.ID}})
	if err != nil || read.IsError {
		t.Fatalf("read_fulltext: %v %+v", err, read)
	}
	var pages FulltextReadResult
	decode(read, &pages)
	if pages.TotalPages != 1 || len(pages.Pages) != 1 || !strings.Contains(pages.Pages[0].Text, "Protoplasts were isolated") {
		t.Fatalf("read result = %+v", pages)
	}
	described, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "describe_library", Arguments: map[string]any{}})
	if err != nil || described.IsError {
		t.Fatalf("describe_library: %v %+v", err, described)
	}
	var description describeLibraryOutput
	decode(described, &description)
	if !strings.Contains(strings.Join(description.AvailableTools, " "), "search_fulltext read_fulltext") {
		t.Fatalf("available tools = %v", description.AvailableTools)
	}
}
