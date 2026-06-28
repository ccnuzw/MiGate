package web

import (
	"context"
	"path/filepath"
	"sync/atomic"
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

func TestCoreApplyJobRetriesRealAttemptTimeout(t *testing.T) {
	manager := newCoreApplyJobManager()
	cfg := &routerConfig{
		applyJobs:           manager,
		coreApplyTimeout:    30 * time.Millisecond,
		coreApplyRetryDelay: func(int) time.Duration { return time.Millisecond },
	}
	var calls atomic.Int32
	job := runCoreApplyJobWithRetry(context.Background(), cfg, "xray", "sync", nil, 2, func(ctx context.Context) (bool, string, string, string) {
		call := calls.Add(1)
		if call == 1 {
			select {
			case <-ctx.Done():
				return false, "timeout", "apply_timeout", ctx.Err().Error()
			case <-time.After(200 * time.Millisecond):
				return false, "unexpected", "unexpected", ""
			}
		}
		return true, "ok", "", ""
	})
	if job == nil {
		t.Fatal("expected job to start")
	}
	waitForCoreApplyJobCondition(t, time.Second, func() bool {
		current := manager.get(job.ID)
		return current != nil && current.Status == "succeeded"
	})
	if calls.Load() != 2 {
		t.Fatalf("expected timed-out attempt plus retry success, got %d calls", calls.Load())
	}
	current := manager.get(job.ID)
	if current == nil || current.Status != "succeeded" || current.RetryCount != 1 {
		t.Fatalf("expected succeeded job with one retry, got %+v", current)
	}
}

func TestCoreApplyJobWaitsForTimedOutAttemptBeforeRetry(t *testing.T) {
	manager := newCoreApplyJobManager()
	cfg := &routerConfig{
		applyJobs:           manager,
		coreApplyTimeout:    100 * time.Millisecond,
		coreApplyRetryDelay: func(int) time.Duration { return time.Millisecond },
	}
	var calls atomic.Int32
	firstCancelled := make(chan struct{})
	releaseFirst := make(chan struct{})
	secondStarted := make(chan struct{})

	job := runCoreApplyJobWithRetry(context.Background(), cfg, "xray", "sync", nil, 2, func(ctx context.Context) (bool, string, string, string) {
		call := calls.Add(1)
		if call == 1 {
			<-ctx.Done()
			close(firstCancelled)
			<-releaseFirst
			return false, "timeout", "apply_timeout", ctx.Err().Error()
		}
		close(secondStarted)
		return true, "ok", "", ""
	})
	if job == nil {
		t.Fatal("expected job to start")
	}
	select {
	case <-firstCancelled:
	case <-time.After(time.Second):
		t.Fatal("first attempt was not cancelled")
	}
	select {
	case <-secondStarted:
		t.Fatal("retry started before timed-out attempt exited")
	case <-time.After(30 * time.Millisecond):
	}
	close(releaseFirst)
	waitForCoreApplyJobCondition(t, time.Second, func() bool {
		current := manager.get(job.ID)
		return current != nil && current.Status == "succeeded"
	})
	if calls.Load() != 2 {
		t.Fatalf("expected timed-out attempt plus retry success, got %d calls", calls.Load())
	}
}

func TestCoreApplyJobFailsWhenTimedOutAttemptDoesNotExit(t *testing.T) {
	manager := newCoreApplyJobManager()
	cfg := &routerConfig{
		applyJobs:           manager,
		coreApplyTimeout:    20 * time.Millisecond,
		coreApplyRetryDelay: func(int) time.Duration { return time.Millisecond },
	}
	firstCancelled := make(chan struct{})

	job := runCoreApplyJobWithRetry(context.Background(), cfg, "xray", "sync", nil, 2, func(ctx context.Context) (bool, string, string, string) {
		<-ctx.Done()
		close(firstCancelled)
		select {}
	})
	if job == nil {
		t.Fatal("expected job to start")
	}
	select {
	case <-firstCancelled:
	case <-time.After(time.Second):
		t.Fatal("attempt was not cancelled")
	}
	waitForCoreApplyJobCondition(t, time.Second, func() bool {
		current := manager.get(job.ID)
		return current != nil && current.Status == "failed" && current.Error == "apply_cleanup_timeout"
	})
	if manager.running("xray") {
		t.Fatal("cleanup timeout must release active job state")
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
