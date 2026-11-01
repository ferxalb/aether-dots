package store

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aether-dots/desktop/internal/task"
)

type Store struct {
	mu   sync.RWMutex
	path string
	db   task.Database
}

func DefaultDataDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "AETHER Dots"), nil
}

func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("data directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dir, "dots.json")
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		s := &Store{path: path, db: task.Database{SchemaVersion: task.SchemaVersion}}
		space, _ := newID()
		s.db.Spaces = []task.Space{{ID: space, Name: "Personal", CreatedAt: time.Now().UTC()}}
		if err := s.persist(s.db); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read task store: %w", err)
	}
	var db task.Database
	if err := json.Unmarshal(b, &db); err != nil {
		return nil, fmt.Errorf("decode task store: %w", err)
	}
	if db.SchemaVersion != task.SchemaVersion {
		return nil, fmt.Errorf("unsupported task store schema %d (expected %d)", db.SchemaVersion, task.SchemaVersion)
	}
	if err := verifyEvents(db); err != nil {
		return nil, fmt.Errorf("task event history failed integrity check: %w", err)
	}
	return &Store{path: path, db: db}, nil
}

func (s *Store) Snapshot() task.Database {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, _ := json.Marshal(s.db)
	var out task.Database
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Store) CreateSpace(name string) (task.Space, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return task.Space{}, errors.New("space name is empty")
	}
	id, err := newID()
	if err != nil {
		return task.Space{}, err
	}
	space := task.Space{ID: id, Name: name, CreatedAt: time.Now().UTC()}
	err = s.change(func(db *task.Database) error {
		for _, existing := range db.Spaces {
			if strings.EqualFold(existing.Name, name) {
				return fmt.Errorf("space %q already exists", name)
			}
		}
		db.Spaces = append(db.Spaces, space)
		return nil
	})
	return space, err
}

func (s *Store) RenameSpace(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("space name is empty")
	}
	return s.change(func(db *task.Database) error {
		for _, other := range db.Spaces {
			if other.ID != id && strings.EqualFold(other.Name, name) {
				return fmt.Errorf("space %q already exists", name)
			}
		}
		for i := range db.Spaces {
			if db.Spaces[i].ID == id {
				db.Spaces[i].Name = name
				return nil
			}
		}
		return errors.New("space not found")
	})
}

func (s *Store) CreateDot(in task.CreateDot) (task.Dot, error) {
	if strings.TrimSpace(in.Objective) == "" {
		return task.Dot{}, errors.New("objective is empty")
	}
	if in.Mode != "local" && in.Mode != "model" {
		return task.Dot{}, errors.New("mode must be local or model")
	}
	id, err := newID()
	if err != nil {
		return task.Dot{}, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = shortName(in.Objective)
	}
	now := time.Now().UTC()
	dot := task.Dot{
		ID: id, SpaceID: in.SpaceID, Name: name, Objective: strings.TrimSpace(in.Objective),
		Status: task.Captured, Mode: in.Mode, SourceRoot: in.SourceRoot,
		OutputDir: in.OutputDir, RoutineID: in.RoutineID, CreatedAt: now, UpdatedAt: now,
		Events: []task.Event{}, Artifacts: []task.Artifact{},
	}
	err = s.change(func(db *task.Database) error {
		if !hasSpace(db, in.SpaceID) {
			return errors.New("space not found")
		}
		db.Dots = append(db.Dots, dot)
		appendEvent(&dot, "captured", "Dot captured with a local-only source and an explicit output folder.")
		db.Dots[len(db.Dots)-1] = dot
		return nil
	})
	return dot, err
}

func (s *Store) GetDot(id string) (task.Dot, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, dot := range s.db.Dots {
		if dot.ID == id {
			b, _ := json.Marshal(dot)
			var out task.Dot
			_ = json.Unmarshal(b, &out)
			return out, true
		}
	}
	return task.Dot{}, false
}

