package db

import (
	"context"
	"strings"
)

func (s *Store) DeleteClientTrafficStatesForTest(ctx context.Context, statsKey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM traffic_states WHERE scope_type='client' AND scope_key=?`, strings.TrimSpace(statsKey))
	return err
}
