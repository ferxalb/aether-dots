package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeGH writes a shell script that stands in for gh. It logs its arguments and
// GH_HOST, prints canned JSON for known paths, and fails like gh otherwise.
func fakeGH(t *testing.T, authOK bool) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	auth := "exit 0"
	if !authOK {
		auth = "echo 'You are not logged into any GitHub hosts. To log in, run: gh auth login' >&2; exit 1"
	}
	script := `#!/bin/sh
echo "$GH_HOST|$*" >> "` + log + `"
if [ "$1" = "auth" ]; then ` + auth + `; fi
for last; do :; done
case "$last" in
  /user) echo '{"login":"octocat","html_url":"https://github.com/octocat"}' ;;
  /user/repos*) echo '[{"id":7,"name":"hello","full_name":"octocat/hello","html_url":"https://github.com/octocat/hello","private":true,"default_branch":"main","owner":{"login":"octocat"}}]' ;;
  /repositories/7) echo '{"id":7}' ;;
  /repos/octocat/hello) echo '{"id":7,"name":"hello","full_name":"octocat/hello","default_branch":"main","html_url":"https://github.com/octocat/hello"}' ;;
  /repos/octocat/limited*) echo 'gh: API rate limit exceeded (HTTP 403)' >&2; exit 1 ;;
  /repos/octocat/big*) head -c 3000000 /dev/zero | tr '\0' 'x' ;;
  *) echo 'gh: Not Found (HTTP 404)' >&2; exit 1 ;;
esac
`
	path := filepath.Join(dir, "gh")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	s := &Service{gh: &ghRunner{path: path}}
	return s, log
}

func TestCLIServiceDiscoversAndReadsThroughGH(t *testing.T) {
	s, log := fakeGH(t, true)
	ctx := context.Background()
	if !s.ClientConfigured() || !s.Connected(ctx) {
		t.Fatal("gh service should be configured and connected")
	}
	if err := s.AuthorizeDevice(ctx, func(DeviceAuthorization) { t.Fatal("no device code in gh mode") }); err != nil {
		t.Fatal(err)
	}
	d, err := s.Discover(ctx)
	if err != nil || d.User.Login != "octocat" || len(d.Repositories) != 1 || d.Repositories[0].InstallationID != CLIInstallationID {
		t.Fatalf("discovery=%#v err=%v", d, err)
	}
	if err := s.RepositoryStillAccessible(ctx, CLIInstallationID, 7); err != nil {
		t.Fatal(err)
	}
	if err := s.RepositoryStillAccessible(ctx, CLIInstallationID, 8); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("revoked repository: %v", err)
	}
	meta, err := s.GetRepository(ctx, "octocat", "hello")
	if err != nil || meta.FullName != "octocat/hello" {
		t.Fatalf("meta=%#v err=%v", meta, err)
	}
	calls, _ := os.ReadFile(log)
	for _, line := range strings.Split(strings.TrimSpace(string(calls)), "\n") {
		if strings.HasPrefix(line, "github.com|api ") && !strings.Contains(line, "--method GET") {
			t.Fatalf("api call without a pinned GET: %s", line)
		}
		if !strings.HasPrefix(line, "github.com|") {
			t.Fatalf("GH_HOST was not pinned: %s", line)
		}
	}
}

func TestCLIServiceErrors(t *testing.T) {
	ctx := context.Background()
	s, _ := fakeGH(t, false)
	if s.Connected(ctx) {
		t.Fatal("signed-out gh must not report connected")
	}
	if err := s.AuthorizeDevice(ctx, nil); !errors.Is(err, ErrCLINotAuthenticated) {
		t.Fatalf("authorize while signed out: %v", err)
	}
	s, _ = fakeGH(t, true)
	if _, err := s.GetRepository(ctx, "octocat", "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("404 mapping: %v", err)
	}
	if _, err := s.GetRepository(ctx, "octocat", "limited"); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("rate limit mapping: %v", err)
	}
	if _, err := s.GetRepository(ctx, "octocat", "bigone"); !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("oversize output: %v", err)
	}
	s.CancelActiveRequests()
	if _, err := s.GetRepository(ctx, "octocat", "hello"); !errors.Is(err, ErrNotConnected) {
		t.Fatalf("after disconnect: %v", err)
	}
}

func TestCLIPathValidation(t *testing.T) {
	c := &APIClient{gh: &ghRunner{path: "/bin/false"}}
	for _, bad := range []string{"user", "//evil/x", "/a/../b", "/a b", "/a\nb", "-H"} {
		if err := c.getJSON(context.Background(), bad, nil, new(any)); err == nil || errors.Is(err, ErrNotConnected) {
			t.Fatalf("path %q was accepted: %v", bad, err)
		}
	}
}
