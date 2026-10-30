package main

import (
	"context"
	"errors"
	"path/filepath"

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
	DatabaseChanged bool            `json:"databaseChanged"`
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

// SavePapers persists paper edits without deleting records added by MCP.
func (l *Library) SavePapers(papers []library.Paper) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.SavePapers(context.Background(), papers)
}

// ListDatasets returns real datasets with repository and paper associations.
func (l *Library) ListDatasets() ([]library.Dataset, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListDatasets(context.Background())
}

// SaveDatasets persists dataset edits while preserving unrelated associations.
func (l *Library) SaveDatasets(datasets []library.Dataset) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.SaveDatasets(context.Background(), datasets)
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

// SaveSettings persists preferences without moving or reopening the database.
func (l *Library) SaveSettings(config settings.Config) (SettingsSaveResult, error) {
	if l.settings == nil {
		return SettingsSaveResult{}, errors.New("application settings are not ready")
	}
	previous := l.settings.Get()
	if err := settings.ValidateMCPToken(config.MCPToken); err != nil {
		return SettingsSaveResult{}, err
	}
	if err := l.settings.Save(config); err != nil {
		return SettingsSaveResult{}, err
	}
	saved := l.settings.Get()
	databaseChanged := previous.DatabasePath != saved.DatabasePath
	mcpTokenChanged := previous.MCPToken != saved.MCPToken
	return SettingsSaveResult{
		Settings:        saved,
		RestartRequired: databaseChanged || mcpTokenChanged,
		DatabaseChanged: databaseChanged,
		MCPTokenChanged: mcpTokenChanged,
	}, nil
}

// ChooseDatabasePath opens a native directory picker and returns a SQLite path.
func (l *Library) ChooseDatabasePath(ctx context.Context) (string, error) {
	if l.settings == nil {
		return "", errors.New("application settings are not ready")
	}
	current := l.settings.Get().DatabasePath
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      mygo.CallerWindow(ctx),
		Title:       "选择数据库存放文件夹",
		Directory:   true,
		DefaultPath: filepath.Dir(current),
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return filepath.Join(paths[0], "bibliocell.sqlite"), nil
}

// MCPInfo returns the loopback endpoint external AI clients can use.
func (l *Library) MCPInfo() MCPInfo {
	return MCPInfo{
		Enabled: l.mcpEndpoint != "", Endpoint: l.mcpEndpoint, Token: l.mcpToken,
		Tools:     []string{"describe_library", "library_status", "list_papers", "get_paper", "add_paper", "import_papers", "update_paper"},
		Resources: []string{"bibliocell://schema/paper", "bibliocell://schema/sql"},
	}
}
