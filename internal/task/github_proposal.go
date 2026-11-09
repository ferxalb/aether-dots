package task

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// GitHub write approvals. A model may only create a pending GitHubProposal; a
// person approves the exact content in the desktop tray before AETHER sends it
// through the user's own gh binary. See "GitHub write approvals" in AGENTS.md.

const (
	MaxGitHubProposalTitleBytes      = 256
	MaxGitHubProposalBodyBytes       = 8000
	MaxGitHubProposalRefBytes        = 255
	MaxGitHubPendingProposalsPerDot  = 10
	MaxGitHubPendingProposalsPerRun  = 3
	MaxGitHubProposals               = 200
	MaxGitHubWriteGrants             = MaxGitHubDotGrants
	GitHubProposalTTL                = 24 * time.Hour
	GitHubProposalExecutionStaleTime = 2 * time.Minute
)

// GitHubWriteKind is the closed set of write operations.
type GitHubWriteKind string

const (
	GitHubIssueComment GitHubWriteKind = "issue_comment"
	GitHubIssueCreate  GitHubWriteKind = "issue_create"
	GitHubPRDraft      GitHubWriteKind = "pr_draft"
)

var GitHubWriteKinds = []GitHubWriteKind{GitHubIssueComment, GitHubIssueCreate, GitHubPRDraft}

func (k GitHubWriteKind) Valid() bool {
	for _, known := range GitHubWriteKinds {
		if k == known {
			return true
		}
	}
	return false
}

// GitHubProposalState is the lifecycle of one proposal.
//
//	pending -> executing -> executed | failed | uncertain
//	pending -> rejected | expired | cancelled
//	uncertain -> executed | failed (the person checks GitHub and resolves it)
type GitHubProposalState string

const (
	GitHubProposalPending   GitHubProposalState = "pending"
	GitHubProposalExecuting GitHubProposalState = "executing"
	GitHubProposalExecuted  GitHubProposalState = "executed"
	GitHubProposalFailed    GitHubProposalState = "failed"
	GitHubProposalUncertain GitHubProposalState = "uncertain"
	GitHubProposalRejected  GitHubProposalState = "rejected"
	GitHubProposalExpired   GitHubProposalState = "expired"
	GitHubProposalCancelled GitHubProposalState = "cancelled"
)

// Terminal reports states that never change again.
func (s GitHubProposalState) Terminal() bool {
	switch s {
	case GitHubProposalExecuted, GitHubProposalFailed, GitHubProposalRejected, GitHubProposalExpired, GitHubProposalCancelled:
		return true
	}
	return false
}

// GitHubWriteGrant lets one Dot propose the listed operations for one
// repository. It never exists without the matching read grant.
type GitHubWriteGrant struct {
	RepositoryID int64             `json:"repository_id"`
	Kinds        []GitHubWriteKind `json:"kinds"`
}

// GitHubProposalResult is what GitHub reported after a successful write.
type GitHubProposalResult struct {
	URL    string `json:"url"`
	Number int64  `json:"number,omitempty"`
}

// GitHubProposal is one pending or finished write. Content is bound to
// ContentHash; approval, edits and sending all recompute it.
type GitHubProposal struct {
	ID             string              `json:"id"`
	DotID          string              `json:"dot_id"`
	ConversationID string              `json:"conversation_id"`
	SpaceID        string              `json:"space_id"`
	Kind           GitHubWriteKind     `json:"kind"`
	RepositoryID   int64               `json:"repository_id"`
	RepositoryName string              `json:"repository_name"` // snapshot for display; the ID is authoritative
	Account        string              `json:"account"`
	Target         int64               `json:"target,omitempty"` // issue number for comments
	Title          string              `json:"title,omitempty"`
	Body           string              `json:"body,omitempty"`
	Head           string              `json:"head,omitempty"`
	Base           string              `json:"base,omitempty"`
	ContentHash    string              `json:"content_hash"`
	State          GitHubProposalState `json:"state"`
	// ReadUntrustedInRun is set when the proposing run had read GitHub data,
	// which may have steered the model. The tray shows it as a warning.
	ReadUntrustedInRun bool                  `json:"read_untrusted_in_run,omitempty"`
	CreatedAt          time.Time             `json:"created_at"`
	ExpiresAt          time.Time             `json:"expires_at"`
	DecidedAt          time.Time             `json:"decided_at,omitempty"`
	ExecutingSince     time.Time             `json:"executing_since,omitempty"`
	Result             *GitHubProposalResult `json:"result,omitempty"`
	FailureReason      string                `json:"failure_reason,omitempty"`
}

