package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestDefaultDoesNotReplaceGlobalLogger(t *testing.T) {
	before := slog.Default()
	_ = Default()
	_ = WithRequestID(context.Background(), "req-1")
	if slog.Default() != before {
		t.Fatal("Default must not replace slog's global default logger")
	}
}

func TestContextLoggerIncludesRequestID(t *testing.T) {
	var buf bytes.Buffer
	Init(slog.NewTextHandler(&buf, nil))
	t.Cleanup(func() { Init(nil) })

	ctx := WithRequestID(context.Background(), "req-123")
	InfoContext(ctx, "hello")
	if !strings.Contains(buf.String(), "request_id=req-123") {
		t.Fatalf("log output missing request_id: %q", buf.String())
	}
	if got := RequestIDFromContext(ctx); got != "req-123" {
		t.Fatalf("RequestIDFromContext = %q, want req-123", got)
	}
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("RequestIDFromContext without ID = %q, want empty", got)
	}
}
