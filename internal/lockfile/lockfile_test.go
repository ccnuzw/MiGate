package lockfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTryAcquireFailsWhenAlreadyHeld(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "apply.lock")
	unlock, err := TryAcquire(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer unlock()

	secondUnlock, err := TryAcquire(lockPath)
	if err == nil {
		secondUnlock()
		t.Fatalf("expected second acquire to fail while lock is held")
	}
	if !strings.Contains(err.Error(), "lock busy") || !strings.Contains(err.Error(), lockPath) {
		t.Fatalf("expected lock busy error with path, got %v", err)
	}
}

func TestTryAcquireSucceedsAfterUnlock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "apply.lock")
	unlock, err := TryAcquire(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	unlock()

	unlockAgain, err := TryAcquire(lockPath)
	if err != nil {
		t.Fatalf("second acquire after unlock: %v", err)
	}
	unlockAgain()
}

func TestTryAcquireDoesNotDependOnRemovingLockFile(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "apply.lock")
	unlock, err := TryAcquire(lockPath)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	unlock()
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("lock file should remain after unlock: %v", err)
	}

	unlockAgain, err := TryAcquire(lockPath)
	if err != nil {
		t.Fatalf("acquire with existing lock file: %v", err)
	}
	unlockAgain()
}
