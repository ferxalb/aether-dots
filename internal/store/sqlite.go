package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	_ "modernc.org/sqlite"

	"aether-dots/internal/task"
)

// The kernel persists the task.Database as rows instead of one JSON document.
// Small entities (spaces, skills, routines, ...) are one JSON row each; the two
// unbounded lists, Dot events and conversation messages, are append-only row
// tables. A commit writes only the rows that differ from the last committed
// state, so the cost of a change is proportional to what changed, not to the
// size of the whole history. WAL mode lets Desktop and CLI processes share the
// file; PRAGMA data_version tells a process that another one committed.

const sqlUserVersion = 1

const sqlSchema = `
CREATE TABLE IF NOT EXISTS meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS spaces (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS dots (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, status TEXT NOT NULL, updated_at TEXT NOT NULL, body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS events (dot_id TEXT NOT NULL, seq INTEGER NOT NULL, type TEXT NOT NULL, ts TEXT NOT NULL, hash TEXT NOT NULL, body TEXT NOT NULL, PRIMARY KEY (dot_id, seq)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS conversations (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS messages (conv_id TEXT NOT NULL, pos INTEGER NOT NULL, id TEXT NOT NULL, role TEXT NOT NULL, created_at TEXT NOT NULL, body TEXT NOT NULL, PRIMARY KEY (conv_id, pos)) WITHOUT ROWID;
CREATE INDEX IF NOT EXISTS messages_by_id ON messages (id);
CREATE TABLE IF NOT EXISTS skills (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS routines (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS github_repositories (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, body TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS mcp_servers (id TEXT PRIMARY KEY, pos INTEGER NOT NULL, body TEXT NOT NULL);
`

