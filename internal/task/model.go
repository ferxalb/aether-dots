package task

import (
	"strconv"
	"time"
)

const SchemaVersion = 6

const (
	MaxConversationNameBytes         = 128
	MaxConversationPurposeBytes      = 4096
	MaxConversationMessageBytes      = 8192
	MaxConversationInstructionsBytes = 4096
	MaxConversationSummaryBytes      = 8192
	MaxSpaceContextBytes             = 4096
	MaxDelegatedDots                 = 12
	MaxDelegationsPerRun             = 3
	MaxDelegatedTaskBytes            = 3072
	MaxDelegatedConstraintBytes      = 1024
	MaxDelegatedOutputBytes          = 1024
	MaxGitHubRepositories            = 2000
	MaxGitHubDotGrants               = 100
	MaxComputerUseScopes             = 2
	MaxConversationContextMessages   = 12
	MaxConversationContextBytes      = 24 * 1024
	MaxSkillNameBytes                = 128
	MaxSkillDescriptionBytes         = 1024
	MaxSkillFieldBytes               = 4096
	MaxSkillDefinitionBytes          = 16 * 1024
	MaxRoutineNameBytes              = 128
	MaxRoutinePromptBytes            = MaxConversationMessageBytes - 512
	MaxRoutineRuns                   = 20
	MaxRoutineRunsPerHour            = 200
	MaxRoutineRunsTotalPerHour       = 300
	// This bounds transient provider prompts, not the provider's context window.
	// The complete transcript remains local; prompts use the explicit memory note
	// and bounded recent turns instead of silently clipping saved chat history.
	MaxConversationObjectiveBytes = 16 * 1024 * 1024
)

const MinRoutineIntervalSeconds int64 = 5 * 60

type Status string

const (
	Captured        Status = "captured"
	Ready           Status = "ready"
	Running         Status = "running"
	WaitingApproval Status = "waiting_approval"
	WaitingDevice   Status = "waiting_device"
	WaitingInput    Status = "waiting_input"
	Done            Status = "done"
	Failed          Status = "failed"
	Paused          Status = "paused"
)

type Space struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Context   string    `json:"context,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	Sequence  uint64    `json:"sequence"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	PrevHash  string    `json:"prev_hash,omitempty"`
	Hash      string    `json:"hash"`
}

type Dot struct {
	ID                          string      `json:"id"`
	SpaceID                     string      `json:"space_id"`
	ConversationID              string      `json:"conversation_id,omitempty"`
	ConversationContextDeferred bool        `json:"conversation_context_deferred,omitempty"`
	Metrics                     *RunMetrics `json:"metrics,omitempty"`
	ParentDotID                 string      `json:"parent_dot_id,omitempty"`
	DelegatedByDotID            string      `json:"delegated_by_dot_id,omitempty"`
	SkillID                     string      `json:"skill_id,omitempty"`
	RoutineID                   string      `json:"routine_id,omitempty"`
	Name                        string      `json:"name"`
	Objective                   string      `json:"objective"`
	Status                      Status      `json:"status"`
	Checkpoint                  string      `json:"checkpoint,omitempty"`
	CreatedAt                   time.Time   `json:"created_at"`
	UpdatedAt                   time.Time   `json:"updated_at"`
	Events                      []Event     `json:"events"`
}

// Conversation is a durable Dot identity with its own chat history and profile.
type Conversation struct {
	ID           string `json:"id"`
	SpaceID      string `json:"space_id"`
	Name         string `json:"name"`
	Icon         string `json:"icon"`
	Color        string `json:"color"`
	Purpose      string `json:"purpose"`
	Instructions string `json:"instructions,omitempty"`
	// ContextSummary is optional user-maintained memory. The full transcript
	// remains in Messages; this summary only replaces older turns in provider prompts.
	ContextSummary string `json:"context_summary,omitempty"`
	// DelegatedDotIDs are the same-Space specialists this Dot may consult.
	// Grants are directional and empty by default.
	DelegatedDotIDs []string `json:"delegated_dot_ids,omitempty"`
	// GitHub access is separately granted by stable repository ID. Grants are
	// bound to the current Space and default to empty.
	GitHubRepositoryIDs                []int64               `json:"github_repository_ids,omitempty"`
	GitHubGrantSpaceID                 string                `json:"github_grant_space_id,omitempty"`
	GitHubModelDisclosureAccepted      bool                  `json:"github_model_disclosure_accepted,omitempty"`
	ComputerUseScopes                  []string              `json:"computer_use_scopes,omitempty"`
	ComputerUseGrantSpaceID            string                `json:"computer_use_grant_space_id,omitempty"`
	ComputerUseModelDisclosureAccepted bool                  `json:"computer_use_model_disclosure_accepted,omitempty"`
	CreatedAt                          time.Time             `json:"created_at"`
	UpdatedAt                          time.Time             `json:"updated_at"`
	Messages                           []ConversationMessage `json:"messages"`
}

