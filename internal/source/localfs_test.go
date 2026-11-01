package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadFolderBoundsScopeAndSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "notes.md"), []byte("TODO: pick the default flow"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "image.png"), []byte("not text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "secret.md"), []byte("private"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outside, "outside.md"), []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "outside.md"), filepath.Join(root, "linked.md")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	_, docs, err := ReadFolder(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 1 || docs[0].Path != "notes.md" {
		t.Fatalf("unexpected documents: %#v", docs)
	}
	if docs[0].SHA256 == "" {
		t.Fatal("source evidence hash missing")
	}
}

func TestReadFolderRejectsOversizedText(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.txt"), make([]byte, maxFileBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadFolder(root); err == nil {
		t.Fatal("oversized file was accepted")
	}
}
