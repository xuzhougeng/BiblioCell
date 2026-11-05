package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"bibliocell/internal/library"
)

// The window's paperfile scheme serves a stored file whole, whatever the
// request asks for: on Windows the scheme is an http URL, and PDF.js would
// otherwise read the file in byte ranges that the WebView2 bridge cannot
// answer as such.
func TestServePaperFileSendsTheWholeFileWithoutRanges(t *testing.T) {
	storage := t.TempDir()
	store, err := library.Open(filepath.Join(storage, "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	paper, err := store.AddPaper(ctx, library.Paper{Title: "A paper with a PDF"})
	if err != nil {
		t.Fatal(err)
	}
	content := "%PDF-1.4\n" + strings.Repeat("x", 300_000) + "\n%%EOF\n"
	source := filepath.Join(t.TempDir(), "paper.pdf")
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{store: store}

	request := httptest.NewRequest(http.MethodGet, "paperfile://localhost/"+file.ID, nil)
	request.Header.Set("Range", "bytes=0-65535")
	recorder := httptest.NewRecorder()
	service.servePaperFile(recorder, request)
	response := recorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a range request", response.StatusCode)
	}
	if got := recorder.Body.String(); got != content {
		t.Fatalf("body has %d bytes, want the whole file of %d", len(got), len(content))
	}
	if response.Header.Get("Accept-Ranges") != "none" || response.Header.Get("Content-Range") != "" {
		t.Fatalf("headers advertise ranges: %v", response.Header)
	}
	if response.Header.Get("Content-Type") != "application/pdf" || response.Header.Get("Content-Length") != strconv.Itoa(len(content)) || response.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers = %v", response.Header)
	}

	head := httptest.NewRecorder()
	service.servePaperFile(head, httptest.NewRequest(http.MethodHead, "paperfile://localhost/"+file.ID, nil))
	if head.Code != http.StatusOK || head.Body.Len() != 0 || head.Header().Get("Content-Length") != strconv.Itoa(len(content)) {
		t.Fatalf("HEAD = %d, %d body bytes, %v", head.Code, head.Body.Len(), head.Header())
	}

	missing := httptest.NewRecorder()
	service.servePaperFile(missing, httptest.NewRequest(http.MethodGet, "paperfile://localhost/R-nope", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown file = %d, want 404", missing.Code)
	}
	if err := os.Remove(filepath.Join(storage, filepath.FromSlash(file.LocalPath))); err != nil {
		t.Fatal(err)
	}
	gone := httptest.NewRecorder()
	service.servePaperFile(gone, httptest.NewRequest(http.MethodGet, "paperfile://localhost/"+file.ID, nil))
	if gone.Code != http.StatusNotFound {
		t.Fatalf("deleted file = %d, want 404", gone.Code)
	}
}
