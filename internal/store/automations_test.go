package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aether-dots/internal/task"
)

func TestSkillSelectionIsExplicitAndScopedToOneTurn(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	skill, err := db.CreateSkill(task.CreateSkill{
		Name: "Weekly review", Description: "Review a work stream.",
		WhenToUse: "When a user asks for the weekly review.", Inputs: "Use facts supplied in the chat.",
		Steps: "1. Group the facts. 2. Identify risks.", Validation: "Check each risk against the supplied facts.",
		Output: "Return summary, risks, and next steps.", Approval: "Do not contact anyone; ask before any external action.",
	})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Review Dot"})
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := db.CreateConversationRun(conversation.ID, "Please use @skill[Weekly review] for this summary.")
	if err != nil {
		t.Fatal(err)
	}
	if first.SkillID != skill.ID {
		t.Fatalf("selected skill id=%q, want %q", first.SkillID, skill.ID)
	}
	for _, part := range []string{skill.Description, skill.WhenToUse, skill.Inputs, skill.Steps, skill.Validation, skill.Output, skill.Approval} {
		if !strings.Contains(first.Objective, part) {
			t.Fatalf("selected skill context omitted %q", part)
		}
	}
	if err := db.SetStatus(first.ID, task.Failed, "test complete"); err != nil {
		t.Fatal(err)
	}
	second, _, err := db.CreateConversationRun(conversation.ID, "Help me summarize this.")
	if err != nil {
		t.Fatal(err)
	}
	if second.SkillID != "" || strings.Contains(second.Objective, "Skill: Weekly review") {
		t.Fatalf("skill leaked into an unselected turn: %#v", second)
	}
}

func TestRoutineLifecycleAndStoreLease(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "store")
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Routine Dot"})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Daily note", Prompt: "Summarize the current context.",
		IntervalSeconds: task.MinRoutineIntervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.StartDueRoutine(routine.ID, time.Now().UTC()); !errors.Is(err, ErrRoutineNotDue) {
		t.Fatalf("new routine ran before its first schedule: %v", err)
	}

	secondStore, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AcquireRoutineLease(); err != nil {
		t.Fatal(err)
	}
	if err := secondStore.AcquireRoutineLease(); err == nil {
		t.Fatal("a second scheduler acquired the same store lease")
	}
	if err := db.ReleaseRoutineLease(); err != nil {
		t.Fatal(err)
	}
	if err := secondStore.AcquireRoutineLease(); err != nil {
		t.Fatalf("scheduler lease was not released: %v", err)
	}
	if err := secondStore.ReleaseRoutineLease(); err != nil {
		t.Fatal(err)
	}

	dueAt := routine.NextRunAt.Add(time.Second)
	started, dot, err := db.StartDueRoutine(routine.ID, dueAt)
	if err != nil {
		t.Fatal(err)
	}
	if dot.RoutineID != routine.ID || started.LastStatus != "running" || !started.NextRunAt.After(dueAt) {
		t.Fatalf("routine did not record and advance its run: routine=%#v dot=%#v", started, dot)
	}
	if _, _, err := db.StartRoutineNow(routine.ID, dueAt.Add(time.Second)); !errors.Is(err, ErrDotAlreadyActive) {
		t.Fatalf("overlapping routine test was not rejected: %v", err)
	}
	if err := db.SetStatus(dot.ID, task.Done, "test completion"); err != nil {
		t.Fatal(err)
	}
	if err := db.FinishRoutineRun(routine.ID, dot.ID, "succeeded", "Routine completed.", dueAt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	updated := db.Snapshot().Routines[0]
	if updated.LastStatus != "succeeded" || len(updated.Runs) != 1 || updated.Runs[0].FinishedAt.IsZero() {
		t.Fatalf("routine completion was not persisted: %#v", updated)
	}
	if err := db.UpdateRoutineActive(routine.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateRoutineActive(routine.ID, true); err != nil {
		t.Fatal(err)
	}
	if !db.Snapshot().Routines[0].Active {
		t.Fatal("routine did not resume")
	}
}

func TestRoutineSkipsMissedIntervalsAndValidatesBounds(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Scheduled Dot"})
	if err != nil {
		t.Fatal(err)
	}
	routine, err := db.CreateRoutine(task.CreateRoutine{
		ConversationID: conversation.ID, Name: "Heartbeat", Prompt: "Summarize state.",
		IntervalSeconds: task.MinRoutineIntervalSeconds,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRoutine(task.CreateRoutine{ConversationID: conversation.ID, Name: "Too soon", Prompt: "Run.", IntervalSeconds: 60}); err == nil {
		t.Fatal("sub-five-minute routine was accepted")
	}
	if _, _, err := db.StartDueRoutine(routine.ID, routine.NextRunAt.Add(4*time.Duration(routine.IntervalSeconds)*time.Second)); err != nil {
		t.Fatal(err)
	}
	got := db.Snapshot().Routines[0]
	want := routine.NextRunAt.Add(5 * time.Duration(routine.IntervalSeconds) * time.Second)
	if !got.NextRunAt.Equal(want) {
		t.Fatalf("missed intervals were not skipped: next=%s want=%s", got.NextRunAt, want)
	}
}

func TestRoutineRateLimitsRecordFailureAndSkipTheNextInterval(t *testing.T) {
	for _, limit := range []struct {
		name       string
		perRoutine bool
		count      int
	}{
		{name: "overall", count: task.MaxRoutineRunsTotalPerHour},
		{name: "per routine", perRoutine: true, count: task.MaxRoutineRunsPerHour},
	} {
		t.Run(limit.name, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "store"))
			if err != nil {
				t.Fatal(err)
			}
			conversation, err := db.CreateConversation(task.CreateConversation{SpaceID: db.Snapshot().Spaces[0].ID, Name: "Rate limited Dot"})
			if err != nil {
				t.Fatal(err)
			}
			routine, err := db.CreateRoutine(task.CreateRoutine{
				ConversationID: conversation.ID, Name: "Metered", Prompt: "Summarize context.",
				IntervalSeconds: task.MinRoutineIntervalSeconds,
			})
			if err != nil {
				t.Fatal(err)
			}
			now := routine.NextRunAt.Add(time.Second)
			starts := make([]time.Time, limit.count)
			for i := range starts {
				starts[i] = now.Add(-time.Minute)
			}
			if err := db.change(func(snapshot *task.Database) error {
				if limit.perRoutine {
					snapshot.Routines[0].RunStarts = starts
				} else {
					snapshot.RoutineRunStarts = starts
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := db.StartDueRoutine(routine.ID, now); !errors.Is(err, ErrRoutineRateLimited) {
				t.Fatalf("rate limit error=%v", err)
			}
			updated := db.Snapshot().Routines[0]
			if updated.LastStatus != "failed" || !updated.NextRunAt.After(now) || len(updated.Runs) != 1 || updated.Runs[0].Message != ErrRoutineRateLimited.Error() {
				t.Fatalf("rate-limited attempt was not recorded or rescheduled: %#v", updated)
			}
		})
	}
}
