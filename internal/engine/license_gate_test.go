package engine

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"aether-dots/internal/license"
	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

// enableFreeLicensing turns licensing on for this test with no license saved,
// so the data folder is on the Free tier.
func enableFreeLicensing(t *testing.T) {
	t.Helper()
	org := license.OrganizationID
	t.Cleanup(func() { license.OrganizationID = org })
	license.OrganizationID = "11111111-2222-3333-4444-555555555555"
}

func TestFreeTierDoesNotRunRoutines(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Routine agent"})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Context check", Prompt: "Summarize.",
		IntervalSeconds: task.MinRoutineIntervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	enableFreeLicensing(t)
	generator := &testGenerator{ready: true, reply: "should not run"}
	if err := RunRoutineOnce(context.Background(), db, routine.ID, generator, nil); !errors.Is(err, license.ErrProRequired) {
		t.Fatalf("RunRoutineOnce on Free = %v, want ErrProRequired", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	before := db.Snapshot().Routines[0]
	started, err := RunDueRoutines(ctx, db, generator, nil)
	if err != nil || started != 0 {
		t.Fatalf("RunDueRoutines on Free started %d, err %v", started, err)
	}
	after := db.Snapshot()
	if len(after.Routines[0].Runs) != 0 || !after.Routines[0].NextRunAt.Equal(before.NextRunAt) || len(after.Conversations[0].Messages) != 0 {
		t.Fatalf("Free tier claimed or ran a routine: %#v", after.Routines[0])
	}
}

func TestFreeTierGetsNoGitHubWriteTools(t *testing.T) {
	e := newWriteEnv(t, []task.GitHubWriteKind{task.GitHubIssueComment})
	if len(e.tools(t).Config.Tools) == 0 {
		t.Fatal("precondition: a granted agent has write tools without licensing")
	}
	enableFreeLicensing(t)
	if n := len(e.tools(t).Config.Tools); n != 0 {
		t.Fatalf("Free tier received %d write tools", n)
	}
}
