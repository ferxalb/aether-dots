package github

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	APIVersion          = "2026-03-10"
	maxAPIResponseBytes = 2 * 1024 * 1024
	maxPages            = 20
	maxPageSize         = 100
)

var (
	ErrPermissionDenied = errors.New("GitHub denied this read operation; check which repositories your GitHub CLI account can read")
	ErrNotFound         = errors.New("the GitHub resource was not found or is no longer accessible")
	ErrRateLimited      = errors.New("GitHub API rate limit reached; try again later")
	ErrAPIUnavailable   = errors.New("GitHub API is temporarily unavailable")
	ErrResponseTooLarge = errors.New("GitHub returned a response that exceeded the size limit")
)

type User struct {
	Login     string `json:"login"`
	Name      string `json:"name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
	HTMLURL   string `json:"html_url"`
}

type Repository struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch,omitempty"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
}

type RepositoryMetadata struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	FullName      string `json:"full_name"`
	Description   string `json:"description,omitempty"`
	HTMLURL       string `json:"html_url"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"default_branch"`
	UpdatedAt     string `json:"updated_at,omitempty"`
}

type Commit struct {
	SHA     string `json:"sha"`
	HTMLURL string `json:"html_url"`
	Commit  struct {
		Message string `json:"message"`
		Author  struct {
			Name string `json:"name"`
			Date string `json:"date"`
		} `json:"author"`
	} `json:"commit"`
}

type Issue struct {
	Number      int64  `json:"number"`
	Title       string `json:"title"`
	Body        string `json:"body,omitempty"`
	State       string `json:"state"`
	HTMLURL     string `json:"html_url"`
	CreatedAt   string `json:"created_at,omitempty"`
	UpdatedAt   string `json:"updated_at,omitempty"`
	PullRequest *struct {
		HTMLURL string `json:"html_url"`
	} `json:"pull_request,omitempty"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels,omitempty"`
}

type PullRequest struct {
	Number    int64  `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body,omitempty"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	Draft     bool   `json:"draft"`
	CreatedAt string `json:"created_at,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	RequestedReviewers []struct {
		Login string `json:"login"`
	} `json:"requested_reviewers,omitempty"`
}

type CheckRun struct {
	ID          int64   `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Conclusion  *string `json:"conclusion"`
	HTMLURL     string  `json:"html_url"`
	DetailsURL  string  `json:"details_url,omitempty"`
	StartedAt   string  `json:"started_at,omitempty"`
	CompletedAt string  `json:"completed_at,omitempty"`
}

type CheckRuns struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []CheckRun `json:"check_runs"`
}

type WorkflowRun struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	DisplayTitle string  `json:"display_title,omitempty"`
	HeadBranch   string  `json:"head_branch"`
	HeadSHA      string  `json:"head_sha"`
	Status       string  `json:"status"`
	Conclusion   *string `json:"conclusion"`
	HTMLURL      string  `json:"html_url"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

type WorkflowRuns struct {
	TotalCount   int           `json:"total_count"`
	WorkflowRuns []WorkflowRun `json:"workflow_runs"`
}

type WorkflowJob struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Status     string  `json:"status"`
	Conclusion *string `json:"conclusion"`
	HTMLURL    string  `json:"html_url"`
	Steps      []struct {
		Name       string  `json:"name"`
		Status     string  `json:"status"`
		Conclusion *string `json:"conclusion"`
	} `json:"steps,omitempty"`
}

type WorkflowJobs struct {
	TotalCount int           `json:"total_count"`
	Jobs       []WorkflowJob `json:"jobs"`
}

// APIClient is a short-lived authenticated client. Production construction is
// fixed to api.github.com and never follows redirects, so an Authorization
// header cannot be forwarded to another host.
// APIClient issues read-only GitHub REST requests through the GitHub CLI.
type APIClient struct {
	gh *ghRunner
}

