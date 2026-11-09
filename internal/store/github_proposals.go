package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
)

// GitHub write approvals (see "GitHub write approvals" in AGENTS.md).
//
// The store owns every state transition. A proposal can only move from
// pending to executing through BeginGitHubProposalExecution, which runs under
// the same BEGIN IMMEDIATE write lock as every other change, so two processes
// can never both send it. Grants and expiry are re-evaluated on every
// transition and, via reconcileGitHubProposals, on every committed change.

var (
	ErrGitHubProposalNotFound = errors.New("GitHub proposal not found")
	ErrGitHubWriteNotAllowed  = errors.New("this Dot is not allowed to propose that GitHub write")
)

const maxGitHubFailureReasonBytes = 300

// GitHubWriteToolGrant is one repository a Dot may propose writes for.
type GitHubWriteToolGrant struct {
	Repository task.GitHubRepository
	Kinds      []task.GitHubWriteKind
}

// writeGrantConversation returns the conversation behind a Dot run when that
// run may use write grants at all: connected, interactive (not a Routine, not
// delegated), with the write disclosure accepted in the current Space.
func writeGrantConversation(db *task.Database, dot *task.Dot) (*task.Conversation, bool) {
	if !db.GitHubConnected || dot == nil || dot.ConversationID == "" || dot.RoutineID != "" || dot.DelegatedByDotID != "" {
		return nil, false
	}
	ci := conversationIndex(db, dot.ConversationID)
	if ci < 0 {
		return nil, false
	}
	conv := &db.Conversations[ci]
	if !conv.GitHubWriteDisclosureAccepted || conv.GitHubWriteGrantSpaceID == "" ||
		conv.GitHubWriteGrantSpaceID != conv.SpaceID || conv.SpaceID != dot.SpaceID {
		return nil, false
	}
	return conv, true
}

// writeGrantFor reports whether conv may propose kind for the repository: the
// repository is selected, the read grant (with its own disclosure and Space
// binding) is intact, and a write grant lists the kind.
func writeGrantFor(db *task.Database, conv *task.Conversation, repositoryID int64, kind task.GitHubWriteKind) (task.GitHubRepository, bool) {
	if !conv.GitHubModelDisclosureAccepted || conv.GitHubGrantSpaceID == "" || conv.GitHubGrantSpaceID != conv.SpaceID {
		return task.GitHubRepository{}, false
	}
	read := false
	for _, id := range conv.GitHubRepositoryIDs {
		if id == repositoryID {
			read = true
			break
		}
	}
	if !read {
		return task.GitHubRepository{}, false
	}
	for _, grant := range conv.GitHubWriteGrants {
		if grant.RepositoryID != repositoryID {
			continue
		}
		for _, k := range grant.Kinds {
			if k != kind {
				continue
			}
			for _, repo := range db.GitHubRepositories {
				if repo.ID == repositoryID && repo.Selected {
					return repo, true
				}
			}
		}
	}
	return task.GitHubRepository{}, false
}

// GitHubWriteGrants returns what the Dot's run may propose, rechecked against
// freshly loaded state. It fails closed when the store cannot be re-read.
func (s *Store) GitHubWriteGrants(dotID string) ([]GitHubWriteToolGrant, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return nil, false
	}
	di := dotIndex(&s.db, dotID)
	if di < 0 {
		return nil, false
	}
	conv, ok := writeGrantConversation(&s.db, &s.db.Dots[di])
	if !ok {
		return nil, false
	}
	var out []GitHubWriteToolGrant
	for _, grant := range conv.GitHubWriteGrants {
		var kinds []task.GitHubWriteKind
		var repo task.GitHubRepository
		for _, kind := range grant.Kinds {
			if r, ok := writeGrantFor(&s.db, conv, grant.RepositoryID, kind); ok {
				repo = r
				kinds = append(kinds, kind)
			}
		}
		if len(kinds) > 0 {
			out = append(out, GitHubWriteToolGrant{Repository: repo, Kinds: kinds})
		}
	}
	return out, len(out) > 0
}

