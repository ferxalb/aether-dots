package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"aether-dots/internal/task"
)

var (
	ErrRoutineNotDue      = errors.New("routine is not due")
	ErrDotAlreadyActive   = errors.New("a Dot delegation is already queued or running")
	ErrRoutineRateLimited = errors.New("Automation run rate limit exceeded")
)

func (s *Store) CreateRoutine(in task.CreateRoutine) (task.Routine, error) {
	in.ConversationID = strings.TrimSpace(in.ConversationID)
	in.Name = strings.TrimSpace(in.Name)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if err := validateRoutineInput(in); err != nil {
		return task.Routine{}, err
	}
	id, err := newID()
	if err != nil {
		return task.Routine{}, err
	}
	now := time.Now().UTC()
	routine := task.Routine{
		ID: id, ConversationID: in.ConversationID, Name: in.Name, Prompt: in.Prompt,
		GitHubEnabled:   in.GitHubEnabled,
		IntervalSeconds: in.IntervalSeconds, Active: true,
		NextRunAt: now.Add(time.Duration(in.IntervalSeconds) * time.Second),
		Runs:      []task.RoutineRun{}, CreatedAt: now, UpdatedAt: now,
	}
	err = s.change(func(db *task.Database) error {
		if conversationIndex(db, in.ConversationID) < 0 {
			return errors.New("routine Dot not found")
		}
		if in.GitHubEnabled && !conversationHasValidGitHubGrant(db, in.ConversationID) {
			return errors.New("enable GitHub for a Routine only after its Dot has an authorized repository grant")
		}
		count := 0
		for _, existing := range db.Routines {
			if existing.ConversationID == in.ConversationID {
				count++
				if strings.EqualFold(existing.Name, in.Name) {
					return fmt.Errorf("routine %q already exists for this Dot", in.Name)
				}
			}
		}
		if count >= 50 {
			return errors.New("a Dot can have at most 50 routines")
		}
		db.Routines = append(db.Routines, routine)
		return nil
	})
	return routine, err
}

func (s *Store) UpdateRoutine(id string, in task.CreateRoutine) error {
	in.ConversationID = strings.TrimSpace(in.ConversationID)
	in.Name = strings.TrimSpace(in.Name)
	in.Prompt = strings.TrimSpace(in.Prompt)
	if err := validateRoutineInput(in); err != nil {
		return err
	}
	return s.change(func(db *task.Database) error {
		ri := routineIndex(db, id)
		if ri < 0 {
			return errors.New("routine not found")
		}
		for _, dot := range db.Dots {
			if dot.RoutineID == id && (dot.Status == task.Captured || dot.Status == task.Ready || dot.Status == task.Running) {
				return errors.New("wait until the active routine run finishes before editing it")
			}
		}
		if conversationIndex(db, in.ConversationID) < 0 {
			return errors.New("routine Dot not found")
		}
		if in.GitHubEnabled && !conversationHasValidGitHubGrant(db, in.ConversationID) {
			return errors.New("enable GitHub for a Routine only after its Dot has an authorized repository grant")
		}
		count := 0
		for _, other := range db.Routines {
			if other.ID == id || other.ConversationID != in.ConversationID {
				continue
			}
			count++
			if strings.EqualFold(other.Name, in.Name) {
				return fmt.Errorf("routine %q already exists for this Dot", in.Name)
			}
		}
		if count >= 50 {
			return errors.New("a Dot can have at most 50 routines")
		}
		routine := &db.Routines[ri]
		intervalChanged := routine.IntervalSeconds != in.IntervalSeconds
		routine.ConversationID, routine.Name = in.ConversationID, in.Name
		routine.Prompt, routine.IntervalSeconds = in.Prompt, in.IntervalSeconds
		routine.GitHubEnabled = in.GitHubEnabled
		if intervalChanged && routine.Active {
			routine.NextRunAt = time.Now().UTC().Add(time.Duration(in.IntervalSeconds) * time.Second)
		}
		routine.UpdatedAt = time.Now().UTC()
		return nil
	})
}