// openSQL opens (creating if needed) the database with one dedicated
// connection, so PRAGMA data_version is meaningful for this process.
func openSQL(path string) (*sql.DB, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(15000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(OFF)&_pragma=wal_autocheckpoint(1000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)
	// Several processes may open a fresh folder at once; WAL setup and DDL can
	// report SQLITE_BUSY before the busy timeout applies, so retry briefly.
	var initErr error
	for attempt := 0; attempt < 100; attempt++ {
		if _, initErr = db.Exec(sqlSchema); initErr == nil {
			break
		}
		time.Sleep(time.Duration(5+attempt) * time.Millisecond)
	}
	if initErr != nil {
		db.Close()
		return nil, fmt.Errorf("initialise task store: %w", initErr)
	}
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version > sqlUserVersion {
		db.Close()
		return nil, fmt.Errorf("task store layout %d is newer than this build supports (%d)", version, sqlUserVersion)
	}
	if version < sqlUserVersion {
		if _, err := db.Exec("PRAGMA user_version = " + strconv.Itoa(sqlUserVersion)); err != nil {
			db.Close()
			return nil, err
		}
	}
	return db, nil
}

func dataVersion(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (int64, error) {
	var v int64
	err := q.QueryRowContext(ctx, "PRAGMA data_version").Scan(&v)
	return v, err
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type execer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func ts(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

// loadAll reads the whole committed state. ok is false for an empty database
// (no meta row), which Open treats as a fresh store or a legacy import.
func loadAll(ctx context.Context, q queryer) (db task.Database, ok bool, err error) {
	meta := map[string]string{}
	rows, err := q.QueryContext(ctx, "SELECT key, value FROM meta")
	if err != nil {
		return db, false, err
	}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return db, false, err
		}
		meta[k] = v
	}
	if err := rows.Close(); err != nil {
		return db, false, err
	}
	if _, has := meta["schema_version"]; !has {
		return db, false, nil
	}
	db.SchemaVersion, _ = strconv.Atoi(meta["schema_version"])
	db.GitHubConnected = meta["github_connected"] == "1"
	if v := meta["github_last_verified_at"]; v != "" {
		db.GitHubLastVerifiedAt, _ = time.Parse(time.RFC3339Nano, v)
	}
	if v := meta["routine_run_starts"]; v != "" {
		if err := json.Unmarshal([]byte(v), &db.RoutineRunStarts); err != nil {
			return db, false, fmt.Errorf("decode routine run starts: %w", err)
		}
	}
	if err := loadBodies(ctx, q, "spaces", &db.Spaces); err != nil {
		return db, false, err
	}
	if err := loadBodies(ctx, q, "skills", &db.Skills); err != nil {
		return db, false, err
	}
	if err := loadBodies(ctx, q, "routines", &db.Routines); err != nil {
		return db, false, err
	}
	if err := loadBodies(ctx, q, "github_repositories", &db.GitHubRepositories); err != nil {
		return db, false, err
	}
	if err := loadBodies(ctx, q, "mcp_servers", &db.MCPServers); err != nil {
		return db, false, err
	}
	if err := loadBodies(ctx, q, "dots", &db.Dots); err != nil {
		return db, false, err
	}
	if err := loadBodies(ctx, q, "conversations", &db.Conversations); err != nil {
		return db, false, err
	}
	dotAt := map[string]int{}
	for i, d := range db.Dots {
		dotAt[d.ID] = i
	}
	rows, err = q.QueryContext(ctx, "SELECT dot_id, body FROM events ORDER BY dot_id, seq")
	if err != nil {
		return db, false, err
	}
	for rows.Next() {
		var id, body string
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return db, false, err
		}
		i, known := dotAt[id]
		if !known {
			rows.Close()
			return db, false, fmt.Errorf("event for unknown Dot %s", id)
		}
		var e task.Event
		if err := json.Unmarshal([]byte(body), &e); err != nil {
			rows.Close()
			return db, false, fmt.Errorf("decode event: %w", err)
		}
		db.Dots[i].Events = append(db.Dots[i].Events, e)
	}
	if err := rows.Close(); err != nil {
		return db, false, err
	}
	convAt := map[string]int{}
	for i, c := range db.Conversations {
		convAt[c.ID] = i
	}
	rows, err = q.QueryContext(ctx, "SELECT conv_id, body FROM messages ORDER BY conv_id, pos")
	if err != nil {
		return db, false, err
	}
	for rows.Next() {
		var id, body string
		if err := rows.Scan(&id, &body); err != nil {
			rows.Close()
			return db, false, err
		}
		i, known := convAt[id]
		if !known {
			rows.Close()
			return db, false, fmt.Errorf("message for unknown conversation %s", id)
		}
		var m task.ConversationMessage
		if err := json.Unmarshal([]byte(body), &m); err != nil {
			rows.Close()
			return db, false, fmt.Errorf("decode message: %w", err)
		}
		db.Conversations[i].Messages = append(db.Conversations[i].Messages, m)
	}
	if err := rows.Close(); err != nil {
		return db, false, err
	}
	for i := range db.Dots {
		if db.Dots[i].Events == nil {
			db.Dots[i].Events = []task.Event{}
		}
	}
	for i := range db.Conversations {
		if db.Conversations[i].Messages == nil {
			db.Conversations[i].Messages = []task.ConversationMessage{}
		}
	}
	return db, true, nil
}

func loadBodies[T any](ctx context.Context, q queryer, table string, out *[]T) error {
	rows, err := q.QueryContext(ctx, "SELECT body FROM "+table+" ORDER BY pos")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var body string
		if err := rows.Scan(&body); err != nil {
			return err
		}
		var v T
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			return fmt.Errorf("decode %s row: %w", table, err)
		}
		*out = append(*out, v)
	}
	return rows.Err()
}

// writeDiff applies the difference between prev and next. A nil-equivalent
// prev (zero Database) writes everything.
func writeDiff(ctx context.Context, x execer, prev, next task.Database) error {
	if err := writeMeta(ctx, x, prev, next); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "spaces", prev.Spaces, next.Spaces, func(v task.Space) string { return v.ID }, nil); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "skills", prev.Skills, next.Skills, func(v task.Skill) string { return v.ID }, nil); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "routines", prev.Routines, next.Routines, func(v task.Routine) string { return v.ID }, nil); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "github_repositories", prev.GitHubRepositories, next.GitHubRepositories,
		func(v task.GitHubRepository) string { return strconv.FormatInt(v.ID, 10) }, nil); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "mcp_servers", prev.MCPServers, next.MCPServers, func(v task.MCPServer) string { return v.ID }, nil); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "dots", prev.Dots, next.Dots, func(v task.Dot) string { return v.ID },
		func(v task.Dot) (any, []string) {
			v.Events = nil
			return v, []string{string(v.Status), ts(v.UpdatedAt)}
		}); err != nil {
		return err
	}
	if err := diffRows(ctx, x, "conversations", prev.Conversations, next.Conversations, func(v task.Conversation) string { return v.ID },
		func(v task.Conversation) (any, []string) { v.Messages = nil; return v, nil }); err != nil {
		return err
	}
	return writeChildren(ctx, x, prev, next)
}