// SetConversationGitHubWriteGrants stores a Dot's write grants. They require
// an intact read grant for the same repository and the write disclosure.
func (s *Store) SetConversationGitHubWriteGrants(conversationID string, grants []task.GitHubWriteGrant, disclosureAccepted bool) error {
	if len(grants) > task.MaxGitHubWriteGrants {
		return fmt.Errorf("a Dot can be granted GitHub writes for at most %d repositories", task.MaxGitHubWriteGrants)
	}
	return s.change(func(db *task.Database) error {
		ci := conversationIndex(db, conversationID)
		if ci < 0 {
			return errors.New("Dot profile not found")
		}
		conv := &db.Conversations[ci]
		var clean []task.GitHubWriteGrant
		seen := map[int64]bool{}
		for _, grant := range grants {
			if seen[grant.RepositoryID] {
				return errors.New("a GitHub write grant repeats a repository")
			}
			seen[grant.RepositoryID] = true
			kinds := map[task.GitHubWriteKind]bool{}
			var ordered []task.GitHubWriteKind
			for _, kind := range grant.Kinds {
				if !kind.Valid() {
					return errors.New("a GitHub write grant names an unsupported operation")
				}
				if !kinds[kind] {
					kinds[kind] = true
					ordered = append(ordered, kind)
				}
			}
			if len(ordered) == 0 {
				continue
			}
			clean = append(clean, task.GitHubWriteGrant{RepositoryID: grant.RepositoryID, Kinds: ordered})
		}
		if len(clean) == 0 {
			conv.GitHubWriteGrants = nil
			conv.GitHubWriteGrantSpaceID = ""
			conv.GitHubWriteDisclosureAccepted = false
			return nil
		}
		if !db.GitHubConnected {
			return errors.New("connect GitHub before granting writes")
		}
		if !disclosureAccepted {
			return errors.New("acknowledge that approved content is published under your GitHub account before granting writes")
		}
		for _, grant := range clean {
			if !hasReadGrant(db, conv, grant.RepositoryID) {
				return errors.New("a GitHub write grant needs the Dot's read grant for the same repository")
			}
		}
		conv.GitHubWriteGrants = clean
		conv.GitHubWriteGrantSpaceID = conv.SpaceID
		conv.GitHubWriteDisclosureAccepted = true
		return nil
	})
}

func hasReadGrant(db *task.Database, conv *task.Conversation, repositoryID int64) bool {
	if !conv.GitHubModelDisclosureAccepted || conv.GitHubGrantSpaceID == "" || conv.GitHubGrantSpaceID != conv.SpaceID {
		return false
	}
	for _, id := range conv.GitHubRepositoryIDs {
		if id != repositoryID {
			continue
		}
		for _, repo := range db.GitHubRepositories {
			if repo.ID == id && repo.Selected {
				return true
			}
		}
	}
	return false
}

