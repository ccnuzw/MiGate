package web

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
	"github.com/imzyb/MiGate/internal/logger"
)

func openLimiterStore(t *testing.T, path string) *db.Store {
	t.Helper()
	store, err := db.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store
}

// newTestPersistentLimiter returns a limiter whose clock reads *now, if now is set.
func newTestPersistentLimiter(store loginRateLimitStore, failureLimit int, now *time.Time) *persistentLoginLimiter {
	limiter := newPersistentLoginLimiter(store, failureLimit, time.Minute)
	if now != nil {
		limiter.memory.now = func() time.Time { return *now }
	}
	return limiter
}

func TestPersistentLoginLimiterSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migate.db")
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	store := openLimiterStore(t, path)
	limiter := newTestPersistentLimiter(store, 2, &now)
	limiter.recordFailure("ip:192.0.2.1")
	if !limiter.allow("ip:192.0.2.1") {
		t.Fatal("one failure should not block")
	}
	limiter.recordFailure("ip:192.0.2.1")
	if limiter.allow("ip:192.0.2.1") {
		t.Fatal("reaching the failure limit should block")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	store = openLimiterStore(t, path)
	defer store.Close()
	restarted := newTestPersistentLimiter(store, 2, &now)
	if restarted.allow("ip:192.0.2.1") {
		t.Fatal("block must survive a restart")
	}
	now = now.Add(time.Minute)
	if !restarted.allow("ip:192.0.2.1") {
		t.Fatal("block must expire after the cooldown")
	}
}

func TestPersistentLoginLimiterResetClearsStoredBlock(t *testing.T) {
	store := openLimiterStore(t, ":memory:")
	defer store.Close()
	keys := []string{"ip:192.0.2.1", "ip_user:192.0.2.1:admin"}

	limiter := newTestPersistentLimiter(store, 1, nil)
	limiter.recordFailure(keys...)
	if limiter.allow(keys...) {
		t.Fatal("expected block after reaching the failure limit")
	}
	limiter.reset(keys...)
	if !limiter.allow(keys...) {
		t.Fatal("reset should clear the block in memory")
	}
	if !newTestPersistentLimiter(store, 1, nil).allow(keys...) {
		t.Fatal("reset should clear the stored block")
	}
}

// failingReadStore fails every read and prune; writes succeed and are counted.
type failingReadStore struct {
	saves, deletes int
}

func (s *failingReadStore) GetLoginRateLimit(context.Context, string) (db.LoginRateLimit, bool, error) {
	return db.LoginRateLimit{}, false, errors.New("disk I/O error")
}

func (s *failingReadStore) SaveLoginRateLimit(context.Context, db.LoginRateLimit) error {
	s.saves++
	return nil
}

func (s *failingReadStore) DeleteLoginRateLimit(context.Context, string) error {
	s.deletes++
	return nil
}

func (s *failingReadStore) PruneLoginRateLimits(context.Context, time.Time) (int64, error) {
	return 0, errors.New("disk I/O error")
}

func TestPersistentLoginLimiterFallsBackToMemoryWhenStoreFails(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(slog.NewTextHandler(&buf, nil))
	t.Cleanup(func() { logger.Init(nil) })

	store := &failingReadStore{}
	limiter := newTestPersistentLimiter(store, 2, nil)
	key := "ip:192.0.2.1"
	if !limiter.allow(key) {
		t.Fatal("a failing store must not block logins")
	}
	limiter.recordFailure(key)
	limiter.recordFailure(key)
	if limiter.allow(key) {
		t.Fatal("limits must still be enforced from memory")
	}
	if store.saves != 0 {
		t.Fatalf("unread keys must not be overwritten, got %d saves", store.saves)
	}
	for i := 0; i < 20; i++ {
		limiter.allow(key)
	}
	if n := strings.Count(buf.String(), "login rate limit store failed"); n != 1 {
		t.Fatalf("store errors should be logged once per interval, got %d:\n%s", n, buf.String())
	}
	limiter.reset(key)
	if store.deletes != 1 {
		t.Fatalf("reset should delete the stored key even if it was never read, got %d deletes", store.deletes)
	}
}

func TestRouterWithStoreSharesLoginBlockAcrossInstances(t *testing.T) {
	store := openLimiterStore(t, ":memory:")
	defer store.Close()

	first := NewRouter(WithAuth("admin", "secret"), WithStore(store), WithLoginRateLimit(1, time.Minute))
	wrong := httptest.NewRecorder()
	first.ServeHTTP(wrong, localLoginRequest(`{"username":"admin","password":"wrong"}`))
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong password: got %d, want 401", wrong.Code)
	}

	// A new router on the same store, as after a panel restart, must keep the block.
	second := NewRouter(WithAuth("admin", "secret"), WithStore(store), WithLoginRateLimit(1, time.Minute))
	blocked := httptest.NewRecorder()
	second.ServeHTTP(blocked, localLoginRequest(`{"username":"admin","password":"secret"}`))
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("login after restart: got %d, want 429: %s", blocked.Code, blocked.Body.String())
	}
}

func TestLoginLogsBlockOnceWithProxyAwareClientIP(t *testing.T) {
	var buf bytes.Buffer
	logger.Init(slog.NewTextHandler(&buf, nil))
	t.Cleanup(func() { logger.Init(nil) })

	router := NewRouter(WithAuth("admin", "secret"), WithTrustedProxyHeaders(true), WithLoginRateLimit(2, time.Minute))
	var last *httptest.ResponseRecorder
	for i := 0; i < 5; i++ {
		req := localLoginRequest(`{"username":"admin","password":"wrong"}`)
		req.Header.Set("X-Forwarded-For", "198.51.100.7")
		last = httptest.NewRecorder()
		router.ServeHTTP(last, req)
	}
	if last.Code != http.StatusTooManyRequests {
		t.Fatalf("last attempt: got %d, want 429", last.Code)
	}
	out := buf.String()
	if n := strings.Count(out, "login: invalid credentials"); n != 2 {
		t.Fatalf("want 2 invalid-credential lines (rate-limited attempts are not logged), got %d:\n%s", n, out)
	}
	if n := strings.Count(out, "login: blocking further attempts"); n != 1 {
		t.Fatalf("want 1 block line, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "client_ip=198.51.100.7") || strings.Contains(out, "127.0.0.1") {
		t.Fatalf("logs should use the proxy-aware client IP, not the socket address:\n%s", out)
	}
}
