// Package settings persists desktop application preferences.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	DefaultFontSize = 15
	MinFontSize     = 12
	MaxFontSize     = 20
)

// Config contains preferences that survive application restarts.
type Config struct {
	DatabasePath string `json:"databasePath"`
	FontSize     int    `json:"fontSize"`
	MCPToken     string `json:"mcpToken"`
}

// Store loads and atomically saves a JSON settings file.
type Store struct {
	mu     sync.RWMutex
	path   string
	config Config
}

// Open loads settings. On invalid or unreadable contents it returns usable
// defaults together with the error, allowing the application to start safely.
func Open(path, defaultDatabasePath, defaultMCPToken string) (*Store, error) {
	defaults := Config{DatabasePath: filepath.Clean(defaultDatabasePath), FontSize: DefaultFontSize, MCPToken: defaultMCPToken}
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
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return store, fmt.Errorf("parse settings: %w", err)
	}
	config.DatabasePath = strings.TrimSpace(config.DatabasePath)
	if config.DatabasePath == "" {
		config.DatabasePath = defaults.DatabasePath
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
	config.DatabasePath = filepath.Clean(strings.TrimSpace(config.DatabasePath))
	config.MCPToken = strings.TrimSpace(config.MCPToken)
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
// executable and font sizes outside the supported readable range.
func Validate(config Config) error {
	if strings.TrimSpace(config.DatabasePath) == "" {
		return errors.New("database path is required")
	}
	if !filepath.IsAbs(config.DatabasePath) {
		return errors.New("database path must be absolute")
	}
	if config.FontSize < MinFontSize || config.FontSize > MaxFontSize {
		return fmt.Errorf("font size must be between %d and %d pixels", MinFontSize, MaxFontSize)
	}
	if config.MCPToken != "" {
		if err := ValidateMCPToken(config.MCPToken); err != nil {
			return err
		}
	}
	return nil
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
