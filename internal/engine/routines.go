package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

type GeneratorFactory func() Generator

// RunRoutineOnce performs a manual test run. It uses the same persisted Dot
// conversation and lifecycle as an ordinary chat message.
func RunRoutineOnce(ctx context.Context, db *store.Store, routineID string, generator Generator, onProgress func()) error {
	_, dot, err := db.StartRoutineNow(routineID, time.Now().UTC())
	if err != nil {
		return err
	}
	return runRoutineDot(ctx, db, dot, generator, onProgress)
}

// RunDueRoutines starts each due routine serially. The store atomically claims
// a routine before the model call, so repeated scheduler ticks cannot enqueue
// the same run twice.
func RunDueRoutines(ctx context.Context, db *store.Store, generator Generator, onProgress func()) (int, error) {
	now := time.Now().UTC()
	due := make([]string, 0)
	for _, routine := range db.Snapshot().Routines {
		if routine.Active && !routine.NextRunAt.After(now) {
			due = append(due, routine.ID)
		}
	}
	started := 0
	for _, id := range due {
		if err := ctx.Err(); err != nil {
			return started, err
		}
		_, dot, err := db.StartDueRoutine(id, time.Now().UTC())
		if errors.Is(err, store.ErrRoutineNotDue) || errors.Is(err, store.ErrDotAlreadyActive) {
			continue
		}
		if errors.Is(err, store.ErrRoutineRateLimited) {
			if onProgress != nil {
				onProgress()
			}
			continue
		}
		if err != nil {
			return started, err
		}
		started++
		if onProgress != nil {
			onProgress()
		}
		if err := runRoutineDot(ctx, db, dot, generator, onProgress); err != nil && errors.Is(err, context.Canceled) {
			return started, err
		}
	}
	return started, nil
}

// RunRoutineLoop keeps schedules active while the desktop app or CLI daemon is
// running. Missed intervals are skipped rather than replayed in a burst.
func RunRoutineLoop(ctx context.Context, db *store.Store, generator GeneratorFactory, pollInterval time.Duration, onProgress func()) error {
	if err := db.AcquireRoutineLease(); err != nil {
		return err
	}
	defer func() { _ = db.ReleaseRoutineLease() }()
	if pollInterval <= 0 {
		pollInterval = 30 * time.Second
	}
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var current Generator
		if generator != nil && hasDueRoutine(db, time.Now().UTC()) {
			current = generator()
		}
		if _, err := RunDueRoutines(ctx, db, current, onProgress); err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func hasDueRoutine(db *store.Store, now time.Time) bool {
	for _, routine := range db.Snapshot().Routines {
		if routine.Active && !routine.NextRunAt.After(now) {
			return true
		}
	}
	return false
}

func runRoutineDot(ctx context.Context, db *store.Store, dot task.Dot, generator Generator, onProgress func()) error {
	err := RunWithProgress(ctx, db, dot.ID, generator, nil, onProgress)
	status, message := "succeeded", "Routine completed."
	if err != nil {
		status, message = "failed", strings.TrimSpace(err.Error())
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			status = "paused"
		}
	}
	if finishErr := db.FinishRoutineRun(dot.RoutineID, dot.ID, status, message, time.Now().UTC()); finishErr != nil {
		if err != nil {
			return errors.Join(err, finishErr)
		}
		return finishErr
	}
	if onProgress != nil {
		onProgress()
	}
	return err
}
