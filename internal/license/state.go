package license

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

const stateFile = "license.json"

// Load reads the saved license from dataDir. A missing file is (nil, nil).
// A symlink or a file readable by others is refused rather than trusted.
func Load(dataDir string) (*State, error) {
	path := filepath.Join(dataDir, stateFile)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s must be private (chmod 600)", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", path, err)
	}
	if s.Version != stateVersion {
		return nil, fmt.Errorf("%s has unsupported version %d", path, s.Version)
	}
	return &s, nil
}

// Save writes s to dataDir atomically with owner-only permissions.
func Save(dataDir string, s *State) error {
	if s == nil {
		return errors.New("no license state to save")
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dataDir, ".license-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dataDir, stateFile))
}

// Remove deletes the saved license, if any.
func Remove(dataDir string) error {
	err := os.Remove(filepath.Join(dataDir, stateFile))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