// FetchAvatar retrieves only a small raster image from GitHub's avatar host.
// The request is unauthenticated, bounded, and never follows redirects.
func FetchAvatar(ctx context.Context, avatarURL string, httpClient *http.Client) ([]byte, error) {
	parsed, err := url.Parse(safeAvatarURL(avatarURL))
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Host != "avatars.githubusercontent.com" {
		return nil, errors.New("GitHub account avatar URL is not allowed")
	}
	client := noRedirectClient(httpClient, 5*time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return nil, errors.New("could not prepare GitHub avatar request")
	}
	req.Header.Set("Accept", "image/png,image/jpeg,image/gif")
	req.Header.Set("User-Agent", "AETHER-Dots")
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("could not load GitHub account avatar")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("GitHub account avatar is unavailable")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0]))
	if contentType != "image/png" && contentType != "image/jpeg" && contentType != "image/gif" {
		return nil, errors.New("GitHub account avatar format is not supported")
	}
	data, err := readBounded(response.Body, 256*1024)
	if err != nil {
		return nil, errors.New("GitHub account avatar exceeded the 256 KiB limit")
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 1024 || config.Height > 1024 {
		clear(data)
		return nil, errors.New("GitHub account avatar dimensions are invalid")
	}
	return data, nil
}

type APIError struct {
	StatusCode int
	RetryAfter time.Duration
}

func (e *APIError) Error() string {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return ErrSessionExpired.Error()
	case http.StatusForbidden:
		if e.RetryAfter > 0 {
			return fmt.Sprintf("%s (retry in %s)", ErrRateLimited, e.RetryAfter.Round(time.Second))
		}
		return ErrPermissionDenied.Error()
	case http.StatusTooManyRequests:
		if e.RetryAfter > 0 {
			return fmt.Sprintf("%s (retry in %s)", ErrRateLimited, e.RetryAfter.Round(time.Second))
		}
		return ErrRateLimited.Error()
	case http.StatusNotFound:
		return ErrNotFound.Error()
	default:
		if e.StatusCode >= 500 {
			return fmt.Sprintf("%s (HTTP %d)", ErrAPIUnavailable, e.StatusCode)
		}
		return fmt.Sprintf("GitHub API request failed (HTTP %d)", e.StatusCode)
	}
}

func (e *APIError) Is(target error) bool {
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return target == ErrSessionExpired
	case http.StatusNotFound:
		return target == ErrNotFound
	case http.StatusForbidden:
		if e.RetryAfter > 0 {
			return target == ErrRateLimited
		}
		return target == ErrPermissionDenied
	case http.StatusTooManyRequests:
		return target == ErrRateLimited
	default:
		return e.StatusCode >= 500 && target == ErrAPIUnavailable
	}
}

func (c *APIClient) GetAuthenticatedUser(ctx context.Context) (User, error) {
	var user User
	err := c.getJSON(ctx, "/user", nil, &user)
	user.Login = safeExternalText(user.Login, 100)
	user.Name = safeExternalText(user.Name, 160)
	user.AvatarURL = safeAvatarURL(user.AvatarURL)
	user.HTMLURL = safeGitHubURL(user.HTMLURL)
	if err == nil && user.Login == "" {
		err = errors.New("GitHub returned an incomplete account profile")
	}
	return user, err
}

func (c *APIClient) GetRepository(ctx context.Context, owner, repo string) (RepositoryMetadata, error) {
	var result RepositoryMetadata
	err := c.getJSON(ctx, repositoryPath(owner, repo), nil, &result)
	result.Name = safeExternalText(result.Name, 100)
	result.FullName = safeExternalText(result.FullName, 210)
	result.Description = safeExternalText(result.Description, 1000)
	result.HTMLURL = safeGitHubURL(result.HTMLURL)
	result.DefaultBranch = safeExternalText(result.DefaultBranch, 255)
	if err == nil && (result.ID <= 0 || result.FullName == "") {
		err = errors.New("GitHub returned incomplete repository metadata")
	}
	return result, err
}

