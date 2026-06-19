package command

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunOutputTruncatesCombinedOutput(t *testing.T) {
	runner := RealCommandRunner{OutputLimit: 8}
	out, err := runner.RunOutput(context.Background(), "sh", "-c", "printf 1234567890; printf abc >&2")
	if err != nil {
		t.Fatalf("run output: %v", err)
	}
	if !bytes.Contains(out, []byte("[output truncated]")) {
		t.Fatalf("expected truncation marker, got %q", string(out))
	}
	if !strings.HasPrefix(string(out), "12345678") {
		t.Fatalf("expected output prefix to be preserved, got %q", string(out))
	}
}

func TestRunReturnsCommandError(t *testing.T) {
	runner := RealCommandRunner{}
	if err := runner.Run(context.Background(), "sh", "-c", "exit 7"); err == nil {
		t.Fatal("expected non-zero command to return an error")
	}
}

func TestRunOutputHonorsTimeout(t *testing.T) {
	runner := RealCommandRunner{Timeout: 20 * time.Millisecond}
	start := time.Now()
	_, err := runner.RunOutput(context.Background(), "sh", "-c", "sleep 1")
	if err == nil {
		t.Fatal("expected timeout to return an error")
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took too long: %s", elapsed)
	}
}
