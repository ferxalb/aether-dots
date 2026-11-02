package store

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"aether-dots/internal/task"
)

// GitHubRepositories returns a detached snapshot of the resources AETHER can
// currently discover from the connected account.
func (s *Store) GitHubRepositories() []task.GitHubRepository {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]task.GitHubRepository(nil), s.db.GitHubRepositories...)
}

// ReplaceGitHubRepositories reconciles the locally known repository set with
// the latest authenticated installation listing. Selection is preserved only
// for stable repository IDs that remain accessible.
func (s *Store) ReplaceGitHubRepositories(repositories []task.GitHubRepository) error {
	if len(repositories) > task.MaxGitHubRepositories {
		return fmt.Errorf("GitHub returned more than the %d repository limit", task.MaxGitHubRepositories)
	}
	seen := make(map[int64]bool, len(repositories))
	for i := range repositories {
		repo := &repositories[i]
		if err := validateGitHubRepository(*repo); err != nil {
			return err
		}
		if seen[repo.ID] {
			return errors.New("GitHub returned a duplicate repository ID")
		}
		seen[repo.ID] = true
	}
	return s.change(func(db *task.Database) error {
		if !db.GitHubConnected {
			return errors.New("connect GitHub before refreshing repositories")
		}
		selected := make(map[int64]bool, len(db.GitHubRepositories))
		for _, repo := range db.GitHubRepositories {
			selected[repo.ID] = repo.Selected
		}
		for i := range repositories {
			repositories[i].Selected = selected[repositories[i].ID]
		}
		db.GitHubRepositories = append([]task.GitHubRepository(nil), repositories...)
		db.GitHubLastVerifiedAt = time.Now().UTC()
		pruneGitHubGrants(db)
		return nil
	})
}

func (s *Store) SetGitHubConnected(connected bool) error {
	return s.change(func(db *task.Database) error {
		db.GitHubConnected = connected
		if !connected {
			db.GitHubRepositories = nil
			db.GitHubLastVerifiedAt = time.Time{}
			for i := range db.Conversations {
				db.Conversations[i].GitHubRepositoryIDs = nil
				db.Conversations[i].GitHubGrantSpaceID = ""
				db.Conversations[i].GitHubModelDisclosureAccepted = false
				db.Conversations[i].UpdatedAt = time.Now().UTC()
			}
			for i := range db.Routines {
				db.Routines[i].GitHubEnabled = false
			}
		}
		return nil
	})
}

// SetGitHubRepositorySelection stores Layer 2 consent and immediately removes
// grants for repositories no longer selected.
func (s *Store) SetGitHubRepositorySelection(ids []int64) error {
	selected := make(map[int64]bool, len(ids))
	if len(ids) > task.MaxGitHubRepositories {
		return fmt.Errorf("select at most %d repositories", task.MaxGitHubRepositories)
	}
	for _, id := range ids {
		if id <= 0 {
			return errors.New("repository ID is invalid")
		}
		selected[id] = true
	}
	return s.change(func(db *task.Database) error {
		available := make(map[int64]bool, len(db.GitHubRepositories))
		for _, repo := range db.GitHubRepositories {
			available[repo.ID] = true
		}
		for id := range selected {
			if !available[id] {
				return errors.New("a selected repository is no longer accessible; refresh GitHub repositories")
			}
		}
		for i := range db.GitHubRepositories {
			db.GitHubRepositories[i].Selected = selected[db.GitHubRepositories[i].ID]
		}
		pruneGitHubGrants(db)
		return nil
	})
}

