package library

import (
	"context"
	"path/filepath"
	"testing"
)

func TestTranslationCacheMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.sqlite")
	buildDatabase(t, path, "schema_v13.sql")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the previous release's version and reopen through the upgrade path.
	if _, err := store.db.Exec("DROP TABLE translation_cache; DELETE FROM schema_migrations WHERE version = 15; PRAGMA user_version = 14"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Upgraded == nil || store.Upgraded.From != 14 || store.Upgraded.To != SchemaVersion {
		t.Fatalf("upgrade = %+v", store.Upgraded)
	}
	ctx := context.Background()
	key := TranslationCacheKey("original abstract", "Translate into Chinese")
	if err := store.CacheTranslation(ctx, key, "译文"); err != nil {
		t.Fatal(err)
	}
	if err := store.CacheTranslation(ctx, key, "duplicate result"); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if got, err := store.CachedTranslation(ctx, key); err != nil || got != "译文" {
		t.Fatalf("persistent translation = %q, %v", got, err)
	}
	if err := store.CacheTranslation(ctx, "empty", " "); err == nil {
		t.Fatal("empty translation accepted")
	}
	if TranslationCacheKey("a", "b\x00c") == TranslationCacheKey("a\x00b", "c") {
		t.Fatal("source and prompt boundaries must be unambiguous")
	}
}
