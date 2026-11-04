package library

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Full-text search over the stored papers, for an assistant that must say
// where something is written. Every hit names a page and carries a cite link,
// so the answer can quote the passage and the window can open the PDF on it.

const (
	fulltextSearchDefaultLimit = 20
	fulltextSearchMaxLimit     = 100
	fulltextReadMaxPages       = 8
	// snippetRunes is how much of the page shows on each side of a match.
	snippetRunes = 160
)

// HowToCite tells a model how to quote a page so the window can open it. It
// travels with every search and read result, next to the cite links it uses.
const HowToCite = `Quote a passage as a Markdown link: [text](cite:<paperId>#page=<page> "verbatim sentence from that page"). The link text may be the sentence or your own summary of it; the quoted title must be one sentence copied verbatim from the page (20-300 characters, not translated or rephrased), because the desktop app opens the PDF on that page and highlights exactly that sentence. The cite field of a hit is the link target.`

// FulltextSearchInput is what search_fulltext takes.
type FulltextSearchInput struct {
	Query    string   `json:"query" jsonschema:"Phrase or words to find in stored full texts; matched case-insensitively with whitespace collapsed. A page holding the whole phrase ranks first, then pages holding every word"`
	PaperIDs []string `json:"paperIds,omitempty" jsonschema:"Only search these papers; empty searches every paper with a stored full text"`
	Limit    int      `json:"limit,omitempty" jsonschema:"Maximum hits to return, 1 to 100; default 20"`
}

// FulltextHit is one page of one paper where the query appears.
type FulltextHit struct {
	PaperID string `json:"paperId"`
	Title   string `json:"title"`
	Year    *int   `json:"year,omitempty"`
	FileID  string `json:"fileId"`
	Page    int    `json:"page"`
	// Match is phrase when the page holds the query as written, terms when it
	// holds every word somewhere.
	Match string `json:"match"`
	// Snippet is the passage around the first occurrence, verbatim from the
	// extracted text, so a sentence can be quoted from it.
	Snippet string `json:"snippet"`
	// Cite is the link target for quoting this page: cite:<paperId>#page=<page>.
	Cite string `json:"cite"`
}

// FulltextProblem names a stored full text that could not be read.
type FulltextProblem struct {
	PaperID string `json:"paperId"`
	FileID  string `json:"fileId"`
	Error   string `json:"error"`
}

// FulltextSearchResult is what search_fulltext returns.
type FulltextSearchResult struct {
	Query string        `json:"query"`
	Hits  []FulltextHit `json:"hits"`
	// Total counts every matching page, before the limit.
	Total int `json:"total"`
	// SearchedPapers counts papers whose full text was read.
	SearchedPapers int `json:"searchedPapers"`
	// PapersWithoutFulltext counts papers in scope that have no stored full
	// text; only their abstract and metadata can speak for them.
	PapersWithoutFulltext int               `json:"papersWithoutFulltext"`
	Unreadable            []FulltextProblem `json:"unreadable,omitempty"`
	HowToCite             string            `json:"howToCite"`
}

// FulltextReadInput is what read_fulltext takes.
type FulltextReadInput struct {
	PaperID  string `json:"paperId" jsonschema:"Stable ID of the paper"`
	FromPage int    `json:"fromPage,omitempty" jsonschema:"First page to return, 1-based; default 1"`
	ToPage   int    `json:"toPage,omitempty" jsonschema:"Last page to return; at most 8 pages per call, default fromPage+7"`
}

// FulltextPage is one page of extracted text.
type FulltextPage struct {
	Page int    `json:"page"`
	Text string `json:"text"`
	Cite string `json:"cite"`
}

// FulltextReadResult is what read_fulltext returns.
type FulltextReadResult struct {
	PaperID    string         `json:"paperId"`
	Title      string         `json:"title"`
	FileID     string         `json:"fileId"`
	FileTitle  string         `json:"fileTitle"`
	TotalPages int            `json:"totalPages"`
	Pages      []FulltextPage `json:"pages"`
	HowToCite  string         `json:"howToCite"`
}

