package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/lockfile"
)

type coreApplyJobManager struct {
	mu       sync.Mutex
	applySem chan struct{}
	byCore   map[string]*CoreApplyJobStatus
	byID     map[string]*CoreApplyJobStatus
}

type coreApplyWorkResult struct {
	ok      bool
	message string
	errCode string
	detail  string
}

type coreApplyAttemptResult struct {
	work coreApplyWorkResult
	done <-chan struct{}
}

func newCoreApplyJobManager() *coreApplyJobManager {
	return &coreApplyJobManager{
		applySem: make(chan struct{}, 1),
		byCore:   map[string]*CoreApplyJobStatus{},
		byID:     map[string]*CoreApplyJobStatus{},
	}
}

func (m *coreApplyJobManager) latest(core string) *CoreApplyJobStatus {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.byCore[core]
	return cloneCoreApplyJobStatus(job)
}

func (m *coreApplyJobManager) get(id string) *CoreApplyJobStatus {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.byID[id]
	return cloneCoreApplyJobStatus(job)
}

func (m *coreApplyJobManager) running(core string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.byCore[core]
	return job != nil && coreApplyJobActive(job.Status)
}

func (m *coreApplyJobManager) tryStart(core string, message string, maxRetries int) (*CoreApplyJobStatus, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := m.byCore[core]
	if existing != nil && coreApplyJobActive(existing.Status) {
		return cloneCoreApplyJobStatus(existing), false
	}
	job := &CoreApplyJobStatus{
		ID:         newCoreApplyJobID(),
		Core:       core,
		Status:     "queued",
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
		Message:    message,
		Accepted:   true,
		MaxRetries: maxRetries,
	}
	m.byCore[core] = job
	m.byID[job.ID] = job
	return cloneCoreApplyJobStatus(job), true
}

func (m *coreApplyJobManager) markRunning(id string, message string) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "running"
		job.NextRetryAt = ""
		if message != "" {
			job.Message = message
		}
	})
}

func (m *coreApplyJobManager) markRetrying(id string, message string, errCode string, detail string, retryCount int, maxRetries int, nextRetryAt time.Time) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "retrying"
		job.Message = message
		job.Error = errCode
		job.Detail = detail
		job.RetryCount = retryCount
		job.MaxRetries = maxRetries
		job.NextRetryAt = nextRetryAt.UTC().Format(time.RFC3339)
	})
}

func (m *coreApplyJobManager) markSucceeded(id string, message string, detail string) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "succeeded"
		job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		job.Message = message
		job.Error = ""
		job.Detail = detail
		job.NextRetryAt = ""
	})
}

func (m *coreApplyJobManager) markFailed(id string, message string, errCode string, detail string) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "failed"
		job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		job.Message = message
		job.Error = errCode
		job.Detail = detail
		job.NextRetryAt = ""
	})
}

func (m *coreApplyJobManager) update(id string, mutate func(*CoreApplyJobStatus)) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.byID[id]
	if job == nil {
		return
	}
	mutate(job)
}

func (m *coreApplyJobManager) withApplyLock(ctx context.Context, path string, work func(context.Context) (bool, string, string, string)) (bool, string, string, string) {
	if m == nil {
		return false, "核心应用失败", "apply_jobs_unavailable", "apply_jobs_unavailable"
	}
	select {
	case <-ctx.Done():
		return false, "核心应用取消", "apply_cancelled", ctx.Err().Error()
	case m.applySem <- struct{}{}:
	}
	defer func() { <-m.applySem }()
	unlock, err := lockfile.TryAcquire(path)
	if err != nil {
		return false, "核心应用失败", "apply_locked", err.Error()
	}
	defer unlock()
	return work(ctx)
}

// runCoreApplyJob runs a single asynchronous core apply attempt. If an attempt
// times out, the work context is cancelled and the job records apply_timeout;
// late work results do not overwrite that timeout.
func runCoreApplyJob(ctx context.Context, cfg *routerConfig, core string, message string, keys []string, work func(context.Context) (bool, string, string, string)) *CoreApplyJobStatus {
	return runCoreApplyJobWithRetry(ctx, cfg, core, message, keys, 0, work)
}

func runCoreApplyJobWithRetry(ctx context.Context, cfg *routerConfig, core string, message string, keys []string, maxRetries int, work func(context.Context) (bool, string, string, string)) *CoreApplyJobStatus {
	if cfg == nil || cfg.applyJobs == nil {
		return nil
	}
	job, started := cfg.applyJobs.tryStart(core, message, maxRetries)
	if !started || job == nil {
		return nil
	}
	go func(jobID string) {
		cfg.applyJobs.markRunning(jobID, message)
		timeout := cfg.coreApplyTimeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		result := runCoreApplyWorkWithRetry(ctx, cfg, jobID, core, message, maxRetries, timeout, work)
		if result.ok {
			cfg.applyJobs.markSucceeded(jobID, result.message, result.detail)
		} else {
			cfg.applyJobs.markFailed(jobID, result.message, result.errCode, result.detail)
		}
		if cfg.coreCache != nil {
			cfg.coreCache.invalidate(keys...)
		}
		if result.ok {
			scheduleCoreAutoApplyIfStillDirty(context.WithoutCancel(ctx), cfg, core)
		}
	}(job.ID)
	return job
}

