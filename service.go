package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"golang.org/x/sync/singleflight"

	"bibliocell/internal/agent"
	"bibliocell/internal/library"
	"bibliocell/internal/settings"
)

// Library exposes local library operations to the frontend.
type Library struct {
	store              *library.Store
	settings           *settings.Store
	mcpEndpoint        string
	mcpToken           string
	translationFlights singleflight.Group
	updates            *appUpdater
	// events reaches the browsers of web mode; nil in the desktop app, whose
	// window hears the mygo events directly.
	events *eventHub
	// checks counts the AI second checks still running.
	checks sourceChecks
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

// LookupDOI fetches bibliographic metadata for a DOI without saving it.
func (l *Library) LookupDOI(doi string) (library.Paper, error) {
	return library.LookupDOI(context.Background(), doi, sourceCredentials(l.settings))
}

// servePaperFile answers paperfile://localhost/<id> (the page asks
// http://paperfile.localhost/<id> on Windows, which mygo maps back) with the
// stored file, for the window to render a PDF beside the paper. Only files the
// library knows are served, so the scheme cannot reach anything else on disk.
//
// The file goes out whole, never in byte ranges. On Windows the scheme is an
// http URL, so PDF.js would read the file in range requests as it does from a
// web server; the WebView2 scheme bridge answers each request from a buffer of
// the whole response, which does not line up with the ranges PDF.js asked for,
// and the document comes out as "Invalid PDF structure". Without Accept-Ranges
// PDF.js reads the one response from start to end, as it does on macOS, where
// the custom scheme is not http.
func (l *Library) servePaperFile(w http.ResponseWriter, r *http.Request) {
	if l.store == nil {
		http.Error(w, "library database is not ready", http.StatusServiceUnavailable)
		return
	}
	p, err := l.store.PaperFilePath(r.Context(), strings.Trim(r.URL.Path, "/"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		http.Error(w, fmt.Sprintf("file is missing from the storage folder: %s", filepath.Base(p)), http.StatusNotFound)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		http.Error(w, fmt.Sprintf("file is missing from the storage folder: %s", filepath.Base(p)), http.StatusNotFound)
		return
	}
	if strings.EqualFold(filepath.Ext(p), ".pdf") {
		w.Header().Set("Content-Type", "application/pdf")
	}
	w.Header().Set("Accept-Ranges", "none")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := io.Copy(w, f); err != nil {
		log.Printf("serve paper file %s: %v", filepath.Base(p), err)
	}
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

// ExportResult tells the window where an export landed. An empty path means
// the person cancelled the save dialog.
type ExportResult struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

// ExportLibrary renders the records the window sends (the current filter in
// either mode, Demo included) and saves the file where the person chooses.
// The database is not read, so Demo exports never touch the real library.
func (l *Library) ExportLibrary(ctx context.Context, req library.ExportRequest) (ExportResult, error) {
	file, err := library.Export(req, time.Now())
	if err != nil {
		return ExportResult{}, err
	}
	defaultPath := file.Name
	if dir, err := mygo.App.Path(mygo.PathDownloads); err == nil && dir != "" {
		defaultPath = filepath.Join(dir, file.Name)
	}
	ext := strings.TrimPrefix(filepath.Ext(file.Name), ".")
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:            mygo.CallerWindow(ctx),
		Title:             "导出",
		DefaultPath:       defaultPath,
		Filters:           []mygo.FileFilter{{Name: strings.ToUpper(ext), Extensions: []string{ext}}},
		CreateDirectories: true,
	})
	if err != nil || path == "" {
		return ExportResult{}, err
	}
	if err := os.WriteFile(path, []byte(file.Content), 0o644); err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Path: path, Count: file.Count}, nil
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

// DeleteDataset removes a dataset with its links and evidence; papers stay.
func (l *Library) DeleteDataset(id string) error {
	if l.store == nil {
		return errors.New("library database is not ready")
	}
	return l.store.DeleteDataset(context.Background(), id)
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

// SourceQueryHistory lists a source's query changes, newest first, so a person
// can see an assistant's edits and restore an earlier query.
func (l *Library) SourceQueryHistory(id string) ([]library.QueryChange, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.SourceQueryHistory(context.Background(), id)
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

// ScreenPaper asks the model bound to screening whether one paper meets a
// requirement, reading its stored full text when there is one and its abstract
// otherwise. The answer is a suggestion for the person: it is recorded for the
// comparison view, and the paper itself is never changed.
func (l *Library) ScreenPaper(paperID, requirement string) (library.ScreeningSuggestion, error) {
	return l.ScreenPaperInRun(paperID, requirement, "", "", "screen")
}

// ScreenPaperWithModel uses a selected profile without changing task bindings.
// Like ScreenPaper, it returns a recorded suggestion and never changes the paper.
func (l *Library) ScreenPaperWithModel(paperID, requirement, modelID string) (library.ScreeningSuggestion, error) {
	return l.ScreenPaperInRun(paperID, requirement, modelID, "", "screen")
}

// ScreenPaperInRun screens one paper as part of a batch run or a calibration
// rerun: the suggestion is recorded under runID with the given kind (screen,
// batch or rerun) so a whole run can be compared with the person's decisions.
// An empty modelID uses the profile bound to screening.
func (l *Library) ScreenPaperInRun(paperID, requirement, modelID, runID, kind string) (library.ScreeningSuggestion, error) {
	if strings.TrimSpace(requirement) == "" {
		return library.ScreeningSuggestion{}, errors.New("筛选要求不能为空")
	}
	return l.judge(paperID, library.ScreeningTask, requirement, modelID, runID, kind)
}

// JudgePresets lists the built-in "is this paper X?" tasks.
func (l *Library) JudgePresets() ([]library.JudgeTask, error) {
	return library.JudgePresets(), nil
}

// JudgePaperInRun asks the model one task's question about one paper and
// records the answer under runID. A preset id in task.ID uses the preset's
// question and target; anything else is a custom task that must say both.
// The paper is never changed: AdoptJudgements applies the answers a person
// accepts.
func (l *Library) JudgePaperInRun(paperID string, task library.JudgeTask, modelID, runID string) (library.ScreeningSuggestion, error) {
	if preset, ok := library.JudgePreset(task.ID); ok {
		task = preset
	}
	task, err := library.NormalizeJudgeTask(task)
	if err != nil {
		return library.ScreeningSuggestion{}, err
	}
	if task.Target.Kind == "screening" {
		return library.ScreeningSuggestion{}, errors.New("收录筛选请使用批量筛选")
	}
	return l.judge(paperID, task, task.Question, modelID, runID, "batch")
}

// AdoptJudgements marks the papers whose suggestions answered yes, see
// Store.AdoptJudgements.
func (l *Library) AdoptJudgements(ids []string) (library.AdoptResult, error) {
	if l.store == nil {
		return library.AdoptResult{}, errors.New("library database is not ready")
	}
	return l.store.AdoptJudgements(context.Background(), ids)
}

// judge runs one task over one paper. For screening, requirement is the
// person's requirement; for every other task it is the task's question. A
// task that reads the abstract never loads the stored full text, which keeps
// a pass over hundreds of papers cheap.
func (l *Library) judge(paperID string, task library.JudgeTask, requirement, modelID, runID, kind string) (library.ScreeningSuggestion, error) {
	if l.store == nil || l.settings == nil {
		return library.ScreeningSuggestion{}, errors.New("library database is not ready")
	}
	profile, err := l.screenProfile(modelID)
	if err != nil {
		return library.ScreeningSuggestion{}, err
	}
	provider, err := agent.NewProvider(profile.Config)
	if err != nil {
		return library.ScreeningSuggestion{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	paper, err := l.store.GetPaper(ctx, paperID)
	if err != nil {
		return library.ScreeningSuggestion{}, err
	}
	var pages []string
	if task.Basis == "fulltext" {
		_, pages, err = l.store.PaperFulltextPages(ctx, paperID)
		if err != nil {
			// The abstract still tells something; the reason says the full text was not read.
			log.Printf("screen %s: %v", paperID, err)
			pages = nil
		}
	}
	species := make([]string, 0, len(paper.Species))
	for _, entry := range paper.Species {
		if entry.Label != "" && entry.Label != entry.Name {
			species = append(species, entry.Name+"（"+entry.Label+"）")
		} else {
			species = append(species, entry.Name)
		}
	}
	year := ""
	if paper.Year != nil {
		year = fmt.Sprint(*paper.Year)
	}
	input := agent.ScreenInput{
		Title:      paper.Title,
		Authors:    paper.Authors,
		Year:       year,
		Journal:    paper.Journal,
		Species:    strings.Join(species, "、"),
		Organs:     strings.Join(paper.Organs, "、"),
		Modalities: strings.Join(paper.Modalities, "、"),
		Abstract:   paper.Abstract,
		Fulltext:   strings.TrimSpace(strings.Join(pages, "\n\n")),
	}
	if task.Target.Kind == "screening" {
		input.Requirement = requirement
	} else {
		input.Question = requirement
	}
	judgement, err := agent.Screen(ctx, provider, input)
	if err != nil {
		return library.ScreeningSuggestion{}, err
	}
	suggestion := library.ScreeningSuggestion{
		PaperID: paperID, Requirement: requirement,
		ProfileID: profile.ID, ProfileName: profile.Name, Model: profile.Config.Model, Protocol: profile.Config.Protocol,
		Verdict: judgement.Verdict, Reason: judgement.Reason, Basis: judgement.Basis, Excerpt: judgement.Excerpt,
		Confidence: judgement.Confidence, RunID: runID, Kind: kind,
		InputTokens: judgement.Usage.InputTokens, OutputTokens: judgement.Usage.OutputTokens, Skipped: judgement.Skipped,
		TargetKind: task.Target.Kind, TargetValue: task.Target.Value,
	}
	if task.Target.Kind != "screening" {
		suggestion.TaskID, suggestion.TaskName = task.ID, task.Name
	}
	if judgement.Skipped {
		return suggestion, nil
	}
	if judgement.Basis == "fulltext" {
		suggestion.Page = library.LocatePage(pages, judgement.Excerpt)
	}
	saved, err := l.store.AddScreeningSuggestion(ctx, suggestion)
	if err != nil {
		// The answer is still worth showing; only its record is missing.
		log.Printf("record screening suggestion for %s: %v", paperID, err)
		return suggestion, nil
	}
	return saved, nil
}

// screenProfile returns the selected profile, or the one bound to screening.
func (l *Library) screenProfile(modelID string) (settings.ModelProfile, error) {
	if modelID == "" {
		return l.settings.Get().ModelFor(settings.TaskScreen)
	}
	for _, profile := range l.settings.Get().Models {
		if profile.ID == modelID {
			return profile, nil
		}
	}
	return settings.ModelProfile{}, errors.New("所选模型已不存在，请重新选择")
}

// ListScreeningSuggestions returns the recorded AI suggestions for one paper,
// newest first, or all of them when paperID is empty.
func (l *Library) ListScreeningSuggestions(paperID string) ([]library.ScreeningSuggestion, error) {
	if l.store == nil {
		return nil, errors.New("library database is not ready")
	}
	return l.store.ListScreeningSuggestions(context.Background(), paperID)
}

// CalibrationReport compares recorded AI suggestions with the person's
// decisions: agreement rate, confusion matrix and the papers they disagree on.
func (l *Library) CalibrationReport(filter library.CalibrationFilter) (library.CalibrationReport, error) {
	if l.store == nil {
		return library.CalibrationReport{}, errors.New("library database is not ready")
	}
	return l.store.CalibrationReport(context.Background(), filter)
}

// LocateFulltextPage finds the PDF page a passage sits on, by searching the
// stored full text; 0 means it was not found. It lets the window jump to a
// quote whose page was not recorded.
func (l *Library) LocateFulltextPage(paperID, excerpt string) (int, error) {
	if l.store == nil {
		return 0, errors.New("library database is not ready")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	_, pages, err := l.store.PaperFulltextPages(ctx, paperID)
	if err != nil {
		return 0, err
	}
	return library.LocatePage(pages, excerpt), nil
}

// provider builds the client for the model a task is bound to.
func (l *Library) provider(task string) (agent.Provider, error) {
	profile, err := l.settings.Get().ModelFor(task)
	if err != nil {
		return nil, err
	}
	return agent.NewProvider(profile.Config)
}

// TranslateText uses the saved prompt and the model bound to translation.
func (l *Library) TranslateText(text string) (string, error) {
	if l.settings == nil {
		return "", errors.New("settings are not ready")
	}
	text = strings.TrimSpace(text)
	config := l.settings.Get()
	key := library.TranslationCacheKey(text, config.TranslationPrompt)
	result, err, _ := l.translationFlights.Do(key, func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		if l.store != nil {
			cached, err := l.store.CachedTranslation(ctx, key)
			if err != nil || cached != "" {
				return cached, err
			}
		}
		profile, err := config.ModelFor(settings.TaskTranslate)
		if err != nil {
			return "", err
		}
		provider, err := agent.NewProvider(profile.Config)
		if err != nil {
			return "", err
		}
		translated, err := agent.TranslateWithPrompt(ctx, provider, text, config.TranslationPrompt)
		if err != nil {
			return "", err
		}
		if l.store != nil {
			if err := l.store.CacheTranslation(ctx, key, translated); err != nil {
				return "", err
			}
		}
		return translated, nil
	})
	if err != nil {
		return "", err
	}
	return result.(string), nil
}

// CachedTranslation reads a saved result without invoking the model.
func (l *Library) CachedTranslation(text string) (string, error) {
	if l.settings == nil || l.store == nil {
		return "", errors.New("library settings and database are not ready")
	}
	key := library.TranslationCacheKey(text, l.settings.Get().TranslationPrompt)
	return l.store.CachedTranslation(context.Background(), key)
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
	profile, err := l.settings.Get().ModelFor(settings.TaskChat)
	if err != nil {
		return agent.Result{}, err
	}
	provider, err := agent.NewProvider(profile.Config)
	if err != nil {
		return agent.Result{}, err
	}
	config := profile.Config
	runCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	local, err := library.ConnectLocalMCP(runCtx, l.store, sourceCredentials(l.settings))
	if err != nil {
		return agent.Result{}, err
	}
	defer local.Close()
	return agent.Run(runCtx, provider, agent.MCPTools{Session: local.Session}, messages, agent.Options{
		MaxSteps: agent.Normalize(config).MaxSteps,
		OnEvent:  l.announceAgent,
	})
}
