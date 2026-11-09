package github

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

// Service is AETHER's only GitHub entry point. It runs typed, read-only calls
// through the GitHub CLI and can be disconnected, which cancels in-flight work.
type Service struct {
	gh  *ghRunner
	now func() time.Time

	mu            sync.Mutex
	disconnected  bool
	generation    uint64
	nextOperation uint64
	operations    map[uint64]context.CancelFunc
}

type Discovery struct {
	User         User
	Repositories []Repository
	Truncated    bool
}

// NewService returns a Service. When gh is not installed the service exists but
// reports Installed() == false and every call fails with ErrCLINotInstalled.
func NewService() *Service {
	s := &Service{now: time.Now, operations: make(map[uint64]context.CancelFunc)}
	if path, ok := findGH(); ok {
		s.gh = &ghRunner{path: path}
	}
	return s
}

// Installed reports whether a gh binary was found.
func (s *Service) Installed() bool { return s != nil && s.gh != nil }

// Connected reports whether gh is signed in and AETHER has not been disconnected.
func (s *Service) Connected(ctx context.Context) bool {
	if !s.Installed() {
		return false
	}
	s.mu.Lock()
	blocked := s.disconnected
	s.mu.Unlock()
	return !blocked && s.gh.session(ctx) != ""
}

// Login returns the GitHub login gh is currently signed in as ("" when
// signed out). Callers compare it with the account recorded at connect time.
func (s *Service) Login(ctx context.Context) string {
	if !s.Installed() {
		return ""
	}
	return s.gh.session(ctx)
}

// Connect confirms gh is installed and signed in and re-enables AETHER's use
// of it after a Disconnect. gh owns the login, so no code or token is shown.
func (s *Service) Connect(ctx context.Context) (User, error) {
	if !s.Installed() {
		return User{}, ErrCLINotInstalled
	}
	s.gh.forgetSession()
	if s.gh.session(ctx) == "" {
		if ctx.Err() != nil {
			return User{}, ctx.Err()
		}
		return User{}, ErrCLINotAuthenticated
	}
	s.mu.Lock()
	s.disconnected = false
	s.generation++
	s.mu.Unlock()
	return s.GetAuthenticatedUser(ctx)
}

// Disconnect stops AETHER from using gh and cancels in-flight requests. It
// never touches the user's gh login.
func (s *Service) Disconnect(ctx context.Context) error {
	s.CancelActiveRequests()
	return nil
}

// CancelActiveRequests blocks new calls and cancels running ones.
func (s *Service) CancelActiveRequests() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.disconnected = true
	s.generation++
	for _, cancel := range s.operations {
		cancel()
	}
	s.operations = make(map[uint64]context.CancelFunc)
	s.mu.Unlock()
}

func (s *Service) withClient(ctx context.Context, operation func(context.Context, *APIClient) error) error {
	if !s.Installed() {
		return ErrCLINotInstalled
	}
	s.mu.Lock()
	if s.disconnected {
		s.mu.Unlock()
		return ErrNotConnected
	}
	s.nextOperation++
	id := s.nextOperation
	opCtx, cancel := context.WithCancel(ctx)
	s.operations[id] = cancel
	s.mu.Unlock()
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.operations, id)
		s.mu.Unlock()
	}()
	if s.gh.session(opCtx) == "" {
		if opCtx.Err() != nil {
			return opCtx.Err()
		}
		return ErrCLINotAuthenticated
	}
	return operation(opCtx, &APIClient{gh: s.gh})
}

// Discover lists the repositories the gh account can read (up to 2,000).
func (s *Service) Discover(ctx context.Context) (Discovery, error) {
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

func (s *Service) GetAuthenticatedUser(ctx context.Context) (User, error) {
	var result User
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetAuthenticatedUser(ctx)
		return err
	})
	return result, err
}

// RepositoryStillAccessible verifies the gh account can still read the
// repository before a Dot reads from it.
func (s *Service) RepositoryStillAccessible(ctx context.Context, repositoryID int64) error {
	if repositoryID <= 0 {
		return ErrPermissionDenied
	}
	accessible := false
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var repo struct {
			ID int64 `json:"id"`
		}
		err := client.getJSON(ctx, "/repositories/"+strconv.FormatInt(repositoryID, 10), nil, &repo)
		if err != nil {
			if isAccessDenied(err) {
				return nil
			}
			return err
		}
		accessible = repo.ID == repositoryID
		return nil
	})
	if err != nil {
		return err
	}
	if !accessible {
		return ErrPermissionDenied
	}
	return nil
}

// PurgeLegacyCredentials removes the token an earlier GitHub App sign-in may
// have left in the system credential store. It is best effort and idempotent.
func PurgeLegacyCredentials() {
	_ = mygo.Secrets.Service("com.aetherdots.github").Delete("github-user-credentials")
}
func (s *Service) GetRepository(ctx context.Context, owner, repo string) (RepositoryMetadata, error) {
	var result RepositoryMetadata
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetRepository(ctx, owner, repo)
		return err
	})
	return result, err
}

func (s *Service) GetCommit(ctx context.Context, owner, repo, ref string) (Commit, error) {
	var result Commit
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetCommit(ctx, owner, repo, ref)
		return err
	})
	return result, err
}

func (s *Service) ListOpenIssues(ctx context.Context, owner, repo string, limit int) ([]Issue, error) {
	var result []Issue
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.ListOpenIssues(ctx, owner, repo, limit)
		return err
	})
	return result, err
}

func (s *Service) GetIssue(ctx context.Context, owner, repo string, number int64) (Issue, error) {
	var result Issue
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetIssue(ctx, owner, repo, number)
		return err
	})
	return result, err
}

func (s *Service) ListPullRequests(ctx context.Context, owner, repo string, limit int) ([]PullRequest, error) {
	var result []PullRequest
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.ListPullRequests(ctx, owner, repo, limit)
		return err
	})
	return result, err
}

func (s *Service) GetPullRequest(ctx context.Context, owner, repo string, number int64) (PullRequest, error) {
	var result PullRequest
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetPullRequest(ctx, owner, repo, number)
		return err
	})
	return result, err
}

func (s *Service) GetCheckRuns(ctx context.Context, owner, repo, ref string, limit int) (CheckRuns, error) {
	var result CheckRuns
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetCheckRuns(ctx, owner, repo, ref, limit)
		return err
	})
	return result, err
}

func (s *Service) ListWorkflowRuns(ctx context.Context, owner, repo string, limit int) (WorkflowRuns, error) {
	var result WorkflowRuns
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.ListWorkflowRuns(ctx, owner, repo, limit)
		return err
	})
	return result, err
}

func (s *Service) GetWorkflowRun(ctx context.Context, owner, repo string, runID int64) (WorkflowRun, error) {
	var result WorkflowRun
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetWorkflowRun(ctx, owner, repo, runID)
		return err
	})
	return result, err
}

func (s *Service) GetWorkflowRunJobs(ctx context.Context, owner, repo string, runID int64, limit int) (WorkflowJobs, error) {
	var result WorkflowJobs
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, err = client.GetWorkflowRunJobs(ctx, owner, repo, runID, limit)
		return err
	})
	return result, err
}

func isAccessDenied(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && (apiErr.StatusCode == 404 || apiErr.StatusCode == 403 && apiErr.RetryAfter == 0)
}
