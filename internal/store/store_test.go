package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/task"
)

// legacyDir builds a new data folder holding only a legacy dots.json: the
// store's current state serialized as JSON and then edited by mutate, so
// migration and import paths can be tested against old-format files.
func legacyDir(t *testing.T, s *Store, mutate func(raw map[string]any)) string {
	t.Helper()
	b, err := json.Marshal(s.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	mutate(raw)
	b, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, legacyFileName), b, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestStoreRoundTripAndRejectsChangedEvent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(task.CreateConversation{SpaceID: s.Snapshot().Spaces[0].ID, Name: "Decision Dot"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := s.CreateConversationRun(conversation.ID, "Summarize this discussion")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(dot.ID, "model_started", "Reply started."); err != nil {
		t.Fatal(err)
	}
	opened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, ok := opened.GetDot(dot.ID)
	if !ok || len(reloaded.Events) != 2 {
		t.Fatalf("reloaded Dot: found=%t events=%d", ok, len(reloaded.Events))
	}
	if reloaded.Events[1].PrevHash != reloaded.Events[0].Hash {
		t.Fatal("event hashes are not linked")
	}

	// The same tampering inside an imported legacy dots.json is rejected too.
	legacy := legacyDir(t, opened, func(raw map[string]any) {
		events := raw["dots"].([]any)[0].(map[string]any)["events"].([]any)
		events[1].(map[string]any)["message"] = "changed without updating the hash"
	})
	// Tamper with a stored event row directly; the chain check must reject it.
	opened.Close()
	raw, err := openSQL(filepath.Join(dir, storeFileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(`UPDATE events SET body = replace(body, '"message":"', '"message":"changed without updating the hash ') WHERE seq = 2`); err != nil {
		t.Fatal(err)
	}
	raw.Close()
	if _, err := Open(dir); err == nil {
		t.Fatal("tampered event history unexpectedly loaded")
	}
	if _, err := Open(legacy); err == nil {
		t.Fatal("tampered legacy event history unexpectedly imported")
	}
}

func TestSchemaV1MigrationDropsLocalScopesArtifactsAndRoutines(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(task.CreateConversation{SpaceID: s.Snapshot().Spaces[0].ID, Name: "Old Dot"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := s.CreateConversationRun(conversation.ID, "Old task")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(dot.ID, "source_read", "Read /Users/private/notes/source.md"); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(dot.ID, "artifact_verified", "Saved /Users/private/output/result.md"); err != nil {
		t.Fatal(err)
	}

	legacy := legacyDir(t, s, func(raw map[string]any) {
		raw["schema_version"] = float64(1)
		legacyDot := raw["dots"].([]any)[0].(map[string]any)
		legacyDot["source_root"] = "/Users/private/notes"
		legacyDot["output_dir"] = "/Users/private/output"
		legacyDot["artifacts"] = []any{map[string]any{"path": "/Users/private/output/result.md"}}
		raw["routines"] = []any{map[string]any{"name": "Legacy routine", "source_root": "/Users/private/notes"}}
	})

	reopened, err := Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := reopened.Snapshot()
	if snapshot.SchemaVersion != task.SchemaVersion {
		t.Fatalf("schema version=%d, want %d", snapshot.SchemaVersion, task.SchemaVersion)
	}
	if len(snapshot.Dots) != 1 || len(snapshot.Dots[0].Events) != 3 {
		t.Fatalf("legacy timeline was not retained in sanitized form: %#v", snapshot.Dots)
	}
	for _, event := range snapshot.Dots[0].Events {
		if strings.Contains(event.Message, "/Users/private") || event.Type == "source_read" || event.Type == "artifact_verified" {
			t.Fatalf("legacy local path survived migration: %#v", event)
		}
	}
	serializedJSON, err := json.Marshal(reopened.Snapshot())
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(serializedJSON)
	for _, forbidden := range []string{"/Users/private", "source_root", "output_dir", "artifacts", "routines"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("retired local field %q survived migration: %s", forbidden, serialized)
		}
	}
}

func TestRecoverInterruptedConversationRun(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Recovery"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "A message")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Running, "Started."); err != nil {
		t.Fatal(err)
	}
	simulateCrash(db, dot.ID)
	recovered, err := db.RecoverInterruptedRuns()
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	got, ok := db.GetDot(dot.ID)
	if !ok || got.Status != task.Paused || got.UpdatedAt.After(time.Now().Add(time.Second)) {
		t.Fatalf("unexpected recovered state: %#v", got)
	}
}

func TestSchemaV2MigrationStartsAutomationCollectionsEmpty(t *testing.T) {
	first, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy := legacyDir(t, first, func(raw map[string]any) {
		raw["schema_version"] = float64(2)
		raw["skills"] = []any{map[string]any{"name": "Legacy skill"}}
		raw["routines"] = []any{map[string]any{"name": "Legacy schedule"}}
	})
	db, err := Open(legacy)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := db.Snapshot()
	if snapshot.SchemaVersion != task.SchemaVersion || len(snapshot.Skills) != 0 || len(snapshot.Routines) != 0 {
		t.Fatalf("unexpected schema 2 migration: %#v", snapshot)
	}
}

func TestTwoStoreHandlesMergeInsteadOfOverwriting(t *testing.T) {
	dir := t.TempDir()
	desktop, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	cli, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := desktop.CreateSpace("From Desktop"); err != nil {
		t.Fatal(err)
	}
	// The CLI handle has not seen the Desktop write yet; its own write must
	// merge with it rather than replacing the file with a stale copy.
	if _, err := cli.CreateSpace("From CLI"); err != nil {
		t.Fatal(err)
	}
	for _, handle := range []*Store{desktop, cli} {
		names := map[string]bool{}
		for _, space := range handle.Snapshot().Spaces {
			names[space.Name] = true
		}
		if !names["From Desktop"] || !names["From CLI"] || !names["Personal"] {
			t.Fatalf("lost update across handles: %v", names)
		}
	}
}

func TestConcurrentHandlesNeverLoseSpaces(t *testing.T) {
	dir := t.TempDir()
	const writers, each = 4, 5
	done := make(chan error, writers)
	for w := 0; w < writers; w++ {
		go func(w int) {
			handle, err := Open(dir)
			if err == nil {
				for i := 0; i < each && err == nil; i++ {
					_, err = handle.CreateSpace(fmt.Sprintf("w%d-%d", w, i))
				}
			}
			done <- err
		}(w)
	}
	for w := 0; w < writers; w++ {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	handle, _ := Open(dir)
	if got := len(handle.Snapshot().Spaces); got != 1+writers*each {
		t.Fatalf("expected %d spaces, got %d", 1+writers*each, got)
	}
}
