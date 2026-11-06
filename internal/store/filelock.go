package store

import (
	"fmt"
	"os"
	"syscall"
)

// lockStoreFile takes an exclusive advisory lock beside the store so Desktop
// and CLI processes serialize their read-merge-write cycles. The lock is
// released by the returned function or when the process exits.
func lockStoreFile(path string) (func(), error) {
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open store lock: %w", err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, fmt.Errorf("lock task store: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
