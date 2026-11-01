package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"aether-dots/desktop/internal/task"
)

func TestStoreRoundTripAndRejectsChangedEvent(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	db := s.Snapshot()
	dot, err := s.CreateDot(task.CreateDot{
		SpaceID: db.Spaces[0].ID, Objective: "Read the notes", Mode: "local",
		SourceRoot: filepath.Join(dir, "source"), OutputDir: filepath.Join(dir, "out"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AppendEvent(dot.ID, "source_read", "Read one note."); err != nil {
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

func TestRoutineKeepsExactGrantsAfterEditAndRun(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot()
	readRoot, outputDir := filepath.Join(dir, "notes"), filepath.Join(dir, "briefs")
	dot, err := s.CreateDot(task.CreateDot{SpaceID: snapshot.Spaces[0].ID, Objective: "Prepare decisions", Mode: "local", SourceRoot: readRoot, OutputDir: outputDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddArtifact(dot.ID, task.Artifact{Path: filepath.Join(outputDir, "brief.md"), SHA256: "verified", Bytes: 12, VerifiedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetStatus(dot.ID, task.Done, "verified"); err != nil {
		t.Fatal(err)
	}
	routine, err := s.PromoteRoutine(dot.ID, "Decision brief")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.EditRoutine(routine.ID, "Weekly brief", "Group open decisions"); err != nil {
		t.Fatal(err)
	}
	newDot, err := s.DotFromRoutine(routine.ID, snapshot.Spaces[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if newDot.SourceRoot != readRoot || newDot.OutputDir != outputDir {
		t.Fatalf("routine grants changed: %#v", newDot)
	}
	if newDot.Objective != "Group open decisions" || newDot.Mode != "local" {
		t.Fatalf("routine edit not applied: %#v", newDot)
	}
}
