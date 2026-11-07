package engine

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

type testGenerator struct {
	ready   bool
	reason  string
	reply   string
	started chan struct{}
}

func (g *testGenerator) Generate(ctx context.Context, objective string, onDelta func(string)) (string, error) {
	if g.started != nil {
		close(g.started)
	}
	if g.reply == "wait" {
		<-ctx.Done()
		return "", ctx.Err()
	}
	if onDelta != nil {
		onDelta(g.reply)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return g.reply, nil
}

func (g *testGenerator) Readiness() (bool, string) { return g.ready, g.reason }
func (*testGenerator) Description() string         { return "test provider" }

func TestRunStoresChatReplyWithoutUserSelectedFileAccess(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{
		SpaceID: db.Snapshot().Spaces[0].ID, Name: "Decision partner",
		Purpose: "Help me prepare concise decision briefs.", Instructions: "Use direct language.",
	})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Summarize the open questions.")
	if err != nil {
		t.Fatal(err)
	}
	generator := &testGenerator{ready: true, reply: "There are two open questions."}
	var streamed strings.Builder
	if err := Run(context.Background(), db, dot.ID, generator, func(delta string) { streamed.WriteString(delta) }); err != nil {
		t.Fatal(err)
	}
	got, ok := db.GetDot(dot.ID)
	if !ok || got.Status != task.Done {
		t.Fatalf("Dot status=%s found=%t", got.Status, ok)
	}
	snapshot := db.Snapshot()
	if len(snapshot.Conversations[0].Messages) != 2 || snapshot.Conversations[0].Messages[1].Content != generator.reply || streamed.String() != generator.reply {
		t.Fatalf("reply was not streamed and persisted: %#v", snapshot.Conversations[0].Messages)
	}
	if len(got.Events) < 3 || got.Events[len(got.Events)-1].Type != "status" {
		t.Fatalf("run event trail is incomplete: %#v", got.Events)
	}
}

func TestMissingModelWaitsForInput(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "No key"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "A request")
	if err != nil {
		t.Fatal(err)
	}
	generator := &testGenerator{ready: false, reason: "API key is missing"}
	if err := Run(context.Background(), db, dot.ID, generator, nil); err == nil {
		t.Fatal("missing model key did not stop the run")
	}
	got, _ := db.GetDot(dot.ID)
	if got.Status != task.WaitingInput {
		t.Fatalf("unexpected missing-model state: %s", got.Status)
	}
}

func TestCancellationPausesAndCanBeRetried(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Cancelable"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "Wait for cancellation")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	generator := &testGenerator{ready: true, reply: "wait", started: make(chan struct{})}
	done := make(chan error, 1)
	go func() { done <- Run(ctx, db, dot.ID, generator, nil) }()
	<-generator.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	got, _ := db.GetDot(dot.ID)
	if got.Status != task.Paused {
		t.Fatalf("status after cancellation=%s", got.Status)
	}
}

func TestRunWithProgressPublishesModelPhases(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Progress"})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, "One message")
	if err != nil {
		t.Fatal(err)
	}
	var checkpoints []string
	err = RunWithProgress(context.Background(), db, dot.ID, &testGenerator{ready: true, reply: "Reply"}, nil, func() {
		current, ok := db.GetDot(dot.ID)
		if !ok || (current.Status != task.Running && current.Status != task.Done) {
			t.Fatalf("progress callback ran before status was persisted: %#v", current)
		}
		checkpoints = append(checkpoints, current.Checkpoint)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checkpoints) != 2 || checkpoints[0] != "generating response" || checkpoints[1] != "" {
		t.Fatalf("unexpected persisted phase notifications: %#v", checkpoints)
	}
}

func TestRoutineTestUsesSharedConversationEngineAndRecordsHistory(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Routine Dot"})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Context check", Prompt: "Summarize the latest context.",
		IntervalSeconds: task.MinRoutineIntervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	generator := &testGenerator{ready: true, reply: "Routine summary."}
	if err := RunRoutineOnce(context.Background(), db, routine.ID, generator, nil); err != nil {
		t.Fatal(err)
	}
	snapshot := db.Snapshot()
	if len(snapshot.Routines) != 1 || snapshot.Routines[0].LastStatus != "succeeded" || len(snapshot.Routines[0].Runs) != 1 {
		t.Fatalf("routine run history was not recorded: %#v", snapshot.Routines)
	}
	if len(snapshot.Conversations) != 1 || len(snapshot.Conversations[0].Messages) != 2 ||
		!strings.Contains(snapshot.Conversations[0].Messages[0].Content, "Routine: Context check") ||
		snapshot.Conversations[0].Messages[1].Content != "Routine summary." {
		t.Fatalf("routine did not use the normal conversation path: %#v", snapshot.Conversations)
	}
	if snapshot.Routines[0].Runs[0].Status != "succeeded" {
		t.Fatalf("routine run status=%q", snapshot.Routines[0].Runs[0].Status)
	}
}
