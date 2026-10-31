package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"

	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

// Library exposes local library operations to the frontend.
type Library struct {
	store       *library.Store
	settings    *settings.Store
	mcpEndpoint string
	mcpToken    string
}

// SettingsSaveResult tells the frontend which configuration changes need a restart.
type SettingsSaveResult struct {
	Settings        settings.Config `json:"settings"`
	RestartRequired bool            `json:"restartRequired"`
	StorageChanged  bool            `json:"storageChanged"`
	MCPTokenChanged bool            `json:"mcpTokenChanged"`
}

// MCPInfo describes the optional local AI integration.
type MCPInfo struct {
	Enabled   bool     `json:"enabled"`
	Endpoint  string   `json:"endpoint"`
	Token     string   `json:"token"`
	Tools     []string `json:"tools"`
	Resources []string `json:"resources"`
}

// Status returns the state of the Go-backed SQLite library.
func (l *Library) Status() (library.Status, error) {
	if l.store == nil {
		return library.Status{}, errors.New("library database is not ready")
	}
	return l.store.Status()
}

// ListPapers returns records from the real database, never Demo records.
func (l *Library) ListPapers() ([]library.Paper, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListPapers(context.Background())
}

// SavePaperEdits stores what a person changed and returns the titles of papers
// left unsaved because another writer changed the same fields.
func (l *Library) SavePaperEdits(edits []library.PaperEdit) ([]string, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.SavePaperEdits(context.Background(), edits)
}

// ListPaperFiles returns the files stored for real papers.
func (l *Library) ListPaperFiles() ([]library.PaperFile, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListPaperFiles(context.Background())
}

// ImportPaperFiles asks for files and copies them into the storage folder.
func (l *Library) ImportPaperFiles(ctx context.Context, paperID, resourceType string) ([]library.PaperFile, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:   mygo.CallerWindow(ctx),
		Title:    "选择要导入的文献文件",
		Multiple: true,
	})
	if err != nil {
		return nil, err
	}
	files := []library.PaperFile{}
	for _, path := range paths {
		file, err := l.store.ImportPaperFile(context.Background(), paperID, path, resourceType)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		files = append(files, file)
	}
	return files, nil
}

// OpenPaperFile opens a stored file with its default application.
func (l *Library) OpenPaperFile(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	path, err := l.store.PaperFilePath(context.Background(), id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("file is missing from the storage folder: %s", filepath.Base(path))
	}
	return mygo.Shell.OpenPath(path)
}

// RemovePaperFile moves a stored file to the trash and deletes its record. A
// file that is already gone must not keep the record from being removed.
func (l *Library) RemovePaperFile(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	ctx := context.Background()
	path, err := l.store.PaperFilePath(ctx, id)
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		if err := mygo.Shell.TrashItem(path); err != nil {
			return fmt.Errorf("move file to trash: %w", err)
		}
	}
	return l.store.RemovePaperFile(ctx, id)
}

// ListDatasets returns real datasets with repository and paper associations.
func (l *Library) ListDatasets() ([]library.Dataset, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListDatasets(context.Background())
}

// SaveDatasetEdits stores what a person changed and returns the titles of
// datasets left unsaved because another writer changed the same fields.
func (l *Library) SaveDatasetEdits(edits []library.DatasetEdit) ([]string, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.SaveDatasetEdits(context.Background(), edits)
}

// ListEvidence returns every claim agents recorded, for the review views.
func (l *Library) ListEvidence() ([]library.Evidence, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListEvidence(context.Background(), "", "")
}

// ReviewEvidence records a person's verdict on a claim.
func (l *Library) ReviewEvidence(id, status string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.ReviewEvidence(context.Background(), id, status)
}

// VerifyDatasetLink records a person's check of a paper-dataset link.
func (l *Library) VerifyDatasetLink(paperID, datasetID, role string, verified bool) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.VerifyDatasetLink(context.Background(), paperID, datasetID, role, verified)
}

