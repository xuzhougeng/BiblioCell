package settings

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"bibliocell/internal/agent"
)

var testToken = strings.Repeat("t", 64)

func TestStoreDefaultsAndRoundTrip(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")
	defaultStorage := filepath.Join(directory, "default")
	store, err := Open(path, defaultStorage, testToken)
	if err != nil {
		t.Fatalf("Open() defaults error = %v", err)
	}
	if got := store.Get(); got.StoragePath != defaultStorage || got.FontSize != DefaultFontSize {
		t.Fatalf("defaults = %+v", got)
	}
	if got, want := store.Get().DatabasePath(), filepath.Join(defaultStorage, DatabaseFile); got != want {
		t.Fatalf("DatabasePath() = %q, want %q", got, want)
	}

	want := Config{StoragePath: filepath.Join(directory, "research"), FontSize: 18, MCPToken: testToken}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reopened, err := Open(path, defaultStorage, testToken)
	if err != nil {
		t.Fatalf("Open() saved settings error = %v", err)
	}
	if got := reopened.Get(); !reflect.DeepEqual(got, want) {
		t.Errorf("reopened settings = %+v, want %+v", got, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Errorf("settings permissions = %o, want 600", info.Mode().Perm())
	}

	want.FontSize = 16
	if err := reopened.Save(want); err != nil {
		t.Fatalf("Save() replacement error = %v", err)
	}
	reopenedAgain, err := Open(path, defaultStorage, testToken)
	if err != nil {
		t.Fatalf("Open() replaced settings error = %v", err)
	}
	if got := reopenedAgain.Get(); !reflect.DeepEqual(got, want) {
		t.Errorf("replacement settings = %+v, want %+v", got, want)
	}
}

func TestStoreKeepsAIConfig(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")
	store, err := Open(path, directory, testToken)
	if err != nil {
		t.Fatal(err)
	}
	config := store.Get()
	config.Models = []ModelProfile{
		{ID: "local", Name: " Ollama ", Config: agent.Config{Protocol: "openai", BaseURL: "http://localhost:11434/v1/", APIKey: " key ", Model: "qwen3"}},
		{ID: "cloud", Name: "Claude", Config: agent.Config{Protocol: "anthropic", APIKey: "sk"}},
	}
	config.Tasks = TaskModels{Translate: "cloud"}
	if err := store.Save(config); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path, directory, testToken)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Get()
	if len(got.Models) != 2 || got.Models[0].Name != "Ollama" || got.Models[0].Config.BaseURL != "http://localhost:11434/v1" || got.Models[0].Config.APIKey != "key" {
		t.Fatalf("models = %+v", got.Models)
	}
	// An unbound task falls back to the first profile; a bound one uses its own.
	if chat, _ := got.ModelFor(TaskChat); chat.ID != "local" {
		t.Fatalf("chat model = %+v", chat)
	}
	if translate, _ := got.ModelFor(TaskTranslate); translate.ID != "cloud" {
		t.Fatalf("translate model = %+v", translate)
	}
	if _, err := (Config{}).ModelFor(TaskScreen); !errors.Is(err, ErrNoModel) {
		t.Fatalf("empty config error = %v", err)
	}
	config.Models[0].Config = agent.Config{Protocol: "openai"} // a model name is required
	if err := store.Save(config); err == nil {
		t.Fatal("incomplete model accepted")
	}
	config.Models[0].Config.Model = "qwen3"
	config.Tasks.Screen = "missing"
	if err := store.Save(config); err == nil {
		t.Fatal("binding to an unknown model accepted")
	}
}

func TestStoreMigratesTheSingleLegacyModel(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")
	legacy := `{"storagePath": ` + jsonString(directory) + `, "fontSize": 15, "mcpToken": "` + testToken + `", "ai": {"protocol": "openai", "baseUrl": "http://localhost:11434/v1", "model": "qwen3"}}`
	if err := os.WriteFile(path, []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, directory, testToken)
	if err != nil {
		t.Fatal(err)
	}
	got := store.Get()
	if len(got.Models) != 1 || got.Models[0].ID != "default" || got.Models[0].Name != "qwen3" || got.Models[0].Config.Model != "qwen3" {
		t.Fatalf("migrated models = %+v", got.Models)
	}
	if err := store.Save(got); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), `"ai"`) {
		t.Fatalf("legacy section written back:\n%s", data)
	}
}

func jsonString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func TestStoreRejectsInvalidChanges(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(filepath.Join(directory, "settings.json"), directory, testToken)
	if err != nil {
		t.Fatal(err)
	}
	original := store.Get()
	file := filepath.Join(directory, "library.sqlite")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, config := range []Config{
		{StoragePath: "relative", FontSize: 15},
		{StoragePath: file, FontSize: 15},
		{StoragePath: original.StoragePath, FontSize: MinFontSize - 1},
		{StoragePath: original.StoragePath, FontSize: MaxFontSize + 1},
		{StoragePath: original.StoragePath, FontSize: 15, MCPToken: "too short"},
		{StoragePath: original.StoragePath, FontSize: 15, NCBIAPIKey: "has space"},
		{StoragePath: original.StoragePath, FontSize: 15, ContactEmail: "not an address"},
		{StoragePath: original.StoragePath, FontSize: 15, ContactEmail: "Me <me@example.org>"},
	} {
		if err := store.Save(config); err == nil {
			t.Errorf("Save(%+v) succeeded, want validation error", config)
		}
	}
	if got := store.Get(); !reflect.DeepEqual(got, original) {
		t.Errorf("invalid save changed settings to %+v", got)
	}
}

func TestStoreFallsBackFromInvalidFile(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "settings.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, directory, testToken)
	if err == nil {
		t.Fatal("Open() invalid file succeeded, want error")
	}
	if got := store.Get(); got.FontSize != DefaultFontSize {
		t.Errorf("fallback settings = %+v", got)
	}
}
