package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

type BlacklistedSession struct {
	ID        int64  `json:"id"`
	TokenHash string `json:"token_hash"`
	CreatedAt string `json:"created_at"`
	LastUsed  string `json:"last_used"`
	ExpiresAt string `json:"expires_at"`
	Revoked   bool   `json:"revoked"`
}

var sessionMaxAge = 7 * 24 * time.Hour // 168 hours

// AddToBlacklist inserts a token hash into the token_blacklist table or
// updates it if it already exists (e.g. marks as revoked on logout).
// Used both for initial session tracking (revoked=0) and for revocations (revoked=1).
func (s *Store) AddToBlacklist(ctx context.Context, tokenHash string, expiresAt time.Time, revoked bool) error {
	revokedInt := 0
	if revoked {
		revokedInt = 1
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT INTO token_blacklist (token_hash, created_at, last_used, expires_at, revoked) VALUES (?, ?, ?, ?, ?)
ON CONFLICT(token_hash) DO UPDATE SET revoked=excluded.revoked, last_used=excluded.last_used`,
		tokenHash, now, now, expiresAt.UTC().Format(time.RFC3339), revokedInt)
	return err
}

// IsBlacklisted checks if a token hash exists in the blacklist and is marked as revoked.
func (s *Store) IsBlacklisted(ctx context.Context, tokenHash string) (bool, error) {
	var revoked int
	err := s.db.QueryRowContext(ctx, `SELECT revoked FROM token_blacklist WHERE token_hash=? AND expires_at > ?`,
		tokenHash, time.Now().UTC().Format(time.RFC3339)).Scan(&revoked)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return revoked != 0, nil
}

// RecordSessionTouch updates the last_used timestamp for a session token.
func (s *Store) RecordSessionTouch(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET last_used=? WHERE token_hash=?`,
		time.Now().UTC().Format(time.RFC3339), tokenHash)
	return err
}

// RecordSessionTouchAfter updates last_used only when it is older than minAge.
// This keeps revocation checks current without turning every API request into a
// write transaction.
func (s *Store) RecordSessionTouchAfter(ctx context.Context, tokenHash string, minAge time.Duration) error {
	cutoff := time.Now().UTC().Add(-minAge).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET last_used=? WHERE token_hash=? AND last_used < ?`,
		time.Now().UTC().Format(time.RFC3339), tokenHash, cutoff)
	return err
}

func (s *Store) CleanupExpiredSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM token_blacklist WHERE expires_at < ?`, time.Now().UTC().Format(time.RFC3339))
	return err
}

// PruneActiveSessions revokes older active sessions, keeping only the newest maxActive records.
func (s *Store) PruneActiveSessions(ctx context.Context, maxActive int) error {
	if err := s.CleanupExpiredSessions(ctx); err != nil {
		return err
	}
	if maxActive < 0 {
		maxActive = 0
	}
	_, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET revoked=1 WHERE id IN (
SELECT id FROM token_blacklist WHERE revoked=0 AND expires_at > ? ORDER BY id DESC LIMIT -1 OFFSET ?
)`, time.Now().UTC().Format(time.RFC3339), maxActive)
	return err
}

// RevokeOtherSessions revokes all active sessions except the supplied token hash.
func (s *Store) RevokeOtherSessions(ctx context.Context, currentTokenHash string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET revoked=1 WHERE revoked=0 AND token_hash<>? AND expires_at > ?`,
		currentTokenHash, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ListActiveSessions returns non-revoked, non-expired sessions.
func (s *Store) ListActiveSessions(ctx context.Context) ([]BlacklistedSession, error) {
	if err := s.CleanupExpiredSessions(ctx); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, token_hash, created_at, last_used, expires_at, revoked FROM token_blacklist WHERE revoked=0 AND expires_at > ? ORDER BY id DESC`,
		time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []BlacklistedSession
	for rows.Next() {
		var s BlacklistedSession
		var revoked int
		if err := rows.Scan(&s.ID, &s.TokenHash, &s.CreatedAt, &s.LastUsed, &s.ExpiresAt, &revoked); err != nil {
			return nil, err
		}
		s.Revoked = revoked != 0
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// RevokeSession marks a session as revoked by its database ID.
func (s *Store) RevokeSession(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `UPDATE token_blacklist SET revoked=1 WHERE id=? AND revoked=0`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("active session not found: %d", id)
	}
	return nil
}
