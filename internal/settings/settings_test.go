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
	defaultDatabase := filepath.Join(directory, "default.sqlite")
	store, err := Open(path, defaultDatabase, testToken)
	if err != nil {
		t.Fatalf("Open() defaults error = %v", err)
	}
	if got := store.Get(); got.DatabasePath != defaultDatabase || got.FontSize != DefaultFontSize {
		t.Fatalf("defaults = %+v", got)
	}

	want := Config{DatabasePath: filepath.Join(directory, "research", "library.sqlite"), FontSize: 18, MCPToken: testToken}
	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	reopened, err := Open(path, defaultDatabase, testToken)
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
	reopenedAgain, err := Open(path, defaultDatabase, testToken)
	if err != nil {
		t.Fatalf("Open() replaced settings error = %v", err)
	}
	if got := reopenedAgain.Get(); got != want {
		t.Errorf("replacement settings = %+v, want %+v", got, want)
	}
}

func TestStoreRejectsInvalidChanges(t *testing.T) {
	directory := t.TempDir()
	store, err := Open(filepath.Join(directory, "settings.json"), filepath.Join(directory, "default.sqlite"), testToken)
	if err != nil {
		t.Fatal(err)
	}
	original := store.Get()
	for _, config := range []Config{
		{DatabasePath: "relative.sqlite", FontSize: 15},
		{DatabasePath: original.DatabasePath, FontSize: MinFontSize - 1},
		{DatabasePath: original.DatabasePath, FontSize: MaxFontSize + 1},
		{DatabasePath: original.DatabasePath, FontSize: 15, MCPToken: "too short"},
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
	store, err := Open(path, filepath.Join(directory, "default.sqlite"), testToken)
	if err == nil {
		t.Fatal("Open() invalid file succeeded, want error")
	}
	if got := store.Get(); got.FontSize != DefaultFontSize {
		t.Errorf("fallback settings = %+v", got)
	}
}
