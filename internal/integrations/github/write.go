package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GitHub writes through the user's own gh, and only these three.
//
// This is the second half of the "GitHub write approvals" design in AGENTS.md.
// Nothing here decides *whether* to write: the store hands over a proposal only
// after a person approved its exact content. This file makes the write
// narrow and verifiable:
//
//   - the operation is an enum; method, path template and JSON shape are fixed
//     in code, and the path is re-checked against an allow-list before exec;
//   - the body goes to gh on stdin (`--input -`), never as -f/-F fields, so it
//     cannot inject flags or trigger gh's @file expansion;
//   - the owner/name come from GET /repositories/{id}, not from the proposal;
//   - everything that can be checked with a read is checked first (scopes,
//     rate limit, archived repo, locked issue, branches, duplicate PR);
//   - a POST is never retried, and an unclear outcome is reported as
//     uncertain so the person resolves it on GitHub.

type WriteKind string

const (
	WriteIssueComment WriteKind = "issue_comment"
	WriteIssueCreate  WriteKind = "issue_create"
	WritePRDraft      WriteKind = "pr_draft"
)

const (
	maxWriteTitleBytes    = 256
	maxWriteBodyBytes     = 65536
	maxWriteResponseBytes = 512 * 1024
	minRateLimitRemaining = 3
)

var (
	ErrMissingScope = errors.New("the GitHub CLI token cannot write to this repository; run `gh auth refresh -s repo` in Terminal")
	ErrWriteInvalid = errors.New("the GitHub write request is invalid")
)

// WriteRequest is a typed, already-approved write.
type WriteRequest struct {
	Kind         WriteKind
	RepositoryID int64
	Target       int64 // issue or pull request number, comments only
	Title        string
	Body         string
	Head         string // pull request source branch (same repository)
	Base         string
}

type WriteResult struct {
	URL    string
	Number int64
}

// WriteError reports a failure after the request may have reached GitHub.
type WriteError struct {
	Message string
	// Uncertain is true when GitHub may have applied the write. The caller must
	// not retry; the person checks GitHub and resolves the proposal.
	Uncertain bool
}

func (e *WriteError) Error() string { return e.Message }

// IsUncertainWrite reports whether err leaves the write's outcome unknown.
func IsUncertainWrite(err error) bool {
	var we *WriteError
	return errors.As(err, &we) && we.Uncertain
}

var writePathPattern = regexp.MustCompile(`^/repos/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/(issues|issues/[0-9]+/comments|pulls)$`)

type writeRepository struct {
	ID       int64  `json:"id"`
	FullName string `json:"full_name"`
	Private  bool   `json:"private"`
	Archived bool   `json:"archived"`
	Disabled bool   `json:"disabled"`
	Issues   bool   `json:"has_issues"`
	Owner    struct {
		Login string `json:"login"`
	} `json:"owner"`
	Name string `json:"name"`
}

func (r WriteRequest) validate() error {
	if r.RepositoryID <= 0 {
		return fmt.Errorf("%w: repository", ErrWriteInvalid)
	}
	if len(r.Title) > maxWriteTitleBytes || len(r.Body) > maxWriteBodyBytes ||
		strings.ContainsAny(r.Title, "\r\n\x00") || strings.ContainsRune(r.Body, 0) {
		return fmt.Errorf("%w: content", ErrWriteInvalid)
	}
	switch r.Kind {
	case WriteIssueComment:
		if r.Target <= 0 || strings.TrimSpace(r.Body) == "" || r.Title != "" || r.Head != "" || r.Base != "" {
			return fmt.Errorf("%w: comment", ErrWriteInvalid)
		}
	case WriteIssueCreate:
		if strings.TrimSpace(r.Title) == "" || r.Target != 0 || r.Head != "" || r.Base != "" {
			return fmt.Errorf("%w: issue", ErrWriteInvalid)
		}
	case WritePRDraft:
		if strings.TrimSpace(r.Title) == "" || r.Target != 0 || !validBranch(r.Head) || !validBranch(r.Base) || r.Head == r.Base {
			return fmt.Errorf("%w: pull request", ErrWriteInvalid)
		}
	default:
		return fmt.Errorf("%w: operation", ErrWriteInvalid)
	}
	return nil
}

var branchPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

func validBranch(ref string) bool {
	return len(ref) <= 255 && branchPattern.MatchString(ref) && !strings.Contains(ref, "..") &&
		!strings.Contains(ref, "//") && !strings.HasSuffix(ref, "/") && !strings.HasSuffix(ref, ".") &&
		!strings.HasSuffix(ref, ".lock") && !strings.Contains(ref, "/.")
}

// apiWithHeaders performs one read-only GET with `-i` so response headers
// (token scopes, rate limit) are available. Output is "HTTP/x 200 OK\n
// Header: v\n...\n\nbody".
func (g *ghRunner) apiWithHeaders(ctx context.Context, path string, target any) (http.Header, error) {
	stdout, stderr, err := g.run(ctx, maxAPIResponseBytes, "api", "--method", "GET", "-i",
		"-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: "+APIVersion, path)
	if err != nil {
		return nil, g.classifyReadError(ctx, stderr, err)
	}
	normalized := strings.ReplaceAll(stdout, "\r\n", "\n")
	head, body, found := strings.Cut(normalized, "\n\n")
	if !found {
		return nil, errors.New("GitHub returned an invalid API response")
	}
	headers := http.Header{}
	for _, line := range strings.Split(head, "\n")[1:] {
		if name, value, ok := strings.Cut(line, ":"); ok {
			headers.Add(strings.TrimSpace(name), strings.TrimSpace(value))
		}
	}
	if err := json.Unmarshal([]byte(body), target); err != nil {
		return nil, errors.New("GitHub returned an invalid API response")
	}
	return headers, nil
}

func (g *ghRunner) classifyReadError(ctx context.Context, stderr string, err error) error {
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
	return ErrAPIUnavailable
}

// CheckWrite verifies, using reads only, that the approved write can go out
// as the pinned account. It returns the repository's current full name.
func (s *Service) CheckWrite(ctx context.Context, expectedLogin string, req WriteRequest) (string, error) {
	var fullName string
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		name, err := s.checkWrite(ctx, client, expectedLogin, req)
		fullName = name
		return err
	})
	return fullName, err
}

func (s *Service) checkWrite(ctx context.Context, client *APIClient, expectedLogin string, req WriteRequest) (string, error) {
	if err := req.validate(); err != nil {
		return "", err
	}
	if login := s.gh.session(ctx); expectedLogin == "" || !strings.EqualFold(login, expectedLogin) {
		return "", ErrAccountChanged
	}
	var repo writeRepository
	headers, err := client.gh.apiWithHeaders(ctx, "/repositories/"+strconv.FormatInt(req.RepositoryID, 10), &repo)
	if err != nil {
		return "", err
	}
	owner, name, ok := strings.Cut(repo.FullName, "/")
	if repo.ID != req.RepositoryID || !ok || !validName(owner) || !validName(name) {
		return "", ErrPermissionDenied
	}
	if repo.Archived || repo.Disabled {
		return "", errors.New("the repository is archived or disabled; GitHub does not accept writes there")
	}
	if scopes, present := headers["X-Oauth-Scopes"]; present {
		if !hasWriteScope(scopes, repo.Private) {
			return "", ErrMissingScope
		}
	}
	if left, err := strconv.Atoi(headers.Get("X-Ratelimit-Remaining")); err == nil && left < minRateLimitRemaining {
		return "", ErrRateLimited
	}
	base := "/repos/" + owner + "/" + name
	switch req.Kind {
	case WriteIssueComment:
		var issue struct {
			Number int64 `json:"number"`
			Locked bool  `json:"locked"`
		}
		if err := client.getJSON(ctx, base+"/issues/"+strconv.FormatInt(req.Target, 10), nil, &issue); err != nil {
			return "", err
		}
		if issue.Number != req.Target {
			return "", ErrNotFound
		}
		if issue.Locked {
			return "", errors.New("that conversation is locked; GitHub does not accept new comments")
		}
	case WriteIssueCreate:
		if !repo.Issues {
			return "", errors.New("issues are disabled in that repository")
		}
	case WritePRDraft:
		for _, branch := range []string{req.Head, req.Base} {
			var ref struct {
				Ref string `json:"ref"`
			}
			if err := client.getJSON(ctx, base+"/git/ref/heads/"+branch, nil, &ref); err != nil {
				if errors.Is(err, ErrNotFound) {
					return "", fmt.Errorf("branch %q does not exist in %s; push it to GitHub first", branch, repo.FullName)
				}
				return "", err
			}
		}
		var open []struct {
			HTMLURL string `json:"html_url"`
		}
		query := url.Values{"head": {owner + ":" + req.Head}, "state": {"open"}, "per_page": {"1"}}
		if err := client.gh.api(ctx, base+"/pulls", query, &open); err != nil {
			return "", err
		}
		if len(open) > 0 {
			return "", fmt.Errorf("an open pull request already exists for %q: %s", req.Head, safeGitHubURL(open[0].HTMLURL))
		}
	}
	return repo.FullName, nil
}

