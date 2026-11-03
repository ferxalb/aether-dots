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
	"unicode/utf8"

	"aether-dots/internal/task"
)

type Store struct {
	mu      sync.RWMutex
	leaseMu sync.Mutex
	path    string
	db      task.Database
	lease   *os.File
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
	if db.SchemaVersion < 1 || db.SchemaVersion > task.SchemaVersion {
		return nil, fmt.Errorf("unsupported task store schema %d (expected %d)", db.SchemaVersion, task.SchemaVersion)
	}
	if err := verifyEvents(db); err != nil {
		return nil, fmt.Errorf("task event history failed integrity check: %w", err)
	}
	store := &Store{path: path, db: db}
	if db.SchemaVersion < task.SchemaVersion {
		if db.SchemaVersion < 6 {
			// Computer Use is a new, explicit opt-in. No older store receives
			// an app grant or model-data disclosure during migration.
			for i := range db.Conversations {
				db.Conversations[i].ComputerUseScopes = nil
				db.Conversations[i].ComputerUseGrantSpaceID = ""
				db.Conversations[i].ComputerUseModelDisclosureAccepted = false
			}
		}
		if db.SchemaVersion < 5 {
			// GitHub access is opt-in. Older stores migrate with no selected
			// repositories and no Dot or Routine grants.
			db.GitHubRepositories = nil
			db.GitHubConnected = false
			db.GitHubLastVerifiedAt = time.Time{}
			for i := range db.Conversations {
				db.Conversations[i].GitHubRepositoryIDs = nil
				db.Conversations[i].GitHubGrantSpaceID = ""
				db.Conversations[i].GitHubModelDisclosureAccepted = false
			}
			for i := range db.Routines {
				db.Routines[i].GitHubEnabled = false
			}
		}
		if db.SchemaVersion == 1 {
			migrateLegacyFileAccess(&db)
		}
		if db.SchemaVersion < 3 {
			// Skills and routines are introduced in schema 3. Ignore similarly
			// named fields from older stores rather than importing legacy data.
			db.Skills = nil
			db.Routines = nil
			db.RoutineRunStarts = nil
		}
		db.SchemaVersion = task.SchemaVersion
		if err := store.persist(db); err != nil {
			return nil, fmt.Errorf("migrate task store: %w", err)
		}
		store.db = db
	}
	return store, nil
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
	return s.CreateSpaceWithContext(name, "")
}

func (s *Store) CreateSpaceWithContext(name, context string) (task.Space, error) {
	name = strings.TrimSpace(name)
	context = strings.TrimSpace(context)
	if name == "" || len(name) > task.MaxConversationNameBytes || !utf8.ValidString(name) {
		return task.Space{}, fmt.Errorf("space name must be valid UTF-8 text of at most %d bytes", task.MaxConversationNameBytes)
	}
	if len(context) > task.MaxSpaceContextBytes || !utf8.ValidString(context) {
		return task.Space{}, fmt.Errorf("space context must be valid UTF-8 text of at most %d bytes", task.MaxSpaceContextBytes)
	}
	id, err := newID()
	if err != nil {
		return task.Space{}, err
	}
	space := task.Space{ID: id, Name: name, Context: context, CreatedAt: time.Now().UTC()}
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

// UpdateSpace saves a Space name and reusable, user-authored context.
func (s *Store) UpdateSpace(id, name, context string) error {
	name = strings.TrimSpace(name)
	context = strings.TrimSpace(context)
	if name == "" || len(name) > task.MaxConversationNameBytes || !utf8.ValidString(name) {
		return fmt.Errorf("space name must be valid UTF-8 text of at most %d bytes", task.MaxConversationNameBytes)
	}
	if len(context) > task.MaxSpaceContextBytes || !utf8.ValidString(context) {
		return fmt.Errorf("space context must be valid UTF-8 text of at most %d bytes", task.MaxSpaceContextBytes)
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
				db.Spaces[i].Context = context
				return nil
			}
		}
		return errors.New("space not found")
	})
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

// migrateLegacyFileAccess removes persisted fields and timeline details from
// schema v1, whose tasks stored user-selected local folder grants.
func migrateLegacyFileAccess(db *task.Database) {
	db.Skills = nil
	db.Routines = nil
	db.RoutineRunStarts = nil
	for i := range db.Dots {
		dot := &db.Dots[i]
		dot.Checkpoint = ""
		for j := range dot.Events {
			dot.Events[j].Type = "legacy_activity"
			dot.Events[j].Message = "Legacy task detail hidden after local folder access was removed."
		}
		previous := ""
		for j := range dot.Events {
			dot.Events[j].PrevHash = previous
			dot.Events[j].Hash = eventHash(dot.Events[j])
			previous = dot.Events[j].Hash
		}
	}
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
	runes := []rune(s)
	if len(runes) > 54 {
		s = string(runes[:51]) + "..."
	}
	return s
}
