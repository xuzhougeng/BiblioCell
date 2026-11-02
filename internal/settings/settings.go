// Package settings persists desktop application preferences.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"bibliocell/internal/agent"
)

const (
	DefaultFontSize = 15
	MinFontSize     = 12
	MaxFontSize     = 20

	// DatabaseFile is the SQLite file name inside the storage folder.
	DatabaseFile = "bibliocell.sqlite"
)

// Config contains preferences that survive application restarts.
type Config struct {
	StoragePath string `json:"storagePath"`
	FontSize    int    `json:"fontSize"`
	MCPToken    string `json:"mcpToken"`
	// NCBIAPIKey and ContactEmail identify this user to public APIs. They live here, not
	// in the database, so they stay on this machine when the folder moves.
	NCBIAPIKey   string `json:"ncbiApiKey"`
	ContactEmail string `json:"contactEmail"`
	// Models are the model services this person can reach; the keys stay on
	// this machine like the other credentials. Tasks says which one each
	// feature uses.
	Models []ModelProfile `json:"models"`
	Tasks  TaskModels     `json:"tasks"`
}

// ModelProfile is one model service under a name the person chose.
type ModelProfile struct {
	ID     string       `json:"id"`
	Name   string       `json:"name"`
	Config agent.Config `json:"config"`
}

// TaskModels binds each AI feature to a profile ID. An empty binding uses the
// first profile, so one model works everywhere until the person decides more.
type TaskModels struct {
	// Chat is the built-in assistant.
	Chat string `json:"chat"`
	// Screen judges papers against a requirement while screening.
	Screen string `json:"screen"`
	// Translate renders selected text in Chinese.
	Translate string `json:"translate"`
}

// Task names, as the frontend and CLI refer to them.
const (
	TaskChat      = "chat"
	TaskScreen    = "screen"
	TaskTranslate = "translate"
)

// ErrNoModel means no model service is configured for a task.
var ErrNoModel = errors.New("请先在设置中添加 AI 模型")

// ModelFor returns the profile bound to a task, or the first profile when the
// task has no binding of its own.
func (c Config) ModelFor(task string) (ModelProfile, error) {
	id := ""
	switch task {
	case TaskChat:
		id = c.Tasks.Chat
	case TaskScreen:
		id = c.Tasks.Screen
	case TaskTranslate:
		id = c.Tasks.Translate
	default:
		return ModelProfile{}, fmt.Errorf("unknown AI task %q", task)
	}
	if id != "" {
		for _, profile := range c.Models {
			if profile.ID == id {
				return profile, nil
			}
		}
	}
	if len(c.Models) == 0 {
		return ModelProfile{}, ErrNoModel
	}
	return c.Models[0], nil
}

// legacyConfig reads settings files written before model profiles existed,
// which held one model service under "ai".
type legacyConfig struct {
	Config
	AI agent.Config `json:"ai"`
}

// migrateModels turns the single legacy service into the first profile, bound
// to every task by default.
func migrateModels(legacy legacyConfig) Config {
	config := legacy.Config
	if len(config.Models) == 0 && legacy.AI != (agent.Config{}) {
		name := legacy.AI.Model
		if name == "" {
			name = "默认模型"
		}
		config.Models = []ModelProfile{{ID: "default", Name: name, Config: legacy.AI}}
	}
	return config
}

// DatabasePath returns the SQLite file inside the storage folder.
func (c Config) DatabasePath() string {
	return filepath.Join(c.StoragePath, DatabaseFile)
}

// Store loads and atomically saves a JSON settings file.
type Store struct {
	mu     sync.RWMutex
	path   string
	config Config
}

// Open loads settings. On invalid or unreadable contents it returns usable
// defaults together with the error, allowing the application to start safely.
func Open(path, defaultStoragePath, defaultMCPToken string) (*Store, error) {
	defaults := Config{StoragePath: filepath.Clean(defaultStoragePath), FontSize: DefaultFontSize, MCPToken: defaultMCPToken}
	store := &Store{path: path, config: defaults}
	if err := Validate(defaults); err != nil {
		return store, fmt.Errorf("invalid default settings: %w", err)
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read settings: %w", err)
	}
	var legacy legacyConfig
	if err := json.Unmarshal(data, &legacy); err != nil {
		return store, fmt.Errorf("parse settings: %w", err)
	}
	config := migrateModels(legacy)
	config.StoragePath = strings.TrimSpace(config.StoragePath)
	if config.StoragePath == "" {
		config.StoragePath = defaults.StoragePath
	}
	if config.FontSize == 0 {
		config.FontSize = DefaultFontSize
	}
	if config.MCPToken == "" {
		config.MCPToken = defaults.MCPToken
	}
	if err := Validate(config); err != nil {
		return store, fmt.Errorf("invalid settings: %w", err)
	}
	store.config = config
	return store, nil
}