// GitHubProposalInput is the model- or person-supplied content of a proposal.
type GitHubProposalInput struct {
	Kind         GitHubWriteKind
	RepositoryID int64
	Target       int64
	Title        string
	Body         string
	Head         string
	Base         string
}

var gitHubRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// ValidGitHubRef is deliberately stricter than git: it accepts the common
// branch-name alphabet only, so a ref can never act as a flag or a path trick.
func ValidGitHubRef(ref string) bool {
	if ref == "" || len(ref) > MaxGitHubProposalRefBytes || !gitHubRefPattern.MatchString(ref) {
		return false
	}
	return !strings.Contains(ref, "..") && !strings.Contains(ref, "//") &&
		!strings.HasSuffix(ref, "/") && !strings.HasSuffix(ref, ".") && !strings.HasSuffix(ref, ".lock") &&
		!strings.Contains(ref, "/.")
}

func validProposalText(label, value string, max int, singleLine bool) error {
	if len(value) > max || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return fmt.Errorf("%s must be valid text of at most %d bytes", label, max)
	}
	if singleLine && strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s must be a single line", label)
	}
	return nil
}

// Validate checks the content against the operation's fixed shape.
func (in GitHubProposalInput) Validate() error {
	if !in.Kind.Valid() {
		return errors.New("unsupported GitHub write operation")
	}
	if in.RepositoryID <= 0 {
		return errors.New("a GitHub repository is required")
	}
	if err := validProposalText("title", in.Title, MaxGitHubProposalTitleBytes, true); err != nil {
		return err
	}
	if err := validProposalText("body", in.Body, MaxGitHubProposalBodyBytes, false); err != nil {
		return err
	}
	hasBody := strings.TrimSpace(in.Body) != ""
	hasTitle := strings.TrimSpace(in.Title) != ""
	switch in.Kind {
	case GitHubIssueComment:
		if in.Target <= 0 {
			return errors.New("a comment needs an issue or pull request number")
		}
		if !hasBody {
			return errors.New("a comment needs a body")
		}
		if hasTitle || in.Head != "" || in.Base != "" {
			return errors.New("a comment has only a target and a body")
		}
	case GitHubIssueCreate:
		if !hasTitle {
			return errors.New("an issue needs a title")
		}
		if in.Target != 0 || in.Head != "" || in.Base != "" {
			return errors.New("an issue has only a title and a body")
		}
	case GitHubPRDraft:
		if !hasTitle {
			return errors.New("a pull request needs a title")
		}
		if in.Target != 0 {
			return errors.New("a pull request has no target number")
		}
		if !ValidGitHubRef(in.Head) || !ValidGitHubRef(in.Base) {
			return errors.New("a pull request needs valid head and base branch names")
		}
		if in.Head == in.Base {
			return errors.New("head and base branches must differ")
		}
	}
	return nil
}

// Hash is the SHA-256 of the canonical content. Anything the person approves
// or AETHER sends must reproduce it.
func (in GitHubProposalInput) Hash() string {
	canonical := struct {
		V            int             `json:"v"`
		Kind         GitHubWriteKind `json:"kind"`
		RepositoryID int64           `json:"repository_id"`
		Target       int64           `json:"target"`
		Title        string          `json:"title"`
		Body         string          `json:"body"`
		Head         string          `json:"head"`
		Base         string          `json:"base"`
	}{1, in.Kind, in.RepositoryID, in.Target, in.Title, in.Body, in.Head, in.Base}
	raw, _ := json.Marshal(canonical)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Input returns the proposal's content in its hashable form.
func (p GitHubProposal) Input() GitHubProposalInput {
	return GitHubProposalInput{Kind: p.Kind, RepositoryID: p.RepositoryID, Target: p.Target, Title: p.Title, Body: p.Body, Head: p.Head, Base: p.Base}
}
