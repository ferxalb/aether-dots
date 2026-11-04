package engine

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"aether-dots/internal/store"
	"aether-dots/internal/task"
)

type Generator interface {
	Generate(context.Context, string, func(string)) (string, error)
	Readiness() (bool, string)
	Description() string
}

func Run(ctx context.Context, db *store.Store, dotID string, generator Generator, onDelta func(string)) error {
	return RunWithProgress(ctx, db, dotID, generator, onDelta, nil)
}

// RunWithProgress executes one chat turn and persists the assistant response
// into the Dot conversation. The runner has no filesystem input or output.
func RunWithProgress(ctx context.Context, db *store.Store, dotID string, generator Generator, onDelta func(string), onProgress func()) error {
	if closer, ok := generator.(interface{ Close() error }); ok {
		defer func() { _ = closer.Close() }()
	}
	progress := func() {
		if onProgress != nil {
			onProgress()
		}
	}
	dot, ok := db.GetDot(dotID)
	if !ok {
		return errors.New("Dot not found")
	}
	if dot.ConversationID == "" {
		return fail(db, dotID, errors.New("this legacy task has no Dot conversation and cannot be run"))
	}
	if dot.Status == task.Running {
		return errors.New("Dot already has an active run")
	}
	if dot.Status == task.Done {
		return errors.New("Dot message is already complete")
	}
	if generator == nil {
		return waitForInput(db, dotID, "Connect a model in Settings before sending messages.")
	}
	if ready, reason := generator.Readiness(); !ready {
		if strings.TrimSpace(reason) == "" {
			reason = "model is not configured"
		}
		return waitForInput(db, dotID, reason)
	}
	objective, err := db.ConversationObjective(dotID)
	if err != nil {
		return fail(db, dotID, err)
	}
	if err := db.SetStatus(dotID, task.Running, "Preparing a reply from this Dot's profile and chat history."); err != nil {
		return err
	}
	progress()
	if err := db.SetCheckpoint(dotID, "generating response"); err != nil {
		return fail(db, dotID, err)
	}
	progress()
	startedAt := time.Now().UTC()
	var timingMu sync.Mutex
	var firstTokenAt time.Time
	var outputBytes int
	trackedDelta := func(delta string) {
		if delta != "" {
			timingMu.Lock()
			if firstTokenAt.IsZero() {
				firstTokenAt = time.Now().UTC()
			}
			outputBytes += len(delta)
			timingMu.Unlock()
		}
		if onDelta != nil {
			onDelta(delta)
		}
	}
	response, err := generator.Generate(ctx, objective, trackedDelta)
	finishedAt := time.Now().UTC()
	timingMu.Lock()
	metrics := task.RunMetrics{
		Provider:              generator.Description(),
		StartedAt:             startedAt,
		FinishedAt:            finishedAt,
		DurationMillis:        finishedAt.Sub(startedAt).Milliseconds(),
		EstimatedInputTokens:  estimatedTokens(len(objective)),
		EstimatedOutputTokens: estimatedTokens(outputBytes),
	}
	if !firstTokenAt.IsZero() {
		metrics.FirstTokenLatencyMillis = firstTokenAt.Sub(startedAt).Milliseconds()
	}
	timingMu.Unlock()
	_ = db.SetRunMetrics(dotID, metrics)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return pause(db, dotID)
		}
		return fail(db, dotID, err)
	}
	if err := ctx.Err(); err != nil {
		return pause(db, dotID)
	}
	response = strings.TrimSpace(response)
	if response == "" {
		return fail(db, dotID, errors.New("model returned an empty response"))
	}
	if err := db.AppendConversationResult(dot.ConversationID, dotID, response); err != nil {
		return fail(db, dotID, err)
	}
	if err := db.SetCheckpoint(dotID, ""); err != nil {
		return fail(db, dotID, err)
	}
	progress()
	return db.SetStatus(dotID, task.Done, "Reply saved in the Dot conversation.")
}

func estimatedTokens(bytes int) int64 {
	if bytes <= 0 {
		return 0
	}
	return int64((bytes + 3) / 4)
}

func waitForInput(db *store.Store, id, message string) error {
	_ = db.SetStatus(id, task.WaitingInput, message)
	return errors.New(message)
}

func pause(db *store.Store, id string) error {
	_ = db.SetCheckpoint(id, "safe to retry this message")
	_ = db.SetStatus(id, task.Paused, "Reply paused. Retry the message to ask the Dot again.")
	return context.Canceled
}

func fail(db *store.Store, id string, cause error) error {
	_ = db.AppendEvent(id, "error", cause.Error())
	_ = db.SetStatus(id, task.Failed, "Reply failed: "+cause.Error())
	return cause
}
