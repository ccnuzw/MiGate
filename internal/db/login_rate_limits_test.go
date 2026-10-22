package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/imzyb/MiGate/internal/db"
)

func TestLoginRateLimitsRoundTripAndPrune(t *testing.T) {
	ctx := context.Background()
	store, err := db.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	if _, ok, err := store.GetLoginRateLimit(ctx, "ip:192.0.2.1"); err != nil || ok {
		t.Fatalf("missing key: ok=%v err=%v", ok, err)
	}

	blockedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	rec := db.LoginRateLimit{Key: "ip:192.0.2.1", Failures: 5, BlockedAt: blockedAt, UpdatedAt: blockedAt}
	if err := store.SaveLoginRateLimit(ctx, rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	rec.Failures = 6
	if err := store.SaveLoginRateLimit(ctx, rec); err != nil {
		t.Fatalf("save again: %v", err)
	}
	got, ok, err := store.GetLoginRateLimit(ctx, rec.Key)
	if err != nil || !ok || got.Failures != 6 || !got.BlockedAt.Equal(blockedAt) {
		t.Fatalf("get = %+v ok=%v err=%v", got, ok, err)
	}

	if n, err := store.PruneLoginRateLimits(ctx, blockedAt); err != nil || n != 0 {
		t.Fatalf("prune before update time: n=%d err=%v", n, err)
	}
	if n, err := store.PruneLoginRateLimits(ctx, blockedAt.Add(time.Second)); err != nil || n != 1 {
		t.Fatalf("prune after update time: n=%d err=%v", n, err)
	}

	unblocked := db.LoginRateLimit{Key: "ip_user:192.0.2.1:admin", Failures: 1, UpdatedAt: blockedAt}
	if err := store.SaveLoginRateLimit(ctx, unblocked); err != nil {
		t.Fatalf("save unblocked: %v", err)
	}
	if got, _, _ := store.GetLoginRateLimit(ctx, unblocked.Key); !got.BlockedAt.IsZero() {
		t.Fatalf("unblocked key has BlockedAt %v", got.BlockedAt)
	}
	if err := store.DeleteLoginRateLimit(ctx, unblocked.Key); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := store.GetLoginRateLimit(ctx, unblocked.Key); ok {
		t.Fatal("key still present after delete")
	}
}