// GitHubToolRepositories returns only the current Dot's explicit grants that
// remain selected in AETHER. Routine runs need a separate routine-level opt-in.
func (s *Store) GitHubToolRepositories(dotID string) ([]task.GitHubRepository, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var dot *task.Dot
	for i := range s.db.Dots {
		if s.db.Dots[i].ID == dotID {
			dot = &s.db.Dots[i]
			break
		}
	}
	if !s.db.GitHubConnected || dot == nil || dot.ConversationID == "" {
		return nil, false
	}
	conversationIndex := conversationIndex(&s.db, dot.ConversationID)
	if conversationIndex < 0 {
		return nil, false
	}
	conversation := s.db.Conversations[conversationIndex]
	if !conversation.GitHubModelDisclosureAccepted || conversation.GitHubGrantSpaceID == "" || conversation.GitHubGrantSpaceID != conversation.SpaceID || conversation.SpaceID != dot.SpaceID {
		return nil, false
	}
	if dot.RoutineID != "" {
		allowed := false
		for _, routine := range s.db.Routines {
			if routine.ID == dot.RoutineID && routine.GitHubEnabled {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, false
		}
	}
	granted := make(map[int64]bool, len(conversation.GitHubRepositoryIDs))
	for _, id := range conversation.GitHubRepositoryIDs {
		granted[id] = true
	}
	var result []task.GitHubRepository
	for _, repo := range s.db.GitHubRepositories {
		if repo.Selected && granted[repo.ID] {
			result = append(result, repo)
		}
	}
	return result, len(result) > 0
}

// GitHubToolRepository rechecks the repository grant against current state at
// invocation time. Model arguments cannot widen this lookup.
func (s *Store) GitHubToolRepository(dotID string, repoID int64) (task.GitHubRepository, bool) {
	repositories, ok := s.GitHubToolRepositories(dotID)
	if !ok {
		return task.GitHubRepository{}, false
	}
	for _, repo := range repositories {
		if repo.ID == repoID {
			return repo, true
		}
	}
	return task.GitHubRepository{}, false
}

// DisconnectGitHub clears locally selected resources and every Dot grant.
// It intentionally leaves the append-only task event chains untouched.
func (s *Store) DisconnectGitHub() error {
	return s.change(func(db *task.Database) error {
		db.GitHubConnected = false
		db.GitHubLastVerifiedAt = time.Time{}
		db.GitHubRepositories = nil
		for i := range db.Conversations {
			db.Conversations[i].GitHubRepositoryIDs = nil
			db.Conversations[i].GitHubGrantSpaceID = ""
			db.Conversations[i].GitHubModelDisclosureAccepted = false
			db.Conversations[i].UpdatedAt = time.Now().UTC()
		}
		for i := range db.Routines {
			db.Routines[i].GitHubEnabled = false
		}
		return nil
	})
}

func pruneGitHubGrants(db *task.Database) {
	selected := make(map[int64]bool, len(db.GitHubRepositories))
	for _, repo := range db.GitHubRepositories {
		if repo.Selected {
			selected[repo.ID] = true
		}
	}
	for i := range db.Conversations {
		conversation := &db.Conversations[i]
		remaining := conversation.GitHubRepositoryIDs[:0]
		for _, id := range conversation.GitHubRepositoryIDs {
			if selected[id] {
				remaining = append(remaining, id)
			}
		}
		if len(remaining) != len(conversation.GitHubRepositoryIDs) {
			conversation.GitHubRepositoryIDs = remaining
			if len(remaining) == 0 {
				conversation.GitHubGrantSpaceID = ""
				conversation.GitHubModelDisclosureAccepted = false
			}
			conversation.UpdatedAt = time.Now().UTC()
		}
		if len(conversation.GitHubRepositoryIDs) == 0 {
			disableConversationGitHubRoutines(db, conversation.ID)
		}
	}
}

func disableConversationGitHubRoutines(db *task.Database, conversationID string) {
	for i := range db.Routines {
		if db.Routines[i].ConversationID == conversationID && db.Routines[i].GitHubEnabled {
			db.Routines[i].GitHubEnabled = false
			db.Routines[i].UpdatedAt = time.Now().UTC()
		}
	}
}

func conversationHasValidGitHubGrant(db *task.Database, conversationID string) bool {
	ci := conversationIndex(db, conversationID)
	if ci < 0 || !db.GitHubConnected {
		return false
	}
	conversation := db.Conversations[ci]
	if !conversation.GitHubModelDisclosureAccepted || conversation.GitHubGrantSpaceID == "" || conversation.GitHubGrantSpaceID != conversation.SpaceID {
		return false
	}
	selected := make(map[int64]bool, len(db.GitHubRepositories))
	for _, repo := range db.GitHubRepositories {
		if repo.Selected {
			selected[repo.ID] = true
		}
	}
	for _, id := range conversation.GitHubRepositoryIDs {
		if selected[id] {
			return true
		}
	}
	return false
}

// SetConversationGitHubGrants persists a Dot-specific repository grant. A
// caller must affirm the model-provider disclosure for any non-empty grant.
func (s *Store) SetConversationGitHubGrants(conversationID string, repositoryIDs []int64, disclosureAccepted bool) error {
	return s.change(func(db *task.Database) error {
		ci := conversationIndex(db, conversationID)
		if ci < 0 {
			return errors.New("Dot not found")
		}
		if len(repositoryIDs) > 0 && !db.GitHubConnected {
			return errors.New("connect GitHub before granting repositories")
		}
		grants, err := validateGitHubRepositoryGrants(db, repositoryIDs)
		if err != nil {
			return err
		}
		if len(grants) > 0 && !disclosureAccepted {
			return errors.New("acknowledge that selected GitHub data may be sent to the configured model before granting access")
		}
		conversation := &db.Conversations[ci]
		conversation.GitHubRepositoryIDs = grants
		conversation.GitHubGrantSpaceID = ""
		conversation.GitHubModelDisclosureAccepted = len(grants) > 0 && disclosureAccepted
		if len(grants) > 0 {
			conversation.GitHubGrantSpaceID = conversation.SpaceID
		}
		conversation.UpdatedAt = time.Now().UTC()
		if len(grants) == 0 {
			disableConversationGitHubRoutines(db, conversationID)
		}
		return nil
	})
}

func validateGitHubRepositoryGrants(db *task.Database, ids []int64) ([]int64, error) {
	if len(ids) > task.MaxGitHubDotGrants {
		return nil, fmt.Errorf("a Dot can be granted at most %d GitHub repositories", task.MaxGitHubDotGrants)
	}
	selected := make(map[int64]bool, len(db.GitHubRepositories))
	for _, repo := range db.GitHubRepositories {
		if repo.Selected {
			selected[repo.ID] = true
		}
	}
	seen := make(map[int64]bool, len(ids))
	grants := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return nil, errors.New("a GitHub repository grant is invalid")
		}
		if seen[id] {
			continue
		}
		if !selected[id] {
			return nil, errors.New("a GitHub repository grant must reference a repository selected in Integrations")
		}
		seen[id] = true
		grants = append(grants, id)
	}
	return grants, nil
}

func validateGitHubRepository(repo task.GitHubRepository) error {
	if repo.ID <= 0 || repo.InstallationID <= 0 || !validGitHubName(repo.Owner) || !validGitHubName(repo.Name) {
		return errors.New("GitHub returned an invalid repository identity")
	}
	if repo.FullName != repo.Owner+"/"+repo.Name || len(repo.FullName) > 210 {
		return errors.New("GitHub returned an invalid repository name")
	}
	parsed, err := url.Parse(repo.HTMLURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("GitHub returned an invalid repository link")
	}
	if strings.Trim(parsed.Path, "/") != repo.FullName {
		return errors.New("GitHub returned a repository link that does not match its identity")
	}
	if len(repo.DefaultBranch) > 255 || strings.ContainsAny(repo.DefaultBranch, "\r\n\x00") {
		return errors.New("GitHub returned an invalid default branch")
	}
	return nil
}

func validGitHubName(value string) bool {
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
