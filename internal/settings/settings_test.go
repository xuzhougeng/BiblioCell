package settings

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if got := reopened.Get(); got != want {
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
	if got := reopenedAgain.Get(); got != want {
		t.Errorf("replacement settings = %+v, want %+v", got, want)
	}
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
	} {
		if err := store.Save(config); err == nil {
			t.Errorf("Save(%+v) succeeded, want validation error", config)
		}
	}
	if got := store.Get(); got != original {
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
