package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Bearer tokens for MCP servers are kept outside dots.json (which Desktop,
// the CLI, and exports may copy) in one 0600 file per server record, so they
// never appear in listings, events, or logs.

var tokenServerID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

func tokenPath(dataDir, serverID string) (string, error) {
	if dataDir == "" || !tokenServerID.MatchString(serverID) {
		return "", errors.New("invalid MCP token location")
	}
	return filepath.Join(dataDir, "mcp-tokens", serverID), nil
}

// SaveToken atomically replaces the token for a configured server.
func SaveToken(dataDir, serverID, token string) error {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 512 || strings.ContainsAny(token, "\r\n\x00 ") {
		return errors.New("MCP bearer token is empty or malformed")
	}
	path, err := tokenPath(dataDir, serverID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create token directory: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".token-*")
	if err != nil {
		return fmt.Errorf("save MCP token: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(token + "\n"); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// LoadToken returns the saved token, or "" when none exists. A token file that
// is readable by other users is refused so a loosened chmod fails closed.
func LoadToken(dataDir, serverID string) (string, error) {
	path, err := tokenPath(dataDir, serverID)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("MCP token file permissions are too open; expected 0600")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

func DeleteToken(dataDir, serverID string) error {
	path, err := tokenPath(dataDir, serverID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
