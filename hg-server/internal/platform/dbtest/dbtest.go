// Package dbtest gives each test its own migrated schema in a real
// PostgreSQL 18 database. Tests are skipped when HG_TEST_DATABASE_URL is not
// set, so `go test ./...` still runs the pure unit tests anywhere.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/migrations"
)

// New returns a DB whose search_path points at a fresh, migrated schema that
// is dropped when the test ends.
func New(t testing.TB) *db.DB { return NewWithMaxConns(t, 8) }

// NewWithMaxConns is New with a chosen pool size (to test pool exhaustion).
func NewWithMaxConns(t testing.TB, maxConns int32) *db.DB {
	t.Helper()
	url := os.Getenv("HG_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("HG_TEST_DATABASE_URL not set; skipping database test")
	}
	ctx := context.Background()

	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(b)

	admin, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.MaxConns = maxConns
	d, err := db.OpenConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := d.Migrate(ctx, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		d.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})
	return d
}
