package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"aether-dots/internal/task"
)

// Run ownership. A Dot is Running only while some live process holds an
// exclusive flock on runs/<dot-id>.lease. The kernel drops the lock when the
// owner exits for any reason (including kill -9), so "Running with no live
// lease" reliably means the owner crashed. Desktop startup recovery and CLI
// retries use this to recover only orphaned runs and never touch a run that
// another Desktop or CLI process is actively executing.

var runLeaseID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// ErrRunOwned reports that another live process is executing the Dot.
var ErrRunOwned = errors.New("this Dot is already running in another AETHER process")

func (s *Store) runLeasePath(dotID string) (string, error) {
	if !runLeaseID.MatchString(dotID) {
		return "", errors.New("invalid Dot id")
	}
	return filepath.Join(s.Dir(), "runs", dotID+".lease"), nil
}

// tryRunLease opens the Dot's lease file and takes a non-blocking exclusive lock.
func (s *Store) tryRunLease(dotID string) (*os.File, error) {
	path, err := s.runLeasePath(dotID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create run lease directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open run lease: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, ErrRunOwned
		}
		return nil, fmt.Errorf("lock run lease: %w", err)
	}
	return file, nil
}

// acquireRunLease claims a Dot for this process. It is idempotent per process.
func (s *Store) acquireRunLease(dotID string) error {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	if _, held := s.runLeases[dotID]; held {
		return nil
	}
	file, err := s.tryRunLease(dotID)
	if err != nil {
		return err
	}
	_ = file.Truncate(0)
	_, _ = file.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	if s.runLeases == nil {
		s.runLeases = map[string]*os.File{}
	}
	s.runLeases[dotID] = file
	return nil
}

func (s *Store) releaseRunLease(dotID string) {
	s.leaseMu.Lock()
	defer s.leaseMu.Unlock()
	if file, ok := s.runLeases[dotID]; ok {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		delete(s.runLeases, dotID)
	}
}

// RunOwnerPID returns the pid recorded by a live owner, or 0 if the Dot has no
// live owner.
func (s *Store) RunOwnerPID(dotID string) int {
	file, err := s.tryRunLease(dotID)
	if err == nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return 0
	}
	path, _ := s.runLeasePath(dotID)
	raw, readErr := os.ReadFile(path)
	if readErr != nil {
		return -1
	}
	pid, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
	if pid <= 0 {
		return -1
	}
	return pid
}

// RecoverOrphanedRuns pauses every Running Dot whose owner process is gone and
// leaves Dots owned by live processes untouched. It returns the number
// recovered and the number still owned by live processes.
func (s *Store) RecoverOrphanedRuns() (recovered, live int, err error) {
	err = s.change(func(next *task.Database) error {
		for i := range next.Dots {
			dot := &next.Dots[i]
			if dot.ConversationID == "" || dot.Status != task.Running {
				continue
			}
			lease, leaseErr := s.tryRunLease(dot.ID)
			if errors.Is(leaseErr, ErrRunOwned) {
				live++
				continue
			}
			if leaseErr != nil {
				return leaseErr
			}
			dot.Status = task.Paused
			dot.UpdatedAt = time.Now().UTC()
			if dot.Checkpoint == "" || dot.Checkpoint == "generating response" {
				dot.Checkpoint = "previous run ended before the response completed; retry this message"
			}
			appendEvent(dot, "interrupted", "The process running this Dot ended before the response completed.")
			recovered++
			_ = syscall.Flock(int(lease.Fd()), syscall.LOCK_UN)
			_ = lease.Close()
		}
		return nil
	})
	return recovered, live, err
}

// RecoverOrphanedRun recovers one Dot if (and only if) its owner is gone.
func (s *Store) RecoverOrphanedRun(dotID string) (bool, error) {
	owner := s.RunOwnerPID(dotID)
	if owner != 0 {
		return false, ErrRunOwned
	}
	recovered, _, err := s.RecoverOrphanedRuns()
	return recovered > 0, err
}
