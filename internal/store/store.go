package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
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

const (
	storeFileName  = "dots.db"
	legacyFileName = "dots.json"
)

type Store struct {
	mu      sync.RWMutex
	leaseMu sync.Mutex
	path    string
	db      task.Database
	sqlDB   *sql.DB
	lease   *os.File
	// runLeases holds the per-Dot run ownership locks this process owns.
	runLeases map[string]*os.File
	// dataVersion is SQLite's PRAGMA data_version as of the state in s.db. It
	// changes only when another process commits, which is the cheap signal that
	// a second process (Desktop or CLI) sharing the folder wrote something.
	dataVersion int64
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

// Open opens the task store in dir, importing a legacy dots.json the first time.
func Open(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("data directory is empty")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	path := filepath.Join(dir, storeFileName)
	legacy := filepath.Join(dir, legacyFileName)
	sqlDB, err := openSQL(path)
	if err != nil {
		return nil, fmt.Errorf("open task store: %w", err)
	}
	s := &Store{path: path, sqlDB: sqlDB}
	if err := s.initialize(legacy); err != nil {
		sqlDB.Close()
		return nil, err
	}
	for _, name := range []string{path, path + "-wal", path + "-shm"} {
		_ = os.Chmod(name, 0o600)
	}
	return s, nil
}

// Close releases the database handle. Dots and history stay on disk.
func (s *Store) Close() error { return s.sqlDB.Close() }

func (s *Store) initialize(legacyPath string) error {
	ctx := context.Background()
	conn, err := s.sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("lock task store: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	loaded, ok, err := loadAll(ctx, conn)
	if err != nil {
		return fmt.Errorf("read task store: %w", err)
	}
	imported := false
	var db task.Database
	switch {
	case ok:
		db = loaded
	default:
		b, err := os.ReadFile(legacyPath)
		switch {
		case err == nil:
			if err := json.Unmarshal(b, &db); err != nil {
				return fmt.Errorf("decode task store: %w", err)
			}
			imported = true
		case errors.Is(err, os.ErrNotExist):
			space, _ := newID()
			db = task.Database{SchemaVersion: task.SchemaVersion, Spaces: []task.Space{{ID: space, Name: "Personal", CreatedAt: time.Now().UTC()}}}
		default:
			return fmt.Errorf("read task store: %w", err)
		}
	}
	if db.SchemaVersion < 1 || db.SchemaVersion > task.SchemaVersion {
		return fmt.Errorf("unsupported task store schema %d (expected %d)", db.SchemaVersion, task.SchemaVersion)
	}
	if err := verifyEvents(db); err != nil {
		return fmt.Errorf("task event history failed integrity check: %w", err)
	}
	if db.SchemaVersion < task.SchemaVersion {
		migrateDatabase(&db)
	}
	if err := s.verifyEventsIncremental(db); err != nil {
		return err
	}
	prev := loaded
	if !ok {
		prev = task.Database{}
	}
	if !ok || prev.SchemaVersion != db.SchemaVersion || !reflectEqualState(prev, db) {
		if err := writeDiff(ctx, conn, prev, db); err != nil {
			return fmt.Errorf("write task store: %w", err)
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit task store: %w", err)
	}
	committed = true
	dv, err := dataVersion(ctx, conn)
	if err != nil {
		return err
	}
	s.db, s.dataVersion = db, dv
	if imported {
		// Keep the original as a backup; it is no longer read.
		if err := os.Rename(legacyPath, legacyPath+".migrated"); err != nil {
			return fmt.Errorf("retire legacy store: %w", err)
		}
	}
	return nil
}

func reflectEqualState(a, b task.Database) bool {
	// Migrations that changed nothing need no rewrite; DeepEqual on the cloned
	// state is cheap relative to a full rewrite.
	return a.SchemaVersion == b.SchemaVersion && deepEqualDB(a, b)
}

// migrateDatabase upgrades an older task schema in memory. Newer opt-in
// features start closed: no migration grants any access.
func migrateDatabase(db *task.Database) {
	if db.SchemaVersion < 8 {
		// GitHub write approvals are opt-in: no Dot starts with a write grant
		// and the tray starts empty.
		db.GitHubProposals = nil
		for i := range db.Conversations {
			db.Conversations[i].GitHubWriteGrants = nil
			db.Conversations[i].GitHubWriteGrantSpaceID = ""
			db.Conversations[i].GitHubWriteDisclosureAccepted = false
		}
	}
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
		migrateLegacyFileAccess(db)
	}
	if db.SchemaVersion < 3 {
		// Skills and routines are introduced in schema 3. Ignore similarly
		// named fields from older stores rather than importing legacy data.
		db.Skills = nil
		db.Routines = nil
		db.RoutineRunStarts = nil
	}
	db.SchemaVersion = task.SchemaVersion
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
// When nothing else committed it costs a single PRAGMA read.
func (s *Store) reloadLocked() error {
	ctx := context.Background()
	dv, err := dataVersion(ctx, s.sqlDB)
	if err != nil {
		return err
	}
	if dv == s.dataVersion {
		return nil
	}
	return s.adoptLocked(ctx, s.sqlDB, dv)
}

func (s *Store) adoptLocked(ctx context.Context, q queryer, dv int64) error {
	db, ok, err := loadAll(ctx, q)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("task store is empty")
	}
	if db.SchemaVersion != task.SchemaVersion {
		return fmt.Errorf("task store schema changed to %d while running (expected %d)", db.SchemaVersion, task.SchemaVersion)
	}
	if err := s.verifyEventsIncremental(db); err != nil {
		return fmt.Errorf("task event history failed integrity check: %w", err)
	}
	s.db, s.dataVersion = db, dv
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

// change runs fn against a copy of the latest committed state and commits only
// the rows it altered. BEGIN IMMEDIATE serializes writers across processes.
func (s *Store) change(fn func(*task.Database) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx := context.Background()
	conn, err := s.sqlDB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("lock task store: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(ctx, "ROLLBACK")
		}
	}()
	dv, err := dataVersion(ctx, conn)
	if err != nil {
		return err
	}
	if dv != s.dataVersion {
		if err := s.adoptLocked(ctx, conn, dv); err != nil {
			return err
		}
	}
	next := cloneDatabase(s.db)
	if err := fn(&next); err != nil {
		return err
	}
	reconcileGitHubProposals(&next, time.Now().UTC())
	pruneGitHubWriteGrants(&next)
	if err := writeDiff(ctx, conn, s.db, next); err != nil {
		return fmt.Errorf("write task store: %w", err)
	}
	verified := s.verified
	if err := s.verifyEventsIncremental(next); err != nil {
		s.verified = verified
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		s.verified = verified
		return fmt.Errorf("commit task store: %w", err)
	}
	committed = true
	s.db = next
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
