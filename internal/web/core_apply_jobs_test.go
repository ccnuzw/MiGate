package web

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestCoreApplyLockCancellationDoesNotLeak(t *testing.T) {
	manager := newCoreApplyJobManager()
	lockPath := filepath.Join(t.TempDir(), "apply.lock")

	releaseFirst := make(chan struct{})
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		ok, _, errCode, detail := manager.withApplyLock(context.Background(), lockPath, func(context.Context) (bool, string, string, string) {
			<-releaseFirst
			return true, "ok", "", ""
		})
		if !ok || errCode != "" || detail != "" {
			t.Errorf("first lock holder failed: ok=%v err=%q detail=%q", ok, errCode, detail)
		}
	}()

	waitForCoreApplyJobCondition(t, time.Second, func() bool { return len(manager.applySem) == 1 })

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, _, errCode, _ := manager.withApplyLock(cancelledCtx, lockPath, func(context.Context) (bool, string, string, string) {
		t.Fatal("cancelled lock wait must not run work")
		return true, "unexpected", "", ""
	})
	if ok || errCode != "apply_cancelled" {
		t.Fatalf("expected cancelled wait, ok=%v err=%q", ok, errCode)
	}

	close(releaseFirst)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first lock holder did not finish")
	}

	ok, _, errCode, detail := manager.withApplyLock(context.Background(), lockPath, func(context.Context) (bool, string, string, string) {
		return true, "ok", "", ""
	})
	if !ok || errCode != "" || detail != "" {
		t.Fatalf("subsequent lock should proceed after cancellation, ok=%v err=%q detail=%q", ok, errCode, detail)
	}
}

func waitForCoreApplyJobCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if condition() {
		return
	}
	t.Fatal("condition not met before timeout")
}
