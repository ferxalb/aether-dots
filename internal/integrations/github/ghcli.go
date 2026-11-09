package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// GitHub through the user's own GitHub CLI, and nothing else.
//
// AETHER holds no GitHub token: `gh` owns the login (its keyring). The only
// process started is the located gh binary, with `api --method GET -H ...
// <validated /path>`; arguments, method and host (GH_HOST=github.com, minimal
// environment) are fixed in code and never come from a model. Output is size
// capped. Every call still passes through the typed read operations and the
// per-Dot repository grants in the engine.

var (
	ErrNotConnected        = errors.New("GitHub is not connected")
	ErrSessionExpired      = errors.New("GitHub CLI session expired; run `gh auth login` in Terminal, then connect again")
	ErrAccountChanged      = errors.New("the GitHub CLI account changed since AETHER was connected; connect GitHub again to use the new account")
	ErrCLINotInstalled     = errors.New("GitHub CLI (gh) is not installed. Install it from https://cli.github.com, then run `gh auth login`")
	ErrCLINotAuthenticated = errors.New("GitHub CLI is not signed in. Run `gh auth login` in Terminal, then connect again")
)

var ghHTTPStatus = regexp.MustCompile(`\(HTTP (\d{3})\)`)

const sessionCacheTTL = 20 * time.Second

type ghRunner struct {
	path string

	mu        sync.Mutex
	checkedAt time.Time
	login     string
}

// findGH locates the gh binary. GUI apps start with a minimal PATH, so the
// usual Homebrew and system locations are checked explicitly.
func findGH() (string, bool) {
	var candidates []string
	if override, ok := os.LookupEnv("AETHER_GH_PATH"); ok {
		// An explicit path is used exclusively; an empty value means "no gh".
		if override == "" {
			return "", false
		}
		candidates = []string{override}
	} else {
		candidates = []string{"/opt/homebrew/bin/gh", "/usr/local/bin/gh", "/usr/bin/gh"}
		if p, err := exec.LookPath("gh"); err == nil {
			candidates = append([]string{p}, candidates...)
		}
	}
	for _, p := range candidates {
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return abs, true
		}
	}
	return "", false
}

// ghEnvironment is deliberately small: gh finds its own config through HOME,
// and the host is pinned so a stray GH_HOST cannot redirect requests.
func ghEnvironment() []string {
	env := []string{
		"GH_HOST=github.com", "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1",
		"GH_NO_EXTENSION_UPDATE_NOTIFIER=1", "NO_COLOR=1", "GH_SPINNER_DISABLED=1",
	}
	for _, key := range []string{"HOME", "PATH", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "GH_CONFIG_DIR", "GH_TOKEN", "GITHUB_TOKEN", "TMPDIR", "USER", "LANG"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}

type limitedBuffer struct {
	buf   bytes.Buffer
	limit int
	over  bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.buf.Len()+len(p) > b.limit {
		b.over = true
		return len(p), nil
	}
	return b.buf.Write(p)
}

func (g *ghRunner) run(ctx context.Context, limit int, args ...string) (string, string, error) {
	return g.runInput(ctx, limit, nil, args...)
}

// runInput is run with an optional stdin payload. Only the typed write path
// passes one (a request body built in code, never model-chosen arguments).
func (g *ghRunner) runInput(ctx context.Context, limit int, stdin []byte, args ...string) (string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.path, args...)
	cmd.Env = ghEnvironment()
	cmd.Stdin = nil
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, errOut := &limitedBuffer{limit: limit}, &limitedBuffer{limit: 16 * 1024}
	cmd.Stdout, cmd.Stderr = out, errOut
	err := cmd.Run()
	if out.over {
		return "", "", ErrResponseTooLarge
	}
	return out.buf.String(), errOut.buf.String(), err
}

// session returns the signed-in login, or "" when gh is signed out. It calls
// GET /user, which proves the token works (not merely that a config exists),
// and is cached briefly so tool calls do not each spawn extra processes.
func (g *ghRunner) session(ctx context.Context) string {
	g.mu.Lock()
	if !g.checkedAt.IsZero() && time.Since(g.checkedAt) < sessionCacheTTL {
		login := g.login
		g.mu.Unlock()
		return login
	}
	g.mu.Unlock()
	var user struct {
		Login string `json:"login"`
	}
	login := ""
	if err := g.api(ctx, "/user", nil, &user); err == nil && validLogin(user.Login) {
		login = user.Login
	}
	if ctx.Err() == nil {
		g.mu.Lock()
		g.checkedAt, g.login = time.Now(), login
		g.mu.Unlock()
	}
	return login
}

func (g *ghRunner) forgetSession() {
	g.mu.Lock()
	g.checkedAt, g.login = time.Time{}, ""
	g.mu.Unlock()
}

func validLogin(v string) bool { return validName(v) }

// api performs one read-only GET. The path was already validated by getJSON.
func (g *ghRunner) api(ctx context.Context, path string, query url.Values, target any) error {
	endpoint := path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	stdout, stderr, err := g.run(ctx, maxAPIResponseBytes, "api", "--method", "GET",
		"-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: "+APIVersion, endpoint)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, ErrResponseTooLarge) {
			return err
		}
		if m := ghHTTPStatus.FindStringSubmatch(stderr); m != nil {
			status, _ := strconv.Atoi(m[1])
			apiErr := &APIError{StatusCode: status}
			if status == 429 || status == 403 && strings.Contains(strings.ToLower(stderr), "rate limit") {
				apiErr.RetryAfter = time.Minute
			}
			if status == 401 {
				g.forgetSession()
			}
			return apiErr
		}
		lower := strings.ToLower(stderr)
		if strings.Contains(lower, "gh auth login") || strings.Contains(lower, "authentication") {
			g.forgetSession()
			return ErrCLINotAuthenticated
		}
		return ErrAPIUnavailable
	}
	if err := json.Unmarshal([]byte(stdout), target); err != nil {
		return errors.New("GitHub returned an invalid API response")
	}
	return nil
}