func runCoreApplyWorkWithRetry(ctx context.Context, cfg *routerConfig, jobID string, core string, message string, maxRetries int, timeout time.Duration, work func(context.Context) (bool, string, string, string)) coreApplyWorkResult {
	retryCount := 0
	for {
		if retryCount > 0 {
			cfg.applyJobs.markRunning(jobID, message)
		}
		attempt := runCoreApplyAttempt(ctx, timeout, work)
		result := attempt.work
		if result.ok || retryCount >= maxRetries || !coreApplyErrorRetryable(result.errCode, result.detail) {
			return result
		}
		if cleanup := waitForCoreApplyAttemptCleanup(ctx, attempt.done, timeout); !cleanup.ok {
			return cleanup
		}
		retryCount++
		delay := coreApplyRetryDelay(cfg, retryCount)
		nextRetryAt := time.Now().Add(delay)
		cfg.applyJobs.markRetrying(jobID, coreAutoRetryMessage(core), result.errCode, result.detail, retryCount, maxRetries, nextRetryAt)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return coreApplyWorkResult{ok: false, message: "核心应用取消", errCode: "apply_cancelled", detail: ctx.Err().Error()}
		case <-timer.C:
		}
	}
}

func waitForCoreApplyAttemptCleanup(ctx context.Context, done <-chan struct{}, attemptTimeout time.Duration) coreApplyWorkResult {
	if done == nil {
		return coreApplyWorkResult{ok: true}
	}
	grace := coreApplyAttemptCleanupGrace(attemptTimeout)
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return coreApplyWorkResult{ok: false, message: "核心应用取消", errCode: "apply_cancelled", detail: ctx.Err().Error()}
	case <-done:
		return coreApplyWorkResult{ok: true}
	case <-timer.C:
		return coreApplyWorkResult{ok: false, message: "核心应用清理超时", errCode: "apply_cleanup_timeout", detail: "timed out waiting for cancelled core apply attempt to exit"}
	}
}

func coreApplyAttemptCleanupGrace(attemptTimeout time.Duration) time.Duration {
	if attemptTimeout <= 0 {
		return 5 * time.Second
	}
	if attemptTimeout < 5*time.Second {
		return attemptTimeout
	}
	return 5 * time.Second
}

func runCoreApplyAttempt(ctx context.Context, timeout time.Duration, work func(context.Context) (bool, string, string, string)) coreApplyAttemptResult {
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	resultCh := make(chan coreApplyWorkResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ok, successMessage, errCode, detail := work(attemptCtx)
		resultCh <- coreApplyWorkResult{ok: ok, message: successMessage, errCode: errCode, detail: detail}
	}()
	select {
	case result := <-resultCh:
		cancel()
		return coreApplyAttemptResult{work: result, done: done}
	case <-attemptCtx.Done():
		err := attemptCtx.Err()
		cancel()
		if err == context.Canceled {
			return coreApplyAttemptResult{work: coreApplyWorkResult{ok: false, message: "核心应用取消", errCode: "apply_cancelled", detail: err.Error()}, done: done}
		}
		return coreApplyAttemptResult{work: coreApplyWorkResult{ok: false, message: "核心应用超时", errCode: "apply_timeout", detail: err.Error()}, done: done}
	}
}

func cloneCoreApplyJobStatus(job *CoreApplyJobStatus) *CoreApplyJobStatus {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}

func coreApplyJobActive(status string) bool {
	switch status {
	case "queued", "running", "retrying":
		return true
	default:
		return false
	}
}

func coreApplyErrorRetryable(errCode string, detail string) bool {
	switch errCode {
	case "apply_locked", "apply_timeout", "status_failed", "service_status_failed", "listener_check_failed", "port_check_failed", "restart_in_progress":
		return true
	case "apply_cleanup_timeout", "build_failed", "build_xray_config_failed", "validation_failed", "validate_failed", "config_invalid", "unsupported_core", "store_unavailable", "core_apply_state_store_unavailable", "record_apply_state_failed", "permission_denied", "write_failed", "write_config_failed", "cert_failed":
		return false
	}
	text := errCode + " " + detail
	text = strings.ToLower(text)
	return strings.Contains(text, "temporar") ||
		strings.Contains(text, "timeout") ||
		strings.Contains(text, "locked") ||
		strings.Contains(text, "lock") ||
		strings.Contains(text, "systemd") && strings.Contains(text, "busy") ||
		strings.Contains(text, "restart") && strings.Contains(text, "progress")
}

func coreApplyRetryDelay(cfg *routerConfig, retryCount int) time.Duration {
	if cfg != nil && cfg.coreApplyRetryDelay != nil {
		delay := cfg.coreApplyRetryDelay(retryCount)
		if delay > 0 {
			return delay
		}
	}
	switch retryCount {
	case 1:
		return time.Second
	case 2:
		return 3 * time.Second
	default:
		return 8 * time.Second
	}
}

func newCoreApplyJobID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		now := time.Now().UTC().UnixNano()
		return hex.EncodeToString([]byte(time.Unix(0, now).UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf[:])
}
