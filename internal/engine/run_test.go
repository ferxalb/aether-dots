package engine

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aether-dots/internal/model"
	"aether-dots/internal/source"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

func TestLocalRunWritesVerifiesAndPersistsDot(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := filepath.Join(dir, "notes"), filepath.Join(dir, "output")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "flow.md"), []byte("# Checkout\nTODO: decide whether guests can save addresses\nThe total updates after payment."), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	dot, err := db.CreateDot(task.CreateDot{SpaceID: db.Snapshot().Spaces[0].ID, Objective: "Find open design decisions", Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), db, dot.ID, nil, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := db.GetDot(dot.ID)
	if got.Status != task.Done || len(got.Artifacts) != 1 {
		t.Fatalf("status=%s artifacts=%d", got.Status, len(got.Artifacts))
	}
	b, err := os.ReadFile(got.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(string(b), "flow.md:2") || !contains(string(b), "TODO: decide") {
		t.Fatalf("report lacks source evidence: %s", b)
	}
	if got.Artifacts[0].SHA256 == "" {
		t.Fatal("artifact hash missing")
	}
	if err := Run(context.Background(), db, dot.ID, nil, nil); err == nil {
		t.Fatal("completed Dot reran without explicit new capture")
	}
	if _, err := store.Open(filepath.Join(dir, "store")); err != nil {
		t.Fatalf("persisted event chain did not reopen: %v", err)
	}
}

func TestMissingModelStopsAtWaitingInputAndCancellationPauses(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := filepath.Join(dir, "notes"), filepath.Join(dir, "output")
	if err := os.MkdirAll(sourceDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "note.txt"), []byte("Decision: choose a flow"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	makeDot := func(mode string) task.Dot {
		dot, err := db.CreateDot(task.CreateDot{SpaceID: db.Snapshot().Spaces[0].ID, Objective: "Prepare a brief", Mode: mode, SourceRoot: sourceDir, OutputDir: outputDir})
		if err != nil {
			t.Fatal(err)
		}
		return dot
	}
	missing := makeDot("model")
	generator := model.Generator{Config: model.Config{}}
	if err := Run(context.Background(), db, missing.ID, &generator, nil); err == nil {
		t.Fatal("missing model key did not stop the run")
	}
	got, _ := db.GetDot(missing.ID)
	if got.Status != task.WaitingInput || len(got.Artifacts) != 0 {
		t.Fatalf("unexpected missing-model state: %s", got.Status)
	}

	paused := makeDot("model")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Run(ctx, db, paused.ID, &generatorWithCancel{}, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	got, _ = db.GetDot(paused.ID)
	if got.Status != task.Paused {
		t.Fatalf("status after cancellation = %s", got.Status)
	}
}

func TestOutputFolderCannotBeInsideSource(t *testing.T) {
	dir := t.TempDir()
	sourceDir, outputDir := filepath.Join(dir, "notes"), filepath.Join(dir, "notes", "output")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "note.md"), []byte("TODO: choose a default"), 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	dot, err := db.CreateDot(task.CreateDot{SpaceID: db.Snapshot().Spaces[0].ID, Objective: "Prepare a brief", Mode: "local", SourceRoot: sourceDir, OutputDir: outputDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := Run(context.Background(), db, dot.ID, nil, nil); err == nil {
		t.Fatal("output nested inside the source was accepted")
	}
	got, _ := db.GetDot(dot.ID)
	if got.Status != task.Failed || len(got.Artifacts) != 0 {
		t.Fatalf("unexpected state after scope failure: status=%s artifacts=%d", got.Status, len(got.Artifacts))
	}
}

type generatorWithCancel struct{}

func (*generatorWithCancel) Generate(ctx context.Context, _ string, _ []source.Document, _ func(string)) (string, error) {
	<-ctx.Done()
	return "", ctx.Err()
}

func (*generatorWithCancel) Readiness() (bool, string) { return true, "" }

func (*generatorWithCancel) Description() string { return "test provider" }

func contains(s, part string) bool {
	return len(s) >= len(part) && strings.Contains(s, part)
}
