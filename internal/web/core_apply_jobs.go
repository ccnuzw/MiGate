package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

func (m *coreApplyJobManager) running(core string) bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	job := m.byCore[core]
	return job != nil && (job.Status == "queued" || job.Status == "running")
}

func (m *coreApplyJobManager) tryStart(core string, message string) (*CoreApplyJobStatus, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing := m.byCore[core]
	if existing != nil && (existing.Status == "queued" || existing.Status == "running") {
		return cloneCoreApplyJobStatus(existing), false
	}
	job := &CoreApplyJobStatus{
		ID:        newCoreApplyJobID(),
		Core:      core,
		Status:    "queued",
		StartedAt: time.Now().UTC().Format(time.RFC3339),
		Message:   message,
		Accepted:  true,
	}
	m.byCore[core] = job
	m.byID[job.ID] = job
	return cloneCoreApplyJobStatus(job), true
}

func (m *coreApplyJobManager) markRunning(id string, message string) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "running"
		if message != "" {
			job.Message = message
		}
	})
}

func (m *coreApplyJobManager) markSucceeded(id string, message string, detail string) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "succeeded"
		job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		job.Message = message
		job.Error = ""
		job.Detail = detail
	})
}

func (m *coreApplyJobManager) markFailed(id string, message string, errCode string, detail string) {
	m.update(id, func(job *CoreApplyJobStatus) {
		job.Status = "failed"
		job.FinishedAt = time.Now().UTC().Format(time.RFC3339)
		job.Message = message
		job.Error = errCode
		job.Detail = detail
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

// runCoreApplyJob reports apply_timeout promptly for UI feedback. If the work
// later returns despite cancellation, the job is updated to the actual result.
func runCoreApplyJob(ctx context.Context, cfg *routerConfig, core string, message string, keys []string, work func(context.Context) (bool, string, string, string)) *CoreApplyJobStatus {
	if cfg == nil || cfg.applyJobs == nil {
		return nil
	}
	job, started := cfg.applyJobs.tryStart(core, message)
	if !started || job == nil {
		return nil
	}
	go func(jobID string) {
		cfg.applyJobs.markRunning(jobID, message)
		timeout := cfg.coreApplyTimeout
		if timeout <= 0 {
			timeout = 2 * time.Minute
		}
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		type workResult struct {
			ok      bool
			message string
			errCode string
			detail  string
		}
		resultCh := make(chan workResult, 1)
		go func() {
			ok, successMessage, errCode, detail := work(timeoutCtx)
			result := workResult{ok: ok, message: successMessage, errCode: errCode, detail: detail}
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
			resultCh <- result
		}()
		select {
		case result := <-resultCh:
			_ = result
		case <-timeoutCtx.Done():
			cfg.applyJobs.markFailed(jobID, "核心应用超时", "apply_timeout", timeoutCtx.Err().Error())
			if cfg.coreCache != nil {
				cfg.coreCache.invalidate(keys...)
			}
		}
	}(job.ID)
	return job
}

func cloneCoreApplyJobStatus(job *CoreApplyJobStatus) *CoreApplyJobStatus {
	if job == nil {
		return nil
	}
	cloned := *job
	return &cloned
}

func newCoreApplyJobID() string {
	var buf [12]byte
	if _, err := rand.Read(buf[:]); err != nil {
		now := time.Now().UTC().UnixNano()
		return hex.EncodeToString([]byte(time.Unix(0, now).UTC().Format("20060102150405.000000000")))
	}
	return hex.EncodeToString(buf[:])
}
