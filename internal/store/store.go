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
	// runLeases holds the per-Dot run ownership locks this process owns.
	runLeases map[string]*os.File
	// seen identifies the on-disk bytes this process last read or wrote, so a
	// second process (Desktop or CLI) sharing the folder is detected and merged
	// instead of overwritten.
	seen [sha256.Size]byte
	// fp is a cheap file fingerprint (size, mtime, inode) of the bytes in
	// s.db; when it is unchanged the file is not re-read or re-hashed.
	fp fileFP
	// verified records, per Dot, how many events passed chain verification
	// and the hash of the last one, so reloads only verify new events.
	verified map[string]verifiedChain
}

type verifiedChain struct {
	count int
	last  string
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
	unlock, err := lockStoreFile(path)
	if err != nil {
		return nil, err
	}
	defer unlock()
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
	store := &Store{path: path, db: db, seen: sha256.Sum256(b)}
	if db.SchemaVersion < task.SchemaVersion {
		if db.SchemaVersion < 7 {
			// MCP connections and Dot-specific model overrides are opt-in. No
			// existing profile receives a server connection or tool grant.
			db.MCPServers = nil
			for i := range db.Conversations {
				db.Conversations[i].ModelProvider = ""
				db.Conversations[i].Model = ""
				db.Conversations[i].MCPToolGrants = nil
				db.Conversations[i].MCPGrantSpaceID = ""
				db.Conversations[i].MCPModelDisclosureAccepted = false
			}
		}
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

// Snapshot is best effort for display: if another process left the store
// unreadable it returns the last good state. Anything that makes a security
// decision (grants, servers, tokens) must use SnapshotChecked instead.
func (s *Store) Snapshot() task.Database {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.reloadLocked()
	return cloneDatabase(s.db)
}

// SnapshotChecked returns the current committed state, or an error when the
// store cannot be re-read (corrupt, schema changed, failed integrity check).
// Callers fail closed rather than act on a stale view of grants or servers.
func (s *Store) SnapshotChecked() (task.Database, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.reloadLocked(); err != nil {
		return task.Database{}, fmt.Errorf("task store is unavailable: %w", err)
	}
	return cloneDatabase(s.db), nil
}

// reloadLocked adopts another process's committed changes. Callers hold s.mu.
func (s *Store) reloadLocked() error {
	if fp, ok := statFP(s.path); ok && fp == s.fp && s.fp != (fileFP{}) {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	if sum == s.seen {
		return nil
	}
	var db task.Database
	if err := json.Unmarshal(b, &db); err != nil {
		return fmt.Errorf("decode task store: %w", err)
	}
	if db.SchemaVersion < task.SchemaVersion || db.SchemaVersion > task.SchemaVersion {
		return fmt.Errorf("task store schema changed to %d while running (expected %d)", db.SchemaVersion, task.SchemaVersion)
	}
	if err := s.verifyEventsIncremental(db); err != nil {
		return fmt.Errorf("task event history failed integrity check: %w", err)
	}
	s.db = db
	s.seen = sum
	s.fp, _ = statFP(s.path)
	return nil
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
			out := dot
			out.Events = cloneSlice(dot.Events)
			if dot.Metrics != nil {
				metrics := *dot.Metrics
				out.Metrics = &metrics
			}
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
	if status == task.Running {
		// Claim ownership before the status flips so two processes can never
		// both believe they are running the same Dot.
		if err := s.acquireRunLease(id); err != nil {
			return err
		}
	}
	err := s.setStatus(id, status, message)
	if status != task.Running || err != nil {
		s.releaseRunLease(id)
	}
	return err
}

func (s *Store) setStatus(id string, status task.Status, message string) error {
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
	unlock, err := lockStoreFile(s.path)
	if err != nil {
		return err
	}
	defer unlock()
	if err := s.reloadLocked(); err != nil {
		return err
	}
	next := cloneDatabase(s.db)
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
	b, err := json.Marshal(db)
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
	s.seen = sha256.Sum256(b)
	s.fp, _ = statFP(s.path)
	if err := s.verifyEventsIncremental(db); err != nil {
		return err
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

// verifyEventsIncremental checks only events appended since the last verified
// point of each Dot, plus that the previously verified tail is unchanged.
func (s *Store) verifyEventsIncremental(db task.Database) error {
	next := make(map[string]verifiedChain, len(db.Dots))
	for _, dot := range db.Dots {
		start, previous := 0, ""
		if v, ok := s.verified[dot.ID]; ok && v.count <= len(dot.Events) &&
			(v.count == 0 || dot.Events[v.count-1].Hash == v.last) {
			start, previous = v.count, v.last
		}
		for i := start; i < len(dot.Events); i++ {
			e := dot.Events[i]
			if e.Sequence != uint64(i+1) || e.PrevHash != previous || e.Hash != eventHash(e) {
				return fmt.Errorf("Dot %s event %d is invalid", dot.ID, i+1)
			}
			previous = e.Hash
		}
		next[dot.ID] = verifiedChain{count: len(dot.Events), last: previous}
	}
	s.verified = next
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
