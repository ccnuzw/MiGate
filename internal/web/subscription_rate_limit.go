package web

import (
	"net/http"
	"sync"
	"time"
)

const (
	subscriptionRateLimit       = 30 // requests per client IP per window
	subscriptionRateLimitWindow = time.Minute
)

// subscriptionRateLimiter is a fixed-window counter per client IP. It slows
// down guessing of subscription tokens.
type subscriptionRateLimiter struct {
	mu      sync.Mutex
	now     func() time.Time
	entries map[string]subscriptionRateLimitEntry
}

type subscriptionRateLimitEntry struct {
	count       int
	windowStart time.Time
}

func newSubscriptionRateLimiter() *subscriptionRateLimiter {
	return &subscriptionRateLimiter{now: time.Now, entries: map[string]subscriptionRateLimitEntry{}}
}

func (l *subscriptionRateLimiter) allow(key string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	if !ok || now.Sub(entry.windowStart) >= subscriptionRateLimitWindow {
		l.cleanup(now)
		l.entries[key] = subscriptionRateLimitEntry{count: 1, windowStart: now}
		return true
	}
	if entry.count >= subscriptionRateLimit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

// cleanup drops windows that have ended. Callers must hold l.mu.
func (l *subscriptionRateLimiter) cleanup(now time.Time) {
	for key, entry := range l.entries {
		if now.Sub(entry.windowStart) >= subscriptionRateLimitWindow {
			delete(l.entries, key)
		}
	}
}

func subscriptionRateLimitKey(r *http.Request, trustProxy bool) string {
	return "sub:" + remoteIP(r, trustProxy)
}
