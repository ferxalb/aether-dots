package store

import (
	"errors"
	"strings"

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
