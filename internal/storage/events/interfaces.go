package events

import (
	"context"
	"database/sql"
)

// DBTX is what *sql.DB and *sql.Tx have in common, so the repository can
// join a transaction spanning entities.
type DBTX interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
}
