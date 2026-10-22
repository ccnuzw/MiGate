package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// LoginRateLimit is the persisted login rate-limit state for one limiter key
// (for example "ip:192.0.2.1"). BlockedAt is zero while the key is not blocked.
type LoginRateLimit struct {
	Key       string
	Failures  int
	BlockedAt time.Time
	UpdatedAt time.Time
}

// GetLoginRateLimit returns the stored state for key; ok is false if there is none.
func (s *Store) GetLoginRateLimit(ctx context.Context, key string) (LoginRateLimit, bool, error) {
	var failures int
	var blockedAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `SELECT failures, blocked_at, updated_at FROM login_rate_limits WHERE key=?`, key).Scan(&failures, &blockedAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return LoginRateLimit{}, false, nil
	}
	if err != nil {
		return LoginRateLimit{}, false, err
	}
	return LoginRateLimit{
		Key:       key,
		Failures:  failures,
		BlockedAt: parseLoginRateLimitTime(blockedAt),
		UpdatedAt: parseLoginRateLimitTime(updatedAt),
	}, true, nil
}

// SaveLoginRateLimit stores rec, replacing any existing row for rec.Key.
func (s *Store) SaveLoginRateLimit(ctx context.Context, rec LoginRateLimit) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO login_rate_limits (key, failures, blocked_at, updated_at) VALUES (?, ?, ?, ?)
ON CONFLICT(key) DO UPDATE SET failures=excluded.failures, blocked_at=excluded.blocked_at, updated_at=excluded.updated_at`,
		rec.Key, rec.Failures, formatLoginRateLimitTime(rec.BlockedAt), formatLoginRateLimitTime(rec.UpdatedAt))
	return err
}

// DeleteLoginRateLimit removes the stored state for key, if any.
func (s *Store) DeleteLoginRateLimit(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM login_rate_limits WHERE key=?`, key)
	return err
}

// PruneLoginRateLimits deletes rows last updated before cutoff and returns how many were removed.
func (s *Store) PruneLoginRateLimits(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM login_rate_limits WHERE updated_at < ?`, cutoff.UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func formatLoginRateLimitTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// parseLoginRateLimitTime returns the zero time for empty or malformed values,
// which the limiter treats as "not blocked" and prune treats as stale.
func parseLoginRateLimitTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return t
}
