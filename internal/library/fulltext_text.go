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
	files, err := s.ListPaperFiles(ctx)
	if err != nil {
		return "", err
	}
	for _, file := range files {
		if file.PaperID != paperID || file.Type != "fulltext" {
			continue
		}
		path, err := s.PaperFilePath(ctx, file.ID)
		if err != nil {
			return "", err
		}
		return fulltextFileText(path)
	}
	return "", nil
}

func fulltextFileText(path string) (text string, err error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf":
		return pdfText(path)
	case ".txt", ".md", ".markdown", ".xml", ".html", ".htm":
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read full text: %w", err)
		}
		if len(data) > maxFulltextTextBytes {
			data = data[:maxFulltextTextBytes]
		}
		return string(data), nil
	}
	return "", fmt.Errorf("全文文件 %s 不是可读取文本的格式", filepath.Base(path))
}

// pdfText extracts the text layer of a PDF. The parser panics on some
// malformed files, so the panic is turned into an error here.
func pdfText(path string) (text string, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("extract pdf text from %s: %v", path, r)
			err = fmt.Errorf("无法读取 PDF 文本：%v", r)
		}
	}()
	file, reader, err := pdf.Open(path)
	if err != nil {
		return "", fmt.Errorf("打开 PDF：%w", err)
	}
	defer file.Close()
	var b strings.Builder
	for i := 1; i <= reader.NumPage(); i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		b.WriteString(content)
		b.WriteString("\n\n")
		if b.Len() > maxFulltextTextBytes {
			break
		}
	}
	text = strings.TrimSpace(b.String())
	if text == "" {
		return "", fmt.Errorf("PDF 没有可提取的文本层（可能是扫描件）")
	}
	return text, nil
}

