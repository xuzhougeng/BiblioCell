package library

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fileFixture opens a store with one real paper and writes a source file.
func fileFixture(t *testing.T, storage string) (*Store, Paper) {
	t.Helper()
	store, err := Open(filepath.Join(storage, "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	paper, err := store.AddPaper(context.Background(), Paper{Title: "A paper with files"})
	if err != nil {
		t.Fatal(err)
	}
	return store, paper
}

func writeSource(t *testing.T, name, content string) string {
	t.Helper()
	source := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(source, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return source
}

func storedNames(t *testing.T, storage string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(storage, PapersDir))
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func TestImportPaperFile(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	store, paper := fileFixture(t, storage)
	source := writeSource(t, "图 1 (最终版).PDF", "pdf content")

	file, err := store.ImportPaperFile(ctx, paper.ID, source, "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if file.Title != "图 1 (最终版).PDF" || file.PaperID != paper.ID || file.SizeBytes != int64(len("pdf content")) {
		t.Fatalf("imported file = %+v", file)
	}
	// Paths stay relative and ASCII whatever the original name was.
	if file.LocalPath != "papers/"+file.ID+".pdf" {
		t.Fatalf("LocalPath = %q", file.LocalPath)
	}
	if got, err := os.ReadFile(filepath.Join(storage, "papers", file.ID+".pdf")); err != nil || string(got) != "pdf content" {
		t.Fatalf("stored file = %q, %v", got, err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "pdf content" {
		t.Fatalf("source changed: %q, %v", got, err)
	}
	if got, err := store.GetPaper(ctx, paper.ID); err != nil || got.Fulltext != "stored" {
		t.Fatalf("paper after import = %+v, %v", got, err)
	}

	again, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "renamed.pdf", "pdf content"), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if again.ID != file.ID || len(storedNames(t, storage)) != 1 {
		t.Fatalf("duplicate imported again: %+v, %v", again, storedNames(t, storage))
	}

	odd, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "table.final draft", "table"), "supplement")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(odd.LocalPath, " ") || odd.Type != "supplement" {
		t.Fatalf("unsafe extension kept: %+v", odd)
	}
	files, err := store.ListPaperFiles(ctx)
	if err != nil || len(files) != 2 {
		t.Fatalf("ListPaperFiles() = %+v, %v", files, err)
	}
}

func TestImportPaperFileRejectsWithoutLeavingFiles(t *testing.T) {
	ctx := context.Background()
	storage := t.TempDir()
	store, paper := fileFixture(t, storage)
	source := writeSource(t, "paper.pdf", "pdf content")

	if _, err := store.ImportPaperFile(ctx, "P-missing", source, "fulltext"); !errors.Is(err, ErrPaperNotFound) {
		t.Errorf("unknown paper error = %v", err)
	}
	if _, err := store.ImportPaperFile(ctx, paper.ID, source, "raw_reads"); err == nil {
		t.Error("unsupported type accepted")
	}
	if _, err := store.ImportPaperFile(ctx, paper.ID, filepath.Join(storage, "missing.pdf"), "fulltext"); err == nil {
		t.Error("missing source accepted")
	}
	if _, err := store.ImportPaperFile(ctx, paper.ID, t.TempDir(), "fulltext"); err == nil {
		t.Error("directory accepted")
	}
	if names := storedNames(t, storage); len(names) != 0 {
		t.Errorf("rejected imports left files: %v", names)
	}
}

func TestPaperFileSurvivesFolderCopy(t *testing.T) {
	ctx := context.Background()
	original := t.TempDir()
	store, paper := fileFixture(t, original)
	file, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "paper.pdf", "pdf content"), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	copied := filepath.Join(t.TempDir(), "moved")
	if err := os.CopyFS(copied, os.DirFS(original)); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(original); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(filepath.Join(copied, "bibliocell.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	resolved, err := reopened.PaperFilePath(ctx, file.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resolved, copied+string(os.PathSeparator)) {
		t.Fatalf("resolved %q outside the copied folder %q", resolved, copied)
	}
	if got, err := os.ReadFile(resolved); err != nil || !bytes.Equal(got, []byte("pdf content")) {
		t.Fatalf("file after copy = %q, %v", got, err)
	}
	if got, err := reopened.GetPaper(ctx, paper.ID); err != nil || got.Fulltext != "stored" {
		t.Fatalf("paper after copy = %+v, %v", got, err)
	}
}

func TestPaperFilePathStaysInsideStorageFolder(t *testing.T) {
	ctx := context.Background()
	store, _ := fileFixture(t, t.TempDir())
	for id, unsafe := range map[string]string{"R-parent": "../secret.pdf", "R-absolute": "/etc/passwd"} {
		if _, err := store.db.ExecContext(ctx,
			"INSERT INTO resources (id, resource_type, title, local_path, local_state) VALUES (?, 'fulltext', 'x', ?, 'stored')", id, unsafe,
		); err != nil {
			t.Fatal(err)
		}
		if resolved, err := store.PaperFilePath(ctx, id); err == nil {
			t.Errorf("%s resolved to %q", unsafe, resolved)
		}
	}
	if _, err := store.PaperFilePath(ctx, "R-missing"); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("missing file error = %v", err)
	}
}

func TestRemovePaperFile(t *testing.T) {
	ctx := context.Background()
	store, paper := fileFixture(t, t.TempDir())
	file, err := store.ImportPaperFile(ctx, paper.ID, writeSource(t, "paper.pdf", "pdf content"), "fulltext")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RemovePaperFile(ctx, file.ID); err != nil {
		t.Fatal(err)
	}
	if files, err := store.ListPaperFiles(ctx); err != nil || len(files) != 0 {
		t.Fatalf("files after removal = %+v, %v", files, err)
	}
	if got, err := store.GetPaper(ctx, paper.ID); err != nil || got.Fulltext == "stored" {
		t.Fatalf("paper after removal = %+v, %v", got, err)
	}
	if err := store.RemovePaperFile(ctx, file.ID); !errors.Is(err, ErrFileNotFound) {
		t.Errorf("second removal error = %v", err)
	}
}
