package web

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	defaultLoginFailureLimit = 5
	defaultLoginCooldown     = 5 * time.Minute
	// IP-level limit should be more restrictive to prevent username enumeration
	ipOnlyFailureMultiplier = 2 // IP gets blocked after 2x the per-user limit
)

type loginLimiter struct {
	mu           sync.Mutex
	now          func() time.Time
	failureLimit int
	cooldown     time.Duration
	entries      map[string]loginLimitEntry
}

type loginLimitEntry struct {
	failures  int
	blockedAt time.Time
}

type loginRateLimiter interface {
	allow(keys ...string) bool
	// recordFailure reports whether this failure just blocked one of the keys.
	recordFailure(keys ...string) bool
	reset(keys ...string)
}

func newLoginLimiter(failureLimit int, cooldown time.Duration) *loginLimiter {
	if failureLimit <= 0 {
		failureLimit = defaultLoginFailureLimit
	}
	if cooldown <= 0 {
		cooldown = defaultLoginCooldown
	}
	return &loginLimiter{
		now:          time.Now,
		failureLimit: failureLimit,
		cooldown:     cooldown,
		entries:      map[string]loginLimitEntry{},
	}
}

func (l *loginLimiter) allow(keys ...string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		entry := l.entries[key]
		if entry.failures >= l.failureLimit && now.Sub(entry.blockedAt) < l.cooldown {
			return false
		}
		if entry.failures >= l.failureLimit && now.Sub(entry.blockedAt) >= l.cooldown {
			delete(l.entries, key)
		}
	}
	return true
}

func (l *loginLimiter) recordFailure(keys ...string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	blocked := false
	for _, key := range keys {
		entry := l.entries[key]
		entry.failures++
		if entry.failures >= l.failureLimit {
			entry.blockedAt = now
		}
		if entry.failures == l.failureLimit {
			blocked = true
		}
		l.entries[key] = entry
	}
	return blocked
}

func (l *loginLimiter) reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		delete(l.entries, key)
	}
}

func (l *loginLimiter) lookup(key string) (loginLimitEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.entries[key]
	return entry, ok
}

// seed merges persisted state into memory. It only raises the failure count
// and block time, so it never undoes failures recorded in this process, and it
// ignores state whose block has already expired.
func (l *loginLimiter) seed(key string, stored loginLimitEntry) {
	now := l.now()
	if stored.failures <= 0 || (stored.failures >= l.failureLimit && now.Sub(stored.blockedAt) >= l.cooldown) {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	entry := l.entries[key]
	if stored.failures > entry.failures {
		entry.failures = stored.failures
	}
	if stored.blockedAt.After(entry.blockedAt) {
		entry.blockedAt = stored.blockedAt
	}
	l.entries[key] = entry
}

func loginRateLimitKeys(r *http.Request, username string, trustProxy bool) []string {
	ip := remoteIP(r, trustProxy)
	user := strings.ToLower(strings.TrimSpace(username))
	if user == "" {
		user = "-"
	}
	// Return IP-only key first (most restrictive), then per-user key
	// This prevents bypassing rate limit by trying different usernames
	return []string{"ip:" + ip, "ip_user:" + ip + ":" + user}
}

// remoteIP returns the client address used for login rate limiting and logs.
// With trustProxy it takes the last X-Forwarded-For entry, which is the address
// the trusted proxy itself saw. Earlier entries come from the client and can be
// forged, so trusting the first one would let a client pick a fresh rate-limit
// key on every request. The last header line is used because some proxies add
// their own line instead of extending the client's.
func remoteIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if values := r.Header.Values("X-Forwarded-For"); len(values) > 0 {
			entries := strings.Split(values[len(values)-1], ",")
			last := strings.TrimSpace(entries[len(entries)-1])
			if net.ParseIP(last) != nil {
				return last
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}
