package db

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

func (s *Store) DeleteClientTrafficStatesForTest(ctx context.Context, statsKey string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM traffic_states WHERE scope_type='client' AND scope_key=?`, strings.TrimSpace(statsKey))
	return err
}

func (s *Store) ExecForTest(ctx context.Context, query string, args ...interface{}) error {
	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

type TableColumnForTest struct {
	Name string
	Type string
}

var tableIdentifierForTest = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (s *Store) TableColumnsForTest(t *testing.T, table string) []TableColumnForTest {
	t.Helper()
	if !tableIdentifierForTest.MatchString(table) {
		t.Fatalf("invalid table identifier %q", table)
	}
	rows, err := s.db.QueryContext(context.Background(), `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("table info %s: %v", table, err)
	}
	defer rows.Close()
	columns := []TableColumnForTest{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("scan table info %s: %v", table, err)
		}
		columns = append(columns, TableColumnForTest{Name: name, Type: typ})
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table info rows %s: %v", table, err)
	}
	return columns
}
