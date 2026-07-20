// Package store is the SQLite persistence layer. It implements tree.Store
// plus the queries the daemon needs to load state at startup and serve
// profiles, repos, worktrees, events and settings.
//
// Rows are immutable snapshots in and out: callers pass whole core structs in,
// Store returns whole core structs back. Nothing here mutates a caller's
// value.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

// pragmas are applied once, right after Open, before migrations run.
var pragmas = []string{
	"PRAGMA journal_mode = WAL",
	"PRAGMA foreign_keys = ON",
	"PRAGMA busy_timeout = 5000",
	"PRAGMA synchronous = NORMAL",
}

// dbDirPerm is the permission used when creating the database's parent
// directory tree.
const dbDirPerm = 0o750

// Store is a SQLite-backed persistence layer.
//
// The database is opened with exactly one connection (SetMaxOpenConns(1)).
// Grove has a single writer: the tree actor already serializes every
// mutation behind its own mutex, so there is never write contention to
// arbitrate here. Pinning one connection makes the per-connection PRAGMAs
// below (foreign_keys, busy_timeout, synchronous) apply for the lifetime of
// the process instead of needing to be re-applied to every pooled
// connection, and avoids SQLITE_BUSY races between connections that a pool
// would otherwise invite.
type Store struct {
	db *sql.DB
}

// Open opens the SQLite database at path, creating its parent directory and
// the file itself if needed, applies pragmas, and runs any pending
// migrations. The returned Store must be closed with Close.
func Open(ctx context.Context, path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, dbDirPerm); err != nil {
			return nil, fmt.Errorf("create db directory %s: %w", dir, err)
		}
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	for _, p := range pragmas {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("apply pragma %q: %w", p, err)
		}
	}

	s := &Store{db: db}
	if err := s.migrate(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate %s: %w", path, err)
	}
	return s, nil
}

// Close closes the underlying database handle.
func (s *Store) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("close store: %w", err)
	}
	return nil
}

// inTx runs fn inside a transaction: commits on success, rolls back on error
// (fn's or Commit's). Every write method is one call to inTx, giving it the
// "one call = one transaction, atomic" property the tree actor relies on.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