// proposalAuthorized re-evaluates the grant behind a proposal.
func proposalAuthorized(db *task.Database, p *task.GitHubProposal) bool {
	ci := conversationIndex(db, p.ConversationID)
	if ci < 0 || !db.GitHubConnected || !strings.EqualFold(db.GitHubAccount, p.Account) {
		return false
	}
	conv := &db.Conversations[ci]
	if conv.SpaceID != p.SpaceID || !conv.GitHubWriteDisclosureAccepted ||
		conv.GitHubWriteGrantSpaceID == "" || conv.GitHubWriteGrantSpaceID != conv.SpaceID {
		return false
	}
	_, ok := writeGrantFor(db, conv, p.RepositoryID, p.Kind)
	return ok
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// proposalEvent records a transition in the proposing Dot's hash chain. It
// names the kind, repository and content hash, never the body.
func proposalEvent(db *task.Database, p *task.GitHubProposal, kind string) {
	di := dotIndex(db, p.DotID)
	if di < 0 {
		return
	}
	target := ""
	if p.Target > 0 {
		target = fmt.Sprintf(" #%d", p.Target)
	}
	appendEvent(&db.Dots[di], kind, fmt.Sprintf("GitHub %s%s on %s (proposal %s, content %s).",
		p.Kind, target, p.RepositoryName, shortHash(p.ID), shortHash(p.ContentHash)))
	db.Dots[di].UpdatedAt = time.Now().UTC()
}

func finishProposal(db *task.Database, p *task.GitHubProposal, state task.GitHubProposalState, reason, event string, now time.Time) {
	p.State = state
	p.FailureReason = reason
	p.DecidedAt = now
	proposalEvent(db, p, event)
}

// reconcileGitHubProposals enforces the lifecycle invariants after any change:
// stale pending proposals expire, pending proposals whose grant disappeared
// (revocation, disconnect, another gh account, a moved Dot) are cancelled, and
// an executing proposal that nobody finished becomes uncertain.
func reconcileGitHubProposals(db *task.Database, now time.Time) {
	for i := range db.GitHubProposals {
		p := &db.GitHubProposals[i]
		switch p.State {
		case task.GitHubProposalPending:
			if !now.Before(p.ExpiresAt) {
				finishProposal(db, p, task.GitHubProposalExpired, "Not approved within 24 hours.", "github_write_expired", now)
			} else if !proposalAuthorized(db, p) {
				finishProposal(db, p, task.GitHubProposalCancelled, "The GitHub write grant was removed or the connection changed.", "github_write_cancelled", now)
			}
		case task.GitHubProposalExecuting:
			if now.Sub(p.ExecutingSince) > task.GitHubProposalExecutionStaleTime {
				p.State = task.GitHubProposalUncertain
				p.FailureReason = "AETHER stopped before GitHub confirmed the write. Check GitHub, then resolve it."
				proposalEvent(db, p, "github_write_uncertain")
			}
		}
	}
}

// SweepGitHubProposals applies expiry and recovery without another change.
func (s *Store) SweepGitHubProposals() error {
	return s.change(func(*task.Database) error { return nil })
}

func proposalIndex(db *task.Database, id string) int {
	for i := range db.GitHubProposals {
		if db.GitHubProposals[i].ID == id {
			return i
		}
	}
	return -1
}

func cleanReason(reason string) string {
	reason = strings.TrimSpace(strings.ReplaceAll(reason, "\x00", ""))
	if !utf8.ValidString(reason) {
		reason = strings.ToValidUTF8(reason, "")
	}
	if len(reason) > maxGitHubFailureReasonBytes {
		reason = strings.ToValidUTF8(reason[:maxGitHubFailureReasonBytes], "")
	}
	return reason
}

// CreateGitHubProposal queues a write for human approval. It never sends
// anything. existing is true when an identical pending proposal was reused.
func (s *Store) CreateGitHubProposal(dotID string, in task.GitHubProposalInput, readUntrusted bool) (proposal task.GitHubProposal, existing bool, err error) {
	if err := in.Validate(); err != nil {
		return task.GitHubProposal{}, false, err
	}
	id, err := newID()
	if err != nil {
		return task.GitHubProposal{}, false, err
	}
	err = s.change(func(db *task.Database) error {
		di := dotIndex(db, dotID)
		if di < 0 {
			return errors.New("Dot not found")
		}
		dot := &db.Dots[di]
		conv, ok := writeGrantConversation(db, dot)
		if !ok {
			return ErrGitHubWriteNotAllowed
		}
		repo, ok := writeGrantFor(db, conv, in.RepositoryID, in.Kind)
		if !ok {
			return ErrGitHubWriteNotAllowed
		}
		if db.GitHubAccount == "" {
			return errors.New("GitHub is not connected to an account")
		}
		now := time.Now().UTC()
		hash := in.Hash()
		pendingForDot, pendingForRun := 0, 0
		for i := range db.GitHubProposals {
			p := &db.GitHubProposals[i]
			if p.ConversationID != conv.ID || p.State != task.GitHubProposalPending || !now.Before(p.ExpiresAt) {
				continue
			}
			if p.ContentHash == hash {
				proposal, existing = *p, true
				return nil
			}
			pendingForDot++
			if p.DotID == dotID {
				pendingForRun++
			}
		}
		if pendingForDot >= task.MaxGitHubPendingProposalsPerDot {
			return fmt.Errorf("this Dot already has %d GitHub proposals awaiting approval", task.MaxGitHubPendingProposalsPerDot)
		}
		if pendingForRun >= task.MaxGitHubPendingProposalsPerRun {
			return fmt.Errorf("a run can queue at most %d GitHub proposals", task.MaxGitHubPendingProposalsPerRun)
		}
		if len(db.GitHubProposals) >= task.MaxGitHubProposals && !pruneOldestFinishedProposal(db) {
			return errors.New("the GitHub approval tray is full; resolve pending proposals first")
		}
		proposal = task.GitHubProposal{
			ID: id, DotID: dotID, ConversationID: conv.ID, SpaceID: conv.SpaceID,
			Kind: in.Kind, RepositoryID: repo.ID, RepositoryName: repo.FullName, Account: db.GitHubAccount,
			Target: in.Target, Title: in.Title, Body: in.Body, Head: in.Head, Base: in.Base,
			ContentHash: hash, State: task.GitHubProposalPending, ReadUntrustedInRun: readUntrusted,
			CreatedAt: now, ExpiresAt: now.Add(task.GitHubProposalTTL),
		}
		db.GitHubProposals = append(db.GitHubProposals, proposal)
		proposalEvent(db, &proposal, "github_write_proposed")
		return nil
	})
	if err != nil {
		return task.GitHubProposal{}, false, err
	}
	return proposal, existing, nil
}

func pruneOldestFinishedProposal(db *task.Database) bool {
	for i := range db.GitHubProposals {
		if db.GitHubProposals[i].State.Terminal() {
			db.GitHubProposals = append(db.GitHubProposals[:i], db.GitHubProposals[i+1:]...)
			return true
		}
	}
	return false
}

// GitHubProposals returns the tray, oldest first, from freshly loaded state.
func (s *Store) GitHubProposals() []task.GitHubProposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.reloadLocked()
	out := make([]task.GitHubProposal, len(s.db.GitHubProposals))
	copy(out, s.db.GitHubProposals)
	for i := range out {
		if out[i].Result != nil {
			result := *out[i].Result
			out[i].Result = &result
		}
	}
	return out
}

