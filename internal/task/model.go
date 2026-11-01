package task

import "time"

const SchemaVersion = 2

const (
	MaxConversationNameBytes         = 128
	MaxConversationPurposeBytes      = 4096
	MaxConversationMessageBytes      = 8192
	MaxConversationInstructionsBytes = 4096
	MaxConversationContextMessages   = 12
	MaxConversationContextBytes      = 24 * 1024
	MaxConversationObjectiveBytes    = MaxConversationPurposeBytes + MaxConversationInstructionsBytes + MaxConversationMessageBytes + MaxConversationContextBytes + 1024
)

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
	ID             string    `json:"id"`
	SpaceID        string    `json:"space_id"`
	ConversationID string    `json:"conversation_id,omitempty"`
	ParentDotID    string    `json:"parent_dot_id,omitempty"`
	Name           string    `json:"name"`
	Objective      string    `json:"objective"`
	Status         Status    `json:"status"`
	Checkpoint     string    `json:"checkpoint,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Events         []Event   `json:"events"`
}

// Conversation is a durable Dot identity with its own chat history and profile.
type Conversation struct {
	ID           string                `json:"id"`
	SpaceID      string                `json:"space_id"`
	Name         string                `json:"name"`
	Icon         string                `json:"icon"`
	Color        string                `json:"color"`
	Purpose      string                `json:"purpose"`
	Instructions string                `json:"instructions,omitempty"`
	CreatedAt    time.Time             `json:"created_at"`
	UpdatedAt    time.Time             `json:"updated_at"`
	Messages     []ConversationMessage `json:"messages"`
}

// ConversationMessage is an append-only, persisted user request or verified
// assistant result. DotID links the message to its real task run.
type ConversationMessage struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	DotID     string    `json:"dot_id"`
	CreatedAt time.Time `json:"created_at"`
}

type Database struct {
	SchemaVersion int            `json:"schema_version"`
	Spaces        []Space        `json:"spaces"`
	Dots          []Dot          `json:"dots"`
	Conversations []Conversation `json:"conversations,omitempty"`
}

type CreateConversation struct {
	SpaceID      string
	Name         string
	Icon         string
	Color        string
	Purpose      string
	Instructions string
}
