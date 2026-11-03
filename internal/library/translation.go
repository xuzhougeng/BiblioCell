package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// TranslationCacheKey distinguishes source text and translation instructions.
func TranslationCacheKey(text, prompt string) string {
	data, _ := json.Marshal([2]string{strings.TrimSpace(text), strings.TrimSpace(prompt)})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// CachedTranslation returns an empty string when this library has no matching result.
func (s *Store) CachedTranslation(ctx context.Context, key string) (string, error) {
	var text string
	err := s.db.QueryRowContext(ctx, "SELECT translated_text FROM translation_cache WHERE cache_key = ?", key).Scan(&text)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read translation cache: %w", err)
	}
	return text, nil
}

// CacheTranslation stores successful results atomically, including repeated requests.
func (s *Store) CacheTranslation(ctx context.Context, key, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("cannot cache an empty translation")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO translation_cache(cache_key, translated_text)
		VALUES (?, ?) ON CONFLICT(cache_key) DO NOTHING`, key, text)
	if err != nil {
		return fmt.Errorf("save translation cache: %w", err)
	}
	return nil
}