func hasWriteScope(scopes []string, private bool) bool {
	for _, header := range scopes {
		for _, scope := range strings.Split(header, ",") {
			switch strings.TrimSpace(scope) {
			case "repo":
				return true
			case "public_repo":
				if !private {
					return true
				}
			}
		}
	}
	return false
}

// Write checks and then sends one approved write as the pinned account. It
// never retries. Errors after the POST started are *WriteError.
func (s *Service) Write(ctx context.Context, expectedLogin string, req WriteRequest) (WriteResult, error) {
	var result WriteResult
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		fullName, err := s.checkWrite(ctx, client, expectedLogin, req)
		if err != nil {
			return err
		}
		result, err = s.send(ctx, client.gh, expectedLogin, fullName, req)
		return err
	})
	return result, err
}

func (s *Service) send(ctx context.Context, gh *ghRunner, login, fullName string, req WriteRequest) (WriteResult, error) {
	path, payload, err := buildWrite(fullName, req)
	if err != nil {
		return WriteResult{}, err
	}
	stdout, stderr, runErr := gh.runInput(ctx, maxWriteResponseBytes, payload, "api", "--method", "POST",
		"-H", "Accept: application/vnd.github+json", "-H", "X-GitHub-Api-Version: "+APIVersion,
		"--input", "-", path)
	if runErr != nil {
		return WriteResult{}, classifyWriteError(ctx, gh, stderr, runErr)
	}
	result, err := verifyWriteResponse(stdout, login, fullName, req)
	if err != nil {
		return WriteResult{}, &WriteError{Uncertain: true, Message: err.Error() + "; check GitHub to confirm what was created"}
	}
	return result, nil
}

// buildWrite returns the fixed endpoint and the JSON body for a request. The
// only variable parts are the validated repository name, the issue number and
// the approved text.
func buildWrite(fullName string, req WriteRequest) (string, []byte, error) {
	owner, name, ok := strings.Cut(fullName, "/")
	if !ok || !validName(owner) || !validName(name) {
		return "", nil, ErrWriteInvalid
	}
	base := "/repos/" + owner + "/" + name
	var path string
	var body any
	switch req.Kind {
	case WriteIssueComment:
		path = base + "/issues/" + strconv.FormatInt(req.Target, 10) + "/comments"
		body = struct {
			Body string `json:"body"`
		}{req.Body}
	case WriteIssueCreate:
		path = base + "/issues"
		body = struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}{req.Title, req.Body}
	case WritePRDraft:
		path = base + "/pulls"
		body = struct {
			Title string `json:"title"`
			Body  string `json:"body"`
			Head  string `json:"head"`
			Base  string `json:"base"`
			Draft bool   `json:"draft"`
		}{req.Title, req.Body, req.Head, req.Base, true}
	default:
		return "", nil, ErrWriteInvalid
	}
	if !writePathPattern.MatchString(path) {
		return "", nil, ErrWriteInvalid
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}
	return path, raw, nil
}

