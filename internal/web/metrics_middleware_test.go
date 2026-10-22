package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResponseWrapperPreservesFlushing(t *testing.T) {
	underlying := httptest.NewRecorder()
	w := &responseWrapper{ResponseWriter: underlying, statusCode: http.StatusOK}

	flusher, ok := any(w).(http.Flusher)
	if !ok {
		t.Fatal("response wrapper does not implement http.Flusher")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	if _, err := w.Write([]byte("data: first\n\n")); err != nil {
		t.Fatalf("write response: %v", err)
	}
	flusher.Flush()

	if underlying.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", underlying.Code, http.StatusOK)
	}
	if got := underlying.Body.String(); got != "data: first\n\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestMetricPathHasBoundedCardinality(t *testing.T) {
	if got := metricPath(http.MethodPatch, "/api/inbounds/123"); got != "/api/inbounds/" {
		t.Fatalf("known dynamic route path = %q", got)
	}
	if got := metricPath(http.MethodGet, "/attacker-controlled/unique-1"); got != "unmatched" {
		t.Fatalf("unknown path = %q, want unmatched", got)
	}
	if got := metricPath(http.MethodGet, "/attacker-controlled/unique-2"); got != "unmatched" {
		t.Fatalf("second unknown path = %q, want same bounded label", got)
	}
	if got := metricMethod("CUSTOM-VERB"); got != "OTHER" {
		t.Fatalf("unknown method = %q, want OTHER", got)
	}
}
