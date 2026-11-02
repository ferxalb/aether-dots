package task

import (
	"strconv"
	"time"
)

const SchemaVersion = 4

const (
	MaxConversationNameBytes         = 128
	MaxConversationPurposeBytes      = 4096
	MaxConversationMessageBytes      = 8192
	MaxConversationInstructionsBytes = 4096
	MaxDelegatedDots                 = 12
	MaxDelegationsPerRun             = 3
	MaxDelegatedTaskBytes            = 3072
	MaxDelegatedConstraintBytes      = 1024
	MaxDelegatedOutputBytes          = 1024
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
	// This bounds a transient provider prompt, not the provider's context window.
	// The complete saved conversation is sent until this local resource ceiling;
	// it is never silently clipped to an arbitrary model-sized token count.
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
	ID                          string    `json:"id"`
	SpaceID                     string    `json:"space_id"`
	ConversationID              string    `json:"conversation_id,omitempty"`
	ConversationContextDeferred bool      `json:"conversation_context_deferred,omitempty"`
	ParentDotID                 string    `json:"parent_dot_id,omitempty"`
	DelegatedByDotID            string    `json:"delegated_by_dot_id,omitempty"`
	SkillID                     string    `json:"skill_id,omitempty"`
	RoutineID                   string    `json:"routine_id,omitempty"`
	Name                        string    `json:"name"`
	Objective                   string    `json:"objective"`
	Status                      Status    `json:"status"`
	Checkpoint                  string    `json:"checkpoint,omitempty"`
	CreatedAt                   time.Time `json:"created_at"`
	UpdatedAt                   time.Time `json:"updated_at"`
	Events                      []Event   `json:"events"`
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
	// DelegatedDotIDs are the same-Space specialists this Dot may consult.
	// Grants are directional and empty by default.
	DelegatedDotIDs []string              `json:"delegated_dot_ids,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	UpdatedAt       time.Time             `json:"updated_at"`
	Messages        []ConversationMessage `json:"messages"`
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
	SchemaVersion    int            `json:"schema_version"`
	Spaces           []Space        `json:"spaces"`
	Dots             []Dot          `json:"dots"`
	Conversations    []Conversation `json:"conversations,omitempty"`
	Skills           []Skill        `json:"skills,omitempty"`
	Routines         []Routine      `json:"routines,omitempty"`
	RoutineRunStarts []time.Time    `json:"routine_run_starts,omitempty"`
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