// GetGitHubProposal returns one proposal.
func (s *Store) GetGitHubProposal(id string) (task.GitHubProposal, bool) {
	for _, p := range s.GitHubProposals() {
		if p.ID == id {
			return p, true
		}
	}
	return task.GitHubProposal{}, false
}

// EditGitHubProposal replaces the editable text of a pending proposal and
// re-hashes it. Target, repository, head and base never change by editing.
func (s *Store) EditGitHubProposal(id, title, body string) (task.GitHubProposal, error) {
	var out task.GitHubProposal
	err := s.change(func(db *task.Database) error {
		i := proposalIndex(db, id)
		if i < 0 {
			return ErrGitHubProposalNotFound
		}
		p := &db.GitHubProposals[i]
		now := time.Now().UTC()
		if p.State != task.GitHubProposalPending || !now.Before(p.ExpiresAt) {
			return errors.New("only a pending proposal can be edited")
		}
		if !proposalAuthorized(db, p) {
			return ErrGitHubWriteNotAllowed
		}
		edited := p.Input()
		if p.Kind != task.GitHubIssueComment {
			edited.Title = title
		} else if strings.TrimSpace(title) != "" {
			return errors.New("a comment has no title")
		}
		edited.Body = body
		if err := edited.Validate(); err != nil {
			return err
		}
		p.Title, p.Body, p.ContentHash = edited.Title, edited.Body, edited.Hash()
		proposalEvent(db, p, "github_write_edited")
		out = *p
		return nil
	})
	return out, err
}