func classifyWriteError(ctx context.Context, gh *ghRunner, stderr string, runErr error) error {
	var exit *exec.ExitError
	if !errors.As(runErr, &exit) && ctx.Err() == nil && !errors.Is(runErr, ErrResponseTooLarge) {
		// gh never started, so nothing was sent.
		return ErrAPIUnavailable
	}
	if ctx.Err() != nil || errors.Is(runErr, ErrResponseTooLarge) {
		return &WriteError{Uncertain: true, Message: "the connection ended before GitHub confirmed the write; check GitHub, then resolve it"}
	}
	m := ghHTTPStatus.FindStringSubmatch(stderr)
	if m == nil {
		return &WriteError{Uncertain: true, Message: "GitHub did not confirm the write; check GitHub, then resolve it"}
	}
	status, _ := strconv.Atoi(m[1])
	detail := firstLine(stderr)
	switch {
	case status == http.StatusUnauthorized:
		gh.forgetSession()
		return ErrSessionExpired
	case status == http.StatusTooManyRequests || status == http.StatusForbidden && strings.Contains(strings.ToLower(stderr), "rate limit"):
		return ErrRateLimited
	case status == http.StatusForbidden:
		return &WriteError{Message: "GitHub refused the write: the GitHub CLI account lacks permission, SSO authorization is required, or the conversation is locked"}
	case status == http.StatusNotFound:
		return &WriteError{Message: "GitHub could not find the target, or the GitHub CLI account cannot write there"}
	case status == http.StatusGone:
		return &WriteError{Message: "issues are disabled in that repository"}
	case status == http.StatusUnprocessableEntity:
		return &WriteError{Message: "GitHub rejected the content: " + detail}
	case status >= 500:
		return &WriteError{Uncertain: true, Message: fmt.Sprintf("GitHub answered HTTP %d; the write may have been applied, check GitHub then resolve it", status)}
	default:
		return &WriteError{Message: fmt.Sprintf("GitHub refused the write (HTTP %d)", status)}
	}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return safeExternalText(line, 200)
}

// verifyWriteResponse trusts nothing in the reply: the link must point into
// the expected repository, the author must be the pinned account, and a pull
// request must really be a draft.
func verifyWriteResponse(stdout, login, fullName string, req WriteRequest) (WriteResult, error) {
	var reply struct {
		HTMLURL string `json:"html_url"`
		Number  int64  `json:"number"`
		Draft   *bool  `json:"draft"`
		User    struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal([]byte(stdout), &reply); err != nil {
		return WriteResult{}, errors.New("GitHub returned an unreadable confirmation")
	}
	parsed, err := url.Parse(reply.HTMLURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || len(reply.HTMLURL) > 512 {
		return WriteResult{}, errors.New("GitHub returned an unexpected link")
	}
	prefix := "/" + fullName + "/"
	if len(parsed.Path) <= len(prefix) || !strings.EqualFold(parsed.Path[:len(prefix)], prefix) {
		return WriteResult{}, errors.New("GitHub returned a link outside the expected repository")
	}
	rest := parsed.Path[len(prefix):]
	number := reply.Number
	switch req.Kind {
	case WriteIssueComment:
		number = req.Target
		if !strings.HasPrefix(rest, "issues/") && !strings.HasPrefix(rest, "pull/") || !strings.HasPrefix(parsed.Fragment, "issuecomment-") {
			return WriteResult{}, errors.New("GitHub returned an unexpected comment link")
		}
	case WriteIssueCreate:
		if !strings.HasPrefix(rest, "issues/") || number <= 0 {
			return WriteResult{}, errors.New("GitHub returned an unexpected issue link")
		}
	case WritePRDraft:
		if !strings.HasPrefix(rest, "pull/") || number <= 0 {
			return WriteResult{}, errors.New("GitHub returned an unexpected pull request link")
		}
		if reply.Draft == nil || !*reply.Draft {
			return WriteResult{}, errors.New("GitHub did not create the pull request as a draft")
		}
	}
	if !strings.EqualFold(reply.User.Login, login) {
		return WriteResult{}, errors.New("GitHub reports a different author than the connected account")
	}
	return WriteResult{URL: reply.HTMLURL, Number: number}, nil
}
