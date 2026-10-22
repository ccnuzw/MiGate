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

func (l *loginLimiter) recordFailure(keys ...string) {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		entry := l.entries[key]
		entry.failures++
		if entry.failures >= l.failureLimit {
			entry.blockedAt = now
		}
		l.entries[key] = entry
	}
}

func (l *loginLimiter) reset(keys ...string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, key := range keys {
		delete(l.entries, key)
	}
}

func loginRateLimitKeys(r *http.Request, username string, trustProxy bool) []string {
	ip := remoteIP(r, trustProxy)
	user := strings.ToLower(strings.TrimSpace(username))
	if user == "" {
		user = "-"
	}
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
