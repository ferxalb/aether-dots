package store

import (
	"errors"
	"os"
	"syscall"
	"testing"

	"aether-dots/internal/task"
)

// simulateCrash drops a Dot's run lease without any status update, exactly as
// the kernel does when the owning process is killed.
func simulateCrash(db *Store, dotID string) {
	db.leaseMu.Lock()
	defer db.leaseMu.Unlock()
	if file, ok := db.runLeases[dotID]; ok {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		delete(db.runLeases, dotID)
	}
}

func startRunningDot(t *testing.T, db *Store, text string) task.Dot {
	t.Helper()
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: text})
	if err != nil {
		t.Fatal(err)
	}
	dot, _, err := db.CreateConversationRun(conversation.ID, text)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetStatus(dot.ID, task.Running, "Started."); err != nil {
		t.Fatal(err)
	}
	return dot
}

func TestDesktopRecoveryNeverInterruptsALiveRunOwnedByAnotherProcess(t *testing.T) {
	dir := t.TempDir()
	cli, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	dot := startRunningDot(t, cli, "cli-owned")

	// A second handle (Desktop starting up) must leave the live run alone.
	desktop, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := desktop.RecoverInterruptedRuns()
	if err != nil || recovered != 0 {
		t.Fatalf("live run recovered=%d err=%v", recovered, err)
	}
	if got, _ := desktop.GetDot(dot.ID); got.Status != task.Running {
		t.Fatalf("live CLI run was disturbed: %s", got.Status)
	}
	// The second process cannot start the same Dot either.
	if err := desktop.SetStatus(dot.ID, task.Running, "dup"); !errors.Is(err, ErrRunOwned) {
		t.Fatalf("duplicate start: %v", err)
	}
	if pid := desktop.RunOwnerPID(dot.ID); pid != os.Getpid() {
		t.Fatalf("owner pid=%d", pid)
	}
	// Finishing releases ownership.
	if err := cli.SetStatus(dot.ID, task.Done, "done"); err != nil {
		t.Fatal(err)
	}
	if pid := desktop.RunOwnerPID(dot.ID); pid != 0 {
		t.Fatalf("lease not released: %d", pid)
	}
}

func TestOrphanedRunIsDetectedAndRecoveredAfterOwnerCrash(t *testing.T) {
	dir := t.TempDir()
	cli, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	dot := startRunningDot(t, cli, "crashing")
	simulateCrash(cli, dot.ID)

	desktop, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := desktop.RecoverOrphanedRun(dot.ID)
	if err != nil || !recovered {
		t.Fatalf("recovered=%v err=%v", recovered, err)
	}
	got, _ := desktop.GetDot(dot.ID)
	if got.Status != task.Paused || got.Checkpoint == "" {
		t.Fatalf("not recoverable: %#v", got)
	}
	// Recovery is idempotent and the Dot can be started again.
	if n, _ := desktop.RecoverInterruptedRuns(); n != 0 {
		t.Fatalf("second recovery changed %d dots", n)
	}
	if err := desktop.SetStatus(dot.ID, task.Running, "retry"); err != nil {
		t.Fatalf("retry after recovery: %v", err)
	}
}
