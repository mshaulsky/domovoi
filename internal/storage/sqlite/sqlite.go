package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite" // the driver, registered as "sqlite"
)

// Memory is the path of a private in-memory database: -check migrates it
// instead of the configured file, and tests use it too.
const Memory = ":memory:"

// pragmas applies to every connection: WAL for readers alongside the
// writer, NORMAL sync (at most the last transaction is lost on power cut,
// the file never corrupts), foreign keys on, and a wait instead of an
// immediate "database is locked".
var pragmas = []string{
	"journal_mode(WAL)",
	"synchronous(NORMAL)",
	"foreign_keys(ON)",
	"busy_timeout(5000)",
}

//go:embed migrations/*.sql
var migrations embed.FS

// ErrNewer means the file was written by a newer binary: refuse to touch it
// rather than run old code over an unknown schema.
var ErrNewer = errors.New("sqlite: database schema is newer than this binary knows")

// Open opens the database file (or Memory), creating it when missing,
// applies the PRAGMAs and migrates the schema forward. The pool is limited to one
// connection: one writer is what SQLite has anyway, and the load is a poll
// batch a minute.
func Open(ctx context.Context, path string, log *slog.Logger) (*sql.DB, error) {
	q := url.Values{}
	for _, p := range pragmas {
		q.Add("_pragma", p)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?"+q.Encode())
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	if err := migrate(ctx, db, log); err != nil {
		db.Close()
		return nil, err
	}
	// Foreign keys are per connection; the pragma above covers new ones,
	// this confirms the driver honoured it.
	var fk int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite: read foreign_keys pragma: %w", err)
	}
	if fk != 1 {
		db.Close()
		return nil, errors.New("sqlite: foreign keys are not enforced by this driver")
	}
	return db, nil
}

// migrate runs the embedded migrations forward and refuses a database
// whose version this binary does not know.
func migrate(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("sqlite: migrations: %w", err)
	}
	p, err := goose.NewProvider(goose.DialectSQLite3, db, fsys)
	if err != nil {
		return fmt.Errorf("sqlite: migration provider: %w", err)
	}
	have, err := p.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: read schema version: %w", err)
	}
	var known int64
	for _, s := range p.ListSources() {
		known = max(known, s.Version)
	}
	if have > known {
		return fmt.Errorf("%w: file %d, binary %d", ErrNewer, have, known)
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("sqlite: migrate: %w", err)
	}
	for _, r := range results {
		log.Info("schema migrated", "version", r.Source.Version, "file", r.Source.Path, "took", r.Duration)
	}
	return nil
}
