package source

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	maxFiles       = 32
	maxFileBytes   = 256 * 1024
	maxTotalBytes  = 2 * 1024 * 1024
	MaxOutputBytes = 2 * 1024 * 1024
)

type Document struct {
	Path   string `json:"path"`
	Text   string `json:"text"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// ReadFolder reads small, UTF-8 text documents directly beneath root. It
// never follows symbolic links and skips hidden/build/dependency folders.
func ReadFolder(root string) (string, []Document, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", nil, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", nil, fmt.Errorf("resolve source folder: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", nil, errors.New("source must be an available folder")
	}
	root = filepath.Clean(resolved)
	docs := make([]Document, 0)
	var total int64
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if strings.HasPrefix(name, ".") || skippedDirectory(name) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !supportedText(name) {
			return nil
		}
		if len(docs) >= maxFiles {
			return errors.New("source folder contains more than 32 supported text files")
		}
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("open %s: %w", filepath.Base(path), err)
		}
		b, readErr := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(b) > maxFileBytes {
			return fmt.Errorf("%s exceeds the 256 KiB per-file limit", filepath.Base(path))
		}
		if !utf8.Valid(b) {
			return fmt.Errorf("%s is not valid UTF-8 text", filepath.Base(path))
		}
		total += int64(len(b))
		if total > maxTotalBytes {
			return errors.New("source folder exceeds the 2 MiB total text limit")
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		docs = append(docs, Document{Path: filepath.ToSlash(rel), Text: string(b), SHA256: hex.EncodeToString(sum[:]), Bytes: int64(len(b))})
		return nil
	})
	if err != nil {
		return root, nil, err
	}
	if len(docs) == 0 {
		return root, nil, errors.New("no supported text files were found in the selected folder")
	}
	return root, docs, nil
}

func supportedText(name string) bool {
	if strings.HasPrefix(name, ".") {
		return false
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".mdx", ".txt", ".rst", ".org", ".csv", ".json":
		return true
	default:
		return false
	}
}

func skippedDirectory(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "vendor", ".next", "dist", "build", "target":
		return true
	default:
		return false
	}
}