// RejectGitHubProposal discards a pending proposal.
func (s *Store) RejectGitHubProposal(id string) error {
	return s.change(func(db *task.Database) error {
		i := proposalIndex(db, id)
		if i < 0 {
			return ErrGitHubProposalNotFound
		}
		p := &db.GitHubProposals[i]
		if p.State != task.GitHubProposalPending {
			return errors.New("only a pending proposal can be rejected")
		}
		finishProposal(db, p, task.GitHubProposalRejected, "", "github_write_rejected", time.Now().UTC())
		return nil
	})
}

// BeginGitHubProposalExecution is the only way to leave pending for sending.
// approvedHash is the hash of the content the person saw; liveAccount is the
// login gh reports right now. The transition happens under the store's write
// lock, so exactly one process can claim a proposal.
func (s *Store) BeginGitHubProposalExecution(id, approvedHash, liveAccount string) (task.GitHubProposal, error) {
	var out task.GitHubProposal
	err := s.change(func(db *task.Database) error {
		i := proposalIndex(db, id)
		if i < 0 {
			return ErrGitHubProposalNotFound
		}
		p := &db.GitHubProposals[i]
		now := time.Now().UTC()
		if p.State != task.GitHubProposalPending {
			return errors.New("this proposal is no longer pending")
		}
		if !now.Before(p.ExpiresAt) {
			return errors.New("this proposal expired; ask the Dot to propose it again")
		}
		if !proposalAuthorized(db, p) {
			return ErrGitHubWriteNotAllowed
		}
		if !strings.EqualFold(liveAccount, p.Account) {
			return errors.New("the GitHub CLI account changed since this proposal was made")
		}
		if recomputed := p.Input().Hash(); recomputed != p.ContentHash || approvedHash != p.ContentHash {
			return errors.New("the proposal changed after it was shown; review it again")
		}
		p.State, p.DecidedAt, p.ExecutingSince = task.GitHubProposalExecuting, now, now
		proposalEvent(db, p, "github_write_approved")
		out = *p
		return nil
	})
	return out, err
}

// CompleteGitHubProposal records a confirmed write. It also resolves an
// uncertain proposal the person verified on GitHub.
func (s *Store) CompleteGitHubProposal(id string, result task.GitHubProposalResult) error {
	if !strings.HasPrefix(result.URL, "https://github.com/") || len(result.URL) > 512 {
		return errors.New("GitHub returned an unexpected result link")
	}
	return s.change(func(db *task.Database) error {
		i := proposalIndex(db, id)
		if i < 0 {
			return ErrGitHubProposalNotFound
		}
		p := &db.GitHubProposals[i]
		if p.State != task.GitHubProposalExecuting && p.State != task.GitHubProposalUncertain {
			return errors.New("this proposal is not being sent")
		}
		p.Result = &result
		finishProposal(db, p, task.GitHubProposalExecuted, "", "github_write_executed", time.Now().UTC())
		return nil
	})
}

// FailGitHubProposal records a write that did not happen (uncertain=false) or
// whose outcome is unknown (uncertain=true; only valid while executing).
func (s *Store) FailGitHubProposal(id, reason string, uncertain bool) error {
	return s.change(func(db *task.Database) error {
		i := proposalIndex(db, id)
		if i < 0 {
			return ErrGitHubProposalNotFound
		}
		p := &db.GitHubProposals[i]
		reason := cleanReason(reason)
		switch {
		case p.State == task.GitHubProposalExecuting && uncertain:
			p.State, p.FailureReason = task.GitHubProposalUncertain, reason
			proposalEvent(db, p, "github_write_uncertain")
		case (p.State == task.GitHubProposalExecuting || p.State == task.GitHubProposalUncertain) && !uncertain:
			finishProposal(db, p, task.GitHubProposalFailed, reason, "github_write_failed", time.Now().UTC())
		default:
			return errors.New("this proposal is not being sent")
		}
		return nil
	})
}
