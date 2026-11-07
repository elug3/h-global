package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/elug3/h-global/hg-server/internal/app"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
	"github.com/elug3/h-global/hg-server/internal/platform/jobs"
)

// TestJobsRunOnASmallPool guards against the pool deadlock where every job
// held a connection for its lock while waiting for another one.
func TestJobsRunOnASmallPool(t *testing.T) {
	d := dbtest.NewWithMaxConns(t, 2)
	a := app.New(config.Config{StorageDir: t.TempDir(), MediaBaseURL: "/media", HoldTTL: time.Minute, BcryptCost: 4}, d)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := &jobs.Runner{DB: d, Jobs: a.Jobs, Tick: 10 * time.Millisecond}
	done := make(chan struct{})
	go func() { r.RunAll(ctx); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("jobs did not finish: pool deadlock")
	}
	// The scheduler also runs and keeps the pool usable for requests.
	r.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	pctx, pcancel := context.WithTimeout(ctx, 2*time.Second)
	defer pcancel()
	if err := d.Pool.Ping(pctx); err != nil {
		t.Fatalf("pool unusable while jobs run: %v", err)
	}
}