// Get returns a snapshot of the current preferences.
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// Save validates and atomically replaces the settings file.
func (s *Store) Save(config Config) error {
	config.StoragePath = filepath.Clean(strings.TrimSpace(config.StoragePath))
	config.MCPToken = strings.TrimSpace(config.MCPToken)
	config.NCBIAPIKey = strings.TrimSpace(config.NCBIAPIKey)
	config.ContactEmail = strings.TrimSpace(config.ContactEmail)
	config.Models = trimModels(config.Models)
	if err := Validate(config); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	data = append(data, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(s.path), ".settings-*.json")
	if err != nil {
		return fmt.Errorf("create temporary settings: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("protect temporary settings: %w", err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write temporary settings: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync temporary settings: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary settings: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace settings: %w", err)
	}

	s.mu.Lock()
	s.config = config
	s.mu.Unlock()
	return nil
}

// Validate rejects paths that could unexpectedly resolve relative to the
// executable, an existing file where the storage folder belongs, and font
// sizes outside the supported readable range.
func Validate(config Config) error {
	if strings.TrimSpace(config.StoragePath) == "" {
		return errors.New("storage path is required")
	}
	if !filepath.IsAbs(config.StoragePath) {
		return errors.New("storage path must be absolute")
	}
	if info, err := os.Stat(config.StoragePath); err == nil && !info.IsDir() {
		return errors.New("storage path must be a folder, not a file")
	}
	if config.FontSize < MinFontSize || config.FontSize > MaxFontSize {
		return fmt.Errorf("font size must be between %d and %d pixels", MinFontSize, MaxFontSize)
	}
	if config.MCPToken != "" {
		if err := ValidateMCPToken(config.MCPToken); err != nil {
			return err
		}
	}
	if len(config.NCBIAPIKey) > 256 || strings.ContainsFunc(config.NCBIAPIKey, func(r rune) bool { return r < 33 || r > 126 }) {
		return errors.New("NCBI API key must be at most 256 visible ASCII characters")
	}
	if config.ContactEmail != "" {
		if address, err := mail.ParseAddress(config.ContactEmail); err != nil || address.Address != config.ContactEmail {
			return errors.New("contact email must be a plain email address")
		}
	}
	ids := map[string]bool{}
	for _, profile := range config.Models {
		if profile.ID == "" || profile.Name == "" {
			return errors.New("every AI model needs an id and a name")
		}
		if len(profile.ID) > 64 || len(profile.Name) > 100 {
			return errors.New("AI model id and name must be short")
		}
		if ids[profile.ID] {
			return fmt.Errorf("AI model id %q is used twice", profile.ID)
		}
		ids[profile.ID] = true
		if err := agent.Validate(profile.Config); err != nil {
			return fmt.Errorf("AI model %q: %w", profile.Name, err)
		}
	}
	for task, id := range map[string]string{TaskChat: config.Tasks.Chat, TaskScreen: config.Tasks.Screen, TaskTranslate: config.Tasks.Translate} {
		if id != "" && !ids[id] {
			return fmt.Errorf("AI task %s is bound to an unknown model %q", task, id)
		}
	}
	return nil
}

// trimModels cleans what the person typed in each profile.
func trimModels(models []ModelProfile) []ModelProfile {
	if len(models) == 0 {
		return nil
	}
	cleaned := make([]ModelProfile, 0, len(models))
	for _, profile := range models {
		profile.ID = strings.TrimSpace(profile.ID)
		profile.Name = strings.TrimSpace(profile.Name)
		profile.Config = agent.Trim(profile.Config)
		cleaned = append(cleaned, profile)
	}
	return cleaned
}

// ValidateMCPToken rejects short tokens and characters that cannot safely be
// sent in an HTTP Authorization header.
func ValidateMCPToken(token string) error {
	if len(token) < 32 || len(token) > 256 {
		return errors.New("MCP bearer token must contain between 32 and 256 bytes")
	}
	if strings.TrimSpace(token) != token {
		return errors.New("MCP bearer token cannot contain surrounding whitespace")
	}
	for i := 0; i < len(token); i++ {
		if token[i] < 33 || token[i] > 126 {
			return errors.New("MCP bearer token must use visible ASCII characters only")
		}
	}
	return nil
}