// CiteLink is the target of a citation of one page: cite:<paperId>#page=<page>.
func CiteLink(paperID string, page int) string {
	return fmt.Sprintf("cite:%s#page=%d", paperID, page)
}

// fulltextFiles maps each paper to its first stored full-text file.
func (s *Store) fulltextFiles(ctx context.Context) (map[string]PaperFile, error) {
	files, err := s.ListPaperFiles(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]PaperFile{}
	for _, file := range files {
		if file.Type != "fulltext" {
			continue
		}
		if _, seen := out[file.PaperID]; !seen {
			out[file.PaperID] = file
		}
	}
	return out, nil
}

// fileTextPages returns the text of a stored file page by page. The text is
// kept in fulltext_text under the file's size and modification time, so a
// file is parsed once until it changes. A file that cannot be read is
// remembered too: a scanned PDF must not be parsed again on every search.
func (s *Store) fileTextPages(ctx context.Context, fileID string) ([]string, error) {
	path, err := s.PaperFilePath(ctx, fileID)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read full text: %w", err)
	}
	size, mtime := info.Size(), info.ModTime().UTC().Format(time.RFC3339Nano)
	var pagesJSON, cachedError string
	err = s.db.QueryRowContext(ctx, `SELECT pages, error FROM fulltext_text WHERE resource_id = ? AND file_size = ? AND file_mtime = ?`,
		fileID, size, mtime).Scan(&pagesJSON, &cachedError)
	switch {
	case err == nil:
		if cachedError != "" {
			return nil, errors.New(cachedError)
		}
		var pages []string
		if err := json.Unmarshal([]byte(pagesJSON), &pages); err == nil {
			return pages, nil
		}
		// A row that does not decode is replaced by a fresh extraction.
	case !errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("read full text cache: %w", err)
	}
	pages, extractErr := fulltextFilePages(path)
	stored, message := pages, ""
	if extractErr != nil {
		stored, message = nil, extractErr.Error()
	}
	if stored == nil {
		stored = []string{}
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return pages, extractErr
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO fulltext_text(resource_id, file_size, file_mtime, pages, error, extracted_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(resource_id) DO UPDATE SET file_size = excluded.file_size, file_mtime = excluded.file_mtime,
			pages = excluded.pages, error = excluded.error, extracted_at = excluded.extracted_at`,
		fileID, size, mtime, string(data), message, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		// The text is still good; only the next read pays the extraction again.
		log.Printf("cache full text of %s: %v", fileID, err)
	}
	return pages, extractErr
}

// SearchFulltext finds the pages where a query appears across the stored full
// texts. Matching ignores case and runs of whitespace, as extraction breaks
// lines freely. Pages holding the whole phrase come first, then pages holding
// every word; within each group, library order then page order.
func (s *Store) SearchFulltext(ctx context.Context, input FulltextSearchInput) (FulltextSearchResult, error) {
	query := strings.Join(strings.Fields(input.Query), " ")
	if query == "" {
		return FulltextSearchResult{}, errors.New("query must not be blank")
	}
	limit := input.Limit
	if limit <= 0 {
		limit = fulltextSearchDefaultLimit
	}
	limit = min(limit, fulltextSearchMaxLimit)
	papers, err := s.ListPapers(ctx)
	if err != nil {
		return FulltextSearchResult{}, err
	}
	files, err := s.fulltextFiles(ctx)
	if err != nil {
		return FulltextSearchResult{}, err
	}
	wanted := map[string]bool{}
	for _, id := range input.PaperIDs {
		if id = strings.TrimSpace(id); id != "" {
			wanted[id] = true
		}
	}
	result := FulltextSearchResult{Query: query, Hits: []FulltextHit{}, HowToCite: HowToCite}
	phrase := strings.ToLower(query)
	terms := strings.Fields(phrase)
	var phraseHits, termHits []FulltextHit
	for _, paper := range papers {
		if len(wanted) > 0 && !wanted[paper.ID] {
			continue
		}
		file, ok := files[paper.ID]
		if !ok {
			result.PapersWithoutFulltext++
			continue
		}
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		pages, err := s.fileTextPages(ctx, file.ID)
		if err != nil {
			result.Unreadable = append(result.Unreadable, FulltextProblem{PaperID: paper.ID, FileID: file.ID, Error: err.Error()})
			continue
		}
		result.SearchedPapers++
		for i, page := range pages {
			text := strings.Join(strings.Fields(page), " ")
			if text == "" {
				continue
			}
			lower := strings.ToLower(text)
			hit := FulltextHit{PaperID: paper.ID, Title: paper.Title, Year: paper.Year, FileID: file.ID, Page: i + 1, Cite: CiteLink(paper.ID, i+1)}
			if at := strings.Index(lower, phrase); at >= 0 {
				hit.Match, hit.Snippet = "phrase", snippetAround(text, lower, at, len(phrase))
				phraseHits = append(phraseHits, hit)
			} else if at, length, ok := firstOfAll(lower, terms); ok {
				hit.Match, hit.Snippet = "terms", snippetAround(text, lower, at, length)
				termHits = append(termHits, hit)
			}
		}
	}
	all := append(phraseHits, termHits...)
	result.Total = len(all)
	if len(all) > limit {
		all = all[:limit]
	}
	result.Hits = all
	return result, nil
}

// firstOfAll reports where the earliest of the terms sits when the text holds
// every one of them; a single term was already tried as the phrase.
func firstOfAll(lower string, terms []string) (at, length int, ok bool) {
	if len(terms) < 2 {
		return 0, 0, false
	}
	at = -1
	for _, term := range terms {
		i := strings.Index(lower, term)
		if i < 0 {
			return 0, 0, false
		}
		if at < 0 || i < at {
			at, length = i, len(term)
		}
	}
	return at, length, true
}

// snippetAround returns the text around a match, with an ellipsis at each end
// that was cut. at and length are byte offsets in lower, whose runes line up
// one to one with text's: strings.ToLower maps rune by rune.
func snippetAround(text, lower string, at, length int) string {
	start := utf8.RuneCountInString(lower[:at])
	count := utf8.RuneCountInString(lower[at : at+length])
	runes := []rune(text)
	from := max(0, start-snippetRunes)
	to := min(len(runes), start+count+snippetRunes)
	// Start and end on word boundaries where the cut would land inside a word.
	for from > 0 && from < start && runes[from-1] != ' ' {
		from++
	}
	for to < len(runes) && to > start+count && runes[to] != ' ' {
		to++
	}
	out := string(runes[from:to])
	if from > 0 {
		out = "…" + out
	}
	if to < len(runes) {
		out += "…"
	}
	return out
}

// ReadFulltext returns a run of pages of one paper's stored full text, so a
// model can check the context of a hit and copy a sentence exactly.
func (s *Store) ReadFulltext(ctx context.Context, input FulltextReadInput) (FulltextReadResult, error) {
	paper, err := s.GetPaper(ctx, input.PaperID)
	if err != nil {
		return FulltextReadResult{}, err
	}
	files, err := s.fulltextFiles(ctx)
	if err != nil {
		return FulltextReadResult{}, err
	}
	file, ok := files[paper.ID]
	if !ok {
		return FulltextReadResult{}, errors.New("这篇文献没有已保存的全文；fetch_fulltext 可以查找开放全文，或由人在详情页导入 PDF")
	}
	pages, err := s.fileTextPages(ctx, file.ID)
	if err != nil {
		return FulltextReadResult{}, err
	}
	result := FulltextReadResult{PaperID: paper.ID, Title: paper.Title, FileID: file.ID, FileTitle: file.Title, TotalPages: len(pages), Pages: []FulltextPage{}, HowToCite: HowToCite}
	from := input.FromPage
	if from <= 0 {
		from = 1
	}
	to := input.ToPage
	if to <= 0 || to > from+fulltextReadMaxPages-1 {
		to = from + fulltextReadMaxPages - 1
	}
	to = min(to, len(pages))
	for page := from; page <= to; page++ {
		result.Pages = append(result.Pages, FulltextPage{Page: page, Text: strings.TrimSpace(pages[page-1]), Cite: CiteLink(paper.ID, page)})
	}
	return result, nil
}
