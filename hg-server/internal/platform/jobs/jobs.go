// Package jobs runs periodic background work. Each run holds a PostgreSQL
// advisory lock named after the job, so when hg-server runs more than one
// process, only one of them runs a given job at a time.
//
// Jobs run one after another from a single goroutine. A run holds one pool
// connection for its lock while its work uses others, so running jobs in
// parallel could take every connection and deadlock the pool.
package jobs

import (
	"context"
	"hash/fnv"
	"log/slog"
	"time"

	"github.com/elug3/h-global/hg-server/internal/platform/db"
)

// Job is one periodic task. Run must be safe to repeat.
type Job struct {
	Name  string
	Every time.Duration
	Run   func(ctx context.Context) error
}

// Runner runs jobs until its context ends.
type Runner struct {
	DB   *db.DB
	Jobs []Job
	// Tick is how often the scheduler checks for due jobs (default 1s).
	Tick time.Duration
}

// Start launches the scheduler and returns immediately.
func (r *Runner) Start(ctx context.Context) {
	go r.loop(ctx)
}

func (r *Runner) loop(ctx context.Context) {
	tick := r.Tick
	if tick == 0 {
		tick = time.Second
	}
	next := make([]time.Time, len(r.Jobs)) // zero: due now
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		now := time.Now()
		for i, j := range r.Jobs {
			if ctx.Err() != nil {
				return
			}
			if !now.Before(next[i]) {
				r.RunOnce(ctx, j)
				next[i] = time.Now().Add(j.Every)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// RunAll runs every job once, in order (used by tests and maintenance).
func (r *Runner) RunAll(ctx context.Context) {
	for _, j := range r.Jobs {
		r.RunOnce(ctx, j)
	}
}

// RunOnce runs j now if no other process holds its lock.
func (r *Runner) RunOnce(ctx context.Context, j Job) {
	conn, err := r.DB.Pool.Acquire(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("job: acquire", "job", j.Name, "err", err)
		}
		return
	}
	defer conn.Release()
	key := lockKey(j.Name)
	var got bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&got); err != nil || !got {
		return
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, key) //nolint:errcheck
	if err := j.Run(ctx); err != nil && ctx.Err() == nil {
		slog.Error("job failed", "job", j.Name, "err", err)
	}
}

func lockKey(name string) int64 {
	h := fnv.New64a()
	h.Write([]byte("hg-job:" + name))
	return int64(h.Sum64())
}