// VerifyDatasetRepository records a person's check of a repository record.
func (l *Library) VerifyDatasetRepository(datasetID, recordID string, verified bool) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.VerifyDatasetRepository(context.Background(), datasetID, recordID, verified)
}

// ListSources returns saved connector configurations and their latest runs.
func (l *Library) ListSources() ([]library.SourceStatus, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListSources(context.Background())
}

// SaveSourceConfig persists one connector without starting a network request.
func (l *Library) SaveSourceConfig(config library.SourceConfig) (library.SourceStatus, error) {
	if l.store == nil {
		return library.SourceStatus{}, errors.New("library database is not ready")
	}
	return l.store.SaveSourceConfig(context.Background(), config)
}

// SyncSource fetches one enabled public source and returns the refreshed papers.
func (l *Library) SyncSource(id string) (library.SyncResult, error) {
	if l.store == nil {
		return library.SyncResult{}, errors.New("library database is not ready")
	}
	return l.store.SyncSource(context.Background(), id)
}

// GetSettings returns persisted desktop preferences.
func (l *Library) GetSettings() (settings.Config, error) {
	if l.settings == nil {
		return settings.Config{}, errors.New("application settings are not ready")
	}
	return l.settings.Get(), nil
}

// SaveSettings persists preferences without moving data or reopening the
// database. A new storage folder is opened first: the app cannot start on a
// folder it cannot open, so a bad one must never reach the settings file.
func (l *Library) SaveSettings(config settings.Config) (SettingsSaveResult, error) {
	if l.settings == nil {
		return SettingsSaveResult{}, errors.New("application settings are not ready")
	}
	previous := l.settings.Get()
	if err := settings.ValidateMCPToken(config.MCPToken); err != nil {
		return SettingsSaveResult{}, err
	}
	config.StoragePath = filepath.Clean(strings.TrimSpace(config.StoragePath))
	if err := settings.Validate(config); err != nil {
		return SettingsSaveResult{}, err
	}
	if config.StoragePath != previous.StoragePath {
		candidate, err := library.Open(config.DatabasePath())
		if err != nil {
			return SettingsSaveResult{}, fmt.Errorf("cannot use this folder: %w", err)
		}
		if err := candidate.Close(); err != nil {
			return SettingsSaveResult{}, fmt.Errorf("cannot use this folder: %w", err)
		}
	}
	if err := l.settings.Save(config); err != nil {
		return SettingsSaveResult{}, err
	}
	saved := l.settings.Get()
	storageChanged := previous.StoragePath != saved.StoragePath
	mcpTokenChanged := previous.MCPToken != saved.MCPToken
	return SettingsSaveResult{
		Settings:        saved,
		RestartRequired: storageChanged || mcpTokenChanged,
		StorageChanged:  storageChanged,
		MCPTokenChanged: mcpTokenChanged,
	}, nil
}

// Restart relaunches the app, which then opens the saved storage folder.
func (l *Library) Restart() {
	mygo.App.Relaunch()
}

// ChooseStoragePath opens a native directory picker for the storage folder.
func (l *Library) ChooseStoragePath(ctx context.Context) (string, error) {
	if l.settings == nil {
		return "", errors.New("application settings are not ready")
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       "选择数据存放文件夹",
		Directory:   true,
		DefaultPath: l.settings.Get().StoragePath,
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

// MCPInfo returns the loopback endpoint external AI clients can use.
func (l *Library) MCPInfo() MCPInfo {
	return MCPInfo{
		Enabled: l.mcpEndpoint != "", Endpoint: l.mcpEndpoint, Token: l.mcpToken,
		Tools:     []string{"describe_library", "library_status", "list_papers", "get_paper", "add_paper", "import_papers", "update_paper"},
		Resources: []string{"bibliocell://schema/paper", "bibliocell://schema/sql"},
	}
}
