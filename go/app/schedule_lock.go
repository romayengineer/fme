package app

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// AcquireScheduleLock takes a non-blocking exclusive lock so only one
// scheduler drives the shared Chrome profile at a time. The returned function
// releases the lock and is safe to call once.
func AcquireScheduleLock(path string) (func() error, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, fmt.Errorf("create scheduler lock directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // the lock path is user-configured.
	if err != nil {
		return nil, fmt.Errorf("open scheduler lock %s: %w", path, err)
	}

	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("acquire scheduler lock %s: another fme scheduler may be running: %w", path, err)
	}

	return func() error {
		return releaseLock(file)
	}, nil
}

// releaseLock unlocks and closes the lock file, reporting the first error.
func releaseLock(file *os.File) error {
	unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
	closeErr := file.Close()
	if unlockErr != nil {
		return fmt.Errorf("unlock scheduler lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close scheduler lock: %w", closeErr)
	}
	return nil
}
