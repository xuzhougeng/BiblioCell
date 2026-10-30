package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeSettings(t *testing.T, path, databasePath string) {
	t.Helper()
	data, err := json.Marshal(map[string]any{"databasePath": databasePath, "fontSize": 15})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))
}

func readSettingsDatabasePath(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	return configString(config, "databasePath")
}

func TestMigrateLegacyDirMovesWholeDirectory(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, legacyAppName)
	dataDir := filepath.Join(root, "BiblioCell")
	writeFile(t, filepath.Join(legacyDir, legacyDatabaseName), "legacy database")
	writeSettings(t, filepath.Join(legacyDir, legacySettingsName), filepath.Join(legacyDir, legacyDatabaseName))
	writeFile(t, filepath.Join(legacyDir, legacyTokenName), "secret-token")

	migrateLegacyDir(legacyDir, dataDir)

	if data, err := os.ReadFile(filepath.Join(dataDir, "bibliocell.sqlite")); err != nil || string(data) != "legacy database" {
		t.Fatalf("database not moved into new location: %v", err)
	}
	if _, err := os.Stat(legacyDir); !os.IsNotExist(err) {
		t.Fatalf("legacy directory still present: %v", err)
	}
	if got := readSettingsDatabasePath(t, filepath.Join(dataDir, legacySettingsName)); got != filepath.Join(dataDir, "bibliocell.sqlite") {
		t.Errorf("settings database path = %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, legacyTokenName)); err != nil || string(data) != "secret-token" {
		t.Fatalf("MCP token not moved: %v", err)
	}
}

func TestMigrateLegacyDirCarriesFilesWhenBothExist(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, legacyAppName)
	dataDir := filepath.Join(root, "BiblioCell")
	writeFile(t, filepath.Join(legacyDir, legacyDatabaseName), "legacy database")
	writeSettings(t, filepath.Join(legacyDir, legacySettingsName), filepath.Join(legacyDir, legacyDatabaseName))
	writeFile(t, filepath.Join(legacyDir, legacyTokenName), "secret-token")
	writeFile(t, filepath.Join(dataDir, "other-state.json"), "{}")

	migrateLegacyDir(legacyDir, dataDir)

	if data, err := os.ReadFile(filepath.Join(dataDir, "bibliocell.sqlite")); err != nil || string(data) != "legacy database" {
		t.Fatalf("database not carried over: %v", err)
	}
	if got := readSettingsDatabasePath(t, filepath.Join(dataDir, legacySettingsName)); got != filepath.Join(dataDir, "bibliocell.sqlite") {
		t.Errorf("settings database path = %q", got)
	}
	if data, err := os.ReadFile(filepath.Join(dataDir, legacyTokenName)); err != nil || string(data) != "secret-token" {
		t.Fatalf("MCP token not carried over: %v", err)
	}
}

func TestMigrateLegacyDirKeepsCustomDatabasePath(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, legacyAppName)
	dataDir := filepath.Join(root, "BiblioCell")
	customDB := filepath.Join(root, "custom", "plantcell.sqlite")
	writeFile(t, filepath.Join(legacyDir, legacyDatabaseName), "legacy database")
	writeSettings(t, filepath.Join(legacyDir, legacySettingsName), customDB)
	writeFile(t, customDB, "custom database")

	migrateLegacyDir(legacyDir, dataDir)

	if got := readSettingsDatabasePath(t, filepath.Join(dataDir, legacySettingsName)); got != customDB {
		t.Errorf("settings database path = %q, want untouched %q", got, customDB)
	}
	if data, err := os.ReadFile(customDB); err != nil || string(data) != "custom database" {
		t.Fatalf("custom database was modified: %v", err)
	}
}

func TestMigrateLegacyDirSkipsWithoutLegacyDatabase(t *testing.T) {
	root := t.TempDir()
	legacyDir := filepath.Join(root, legacyAppName)
	dataDir := filepath.Join(root, "BiblioCell")
	writeFile(t, filepath.Join(legacyDir, legacySettingsName), "{}")

	migrateLegacyDir(legacyDir, dataDir)

	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("data directory created without legacy database: %v", err)
	}
}

func TestFixLegacyDatabasePathPreservesUnknownFields(t *testing.T) {
	root := t.TempDir()
	settingsPath := filepath.Join(root, legacySettingsName)
	legacyDir := filepath.Join(root, legacyAppName)
	dataDir := filepath.Join(root, "BiblioCell")
	data, err := json.Marshal(map[string]any{
		"databasePath": filepath.Join(legacyDir, legacyDatabaseName),
		"fontSize":     17,
		"mcpToken":     "token-value",
	})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, settingsPath, string(data))

	fixLegacyDatabasePath(settingsPath, legacyDir, dataDir)

	data, err = os.ReadFile(settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	if configString(config, "mcpToken") != "token-value" || config["fontSize"] != float64(17) {
		t.Errorf("unexpected settings contents: %s", data)
	}
	if got := configString(config, "databasePath"); got != filepath.Join(dataDir, "bibliocell.sqlite") {
		t.Errorf("database path = %q", got)
	}
}
