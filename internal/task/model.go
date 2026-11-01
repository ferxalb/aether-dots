package task

import "time"

const SchemaVersion = 1

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

type Artifact struct {
	Path       string    `json:"path"`
	SHA256     string    `json:"sha256"`
	Bytes      int64     `json:"bytes"`
	VerifiedAt time.Time `json:"verified_at"`
}

type Dot struct {
	ID         string     `json:"id"`
	SpaceID    string     `json:"space_id"`
	Name       string     `json:"name"`
	Objective  string     `json:"objective"`
	Status     Status     `json:"status"`
	Mode       string     `json:"mode"`
	SourceRoot string     `json:"source_root"`
	OutputDir  string     `json:"output_dir"`
	Checkpoint string     `json:"checkpoint,omitempty"`
	RoutineID  string     `json:"routine_id,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
	Events     []Event    `json:"events"`
	Artifacts  []Artifact `json:"artifacts"`
}

// Routine stores a reusable task contract and the exact local grants that
// were used by its verified source Dot. Edits never widen these paths.
type Routine struct {
	ID           string    `json:"id"`
	Version      int       `json:"version"`
	Name         string    `json:"name"`
	Objective    string    `json:"objective"`
	Mode         string    `json:"mode"`
	SourceRoot   string    `json:"source_root"`
	OutputDir    string    `json:"output_dir"`
	Precondition string    `json:"precondition"`
	Verification string    `json:"verification"`
	SourceDotID  string    `json:"source_dot_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Database struct {
	SchemaVersion int       `json:"schema_version"`
	Spaces        []Space   `json:"spaces"`
	Dots          []Dot     `json:"dots"`
	Routines      []Routine `json:"routines"`
}

type CreateDot struct {
	SpaceID    string
	Name       string
	Objective  string
	Mode       string
	SourceRoot string
	OutputDir  string
	RoutineID  string
}