func (s *Store) AppendEvent(id, kind, message string) error {
	return s.change(func(db *task.Database) error {
		i := dotIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		appendEvent(&db.Dots[i], kind, message)
		db.Dots[i].UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (s *Store) SetStatus(id string, status task.Status, message string) error {
	return s.change(func(db *task.Database) error {
		i := dotIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		db.Dots[i].Status = status
		db.Dots[i].UpdatedAt = time.Now().UTC()
		appendEvent(&db.Dots[i], "status", message)
		return nil
	})
}

func (s *Store) SetCheckpoint(id, checkpoint string) error {
	return s.change(func(db *task.Database) error {
		i := dotIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		db.Dots[i].Checkpoint = checkpoint
		db.Dots[i].UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (s *Store) SetMode(id, mode string) error {
	if mode != "local" && mode != "model" {
		return errors.New("mode must be local or model")
	}
	return s.change(func(db *task.Database) error {
		i := dotIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		if db.Dots[i].Status == task.Running {
			return errors.New("cannot change mode while a Dot is running")
		}
		db.Dots[i].Mode = mode
		db.Dots[i].UpdatedAt = time.Now().UTC()
		appendEvent(&db.Dots[i], "mode_changed", "Execution mode changed to "+mode+" by the user.")
		return nil
	})
}

func (s *Store) AddArtifact(id string, artifact task.Artifact) error {
	return s.change(func(db *task.Database) error {
		i := dotIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		db.Dots[i].Artifacts = append(db.Dots[i].Artifacts, artifact)
		db.Dots[i].Checkpoint = ""
		db.Dots[i].UpdatedAt = time.Now().UTC()
		appendEvent(&db.Dots[i], "artifact_verified", fmt.Sprintf("Verified local artifact %s (%s).", artifact.Path, artifact.SHA256))
		return nil
	})
}

func (s *Store) PromoteRoutine(dotID, name string) (task.Routine, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Routine"
	}
	id, err := newID()
	if err != nil {
		return task.Routine{}, err
	}
	var routine task.Routine
	err = s.change(func(db *task.Database) error {
		i := dotIndex(db, dotID)
		if i < 0 {
			return errors.New("Dot not found")
		}
		dot := db.Dots[i]
		if dot.Status != task.Done || len(dot.Artifacts) == 0 {
			return errors.New("only a completed Dot with verified output can become a Routine")
		}
		now := time.Now().UTC()
		routine = task.Routine{
			ID: id, Version: 1, Name: name, Objective: dot.Objective, Mode: dot.Mode,
			SourceRoot: dot.SourceRoot, OutputDir: dot.OutputDir,
			Precondition: "The selected local source folder is available and contains the task inputs.",
			Verification: "The new brief exists in the same output folder and its saved bytes match the recorded SHA-256.",
			SourceDotID:  dot.ID, CreatedAt: now, UpdatedAt: now,
		}
		db.Routines = append(db.Routines, routine)
		appendEvent(&db.Dots[i], "routine_promoted", "Saved an editable Routine with this Dot's exact source and output grants.")
		return nil
	})
	return routine, err
}

func (s *Store) EditRoutine(id, name, objective string) error {
	name, objective = strings.TrimSpace(name), strings.TrimSpace(objective)
	if name == "" || objective == "" {
		return errors.New("routine name and objective are required")
	}
	return s.change(func(db *task.Database) error {
		for i := range db.Routines {
			if db.Routines[i].ID == id {
				db.Routines[i].Name = name
				db.Routines[i].Objective = objective
				db.Routines[i].Version++
				db.Routines[i].UpdatedAt = time.Now().UTC()
				return nil
			}
		}
		return errors.New("Routine not found")
	})
}

func (s *Store) DotFromRoutine(id, spaceID string) (task.Dot, error) {
	db := s.Snapshot()
	var routine *task.Routine
	for i := range db.Routines {
		if db.Routines[i].ID == id {
			routine = &db.Routines[i]
			break
		}
	}
	if routine == nil {
		return task.Dot{}, errors.New("Routine not found")
	}
	return s.CreateDot(task.CreateDot{
		SpaceID: spaceID, Name: routine.Name, Objective: routine.Objective,
		Mode: routine.Mode, SourceRoot: routine.SourceRoot, OutputDir: routine.OutputDir,
		RoutineID: routine.ID,
	})
}

func (s *Store) change(fn func(*task.Database) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, _ := json.Marshal(s.db)
	var next task.Database
	_ = json.Unmarshal(b, &next)
	if err := fn(&next); err != nil {
		return err
	}
	if err := s.persist(next); err != nil {
		return err
	}
	s.db = next
	return nil
}

func (s *Store) persist(db task.Database) error {
	b, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), ".dots-*.tmp")
	if err != nil {
		return fmt.Errorf("create store checkpoint: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("commit task store: %w", err)
	}
	return nil
}

func appendEvent(dot *task.Dot, kind, message string) {
	var prev string
	var seq uint64 = 1
	if n := len(dot.Events); n > 0 {
		prev = dot.Events[n-1].Hash
		seq = dot.Events[n-1].Sequence + 1
	}
	e := task.Event{Sequence: seq, Type: kind, Message: message, Timestamp: time.Now().UTC(), PrevHash: prev}
	e.Hash = eventHash(e)
	dot.Events = append(dot.Events, e)
}

func eventHash(e task.Event) string {
	e.Hash = ""
	b, _ := json.Marshal(e)
	h := sha256.Sum256(append([]byte(e.PrevHash), b...))
	return hex.EncodeToString(h[:])
}

func verifyEvents(db task.Database) error {
	for _, dot := range db.Dots {
		var previous string
		for i, e := range dot.Events {
			if e.Sequence != uint64(i+1) || e.PrevHash != previous || e.Hash != eventHash(e) {
				return fmt.Errorf("Dot %s event %d is invalid", dot.ID, i+1)
			}
			previous = e.Hash
		}
	}
	return nil
}

func hasSpace(db *task.Database, id string) bool {
	for _, s := range db.Spaces {
		if s.ID == id {
			return true
		}
	}
	return false
}

func dotIndex(db *task.Database, id string) int {
	for i := range db.Dots {
		if db.Dots[i].ID == id {
			return i
		}
	}
	return -1
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func shortName(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 54 {
		s = s[:51] + "..."
	}
	return s
}
