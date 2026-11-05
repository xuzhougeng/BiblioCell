package main

import (
	"os"
	"reflect"
	"path/filepath"
	"strings"
	"testing"

	"bibliocell/internal/settings"
)

func TestSaveSettingsRefusesAFolderTheAppCannotOpen(t *testing.T) {
	directory := t.TempDir()
	store, err := settings.Open(filepath.Join(directory, "settings.json"), directory, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	service := &Library{settings: store}
	original := store.Get()

	broken := filepath.Join(directory, "broken")
	if err := os.MkdirAll(broken, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, settings.DatabaseFile), []byte("not a database"), 0o600); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, folder := range []string{broken, file, filepath.Join(file, "below"), "relative"} {
		config := original
		config.StoragePath = folder
		if _, err := service.SaveSettings(config); err == nil {
			t.Errorf("SaveSettings(%q) succeeded", folder)
		}
		if got := store.Get(); !reflect.DeepEqual(got, original) {
			t.Fatalf("a refused folder reached the settings: %+v", got)
		}
	}
	reopened, err := settings.Open(filepath.Join(directory, "settings.json"), directory, strings.Repeat("a", 64))
	if err != nil || reopened.Get().StoragePath != directory {
		t.Fatalf("settings file after refusals = %+v, %v", reopened.Get(), err)
	}

	empty := filepath.Join(directory, "new library")
	config := original
	config.StoragePath = "  " + empty + "/  "
	result, err := service.SaveSettings(config)
	if err != nil {
		t.Fatal(err)
	}
	if !result.StorageChanged || result.Settings.StoragePath != empty {
		t.Fatalf("new folder result = %+v", result)
	}
	if _, err := os.Stat(filepath.Join(empty, settings.DatabaseFile)); err != nil {
		t.Fatalf("new folder has no database: %v", err)
	}
}

func TestSaveSettingsReportsRestartChanges(t *testing.T) {
	directory := t.TempDir()
	store, err := settings.Open(
		filepath.Join(directory, "settings.json"),
		directory,
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
	if result.RestartRequired || result.StorageChanged || result.MCPTokenChanged {
		t.Errorf("unchanged settings requested restart: %+v", result)
	}

	config.MCPToken = strings.Repeat("b", 64)
	result, err = service.SaveSettings(config)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RestartRequired || !result.MCPTokenChanged || result.StorageChanged {
		t.Errorf("token change result = %+v", result)
	}

	config = result.Settings
	config.StoragePath = filepath.Join(directory, "research")
	result, err = service.SaveSettings(config)
	if err != nil {
		t.Fatal(err)
	}
	if !result.RestartRequired || !result.StorageChanged || result.MCPTokenChanged {
		t.Errorf("storage change result = %+v", result)
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
