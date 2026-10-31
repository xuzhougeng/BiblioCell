package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// PaperFile is a local file attached to a paper. LocalPath is relative to the
// storage folder and uses forward slashes, so the folder can be copied anywhere.
type PaperFile struct {
	ID        string `json:"id"`
	PaperID   string `json:"paperId"`
	Type      string `json:"type"`
	Title     string `json:"title"`
	LocalPath string `json:"localPath"`
	SizeBytes int64  `json:"sizeBytes"`
	AddedAt   string `json:"addedAt"`
}

// ErrFileNotFound indicates that no stored file has the requested ID.
var ErrFileNotFound = errors.New("paper file not found")

var fileExtension = regexp.MustCompile(`^\.[a-z0-9]{1,16}$`)

// paperFileSelect is the shared projection of paperFileFields; callers append
// their own conditions.
const paperFileSelect = `
		SELECT r.id, pr.paper_id, r.resource_type, r.title, r.local_path, coalesce(r.size_bytes, 0), r.added_at
		FROM resources r JOIN paper_resources pr ON pr.resource_id = r.id
		WHERE r.local_state <> 'none'`

func paperFileFields(f *PaperFile) []any {
	return []any{&f.ID, &f.PaperID, &f.Type, &f.Title, &f.LocalPath, &f.SizeBytes, &f.AddedAt}
}

// ListPaperFiles returns every stored paper file, oldest first.
func (s *Store) ListPaperFiles(ctx context.Context) ([]PaperFile, error) {
	rows, err := s.db.QueryContext(ctx, paperFileSelect+` ORDER BY r.added_at, r.id`)
	if err != nil {
		return nil, fmt.Errorf("list paper files: %w", err)
	}
	defer rows.Close()

	files := []PaperFile{}
	for rows.Next() {
		var f PaperFile
		if err := rows.Scan(paperFileFields(&f)...); err != nil {
			return nil, fmt.Errorf("read paper file: %w", err)
		}
		files = append(files, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list paper files: %w", err)
	}
	return files, nil
}

// ImportPaperFile copies source into the storage folder and attaches it to a
// paper; source stays untouched. The copy is named after its resource ID,
// because plain ASCII names survive moving the folder between file systems and
// operating systems; the original name is kept as the title. Importing the
// same content into a paper again returns the existing file.
func (s *Store) ImportPaperFile(ctx context.Context, paperID, source, resourceType string) (PaperFile, error) {
	in, err := os.Open(source)
	if err != nil {
		return PaperFile{}, fmt.Errorf("copy paper file: %w", err)
	}
	defer in.Close()
	return s.importPaperFile(ctx, paperID, in, filepath.Base(source), resourceType)
}

// AttachInboxFile moves a file an agent dropped into the storage folder's inbox
// onto a paper. The file is opened through os.Root, so neither .. nor a symbolic
// link reaches anything outside the inbox.
func (s *Store) AttachInboxFile(ctx context.Context, paperID, name, resourceType string) (PaperFile, error) {
	root, err := os.OpenRoot(s.inboxPath())
	if err != nil {
		return PaperFile{}, fmt.Errorf("open inbox: %w", err)
	}
	defer root.Close()
	name = filepath.FromSlash(name)
	in, err := root.Open(name)
	if err != nil {
		return PaperFile{}, fmt.Errorf("open inbox file: %w", err)
	}
	defer in.Close()
	file, err := s.importPaperFile(ctx, paperID, in, filepath.Base(name), resourceType)
	if err != nil {
		return PaperFile{}, err
	}
	in.Close()
	if err := root.Remove(name); err != nil {
		return PaperFile{}, fmt.Errorf("clear inbox file: %w", err)
	}
	return file, nil
}

func (s *Store) importPaperFile(ctx context.Context, paperID string, in io.Reader, name, resourceType string) (PaperFile, error) {
	if resourceType != "fulltext" && resourceType != "supplement" {
		return PaperFile{}, fmt.Errorf("unsupported file type %q", resourceType)
	}
	if _, err := s.GetPaper(ctx, paperID); err != nil {
		return PaperFile{}, err
	}
	id, err := newLocalID("R")
	if err != nil {
		return PaperFile{}, err
	}
	extension := strings.ToLower(filepath.Ext(name))
	if !fileExtension.MatchString(extension) {
		extension = ""
	}
	relative := path.Join(PapersDir, id+extension)
	target, err := s.storageFile(relative)
	if err != nil {
		return PaperFile{}, err
	}

	partial := target + ".part"
	defer os.Remove(partial)
	size, checksum, err := copyFile(in, partial)
	if err != nil {
		return PaperFile{}, fmt.Errorf("copy paper file: %w", err)
	}

	var existing PaperFile
	err = s.db.QueryRowContext(ctx, paperFileSelect+` AND pr.paper_id = ? AND r.checksum_value = ?`, paperID, checksum).
		Scan(paperFileFields(&existing)...)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return PaperFile{}, fmt.Errorf("check duplicate paper file: %w", err)
	}

	if err := os.Rename(partial, target); err != nil {
		return PaperFile{}, fmt.Errorf("store paper file: %w", err)
	}
	if err := s.insertPaperFile(ctx, paperID, id, resourceType, name, strings.TrimPrefix(extension, "."), relative, size, checksum); err != nil {
		_ = os.Remove(target)
		return PaperFile{}, err
	}
	var file PaperFile
	if err := s.db.QueryRowContext(ctx, paperFileSelect+` AND r.id = ?`, id).Scan(paperFileFields(&file)...); err != nil {
		return PaperFile{}, fmt.Errorf("read imported paper file: %w", err)
	}
	return file, nil
}

