package library

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadOrCreateMCPToken returns the persistent local credential for MCP write access.
func LoadOrCreateMCPToken(dataDir string) (string, error) {
	path := filepath.Join(dataDir, "mcp-token")
	content, err := os.ReadFile(path)
	if err == nil {
		token := strings.TrimSpace(string(content))
		if len(token) < 32 {
			return "", errors.New("stored MCP token is invalid")
		}
		return token, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("read MCP token: %w", err)
	}

	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return "", fmt.Errorf("generate MCP token: %w", err)
	}
	token := hex.EncodeToString(secret[:])
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return "", fmt.Errorf("create MCP credential directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(token+"\n"), 0o600); err != nil {
		return "", fmt.Errorf("save MCP token: %w", err)
	}
	return token, nil
}
