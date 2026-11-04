package library

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/ledongthuc/pdf"
)

// maxFulltextTextBytes bounds what one paper's text may occupy in memory.
const maxFulltextTextBytes = 8 << 20

// PaperFulltextText returns the readable text of a paper's stored full text,
// or "" when the paper has no full-text file. A PDF is read page by page; a
// plain-text or Markdown file is read as is. A file that cannot be read is
// reported, since a screening decision should not silently rest on less.
func (s *Store) PaperFulltextText(ctx context.Context, paperID string) (string, error) {
	_, text, err := s.PaperFulltextSource(ctx, paperID)
	return text, err
}

// PaperFulltextSource identifies the exact attachment used as extraction evidence.
func (s *Store) PaperFulltextSource(ctx context.Context, paperID string) (PaperFile, string, error) {
	file, pages, err := s.PaperFulltextPages(ctx, paperID)
	return file, joinPages(pages), err
}

// PaperFulltextPages returns the stored full text page by page, so a quoted
// passage can be traced back to the PDF page it sits on. A plain-text file is
// one page. Without a full-text file every value is empty. The text comes
// from the fulltext_text cache, extracted on first use.
func (s *Store) PaperFulltextPages(ctx context.Context, paperID string) (PaperFile, []string, error) {
	files, err := s.fulltextFiles(ctx)
	if err != nil {
		return PaperFile{}, nil, err
	}
	file, ok := files[paperID]
	if !ok {
		return PaperFile{}, nil, nil
	}
	pages, err := s.fileTextPages(ctx, file.ID)
	return file, pages, err
}

// joinPages is the one way pages become the text a model reads, so a passage
// found in the text is found in exactly one page the same way.
func joinPages(pages []string) string {
	return strings.TrimSpace(strings.Join(pages, "\n\n"))
}

func fulltextFileText(path string) (string, error) {
	pages, err := fulltextFilePages(path)
	return joinPages(pages), err
}

func fulltextFilePages(path string) ([]string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return pdfPages(path)
	case ".txt", ".md", ".markdown", ".xml", ".html", ".htm":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read full text: %w", err)
		}
		if len(data) > maxFulltextTextBytes {
			data = data[:maxFulltextTextBytes]
		}
		return []string{string(data)}, nil
	}
	return nil, fmt.Errorf("全文文件 %s 不是可读取文本的格式", filepath.Base(path))
}

// pdfPages extracts the text layer of a PDF, one entry per page so page
// numbers are kept (an unreadable page is an empty entry). The parser panics
// on some malformed files, so the panic is turned into an error here.
func pdfPages(path string) (pages []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("extract pdf text from %s: %v", path, r)
			err = fmt.Errorf("无法读取 PDF 文本：%v", r)
		}
	}()
	file, reader, err := pdf.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开 PDF：%w", err)
	}
	defer file.Close()
	total := 0
	for i := 1; i <= reader.NumPage(); i++ {
		content := ""
		if page := reader.Page(i); !page.V.IsNull() {
			if text, err := page.GetPlainText(nil); err == nil {
				content = text
			}
		}
		pages = append(pages, content)
		total += len(content)
		if total > maxFulltextTextBytes {
			break
		}
	}
	if strings.TrimSpace(joinPages(pages)) == "" {
		return nil, fmt.Errorf("PDF 没有可提取的文本层（可能是扫描件）")
	}
	return pages, nil
}

// LocatePage finds the 1-based page a quoted passage sits on, 0 when it is
// on none. Whitespace is ignored, as PDF extraction breaks lines freely; a
// passage that spans a page break or was shortened by the model is still
// placed by its opening words.
func LocatePage(pages []string, excerpt string) int {
	quote := strings.Join(strings.Fields(excerpt), " ")
	if quote == "" {
		return 0
	}
	normalized := make([]string, len(pages))
	for i, page := range pages {
		normalized[i] = strings.Join(strings.Fields(page), " ")
	}
	for _, needle := range locateNeedles(quote) {
		for i, page := range normalized {
			if strings.Contains(page, needle) {
				return i + 1
			}
		}
	}
	return 0
}

// locateNeedles returns the passage, then its opening words, then a shorter
// opening, so the first page containing any of them is the answer.
func locateNeedles(quote string) []string {
	runes := []rune(quote)
	needles := []string{quote}
	for _, limit := range []int{80, 40, 20} {
		if len(runes) > limit {
			needles = append(needles, strings.TrimSpace(string(runes[:limit])))
		}
	}
	return needles
}
