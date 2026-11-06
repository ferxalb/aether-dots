package mcp

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
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
	if err := checkTokenDir(filepath.Dir(path)); err != nil {
		return err
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
	if err := checkTokenDir(filepath.Dir(path)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	// O_NOFOLLOW refuses a symlinked token file; the checks below run on the
	// opened descriptor so a swap between check and read cannot slip through.
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("MCP token file is not a regular, unlinked file: %w", err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("MCP token path is not a regular file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", errors.New("MCP token file permissions are too open; expected 0600")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return "", errors.New("MCP token file is owned by another user")
	}
	raw, err := io.ReadAll(io.LimitReader(f, 1024))
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(raw)), nil
}

// checkTokenDir refuses a token directory that is a symlink, is not ours, or
// is accessible to other users.
func checkTokenDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("MCP token directory is not a plain directory")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("MCP token directory permissions are too open; expected 0700")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return errors.New("MCP token directory is owned by another user")
	}
	return nil
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
