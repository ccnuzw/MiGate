package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSubscriptionRateLimiterWindow(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	limiter := newSubscriptionRateLimiter()
	limiter.now = func() time.Time { return now }

	for i := 0; i < subscriptionRateLimit; i++ {
		if !limiter.allow("sub:192.0.2.1") {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	if limiter.allow("sub:192.0.2.1") {
		t.Fatal("request beyond the limit should be blocked")
	}
	if !limiter.allow("sub:192.0.2.2") {
		t.Fatal("other clients must not share the limit")
	}
	now = now.Add(subscriptionRateLimitWindow)
	if !limiter.allow("sub:192.0.2.1") {
		t.Fatal("a new window should allow requests again")
	}
}

func TestSubscriptionEndpointReturns429WhenRateLimited(t *testing.T) {
	router := NewRouter()
	var last *httptest.ResponseRecorder
	for i := 0; i <= subscriptionRateLimit; i++ {
		last = httptest.NewRecorder()
		router.ServeHTTP(last, httptest.NewRequest(http.MethodGet, "/sub/unknown-token", nil))
	}
	if last.Code != http.StatusTooManyRequests || last.Header().Get("Retry-After") == "" {
		t.Fatalf("got %d (Retry-After %q), want 429 with Retry-After", last.Code, last.Header().Get("Retry-After"))
	}
}
