// Package ghtest provides a fake GitHub CLI for tests: a shell script that
// answers chosen endpoints with canned output and logs every invocation.
package ghtest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Route answers an endpoint (shell glob over the request path plus query) with
// Out on stdout or, when Code != 0, Stderr and that exit code. Hang makes the
// call block until it is killed.
type Route struct {
	Glob   string
	Out    string
	Stderr string
	Code   int
	Hang   bool
}

// Script writes the fake gh and returns its path and its call log. Each log
// line is "GH_HOST|<args>". GET /user is answered automatically (signed in as
// octocat) or fails like a signed-out gh.
func Script(t testing.TB, signedIn bool, routes ...Route) (binary, log string) {
	t.Helper()
	login := "octocat"
	if !signedIn {
		login = ""
	}
	return ScriptAs(t, login, routes...)
}

// ScriptAs is Script signed in as login ("" means signed out).
func ScriptAs(t testing.TB, login string, routes ...Route) (binary, log string) {
	t.Helper()
	dir := t.TempDir()
	log = filepath.Join(dir, "calls.log")
	var cases strings.Builder
	for i, r := range routes {
		outFile, errFile := filepath.Join(dir, fmt.Sprintf("out%d", i)), filepath.Join(dir, fmt.Sprintf("err%d", i))
		if err := os.WriteFile(outFile, []byte(r.Out), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(errFile, []byte(r.Stderr), 0o600); err != nil {
			t.Fatal(err)
		}
		body := fmt.Sprintf(`cat %q; cat %q >&2; exit %d`, outFile, errFile, r.Code)
		if r.Hang {
			body = "exec sleep 30"
		}
		fmt.Fprintf(&cases, "  %s) %s ;;\n", r.Glob, body)
	}
	user := `/user) echo '{"login":"` + login + `","html_url":"https://github.com/` + login + `"}' ;;`
	if login == "" {
		user = `/user) echo 'To get started with GitHub CLI, please run:  gh auth login' >&2; exit 4 ;;`
	}
	script := "#!/bin/sh\necho \"$GH_HOST|$*\" >> " + fmt.Sprintf("%q", log) + "\n" +
		"for last; do :; done\ncase \"$last\" in\n  " + user + "\n" + cases.String() +
		"  *) echo 'gh: Not Found (HTTP 404)' >&2; exit 1 ;;\nesac\n"
	binary = filepath.Join(dir, "gh")
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, log
}

// Install makes AETHER find the fake gh (via AETHER_GH_PATH) for this test.
func Install(t *testing.T, signedIn bool, routes ...Route) (log string) {
	t.Helper()
	binary, log := Script(t, signedIn, routes...)
	t.Setenv("AETHER_GH_PATH", binary)
	return log
}

// InstallAs is Install signed in as a different login.
func InstallAs(t *testing.T, login string, routes ...Route) (log string) {
	t.Helper()
	binary, log := ScriptAs(t, login, routes...)
	t.Setenv("AETHER_GH_PATH", binary)
	return log
}

// Missing makes AETHER behave as if gh is not installed.
func Missing(t *testing.T) { t.Helper(); t.Setenv("AETHER_GH_PATH", "") }

// APICalls returns the endpoint of every api call in the log, excluding the
// /user session check.
func APICalls(log string) []string {
	b, _ := os.ReadFile(log)
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || !strings.Contains(line, "|api ") {
			continue
		}
		if last := fields[len(fields)-1]; last != "/user" {
			out = append(out, last)
		}
	}
	return out
}
