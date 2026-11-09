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

// GitHub through the user's own GitHub CLI.
//
// AETHER does not hold a GitHub token in this mode: `gh` owns the login (its
// keyring), and AETHER only runs `gh api --method GET <path>` with a fixed
// argument shape. Dots still cannot choose a binary, a method, a host, flags or
// a path outside the existing typed read operations; every call also keeps the
// per-Dot repository grant and disclosure checks in the engine.

// CLIInstallationID is the stand-in installation ID stored for repositories
// reached through gh. A gh login has no GitHub App installation; the store only
// requires a positive value.
const CLIInstallationID int64 = 1

var (
	ErrCLINotInstalled     = errors.New("GitHub CLI (gh) is not installed. Install it from https://cli.github.com, then run `gh auth login`")
	ErrCLINotAuthenticated = errors.New("GitHub CLI is not signed in. Run `gh auth login` in Terminal, then connect again")
)

var ghHTTPStatus = regexp.MustCompile(`\(HTTP (\d{3})\)`)

type ghRunner struct {
	path string

	mu       sync.Mutex
	authAt   time.Time
	authGood bool
}

// findGH locates the gh binary. GUI apps start with a minimal PATH, so the
// usual Homebrew and system locations are checked explicitly.
func findGH() (string, bool) {
	candidates := []string{"/opt/homebrew/bin/gh", "/usr/local/bin/gh", "/usr/bin/gh"}
	if p, err := exec.LookPath("gh"); err == nil {
		candidates = append([]string{p}, candidates...)
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
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, g.path, args...)
	cmd.Env = ghEnvironment()
	cmd.Stdin = nil
	out, errOut := &limitedBuffer{limit: limit}, &limitedBuffer{limit: 16 * 1024}
	cmd.Stdout, cmd.Stderr = out, errOut
	err := cmd.Run()
	if out.over {
		return "", "", ErrResponseTooLarge
	}
	return out.buf.String(), errOut.buf.String(), err
}

// authenticated reports whether gh has a github.com login. The answer is
// cached briefly so tool calls do not each spawn two processes.
func (g *ghRunner) authenticated(ctx context.Context) bool {
	g.mu.Lock()
	if time.Since(g.authAt) < 20*time.Second {
		good := g.authGood
		g.mu.Unlock()
		return good
	}
	g.mu.Unlock()
	_, _, err := g.run(ctx, 64*1024, "auth", "status", "--hostname", "github.com")
	good := err == nil
	if ctx.Err() == nil {
		g.mu.Lock()
		g.authAt, g.authGood = time.Now(), good
		g.mu.Unlock()
	}
	return good
}

func (g *ghRunner) forgetAuth() {
	g.mu.Lock()
	g.authAt = time.Time{}
	g.mu.Unlock()
}

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
			lower := strings.ToLower(stderr)
			if status == 429 || status == 403 && strings.Contains(lower, "rate limit") {
				apiErr.RetryAfter = time.Minute
			}
			if status == 401 {
				g.forgetAuth()
			}
			return apiErr
		}
		if strings.Contains(strings.ToLower(stderr), "gh auth login") || strings.Contains(strings.ToLower(stderr), "authentication") {
			g.forgetAuth()
			return ErrCLINotAuthenticated
		}
		return ErrAPIUnavailable
	}
	if err := json.Unmarshal([]byte(stdout), target); err != nil {
		return errors.New("GitHub returned an invalid API response")
	}
	return nil
}

// NewCLIService returns a Service backed by the GitHub CLI, or ok=false when gh
// is not installed.
func NewCLIService() (*Service, bool) {
	path, ok := findGH()
	if !ok {
		return nil, false
	}
	return &Service{gh: &ghRunner{path: path}, now: time.Now, operations: make(map[uint64]context.CancelFunc)}, true
}

// NewDefaultService prefers the GitHub CLI when it is installed and otherwise
// keeps the GitHub App device flow. Callers use the same Service either way.
func NewDefaultService(vault CredentialStore) *Service {
	if s, ok := NewCLIService(); ok {
		return s
	}
	return NewService(ConfiguredClientID(), vault, nil)
}

// UsesCLI reports whether this service reads GitHub through gh.
func (s *Service) UsesCLI() bool { return s != nil && s.gh != nil }

// discoverCLI lists the repositories the gh login can read.
func (s *Service) discoverCLI(ctx context.Context) (Discovery, error) {
	var out Discovery
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		user, err := client.GetAuthenticatedUser(ctx)
		if err != nil {
			return err
		}
		out.User = user
		for page := 1; page <= maxPages; page++ {
			query := url.Values{
				"per_page": {strconv.Itoa(maxPageSize)}, "page": {strconv.Itoa(page)},
				"sort": {"pushed"}, "affiliation": {"owner,collaborator,organization_member"},
			}
			var repos []Repository
			if err := client.getJSON(ctx, "/user/repos", query, &repos); err != nil {
				return err
			}
			for _, repo := range repos {
				if len(out.Repositories) == 2000 {
					out.Truncated = true
					return nil
				}
				repo.InstallationID = CLIInstallationID
				out.Repositories = append(out.Repositories, cleanRepository(repo))
			}
			if len(repos) < maxPageSize {
				return nil
			}
		}
		out.Truncated = true
		return nil
	})
	return out, err
}

// repositoryAccessibleCLI confirms the gh login can still read a repository.
func (c *APIClient) repositoryAccessibleCLI(ctx context.Context, repositoryID int64) (bool, error) {
	if repositoryID <= 0 {
		return false, errors.New("GitHub repository identity is invalid")
	}
	var repo struct {
		ID int64 `json:"id"`
	}
	err := c.getJSON(ctx, "/repositories/"+strconv.FormatInt(repositoryID, 10), nil, &repo)
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermissionDenied) {
		return false, nil
	}
	return err == nil && repo.ID == repositoryID, err
}
