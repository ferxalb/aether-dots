package mcp

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTokensAreStoredPrivatelyAndRejectTraversal(t *testing.T) {
	dir := t.TempDir()
	if err := SaveToken(dir, "srv1", "mdn_secret\n"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "mcp-tokens", "srv1"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%v err=%v", info.Mode(), err)
	}
	if got, _ := LoadToken(dir, "srv1"); got != "mdn_secret" {
		t.Fatalf("got %q", got)
	}
	for _, id := range []string{"../x", "a/b", "", ".."} {
		if SaveToken(dir, id, "t") == nil {
			t.Fatalf("id %q accepted", id)
		}
	}
	for _, token := range []string{"", "has space", "line\nbreak"} {
		if SaveToken(dir, "ok", token) == nil {
			t.Fatalf("token %q accepted", token)
		}
	}
	if err := os.Chmod(filepath.Join(dir, "mcp-tokens", "srv1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(dir, "srv1"); err == nil {
		t.Fatal("world-readable token file must fail closed")
	}
	if err := DeleteToken(dir, "srv1"); err != nil {
		t.Fatal(err)
	}
	if got, _ := LoadToken(dir, "srv1"); got != "" {
		t.Fatal("token survived delete")
	}
}

func TestConnectReportsUnauthorizedWithoutLeakingTheToken(t *testing.T) {
	var seen string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Header.Get("Authorization")
		http.Error(w, "authentication required", http.StatusUnauthorized)
	}))
	defer server.Close()
	_, err := Connect(t.Context(), "test", server.URL+"/mcp", WithBearer("mdn_topsecret"))
	if err != ErrUnauthorized {
		t.Fatalf("err=%v", err)
	}
	if seen != "Bearer mdn_topsecret" {
		t.Fatalf("header=%q", seen)
	}
	if strings.Contains(err.Error(), "topsecret") {
		t.Fatal("error leaks the token")
	}
}

func TestLoadTokenRefusesSymlinkAndOpenDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := SaveToken(dir, "srv", "mdn_secret"); err != nil {
		t.Fatal(err)
	}
	real := filepath.Join(dir, "mcp-tokens", "srv")
	// A symlinked token file is refused even when its target looks fine.
	target := filepath.Join(dir, "elsewhere")
	if err := os.WriteFile(target, []byte("mdn_other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, real); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadToken(dir, "srv"); err == nil || got != "" {
		t.Fatalf("symlinked token must fail closed, got %q %v", got, err)
	}
	// Rotation replaces the link with a private regular file.
	if err := SaveToken(dir, "srv", "mdn_rotated"); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadToken(dir, "srv"); err != nil || got != "mdn_rotated" {
		t.Fatalf("rotated token = %q, %v", got, err)
	}
	// A group/world accessible directory fails closed for load and save.
	if err := os.Chmod(filepath.Dir(real), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(dir, "srv"); err == nil {
		t.Fatal("open token directory must be refused")
	}
	if err := SaveToken(dir, "srv", "mdn_x"); err == nil {
		t.Fatal("saving into an open token directory must be refused")
	}
}
