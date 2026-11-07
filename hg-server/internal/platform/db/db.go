// Package db wraps the PostgreSQL pool. Its main job is the unit of work:
// InTx puts a transaction into the context, and every store asks Q(ctx) for
// its querier, so work from several modules commits or rolls back together.
package db

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is satisfied by both the pool and a transaction.
type Querier interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DefaultMaxConns is the pool size when the URL does not set one.
const DefaultMaxConns = 20

// DB is the shared database handle.
type DB struct {
	Pool *pgxpool.Pool
}

// Open connects and pings.
func Open(ctx context.Context, url string) (*DB, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	// pgx defaults to max(4, CPUs) connections, which is tight for SSE-heavy
	// traffic plus jobs. Use 20 unless the URL sets pool_max_conns.
	if !strings.Contains(url, "pool_max_conns") {
		cfg.MaxConns = DefaultMaxConns
	}
	return OpenConfig(ctx, cfg)
}

// OpenConfig connects with a prepared pool config.
func OpenConfig(ctx context.Context, cfg *pgxpool.Config) (*DB, error) {
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &DB{Pool: pool}, nil
}

// Close closes the pool.
func (d *DB) Close() { d.Pool.Close() }

type txKey struct{}

type txState struct {
	tx          pgx.Tx
	afterCommit []func()
}

// Q returns the transaction in ctx, or the pool when there is none.
func (d *DB) Q(ctx context.Context) Querier {
	if s, ok := ctx.Value(txKey{}).(*txState); ok {
		return s.tx
	}
	return d.Pool
}

// InTx runs fn in a transaction. If ctx already carries one, fn joins it, so
// a service can call another service's transactional method safely. Hooks
// registered with AfterCommit run only after the outermost commit succeeds.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if _, ok := ctx.Value(txKey{}).(*txState); ok {
		return fn(ctx)
	}
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	state := &txState{tx: tx}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	if err = fn(context.WithValue(ctx, txKey{}, state)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	for _, f := range state.afterCommit {
		f()
	}
	return nil
}

// AfterCommit runs f after the transaction in ctx commits. Without a
// transaction it runs f immediately.
func AfterCommit(ctx context.Context, f func()) {
	if s, ok := ctx.Value(txKey{}).(*txState); ok {
		s.afterCommit = append(s.afterCommit, f)
		return
	}
	f()
}

// InTxNow reports whether ctx carries a transaction.
func InTxNow(ctx context.Context) bool {
	_, ok := ctx.Value(txKey{}).(*txState)
	return ok
}

// IsNoRows reports pgx.ErrNoRows.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ConstraintViolated returns the name of the violated constraint for unique,
// check and foreign-key violations, or "".
func ConstraintViolated(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23505", "23514", "23503":
			return pe.ConstraintName
		}
	}
	return ""
}