// SetRoutineGitHubEnabled is a separate, explicit consent action for local
// scheduled runs. A Dot's repository grants alone never authorize a Routine.
func (s *Store) SetRoutineGitHubEnabled(id string, enabled bool) error {
	return s.change(func(db *task.Database) error {
		i := routineIndex(db, id)
		if i < 0 {
			return errors.New("routine not found")
		}
		if db.Routines[i].GitHubEnabled == enabled {
			return nil
		}
		if enabled && !conversationHasValidGitHubGrant(db, db.Routines[i].ConversationID) {
			return errors.New("grant this Dot a GitHub repository before enabling Routine access")
		}
		db.Routines[i].GitHubEnabled = enabled
		db.Routines[i].UpdatedAt = time.Now().UTC()
		return nil
	})
}

func validateRoutineInput(in task.CreateRoutine) error {
	if in.Name == "" || len(in.Name) > task.MaxRoutineNameBytes || !utf8.ValidString(in.Name) {
		return fmt.Errorf("routine name must be valid UTF-8 text between 1 and %d bytes", task.MaxRoutineNameBytes)
	}
	if in.Prompt == "" || len(in.Prompt) > task.MaxRoutinePromptBytes || !utf8.ValidString(in.Prompt) {
		return fmt.Errorf("routine instruction must be valid UTF-8 text between 1 and %d bytes", task.MaxRoutinePromptBytes)
	}
	if in.IntervalSeconds < task.MinRoutineIntervalSeconds {
		return fmt.Errorf("routine interval must be at least %s", time.Duration(task.MinRoutineIntervalSeconds)*time.Second)
	}
	if in.IntervalSeconds > int64((365*24*time.Hour)/time.Second) {
		return errors.New("routine interval must be one year or less")
	}
	return nil
}

func (s *Store) UpdateRoutineActive(id string, active bool) error {
	return s.change(func(db *task.Database) error {
		i := routineIndex(db, id)
		if i < 0 {
			return errors.New("routine not found")
		}
		routine := &db.Routines[i]
		if routine.Active == active {
			return nil
		}
		routine.Active = active
		if active {
			routine.NextRunAt = time.Now().UTC().Add(time.Duration(routine.IntervalSeconds) * time.Second)
		}
		routine.UpdatedAt = time.Now().UTC()
		return nil
	})
}

func (s *Store) DeleteRoutine(id string) error {
	return s.change(func(db *task.Database) error {
		i := routineIndex(db, id)
		if i < 0 {
			return errors.New("routine not found")
		}
		for _, dot := range db.Dots {
			if dot.RoutineID == id && (dot.Status == task.Captured || dot.Status == task.Ready || dot.Status == task.Running) {
				return errors.New("pause until the active routine run finishes before deleting it")
			}
		}
		db.Routines = append(db.Routines[:i], db.Routines[i+1:]...)
		return nil
	})
}

// AcquireRoutineLease prevents two desktop/CLI schedulers from executing the
// same local routine store at once. It is advisory so ordinary store reads and
// writes retain their existing lifecycle.
func (s *Store) AcquireRoutineLease() error {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	if s.lease != nil {
		return errors.New("this process already owns the routine scheduler lease")
	}
	path := filepath.Join(filepath.Dir(s.path), ".routine-scheduler.lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("open routine scheduler lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return errors.New("another AETHER process is already running this routine scheduler")
	}
	s.lease = file
	return nil
}

func (s *Store) ReleaseRoutineLease() error {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	if s.lease == nil {
		return nil
	}
	err := syscall.Flock(int(s.lease.Fd()), syscall.LOCK_UN)
	closeErr := s.lease.Close()
	s.lease = nil
	if err != nil {
		return err
	}
	return closeErr
}

// StartDueRoutine atomically claims one due run and advances its next time.
func (s *Store) StartDueRoutine(id string, now time.Time) (task.Routine, task.Dot, error) {
	return s.startRoutine(id, now, true)
}

// StartRoutineNow starts a safe manual test without changing the schedule.
func (s *Store) StartRoutineNow(id string, now time.Time) (task.Routine, task.Dot, error) {
	return s.startRoutine(id, now, false)
}

