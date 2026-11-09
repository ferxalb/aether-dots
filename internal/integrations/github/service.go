package github

import (
	"context"
	"errors"
)

type Discovery struct {
	User          User
	Installations []Installation
	Repositories  []Repository
	Truncated     bool
}

func (s *Service) Discover(ctx context.Context) (Discovery, error) {
	if s != nil && s.gh != nil {
		return s.discoverCLI(ctx)
	}
	profile, err := s.GetAuthenticatedUser(ctx)
	if err != nil {
		return Discovery{}, err
	}
	installations, truncated, err := s.ListInstallations(ctx)
	if err != nil {
		return Discovery{}, err
	}
	discovery := Discovery{User: profile, Installations: installations, Truncated: truncated}
	for _, installation := range installations {
		repositories, more, err := s.ListInstallationRepositories(ctx, installation.ID)
		if err != nil {
			// A stale installation should not hide repositories from other
			// installations, but permission errors are surfaced for clear UI.
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrPermissionDenied) {
				continue
			}
			return Discovery{}, err
		}
		if more {
			discovery.Truncated = true
		}
		for _, repo := range repositories {
			if len(discovery.Repositories) == 2000 {
				discovery.Truncated = true
				break
			}
			discovery.Repositories = append(discovery.Repositories, repo)
		}
		if len(discovery.Repositories) == 2000 {
			discovery.Truncated = true
			break
		}
	}
	return discovery, nil
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

func (s *Service) ListInstallations(ctx context.Context) ([]Installation, bool, error) {
	var result []Installation
	var truncated bool
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, truncated, err = client.ListInstallations(ctx)
		return err
	})
	return result, truncated, err
}

func (s *Service) ListInstallationRepositories(ctx context.Context, installationID int64) ([]Repository, bool, error) {
	var result []Repository
	var truncated bool
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		result, truncated, err = client.ListInstallationRepositories(ctx, installationID)
		return err
	})
	return result, truncated, err
}

// RepositoryStillAccessible verifies the selected installation's current
// accessible-repository listing before a Dot reads repository data.
func (s *Service) RepositoryStillAccessible(ctx context.Context, installationID, repositoryID int64) error {
	if installationID <= 0 || repositoryID <= 0 {
		return ErrPermissionDenied
	}
	accessible := false
	err := s.withClient(ctx, func(ctx context.Context, client *APIClient) error {
		var err error
		if client.gh != nil {
			accessible, err = client.repositoryAccessibleCLI(ctx, repositoryID)
			return err
		}
		accessible, err = client.RepositoryStillAccessible(ctx, installationID, repositoryID)
		return err
	})
	if err != nil {
		return err
	}
	if !accessible {
		return ErrPermissionDenied
	}
	return nil
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
