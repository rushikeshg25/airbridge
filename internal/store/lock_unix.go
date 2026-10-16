//go:build linux || darwin

package store

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Advisory locks are released by the OS even after a crash. Never unlink the
// lock file: doing so would allow different processes to lock different inodes.
func lockDirectory(dir string) (*os.File, error) {
	lock, err := os.OpenFile(filepath.Join(dir, ".airbridge.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lock.Close()
		return nil, fmt.Errorf("data directory is already in use: %w", err)
	}
	return lock, nil
}
