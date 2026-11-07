package store

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
)

// SetRunMetrics records local generation timing and rough token estimates.
func (s *Store) SetRunMetrics(dotID string, metrics task.RunMetrics) error {
	metrics.Provider = strings.TrimSpace(metrics.Provider)
	if metrics.Provider == "" || metrics.DurationMillis < 0 ||
		metrics.EstimatedInputTokens < 0 || metrics.EstimatedOutputTokens < 0 ||
		metrics.FirstTokenLatencyMillis < 0 || metrics.FinishedAt.Before(metrics.StartedAt) {
		return errors.New("run metrics are invalid")
	}
	return s.change(func(db *task.Database) error {
		i := dotIndex(db, dotID)
		if i < 0 {
			return errors.New("Dot not found")
		}
		copy := metrics
		db.Dots[i].Metrics = &copy
		return nil
	})
}

func validRunMetrics(m task.RunMetrics) bool {
	return strings.TrimSpace(m.Provider) != "" && m.DurationMillis >= 0 &&
		m.EstimatedInputTokens >= 0 && m.EstimatedOutputTokens >= 0 &&
		m.FirstTokenLatencyMillis >= 0 && !m.FinishedAt.Before(m.StartedAt)
}

// StartRun claims the run lease and records Running plus its checkpoint in a
// single write.
func (s *Store) StartRun(id, message, checkpoint string) error {
	if err := s.acquireRunLease(id); err != nil {
		return err
	}
	err := s.change(func(db *task.Database) error {
		i := dotIndex(db, id)
		if i < 0 {
			return errors.New("Dot not found")
		}
		db.Dots[i].Status = task.Running
		db.Dots[i].Checkpoint = checkpoint
		db.Dots[i].UpdatedAt = time.Now().UTC()
		appendEvent(&db.Dots[i], "status", message)
		return nil
	})
	if err != nil {
		s.releaseRunLease(id)
	}
	return err
}

// CompleteRun saves metrics, the assistant reply, the cleared checkpoint and
// the Done status in one atomic write.
func (s *Store) CompleteRun(conversationID, dotID, report string, metrics task.RunMetrics) error {
	const maxResponseBytes = 2 * 1024 * 1024
	if len(report) == 0 || len(report) > maxResponseBytes || !utf8.ValidString(report) {
		return fmt.Errorf("conversation response must be valid UTF-8 between 1 and %d bytes", maxResponseBytes)
	}
	messageID, err := newID()
	if err != nil {
		return err
	}
	err = s.change(func(db *task.Database) error {
		if err := appendResultLocked(db, messageID, conversationID, dotID, report); err != nil {
			return err
		}
		i := dotIndex(db, dotID)
		if validRunMetrics(metrics) {
			m := metrics
			db.Dots[i].Metrics = &m
		}
		db.Dots[i].Checkpoint = ""
		db.Dots[i].Status = task.Done
		db.Dots[i].UpdatedAt = time.Now().UTC()
		appendEvent(&db.Dots[i], "status", "Reply saved in the Dot conversation.")
		return nil
	})
	s.releaseRunLease(dotID)
	return err
}
