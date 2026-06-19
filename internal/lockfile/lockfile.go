package lockfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/imzyb/MiGate/internal/paths"
)

// TryAcquire creates a non-blocking file-descriptor lock. The lock file may
// remain on disk, but the lock itself is released automatically when the
// process exits or the descriptor is closed.
func TryAcquire(path string) (func(), error) {
	if path == "" {
		return func() {}, nil
	}
	lockDir := filepath.Dir(path)
	if err := os.MkdirAll(lockDir, 0o750); err != nil {
		return nil, fmt.Errorf("prepare lock dir: %w", err)
	}
	if filepath.Clean(lockDir) == filepath.Clean(paths.RunDir) {
		if err := os.Chmod(lockDir, 0o750); err != nil {
			return nil, fmt.Errorf("chmod lock dir %s: %w", lockDir, err)
		}
	}
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o640)
	if err != nil {
		return nil, fmt.Errorf("open lock %s: %w", path, err)
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, fmt.Errorf("lock busy: %s", path)
		}
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if err := file.Truncate(0); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("truncate lock %s: %w", path, err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
		return nil, fmt.Errorf("seek lock %s: %w", path, err)
	}
	_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
