package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db                        *sql.DB
	trafficCleanupMu          sync.Mutex
	nextTrafficSamplesCleanup time.Time
}

func Open(ctx context.Context, path string) (*Store, error) {
	database, err := sql.Open("sqlite", sqliteDSNWithForeignKeys(path))
	if err != nil {
		return nil, err
	}
	if err := verifyForeignKeysEnabled(ctx, database); err != nil {
		database.Close()
		return nil, err
	}
	store := &Store{db: database}
	if err := store.migrate(ctx); err != nil {
		database.Close()
		return nil, err
	}
	// Enable WAL mode for better concurrent read/write performance
	_, _ = database.ExecContext(ctx, `PRAGMA journal_mode=WAL`)
	return store, nil
}

func sqliteDSNWithForeignKeys(path string) string {
	if strings.Contains(path, "_pragma=foreign_keys") {
		return path
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "_pragma=" + url.QueryEscape("foreign_keys(1)")
}

func verifyForeignKeysEnabled(ctx context.Context, database *sql.DB) error {
	var enabled int
	if err := database.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return fmt.Errorf("sqlite foreign_keys pragma is disabled")
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%s-%s-%s-%s-%s", hex.EncodeToString(b[0:4]), hex.EncodeToString(b[4:6]), hex.EncodeToString(b[6:8]), hex.EncodeToString(b[8:10]), hex.EncodeToString(b[10:16]))
}

func randomHexToken(byteLen int) (string, error) {
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func newSecret(byteLen int) string {
	token, err := randomHexToken(byteLen)
	if err != nil {
		panic(err)
	}
	return token
}