func (c *APIClient) GetCommit(ctx context.Context, owner, repo, ref string) (Commit, error) {
	if !validReference(ref) {
		return Commit{}, errors.New("commit reference is invalid")
	}
	var result Commit
	err := c.getJSON(ctx, repositoryPath(owner, repo)+"/commits/"+url.PathEscape(ref), nil, &result)
	result.SHA = safeExternalText(result.SHA, 64)
	result.HTMLURL = safeGitHubURL(result.HTMLURL)
	result.Commit.Message = safeExternalText(result.Commit.Message, 1200)
	result.Commit.Author.Name = safeExternalText(result.Commit.Author.Name, 160)
	result.Commit.Author.Date = safeExternalText(result.Commit.Author.Date, 64)
	if err == nil && result.SHA == "" {
		err = errors.New("GitHub returned incomplete commit metadata")
	}
	return result, err
}

func (c *APIClient) ListOpenIssues(ctx context.Context, owner, repo string, limit int) ([]Issue, error) {
	limit, err := boundedLimit(limit)
	if err != nil {
		return nil, err
	}
	query := url.Values{"state": {"open"}, "per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var issues []Issue
	if err := c.getJSON(ctx, repositoryPath(owner, repo)+"/issues", query, &issues); err != nil {
		return nil, err
	}
	filtered := make([]Issue, 0, len(issues))
	for _, issue := range issues {
		if issue.PullRequest != nil {
			continue
		}
		item := cleanIssue(issue)
		item.Body = safeExternalText(item.Body, 500)
		filtered = append(filtered, item)
	}
	return filtered, nil
}

func (c *APIClient) GetIssue(ctx context.Context, owner, repo string, number int64) (Issue, error) {
	if number <= 0 {
		return Issue{}, errors.New("issue number must be positive")
	}
	var result Issue
	err := c.getJSON(ctx, repositoryPath(owner, repo)+"/issues/"+strconv.FormatInt(number, 10), nil, &result)
	if result.PullRequest != nil {
		return Issue{}, errors.New("the requested resource is a pull request, not an issue")
	}
	return cleanIssue(result), err
}

func (c *APIClient) ListPullRequests(ctx context.Context, owner, repo string, limit int) ([]PullRequest, error) {
	limit, err := boundedLimit(limit)
	if err != nil {
		return nil, err
	}
	query := url.Values{"state": {"open"}, "per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var result []PullRequest
	if err := c.getJSON(ctx, repositoryPath(owner, repo)+"/pulls", query, &result); err != nil {
		return nil, err
	}
	for i := range result {
		result[i] = cleanPullRequest(result[i])
		result[i].Body = safeExternalText(result[i].Body, 500)
	}
	return result, nil
}

func (c *APIClient) GetPullRequest(ctx context.Context, owner, repo string, number int64) (PullRequest, error) {
	if number <= 0 {
		return PullRequest{}, errors.New("pull request number must be positive")
	}
	var result PullRequest
	err := c.getJSON(ctx, repositoryPath(owner, repo)+"/pulls/"+strconv.FormatInt(number, 10), nil, &result)
	return cleanPullRequest(result), err
}

func (c *APIClient) GetCheckRuns(ctx context.Context, owner, repo, ref string, limit int) (CheckRuns, error) {
	limit, err := boundedLimit(limit)
	if err != nil {
		return CheckRuns{}, err
	}
	if !validReference(ref) {
		return CheckRuns{}, errors.New("commit or branch reference is invalid")
	}
	query := url.Values{"per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var result CheckRuns
	if err := c.getJSON(ctx, repositoryPath(owner, repo)+"/commits/"+url.PathEscape(ref)+"/check-runs", query, &result); err != nil {
		return CheckRuns{}, err
	}
	if len(result.CheckRuns) > limit {
		result.CheckRuns = result.CheckRuns[:limit]
	}
	for i := range result.CheckRuns {
		check := &result.CheckRuns[i]
		check.Name = safeExternalText(check.Name, 160)
		check.Status = safeExternalText(check.Status, 32)
		check.HTMLURL = safeGitHubURL(check.HTMLURL)
		check.DetailsURL = safeGitHubURL(check.DetailsURL)
		check.StartedAt = safeExternalText(check.StartedAt, 64)
		check.CompletedAt = safeExternalText(check.CompletedAt, 64)
		if check.Conclusion != nil {
			value := safeExternalText(*check.Conclusion, 32)
			check.Conclusion = &value
		}
	}
	return result, nil
}

func (c *APIClient) ListWorkflowRuns(ctx context.Context, owner, repo string, limit int) (WorkflowRuns, error) {
	limit, err := boundedLimit(limit)
	if err != nil {
		return WorkflowRuns{}, err
	}
	query := url.Values{"per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var result WorkflowRuns
	if err := c.getJSON(ctx, repositoryPath(owner, repo)+"/actions/runs", query, &result); err != nil {
		return WorkflowRuns{}, err
	}
	if len(result.WorkflowRuns) > limit {
		result.WorkflowRuns = result.WorkflowRuns[:limit]
	}
	for i := range result.WorkflowRuns {
		result.WorkflowRuns[i] = cleanWorkflowRun(result.WorkflowRuns[i])
	}
	return result, nil
}

func (c *APIClient) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (WorkflowRun, error) {
	if runID <= 0 {
		return WorkflowRun{}, errors.New("workflow run ID must be positive")
	}
	var result WorkflowRun
	err := c.getJSON(ctx, repositoryPath(owner, repo)+"/actions/runs/"+strconv.FormatInt(runID, 10), nil, &result)
	return cleanWorkflowRun(result), err
}

func (c *APIClient) GetWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64, limit int) (WorkflowJobs, error) {
	limit, err := boundedLimit(limit)
	if err != nil {
		return WorkflowJobs{}, err
	}
	if runID <= 0 {
		return WorkflowJobs{}, errors.New("workflow run ID must be positive")
	}
	query := url.Values{"per_page": {strconv.Itoa(limit)}, "page": {"1"}}
	var result WorkflowJobs
	err = c.getJSON(ctx, repositoryPath(owner, repo)+"/actions/runs/"+strconv.FormatInt(runID, 10)+"/jobs", query, &result)
	if len(result.Jobs) > limit {
		result.Jobs = result.Jobs[:limit]
	}
	for i := range result.Jobs {
		job := &result.Jobs[i]
		job.Name = safeExternalText(job.Name, 160)
		job.Status = safeExternalText(job.Status, 32)
		job.HTMLURL = safeGitHubURL(job.HTMLURL)
		if job.Conclusion != nil {
			value := safeExternalText(*job.Conclusion, 32)
			job.Conclusion = &value
		}
		if len(job.Steps) > 8 {
			job.Steps = job.Steps[:8]
		}
		for j := range job.Steps {
			job.Steps[j].Name = safeExternalText(job.Steps[j].Name, 80)
			job.Steps[j].Status = safeExternalText(job.Steps[j].Status, 32)
			if job.Steps[j].Conclusion != nil {
				value := safeExternalText(*job.Steps[j].Conclusion, 32)
				job.Steps[j].Conclusion = &value
			}
		}
	}
	return result, err
}

func (c *APIClient) getJSON(ctx context.Context, path string, query url.Values, target any) error {
	if c == nil || c.gh == nil {
		return ErrNotConnected
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, "..") || strings.ContainsAny(path, "\r\n\x00 ") {
		return errors.New("GitHub API path is invalid")
	}
	return c.gh.api(ctx, path, query, target)
}

func repositoryPath(owner, repo string) string {
	if !validName(owner) || !validName(repo) {
		return "/invalid-repository-name"
	}
	return "/repos/" + url.PathEscape(owner) + "/" + url.PathEscape(repo)
}

func validName(value string) bool {
	if value == "" || len(value) > 100 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func validReference(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 255 && !strings.ContainsAny(value, "\r\n\x00")
}

func boundedLimit(limit int) (int, error) {
	if limit < 1 || limit > 25 {
		return 0, errors.New("GitHub result limit must be from 1 to 25")
	}
	return limit, nil
}

func cleanRepository(repo Repository) Repository {
	repo.Owner.Login = safeExternalText(repo.Owner.Login, 100)
	repo.Name = safeExternalText(repo.Name, 100)
	repo.FullName = safeExternalText(repo.FullName, 210)
	repo.HTMLURL = safeGitHubURL(repo.HTMLURL)
	repo.DefaultBranch = safeExternalText(repo.DefaultBranch, 255)
	return repo
}

func cleanIssue(issue Issue) Issue {
	issue.Title = safeExternalText(issue.Title, 300)
	issue.Body = safeExternalText(issue.Body, 1800)
	issue.State = safeExternalText(issue.State, 32)
	issue.HTMLURL = safeGitHubURL(issue.HTMLURL)
	issue.User.Login = safeExternalText(issue.User.Login, 100)
	issue.CreatedAt = safeExternalText(issue.CreatedAt, 64)
	issue.UpdatedAt = safeExternalText(issue.UpdatedAt, 64)
	if len(issue.Labels) > 20 {
		issue.Labels = issue.Labels[:20]
	}
	for i := range issue.Labels {
		issue.Labels[i].Name = safeExternalText(issue.Labels[i].Name, 80)
	}
	if issue.PullRequest != nil {
		issue.PullRequest.HTMLURL = safeGitHubURL(issue.PullRequest.HTMLURL)
	}
	return issue
}

func cleanPullRequest(pullRequest PullRequest) PullRequest {
	pullRequest.Title = safeExternalText(pullRequest.Title, 300)
	pullRequest.Body = safeExternalText(pullRequest.Body, 1800)
	pullRequest.State = safeExternalText(pullRequest.State, 32)
	pullRequest.HTMLURL = safeGitHubURL(pullRequest.HTMLURL)
	pullRequest.User.Login = safeExternalText(pullRequest.User.Login, 100)
	pullRequest.CreatedAt = safeExternalText(pullRequest.CreatedAt, 64)
	pullRequest.UpdatedAt = safeExternalText(pullRequest.UpdatedAt, 64)
	if len(pullRequest.RequestedReviewers) > 20 {
		pullRequest.RequestedReviewers = pullRequest.RequestedReviewers[:20]
	}
	for i := range pullRequest.RequestedReviewers {
		pullRequest.RequestedReviewers[i].Login = safeExternalText(pullRequest.RequestedReviewers[i].Login, 100)
	}
	return pullRequest
}

func cleanWorkflowRun(run WorkflowRun) WorkflowRun {
	run.Name = safeExternalText(run.Name, 160)
	run.DisplayTitle = safeExternalText(run.DisplayTitle, 240)
	run.HeadBranch = safeExternalText(run.HeadBranch, 255)
	run.HeadSHA = safeExternalText(run.HeadSHA, 64)
	run.Status = safeExternalText(run.Status, 32)
	run.HTMLURL = safeGitHubURL(run.HTMLURL)
	run.CreatedAt = safeExternalText(run.CreatedAt, 64)
	run.UpdatedAt = safeExternalText(run.UpdatedAt, 64)
	if run.Conclusion != nil {
		value := safeExternalText(*run.Conclusion, 32)
		run.Conclusion = &value
	}
	return run
}

func safeExternalText(value string, maxBytes int) string {
	value = strings.Map(func(r rune) rune {
		if r == 0 || r == 127 || (r < 32 && r != '\n' && r != '\t') {
			return ' '
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes - len("…")
	if end < 0 {
		end = 0
	}
	for end > 0 && end < len(value) && value[end]&0xc0 == 0x80 {
		end--
	}
	return value[:end] + "…"
}

func safeGitHubURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "github.com" && parsed.Host != "api.github.com") || parsed.User != nil {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func safeAvatarURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || (parsed.Host != "avatars.githubusercontent.com" && parsed.Host != "github.com") || parsed.User != nil {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func isLoopbackHost(host string) bool {
	parsed, err := url.Parse("http://" + host)
	if err != nil {
		return false
	}
	name := parsed.Hostname()
	return name == "localhost" || name == "127.0.0.1" || name == "::1"
}

func readBounded(reader io.Reader, maxBytes int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, errors.New("response too large")
	}
	return data, nil
}

func noRedirectClient(client *http.Client, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	var transport http.RoundTripper
	if client != nil {
		transport = client.Transport
		if client.Timeout > 0 && client.Timeout < timeout {
			timeout = client.Timeout
		}
	}
	return &http.Client{
		Transport: transport, Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}