func (s *Store) insertPaperFile(ctx context.Context, paperID, id, resourceType, title, format, relative string, size int64, checksum string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin paper file import: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO resources (id, resource_type, title, format, local_path, local_state, size_bytes, checksum_algorithm, checksum_value)
		VALUES (?, ?, ?, ?, ?, 'stored', ?, 'sha256', ?)`,
		id, resourceType, title, nullableString(format), relative, size, checksum,
	); err != nil {
		return fmt.Errorf("save paper file: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO paper_resources (paper_id, resource_id) VALUES (?, ?)", paperID, id); err != nil {
		return fmt.Errorf("attach paper file: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit paper file import: %w", err)
	}
	return nil
}

// PaperFilePath resolves a stored file against the current storage folder.
func (s *Store) PaperFilePath(ctx context.Context, id string) (string, error) {
	var relative string
	err := s.db.QueryRowContext(ctx, "SELECT local_path FROM resources WHERE id = ? AND local_state <> 'none'", id).Scan(&relative)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrFileNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read paper file path: %w", err)
	}
	return s.storageFile(relative)
}

// RemovePaperFile deletes the file record and the evidence drawn from the file,
// which has lost its source. The file itself is the caller's to dispose of first:
// a failed removal then keeps the record and can be retried.
func (s *Store) RemovePaperFile(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin file removal: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, "DELETE FROM field_evidence WHERE source_resource_id = ?", id); err != nil {
		return fmt.Errorf("remove file evidence: %w", err)
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM resources WHERE id = ? AND local_state <> 'none'", id)
	if err != nil {
		return fmt.Errorf("remove paper file: %w", err)
	}
	if count, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("remove paper file: %w", err)
	} else if count == 0 {
		return ErrFileNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit file removal: %w", err)
	}
	return nil
}

// deletePaperFiles removes a paper's file records and returns where the stored
// copies are, for the caller to delete once its transaction commits.
func (s *Store) deletePaperFiles(ctx context.Context, tx *sql.Tx, paperID string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT r.id, coalesce(r.local_path, '') FROM resources r
		JOIN paper_resources pr ON pr.resource_id = r.id WHERE pr.paper_id = ?`, paperID)
	if err != nil {
		return nil, fmt.Errorf("list paper files: %w", err)
	}
	defer rows.Close()
	ids, paths := []string{}, []string{}
	for rows.Next() {
		var id, relative string
		if err := rows.Scan(&id, &relative); err != nil {
			return nil, fmt.Errorf("read paper file: %w", err)
		}
		ids = append(ids, id)
		if path, err := s.storageFile(relative); err == nil && relative != "" {
			paths = append(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list paper files: %w", err)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, "DELETE FROM field_evidence WHERE source_resource_id = ?", id); err != nil {
			return nil, fmt.Errorf("remove file evidence: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM resources WHERE id = ?", id); err != nil {
			return nil, fmt.Errorf("remove paper file: %w", err)
		}
	}
	return paths, nil
}

// storageFile turns a recorded relative path into a path on this machine and
// refuses anything that would leave the storage folder.
func (s *Store) storageFile(relative string) (string, error) {
	local := filepath.FromSlash(relative)
	if !filepath.IsLocal(local) {
		return "", fmt.Errorf("file path %q is outside the storage folder", relative)
	}
	return filepath.Join(filepath.Dir(s.path), local), nil
}

// copyFile writes in to a new target and returns its size and SHA-256.
func copyFile(in io.Reader, target string) (int64, string, error) {
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return 0, "", err
	}
	defer out.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(out, hash), in)
	if err != nil {
		return 0, "", err
	}
	if err := out.Sync(); err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
