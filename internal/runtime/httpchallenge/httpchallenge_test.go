package httpchallenge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShutdownBeforeStartDoesNotBlock(t *testing.T) {
	server := New("127.0.0.1:0")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown before start should be a no-op: %v", err)
	}
}

func TestServerCannotRestartAfterShutdown(t *testing.T) {
	server := New("127.0.0.1:0")
	if err := server.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown server: %v", err)
	}
	if err := server.Start(); !errors.Is(err, ErrServerClosed) {
		t.Fatalf("restart after shutdown error = %v, want %v", err, ErrServerClosed)
	}
}

func TestChallengeHandlerServesOnlyKnownTokenOnExpectedPath(t *testing.T) {
	server := New("127.0.0.1:0")
	server.Set("token-123", "challenge-response")
	tests := []struct {
		path       string
		wantStatus int
		wantBody   string
	}{
		{path: "/.well-known/acme-challenge/token-123", wantStatus: http.StatusOK, wantBody: "challenge-response"},
		{path: "/token-123", wantStatus: http.StatusNotFound},
		{path: "/.well-known/acme-challenge/", wantStatus: http.StatusNotFound},
		{path: "/.well-known/acme-challenge/missing", wantStatus: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			server.handle(resp, req)
			if resp.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", resp.Code, tt.wantStatus, resp.Body.String())
			}
			if tt.wantBody != "" && resp.Body.String() != tt.wantBody {
				t.Fatalf("body = %q, want %q", resp.Body.String(), tt.wantBody)
			}
		})
	}
}