const (
	ComputerUseScopeNotesRead    = "notes_read"
	ComputerUseScopeMusicControl = "music_control"
)

// RunMetrics stores local, approximate generation timing and token counts.
// Token counts are estimates because providers expose different usage formats.
type RunMetrics struct {
	Provider                string    `json:"provider"`
	StartedAt               time.Time `json:"started_at"`
	FinishedAt              time.Time `json:"finished_at"`
	FirstTokenLatencyMillis int64     `json:"first_token_latency_ms,omitempty"`
	DurationMillis          int64     `json:"duration_ms"`
	EstimatedInputTokens    int64     `json:"estimated_input_tokens"`
	EstimatedOutputTokens   int64     `json:"estimated_output_tokens"`
}

// ConversationMessage is an append-only, persisted user request or verified
// assistant result. DotID links the message to its real task run.
type ConversationMessage struct {
	ID      string `json:"id"`
	Role    string `json:"role"`
	Content string `json:"content"`
	DotID   string `json:"dot_id"`
	// SourceDotID identifies a specialist that sent or returned this handoff.
	SourceDotID string    `json:"source_dot_id,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Skill is a reusable, user-authored playbook shared by every Dot.
type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	WhenToUse   string    `json:"when_to_use"`
	Inputs      string    `json:"inputs"`
	Steps       string    `json:"steps"`
	Validation  string    `json:"validation"`
	Output      string    `json:"output"`
	Approval    string    `json:"approval"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Routine stores an interval-based recurring prompt. Runs are local to the
// app or CLI process and are recorded in the owning conversation.
type Routine struct {
	ID              string       `json:"id"`
	ConversationID  string       `json:"conversation_id"`
	Name            string       `json:"name"`
	Prompt          string       `json:"prompt"`
	GitHubEnabled   bool         `json:"github_enabled,omitempty"`
	IntervalSeconds int64        `json:"interval_seconds"`
	Active          bool         `json:"active"`
	NextRunAt       time.Time    `json:"next_run_at"`
	LastRunAt       time.Time    `json:"last_run_at,omitempty"`
	LastStatus      string       `json:"last_status,omitempty"`
	Runs            []RoutineRun `json:"runs,omitempty"`
	RunStarts       []time.Time  `json:"run_starts,omitempty"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
}

type RoutineRun struct {
	DotID      string    `json:"dot_id"`
	Status     string    `json:"status"`
	Message    string    `json:"message,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at,omitempty"`
}

type Database struct {
	SchemaVersion        int                `json:"schema_version"`
	Spaces               []Space            `json:"spaces"`
	Dots                 []Dot              `json:"dots"`
	Conversations        []Conversation     `json:"conversations,omitempty"`
	Skills               []Skill            `json:"skills,omitempty"`
	Routines             []Routine          `json:"routines,omitempty"`
	RoutineRunStarts     []time.Time        `json:"routine_run_starts,omitempty"`
	GitHubRepositories   []GitHubRepository `json:"github_repositories,omitempty"`
	GitHubConnected      bool               `json:"github_connected,omitempty"`
	GitHubLastVerifiedAt time.Time          `json:"github_last_verified_at,omitempty"`
}

// GitHubRepository is a non-secret, locally selected resource. Its stable ID
// is the authorization key; owner/name are refreshed display and API fields.
type GitHubRepository struct {
	ID             int64  `json:"id"`
	InstallationID int64  `json:"installation_id"`
	Owner          string `json:"owner"`
	Name           string `json:"name"`
	FullName       string `json:"full_name"`
	Private        bool   `json:"private"`
	HTMLURL        string `json:"html_url"`
	DefaultBranch  string `json:"default_branch,omitempty"`
	Selected       bool   `json:"selected,omitempty"`
}

type CreateConversation struct {
	SpaceID      string
	Name         string
	Icon         string
	Color        string
	Purpose      string
	Instructions string
}

type CreateSkill struct {
	Name        string
	Description string
	WhenToUse   string
	Inputs      string
	Steps       string
	Validation  string
	Output      string
	Approval    string
}

type CreateRoutine struct {
	ConversationID  string
	Name            string
	Prompt          string
	IntervalSeconds int64
	GitHubEnabled   bool
}

func FormatRoutineInterval(seconds int64) string {
	if seconds > 0 && seconds%3600 == 0 {
		return strconv.FormatInt(seconds/3600, 10) + "h"
	}
	if seconds > 0 && seconds%60 == 0 {
		return strconv.FormatInt(seconds/60, 10) + "m"
	}
	return (time.Duration(seconds) * time.Second).String()
}
