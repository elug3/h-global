// Command hg-server runs H-Global's backend: the storefront API (/api),
// the H-Global Studio API (/studio-api), payment webhooks (/hooks), public
// media (/media) and the background jobs.
//
//	hg-server              # HTTP server and jobs in one process (default)
//	hg-server -jobs=false  # HTTP only
//	hg-server -http=false  # jobs only
//	hg-server -migrate     # apply migrations and exit
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/elug3/h-global/hg-server/internal/app"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/jobs"
	"github.com/elug3/h-global/hg-server/migrations"
)

func main() {
	runHTTP := flag.Bool("http", true, "serve HTTP")
	runJobs := flag.Bool("jobs", true, "run background jobs")
	migrateOnly := flag.Bool("migrate", false, "apply migrations and exit")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if err := run(*runHTTP, *runJobs, *migrateOnly); err != nil {
		slog.Error("hg-server stopped", "err", err)
		os.Exit(1)
	}
}

func run(runHTTP, runJobs, migrateOnly bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer d.Close()
	applied, err := d.Migrate(ctx, migrations.FS)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		slog.Info("migrations applied", "versions", applied)
	}
	if migrateOnly {
		return nil
	}

	a := app.New(cfg, d)
	if created, err := a.Account.EnsureOwner(ctx, cfg.OwnerEmail, cfg.OwnerPassword); err != nil {
		return err
	} else if created {
		slog.Info("created the first owner", "email", cfg.OwnerEmail)
	}

	// Deferred after d.Close, so it runs first: jobs stop before the pool closes.
	jobCtx, stopJobs := context.WithCancel(ctx)
	defer stopJobs()
	if runJobs {
		(&jobs.Runner{DB: d, Jobs: a.Jobs}).Start(jobCtx)
		slog.Info("jobs started", "count", len(a.Jobs))
	}
	if !runHTTP {
		<-ctx.Done()
		return nil
	}
	// The relay fills the frame buffer the Studio routes serve, so it runs in
	// the HTTP process.
	if cfg.Relay.Enabled {
		go a.Relay.Run(jobCtx)
		slog.Info("relay enabled", "listen", cfg.Relay.Listen, "youtube_push", cfg.Relay.YouTubeKey != "")
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           a.Handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		// No WriteTimeout: event streams stay open for as long as viewers watch.
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		errc <- srv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}