func writeMeta(ctx context.Context, x execer, prev, next task.Database) error {
	set := func(key, value string) error {
		_, err := x.ExecContext(ctx, "INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
		return err
	}
	b := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	if prev.SchemaVersion != next.SchemaVersion || prev.SchemaVersion == 0 {
		if err := set("schema_version", strconv.Itoa(next.SchemaVersion)); err != nil {
			return err
		}
	}
	if prev.GitHubConnected != next.GitHubConnected || prev.SchemaVersion == 0 {
		if err := set("github_connected", b(next.GitHubConnected)); err != nil {
			return err
		}
	}
	if !prev.GitHubLastVerifiedAt.Equal(next.GitHubLastVerifiedAt) || prev.SchemaVersion == 0 {
		if err := set("github_last_verified_at", ts(next.GitHubLastVerifiedAt)); err != nil {
			return err
		}
	}
	if !reflect.DeepEqual(prev.RoutineRunStarts, next.RoutineRunStarts) || prev.SchemaVersion == 0 {
		raw, err := json.Marshal(next.RoutineRunStarts)
		if err != nil {
			return err
		}
		if err := set("routine_run_starts", string(raw)); err != nil {
			return err
		}
	}
	return nil
}

// diffRows upserts changed rows and deletes removed ones. extra optionally
// returns the row's persisted body (with child lists stripped) plus the extra
// indexed columns for the dots table.
func diffRows[T any](ctx context.Context, x execer, table string, prev, next []T, id func(T) string, extra func(T) (any, []string)) error {
	before := make(map[string]int, len(prev))
	for i, v := range prev {
		before[id(v)] = i
	}
	seen := make(map[string]struct{}, len(next))
	for i, v := range next {
		key := id(v)
		seen[key] = struct{}{}
		body, cols := any(v), []string(nil)
		if extra != nil {
			body, cols = extra(v)
		}
		if j, existed := before[key]; existed && j == i {
			if sameRow(prev[j], v, body) {
				continue
			}
		}
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		var query string
		var args []any
		if table == "dots" {
			query = "INSERT INTO dots (id, pos, status, updated_at, body) VALUES (?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET pos = excluded.pos, status = excluded.status, updated_at = excluded.updated_at, body = excluded.body"
			args = []any{key, i, cols[0], cols[1], string(raw)}
		} else {
			query = "INSERT INTO " + table + " (id, pos, body) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET pos = excluded.pos, body = excluded.body"
			args = []any{key, i, string(raw)}
		}
		if _, err := x.ExecContext(ctx, query, args...); err != nil {
			return fmt.Errorf("write %s: %w", table, err)
		}
	}
	for _, v := range prev {
		key := id(v)
		if _, kept := seen[key]; kept {
			continue
		}
		if _, err := x.ExecContext(ctx, "DELETE FROM "+table+" WHERE id = ?", key); err != nil {
			return fmt.Errorf("delete %s: %w", table, err)
		}
	}
	return nil
}

// writeChildren appends new events and messages. A history that changed
// anywhere before its previous end is rewritten from the first difference.
func writeChildren(ctx context.Context, x execer, prev, next task.Database) error {
	prevDots := make(map[string]*task.Dot, len(prev.Dots))
	for i := range prev.Dots {
		prevDots[prev.Dots[i].ID] = &prev.Dots[i]
	}
	for i := range next.Dots {
		d := &next.Dots[i]
		var old []task.Event
		if p := prevDots[d.ID]; p != nil {
			old = p.Events
		}
		from := commonEventPrefix(old, d.Events)
		if from < len(old) {
			if _, err := x.ExecContext(ctx, "DELETE FROM events WHERE dot_id = ? AND seq > ?", d.ID, from); err != nil {
				return err
			}
		}
		for _, e := range d.Events[from:] {
			raw, err := json.Marshal(e)
			if err != nil {
				return err
			}
			if _, err := x.ExecContext(ctx, "INSERT OR REPLACE INTO events (dot_id, seq, type, ts, hash, body) VALUES (?, ?, ?, ?, ?, ?)",
				d.ID, e.Sequence, e.Type, ts(e.Timestamp), e.Hash, string(raw)); err != nil {
				return fmt.Errorf("write event: %w", err)
			}
		}
	}
	nextDots := make(map[string]struct{}, len(next.Dots))
	for i := range next.Dots {
		nextDots[next.Dots[i].ID] = struct{}{}
	}
	for _, p := range prev.Dots {
		if _, kept := nextDots[p.ID]; !kept {
			if _, err := x.ExecContext(ctx, "DELETE FROM events WHERE dot_id = ?", p.ID); err != nil {
				return err
			}
		}
	}
	prevConvs := make(map[string]*task.Conversation, len(prev.Conversations))
	for i := range prev.Conversations {
		prevConvs[prev.Conversations[i].ID] = &prev.Conversations[i]
	}
	for i := range next.Conversations {
		c := &next.Conversations[i]
		var old []task.ConversationMessage
		if p := prevConvs[c.ID]; p != nil {
			old = p.Messages
		}
		from := 0
		for from < len(old) && from < len(c.Messages) && old[from] == c.Messages[from] {
			from++
		}
		if from < len(old) {
			if _, err := x.ExecContext(ctx, "DELETE FROM messages WHERE conv_id = ? AND pos >= ?", c.ID, from); err != nil {
				return err
			}
		}
		for pos := from; pos < len(c.Messages); pos++ {
			m := c.Messages[pos]
			raw, err := json.Marshal(m)
			if err != nil {
				return err
			}
			if _, err := x.ExecContext(ctx, "INSERT OR REPLACE INTO messages (conv_id, pos, id, role, created_at, body) VALUES (?, ?, ?, ?, ?, ?)",
				c.ID, pos, m.ID, m.Role, ts(m.CreatedAt), string(raw)); err != nil {
				return fmt.Errorf("write message: %w", err)
			}
		}
	}
	nextConvs := make(map[string]struct{}, len(next.Conversations))
	for i := range next.Conversations {
		nextConvs[next.Conversations[i].ID] = struct{}{}
	}
	for _, p := range prev.Conversations {
		if _, found := nextConvs[p.ID]; !found {
			if _, err := x.ExecContext(ctx, "DELETE FROM messages WHERE conv_id = ?", p.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// commonEventPrefix returns how many leading events are unchanged. Events are
// hash-chained, so an identical last-old event proves an identical prefix.
func commonEventPrefix(old, next []task.Event) int {
	if len(old) == 0 {
		return 0
	}
	if len(next) >= len(old) && old[len(old)-1] == next[len(old)-1] {
		return len(old)
	}
	n := 0
	for n < len(old) && n < len(next) && old[n] == next[n] {
		n++
	}
	return n
}

func deepEqualDB(a, b task.Database) bool { return reflect.DeepEqual(a, b) }

// sameRow reports whether two versions of a row have the same persisted body.
// Dots are by far the most numerous rows, so they get a field-by-field
// comparison instead of reflection; TestDotEqualCoversEveryField keeps it honest.
func sameRow[T any](old, cur T, curBody any) bool {
	if a, ok := any(old).(task.Dot); ok {
		return dotEqual(a, any(cur).(task.Dot))
	}
	oldBody := any(old)
	if c, ok := any(old).(task.Conversation); ok {
		c.Messages = nil
		oldBody = c
	}
	return reflect.DeepEqual(oldBody, curBody)
}

func dotEqual(a, b task.Dot) bool {
	if a.Metrics != b.Metrics && (a.Metrics == nil || b.Metrics == nil || *a.Metrics != *b.Metrics) {
		return false
	}
	return a.ID == b.ID && a.SpaceID == b.SpaceID && a.ConversationID == b.ConversationID &&
		a.ConversationContextDeferred == b.ConversationContextDeferred &&
		a.ParentDotID == b.ParentDotID && a.DelegatedByDotID == b.DelegatedByDotID &&
		a.SkillID == b.SkillID && a.RoutineID == b.RoutineID && a.Name == b.Name &&
		a.Objective == b.Objective && a.Status == b.Status && a.Checkpoint == b.Checkpoint &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt)
}
