package db

import (
	"context"
	"database/sql"
	"strings"
)

const sqliteVariableChunkSize = 900

func placeholders(count int) string {
	if count <= 0 {
		return ""
	}
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func nullableInt64(value int64) interface{} {
	if value <= 0 {
		return nil
	}
	return value
}

type sqlExecer interface {
	ExecContext(context.Context, string, ...interface{}) (sql.Result, error)
}

type sqlQuerier interface {
	QueryContext(context.Context, string, ...interface{}) (*sql.Rows, error)
}
