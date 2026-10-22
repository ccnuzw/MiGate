package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/imzyb/MiGate/internal/logger"
)

func serveWithRequestID(incoming string) (contextID string, rec *httptest.ResponseRecorder) {
	handler := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID = logger.RequestIDFromContext(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if incoming != "" {
		req.Header.Set("X-Request-ID", incoming)
	}
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return contextID, rec
}

func TestRequestIDMiddlewareKeepsValidUpstreamID(t *testing.T) {
	got, rec := serveWithRequestID("proxy-abc_123.4")
	if got != "proxy-abc_123.4" || rec.Header().Get("X-Request-ID") != got {
		t.Fatalf("context ID %q, response ID %q", got, rec.Header().Get("X-Request-ID"))
	}
}

func TestRequestIDMiddlewareReplacesMissingOrUnsafeID(t *testing.T) {
	for _, incoming := range []string{"", "has space", `quote"`, strings.Repeat("a", maxRequestIDLength+1)} {
		got, rec := serveWithRequestID(incoming)
		if got == incoming || len(got) != 16 || rec.Header().Get("X-Request-ID") != got {
			t.Fatalf("incoming %q: context ID %q, response ID %q", incoming, got, rec.Header().Get("X-Request-ID"))
		}
	}
}
