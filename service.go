package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"bibliocell/internal/agent"
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

// DeletePaper removes a paper and its stored attachments, preserving datasets.
func (l *Library) DeletePaper(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.DeletePaper(context.Background(), id)
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

// FetchFulltext stores an open-access PDF for a paper.
func (l *Library) FetchFulltext(paperID string) (library.FulltextResult, error) {
	if l.store == nil {
		return library.FulltextResult{}, errors.New("library database is not ready")
	}
	return l.store.FetchFulltext(context.Background(), paperID, sourceCredentials(l.settings))
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

// MarkPaperReviewed takes a paper a source update added out of review.
func (l *Library) MarkPaperReviewed(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.MarkPaperReviewed(context.Background(), id)
}

// MarkDatasetReviewed takes a dataset a source update added out of review.
func (l *Library) MarkDatasetReviewed(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.MarkDatasetReviewed(context.Background(), id)
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
	return l.store.SyncSource(context.Background(), id, sourceCredentials(l.settings))
}

// sourcesChanged tells the window a background update started or finished.
var sourcesChanged = mygo.NewEvent[library.SourceAutomation]("sourcesChanged")

// SourceAutomation returns the update schedule and the latest runs' results.
func (l *Library) SourceAutomation() (library.SourceAutomation, error) {
	if l.store == nil {
		return library.SourceAutomation{}, errors.New("library database is not ready")
	}
	return l.store.SourceAutomation(context.Background())
}

// SaveSourceAutomation persists the update schedule.
func (l *Library) SaveSourceAutomation(enabled bool, intervalHours int) (library.SourceAutomation, error) {
	if l.store == nil {
		return library.SourceAutomation{}, errors.New("library database is not ready")
	}
	return l.store.SaveSourceAutomation(context.Background(), enabled, intervalHours)
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
	info := MCPInfo{
		Enabled: l.mcpEndpoint != "", Endpoint: l.mcpEndpoint, Token: l.mcpToken,
		Resources: []string{"bibliocell://schema/paper", "bibliocell://schema/sql"},
	}
	for _, tool := range l.agentTools() {
		info.Tools = append(info.Tools, tool.Name)
	}
	return info
}

// agentTools lists the MCP tools, which the built-in agent uses as its own.
func (l *Library) agentTools() []agent.ToolDef {
	if l.store == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	local, err := library.ConnectLocalMCP(ctx, l.store, sourceCredentials(l.settings))
	if err != nil {
		return nil
	}
	defer local.Close()
	defs, err := agent.MCPTools{Session: local.Session}.List(ctx)
	if err != nil {
		return nil
	}
	return defs
}

// AgentConversation is a saved exchange with the built-in assistant.
type AgentConversation struct {
	ID           string          `json:"id"`
	Title        string          `json:"title"`
	Messages     []agent.Message `json:"messages"`
	MessageCount int             `json:"messageCount"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

func toAgentConversation(c library.Conversation) (AgentConversation, error) {
	out := AgentConversation{ID: c.ID, Title: c.Title, MessageCount: c.MessageCount, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Messages: []agent.Message{}}
	if len(c.Messages) > 0 {
		if err := json.Unmarshal(c.Messages, &out.Messages); err != nil {
			return AgentConversation{}, fmt.Errorf("decode conversation %s: %w", c.ID, err)
		}
	}
	return out, nil
}

// ListAgentConversations lists saved conversations, newest first.
func (l *Library) ListAgentConversations() ([]library.ConversationSummary, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListConversations(context.Background())
}

// GetAgentConversation returns a saved conversation with its transcript.
func (l *Library) GetAgentConversation(id string) (AgentConversation, error) {
	if l.store == nil {
		return AgentConversation{}, errors.New("library database is not ready")
	}
	c, err := l.store.GetConversation(context.Background(), id)
	if err != nil {
		return AgentConversation{}, err
	}
	return toAgentConversation(c)
}

// SaveAgentConversation stores the transcript; an empty id creates a new one.
func (l *Library) SaveAgentConversation(id, title string, messages []agent.Message) (AgentConversation, error) {
	if l.store == nil {
		return AgentConversation{}, errors.New("library database is not ready")
	}
	data, err := json.Marshal(messages)
	if err != nil {
		return AgentConversation{}, err
	}
	c, err := l.store.SaveConversation(context.Background(), id, title, data)
	if err != nil {
		return AgentConversation{}, err
	}
	return toAgentConversation(c)
}

// DeleteAgentConversation removes a saved conversation.
func (l *Library) DeleteAgentConversation(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.DeleteConversation(context.Background(), id)
}

// agentProgress shows the window each step of a run while it happens.
var agentProgress = mygo.NewEvent[agent.Event]("agentProgress")

// AgentChat runs the built-in agent over the conversation and returns it grown
// by the model's turns and tool calls. The window keeps the transcript, so
// the next call sends it back whole. Tools run in-process against the real
// database with write access, as the person asked for the work.
func (l *Library) AgentChat(ctx context.Context, messages []agent.Message) (agent.Result, error) {
	if l.store == nil || l.settings == nil {
		return agent.Result{}, errors.New("library database is not ready")
	}
	config := l.settings.Get().AI
	if config.Model == "" && agent.Normalize(config).Model == "" {
		return agent.Result{}, errors.New("请先在设置中填写 AI 模型服务")
	}
	provider, err := agent.NewProvider(config)
	if err != nil {
		return agent.Result{}, err
	}
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	local, err := library.ConnectLocalMCP(runCtx, l.store, sourceCredentials(l.settings))
	if err != nil {
		return agent.Result{}, err
	}
	defer local.Close()
	return agent.Run(runCtx, provider, agent.MCPTools{Session: local.Session}, messages, agent.Options{
		MaxSteps: agent.Normalize(config).MaxSteps,
		OnEvent: func(event agent.Event) {
			if err := agentProgress.Broadcast(event); err != nil {
				log.Printf("announce agent progress: %v", err)
			}
		},
	})
}
