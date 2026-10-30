package library

import "testing"

func TestMCPTokenIsPersistentAndUnique(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	first, err := LoadOrCreateMCPToken(firstDir)
	if err != nil {
		t.Fatalf("LoadOrCreateMCPToken() error = %v", err)
	}
	if len(first) != 64 {
		t.Errorf("token length = %d, want 64", len(first))
	}
	reloaded, err := LoadOrCreateMCPToken(firstDir)
	if err != nil {
		t.Fatalf("reload token error = %v", err)
	}
	if reloaded != first {
		t.Error("token changed after reloading")
	}
	second, err := LoadOrCreateMCPToken(secondDir)
	if err != nil {
		t.Fatalf("second token error = %v", err)
	}
	if second == first {
		t.Error("tokens for separate app data directories are identical")
	}
}
