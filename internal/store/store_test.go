package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/task"
)

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

	path := filepath.Join(dir, "dots.json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	dots := raw["dots"].([]any)
	events := dots[0].(map[string]any)["events"].([]any)
	events[1].(map[string]any)["message"] = "changed without updating the hash"
	b, _ = json.Marshal(raw)
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("tampered event history unexpectedly loaded")
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

	data, err := os.ReadFile(filepath.Join(dir, "dots.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["schema_version"] = float64(1)
	dots := raw["dots"].([]any)
	legacyDot := dots[0].(map[string]any)
	legacyDot["source_root"] = "/Users/private/notes"
	legacyDot["output_dir"] = "/Users/private/output"
	legacyDot["artifacts"] = []any{map[string]any{"path": "/Users/private/output/result.md"}}
	raw["routines"] = []any{map[string]any{"name": "Legacy routine", "source_root": "/Users/private/notes"}}
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dots.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
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
	serialized, err := os.ReadFile(filepath.Join(dir, "dots.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"/Users/private", "source_root", "output_dir", "artifacts", "routines"} {
		if strings.Contains(string(serialized), forbidden) {
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
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dots.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["schema_version"] = float64(2)
	raw["skills"] = []any{map[string]any{"name": "Legacy skill"}}
	raw["routines"] = []any{map[string]any{"name": "Legacy schedule"}}
	data, err = json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := db.Snapshot()
	if snapshot.SchemaVersion != task.SchemaVersion || len(snapshot.Skills) != 0 || len(snapshot.Routines) != 0 {
		t.Fatalf("unexpected schema 2 migration: %#v", snapshot)
	}
}
