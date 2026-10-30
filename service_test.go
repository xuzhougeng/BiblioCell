package main

import (
	"path/filepath"
	"strings"
	"testing"

	"bibliocell/internal/settings"
)

func TestSaveSettingsReportsRestartChanges(t *testing.T) {
	directory := t.TempDir()
	store, err := settings.Open(
		filepath.Join(directory, "settings.json"),
		filepath.Join(directory, "bibliocell.sqlite"),
		strings.Repeat("a", 64),
	)
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{settings: store}
	config := store.Get()

	result, err := service.SaveSettings(config)
	if err != nil {
		t.Fatal(err)
	}
	if result.RestartRequired || result.DatabaseChanged || result.MCPTokenChanged {
		t.Errorf("unchanged settings requested restart: %+v", result)
	}

	config.MCPToken = strings.Repeat("b", 64)
	result, err = service.SaveSettings(config)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RestartRequired || !result.MCPTokenChanged || result.DatabaseChanged {
		t.Errorf("token change result = %+v", result)
	}

	config = result.Settings
	config.MCPToken = "short"
	if _, err := service.SaveSettings(config); err == nil {
		t.Fatal("short MCP token succeeded")
	}
	if got := store.Get().MCPToken; got != result.Settings.MCPToken {
		t.Errorf("invalid token replaced saved token")
	}
}