func (s *Store) startRoutine(id string, now time.Time, scheduled bool) (task.Routine, task.Dot, error) {
	now = now.UTC()
	var routine task.Routine
	var dot task.Dot
	var startErr error
	err := s.change(func(db *task.Database) error {
		ri := routineIndex(db, id)
		if ri < 0 {
			return errors.New("routine not found")
		}
		routine = db.Routines[ri]
		if scheduled {
			if !routine.Active || routine.NextRunAt.After(now) {
				return ErrRoutineNotDue
			}
			if interval := time.Duration(routine.IntervalSeconds) * time.Second; routine.NextRunAt.Before(now.Add(-interval)) {
				// Skip missed ticks after sleep or downtime; never burst stale runs.
				routine.NextRunAt = now.Add(interval)
			} else {
				routine.NextRunAt = routine.NextRunAt.Add(interval)
			}
		}
		db.RoutineRunStarts = recentStarts(db.RoutineRunStarts, now)
		routine.RunStarts = recentStarts(routine.RunStarts, now)
		if len(db.RoutineRunStarts) >= task.MaxRoutineRunsTotalPerHour || len(routine.RunStarts) >= task.MaxRoutineRunsPerHour {
			routine.LastRunAt, routine.LastStatus, routine.UpdatedAt = now, "failed", now
			if scheduled {
				routine.NextRunAt = now.Add(time.Duration(routine.IntervalSeconds) * time.Second)
			}
			routine.Runs = append(routine.Runs, task.RoutineRun{
				Status: "failed", Message: ErrRoutineRateLimited.Error(), StartedAt: now, FinishedAt: now,
			})
			if len(routine.Runs) > task.MaxRoutineRuns {
				routine.Runs = append([]task.RoutineRun(nil), routine.Runs[len(routine.Runs)-task.MaxRoutineRuns:]...)
			}
			db.Routines[ri] = routine
			startErr = ErrRoutineRateLimited
			return nil
		}
		content := "Routine: " + routine.Name + "\n\nRun the following instruction using the current Dot profile and recent chat context. Report missing or stale inputs instead of guessing.\n\n" + routine.Prompt
		created, _, err := createConversationRun(db, routine.ConversationID, content, routine.ID, now)
		if err != nil {
			return err
		}
		dot = created
		db.RoutineRunStarts = append(db.RoutineRunStarts, now)
		routine.RunStarts = append(routine.RunStarts, now)
		routine.LastRunAt = now
		routine.LastStatus = "running"
		routine.UpdatedAt = now
		routine.Runs = append(routine.Runs, task.RoutineRun{DotID: dot.ID, Status: "running", StartedAt: now})
		if len(routine.Runs) > task.MaxRoutineRuns {
			routine.Runs = append([]task.RoutineRun(nil), routine.Runs[len(routine.Runs)-task.MaxRoutineRuns:]...)
		}
		db.Routines[ri] = routine
		return nil
	})
	if err != nil {
		return routine, dot, err
	}
	return routine, dot, startErr
}

func recentStarts(starts []time.Time, now time.Time) []time.Time {
	cutoff := now.Add(-time.Hour)
	recent := starts[:0]
	for _, start := range starts {
		if !start.Before(cutoff) && !start.After(now) {
			recent = append(recent, start)
		}
	}
	return recent
}

func (s *Store) FinishRoutineRun(id, dotID, status, message string, now time.Time) error {
	status = strings.TrimSpace(status)
	if status != "succeeded" && status != "failed" && status != "paused" {
		return errors.New("invalid routine run status")
	}
	message = strings.ToValidUTF8(message, "�")
	if len(message) > 2048 {
		end := 2048
		for end > 0 && !utf8.RuneStart(message[end]) {
			end--
		}
		message = message[:end]
	}
	return s.change(func(db *task.Database) error {
		ri := routineIndex(db, id)
		if ri < 0 {
			return errors.New("routine not found")
		}
		routine := &db.Routines[ri]
		for i := len(routine.Runs) - 1; i >= 0; i-- {
			run := &routine.Runs[i]
			if run.DotID != dotID {
				continue
			}
			if run.Status != "running" {
				if run.Status == status && run.Message == message {
					return nil
				}
				return errors.New("routine run is already finished")
			}
			run.Status, run.Message, run.FinishedAt = status, message, now.UTC()
			routine.LastStatus = status
			routine.UpdatedAt = now.UTC()
			return nil
		}
		return errors.New("routine run not found")
	})
}

func routineIndex(db *task.Database, id string) int {
	for i := range db.Routines {
		if db.Routines[i].ID == id {
			return i
		}
	}
	return -1
}
