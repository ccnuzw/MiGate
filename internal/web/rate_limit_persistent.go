package web

import (
	"context"
	"sync"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/logger"
)

// loginRateLimitStore is implemented by *db.Store.
type loginRateLimitStore interface {
	GetLoginRateLimit(ctx context.Context, key string) (db.LoginRateLimit, bool, error)
	SaveLoginRateLimit(ctx context.Context, rec db.LoginRateLimit) error
	DeleteLoginRateLimit(ctx context.Context, key string) error
	PruneLoginRateLimits(ctx context.Context, cutoff time.Time) (int64, error)
}

const (
	loginRateLimitStoreTimeout  = 2 * time.Second
	loginRateLimitPruneInterval = 10 * time.Minute
	loginRateLimitErrorLogEvery = time.Minute
)

// persistentLoginLimiter keeps login blocks across restarts. The in-memory
// loginLimiter makes every decision; the store mirrors its state and is read
// once per key to restore blocks recorded before a restart.
//
// Store errors never block a login: limits are still enforced from memory, and
// only blocks recorded before a restart are missed until the store recovers.
// Store errors are logged at most once per loginRateLimitErrorLogEvery.
type persistentLoginLimiter struct {
	memory *loginLimiter
	store  loginRateLimitStore

	mu        sync.Mutex
	loaded    map[string]bool
	nextPrune time.Time

	// persistMu orders store writes so the last write carries the latest state.
	persistMu sync.Mutex

	errMu        sync.Mutex
	nextErrorLog time.Time
}

func newPersistentLoginLimiter(store loginRateLimitStore, failureLimit int, cooldown time.Duration) *persistentLoginLimiter {
	return &persistentLoginLimiter{
		memory: newLoginLimiter(failureLimit, cooldown),
		store:  store,
		loaded: map[string]bool{},
	}
}

func (l *persistentLoginLimiter) allow(keys ...string) bool {
	l.maybePrune()
	l.load(keys)
	return l.memory.allow(keys...)
}

func (l *persistentLoginLimiter) recordFailure(keys ...string) bool {
	l.load(keys)
	blocked := l.memory.recordFailure(keys...)
	l.persist(keys)
	return blocked
}

func (l *persistentLoginLimiter) reset(keys ...string) {
	l.memory.reset(keys...)
	l.persist(keys)
}

// load seeds memory from the store the first time each key is seen. A key
// whose read fails is retried on its next use.
func (l *persistentLoginLimiter) load(keys []string) {
	for _, key := range keys {
		if l.isLoaded(key) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), loginRateLimitStoreTimeout)
		rec, ok, err := l.store.GetLoginRateLimit(ctx, key)
		cancel()
		if err != nil {
			l.logStoreError("read", err)
			continue
		}
		if ok {
			l.memory.seed(key, loginLimitEntry{failures: rec.Failures, blockedAt: rec.BlockedAt})
		}
		l.mu.Lock()
		l.loaded[key] = true
		l.mu.Unlock()
	}
}

func (l *persistentLoginLimiter) isLoaded(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.loaded[key]
}

// persist mirrors the in-memory state of keys to the store. A key whose stored
// state could not be read is not overwritten, so a failed read never lowers a
// stored failure count. Deleting a key (after a successful login, or once its
// block expired in memory) is always safe.
func (l *persistentLoginLimiter) persist(keys []string) {
	l.persistMu.Lock()
	defer l.persistMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), loginRateLimitStoreTimeout)
	defer cancel()
	now := l.memory.now()
	for _, key := range keys {
		entry, ok := l.memory.lookup(key)
		var err error
		switch {
		case !ok:
			err = l.store.DeleteLoginRateLimit(ctx, key)
		case l.isLoaded(key):
			err = l.store.SaveLoginRateLimit(ctx, db.LoginRateLimit{Key: key, Failures: entry.failures, BlockedAt: entry.blockedAt, UpdatedAt: now})
		default:
			continue
		}
		if err != nil {
			l.logStoreError("write", err)
		}
	}
}

// maybePrune deletes rows untouched for two cooldowns, at most once per
// loginRateLimitPruneInterval. Any block in such a row has already expired.
func (l *persistentLoginLimiter) maybePrune() {
	now := l.memory.now()
	l.mu.Lock()
	if now.Before(l.nextPrune) {
		l.mu.Unlock()
		return
	}
	l.nextPrune = now.Add(loginRateLimitPruneInterval)
	l.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), loginRateLimitStoreTimeout)
	defer cancel()
	if _, err := l.store.PruneLoginRateLimits(ctx, now.Add(-2*l.memory.cooldown)); err != nil {
		l.logStoreError("prune", err)
		return
	}
	// Re-read keys on next use; this also keeps the loaded set from growing without bound.
	l.mu.Lock()
	l.loaded = map[string]bool{}
	l.mu.Unlock()
}

// logStoreError throttles on the real clock so a failing store cannot flood the log.
func (l *persistentLoginLimiter) logStoreError(op string, err error) {
	now := time.Now()
	l.errMu.Lock()
	if now.Before(l.nextErrorLog) {
		l.errMu.Unlock()
		return
	}
	l.nextErrorLog = now.Add(loginRateLimitErrorLogEvery)
	l.errMu.Unlock()
	logger.Error("login rate limit store failed; enforcing limits from memory only", "op", op, "error", err)
}
