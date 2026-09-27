// Package sqlite is the storage infrastructure and nothing else: Open
// creates or opens the database file, applies the PRAGMAs (WAL,
// synchronous=NORMAL, foreign keys, a busy timeout) and runs the embedded
// goose migrations forward. Repositories live in their own packages and
// take the connection Open returns; policy lives in the core.
package sqlite
