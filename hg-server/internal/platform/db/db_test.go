package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

func TestMigrationsAndUUIDs(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()

	var version string
	if err := d.Pool.QueryRow(ctx, `SHOW server_version_num`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < "180000" {
		t.Fatalf("PostgreSQL 18 required, got %s", version)
	}

	id := ids.New()
	var got uuid.UUID
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO staff (id, email, name, role, password_hash) VALUES ($1, 'a@x', 'A', 'cs', 'h') RETURNING id`,
		id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != id {
		t.Fatalf("uuid round trip: %s != %s", got, id)
	}
	// Column default uses PostgreSQL 18's uuidv7().
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO staff (email, name, role, password_hash) VALUES ('b@x', 'B', 'cs', 'h') RETURNING id`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got.Version() != 7 {
		t.Fatalf("default id version %d, want 7", got.Version())
	}
}

func TestVirtualAvailableColumn(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	var pid uuid.UUID
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO products (title, price_won, status, source) VALUES ('x', 1, 'live', 'manual') RETURNING id`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	var avail *int
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO variants (product_id, position, stock_mode, stock_qty, held_qty, sold_qty)
		 VALUES ($1, 0, 'counted', 5, 1, 2) RETURNING available_qty`, pid).Scan(&avail); err != nil {
		t.Fatal(err)
	}
	if avail == nil || *avail != 2 {
		t.Fatalf("available_qty = %v, want 2", avail)
	}
	if err := d.Pool.QueryRow(ctx,
		`INSERT INTO variants (product_id, position, stock_mode) VALUES ($1, 1, 'until_sold_out') RETURNING available_qty`, pid).Scan(&avail); err != nil {
		t.Fatal(err)
	}
	if avail != nil {
		t.Fatalf("until_sold_out available_qty = %v, want NULL", *avail)
	}
}

func TestInTxRollbackAndAfterCommit(t *testing.T) {
	d := dbtest.New(t)
	ctx := context.Background()
	ran := 0
	boom := errors.New("boom")
	err := d.InTx(ctx, func(ctx context.Context) error {
		if _, err := d.Q(ctx).Exec(ctx, `INSERT INTO staff (email, name, role, password_hash) VALUES ('c@x','C','cs','h')`); err != nil {
			return err
		}
		db.AfterCommit(ctx, func() { ran++ })
		// A nested InTx joins the outer transaction.
		return d.InTx(ctx, func(ctx context.Context) error { return boom })
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if ran != 0 {
		t.Fatal("after-commit hook ran on rollback")
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM staff`).Scan(&n)
	if n != 0 {
		t.Fatalf("rolled-back insert visible: %d rows", n)
	}
	if err := d.InTx(ctx, func(ctx context.Context) error {
		db.AfterCommit(ctx, func() { ran++ })
		return nil
	}); err != nil || ran != 1 {
		t.Fatalf("commit: err=%v ran=%d", err, ran)
	}
}
