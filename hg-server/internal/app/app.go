// Package app wires every module into one HTTP handler and one set of jobs.
// cmd/hg-server and the end-to-end test both build the server through New.
package app

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/account"
	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/checkout"
	"github.com/elug3/h-global/hg-server/internal/live"
	"github.com/elug3/h-global/hg-server/internal/order"
	"github.com/elug3/h-global/hg-server/internal/payment"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/events"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
	"github.com/elug3/h-global/hg-server/internal/platform/jobs"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
	"github.com/elug3/h-global/hg-server/internal/relay"
)

// App is the wired server.
type App struct {
	Config   config.Config
	DB       *db.DB
	Bus      *events.Bus
	Storage  storage.Storage
	Sessions *auth.Sessions

	Account  *account.Service
	Catalog  *catalog.Service
	Live     *live.Service
	Orders   *order.Service
	Payments *payment.Service
	Checkout *checkout.Service
	Relay    *relay.Manager // runs only when Config.Relay.Enabled; see cmd/hg-server

	Handler http.Handler
	Routes  []string // "METHOD /path" for every route, for the OpenAPI check
	Jobs    []jobs.Job
}

// New builds the app on an open, migrated database.
func New(cfg config.Config, d *db.DB) *App {
	a := &App{Config: cfg, DB: d, Bus: events.NewBus(d), Storage: storage.Local{Dir: cfg.StorageDir}}

	a.Account = &account.Service{DB: d, Store: account.Postgres{DB: d}, BcryptCost: cfg.BcryptCost}
	a.Sessions = &auth.Sessions{DB: d, CookieSecure: cfg.CookieSecure, StaffTTL: cfg.StaffSessionTTL,
		CustomerTTL: cfg.CustomerSessionTTL, StaffRole: a.Account.StaffRole}
	a.Account.OnStaffDeactivated = func(ctx context.Context, id uuid.UUID) error {
		return a.Sessions.DestroyAll(ctx, auth.KindStaff, id)
	}

	a.Catalog = &catalog.Service{DB: d, Store: catalog.Postgres{DB: d}, MediaBaseURL: cfg.MediaBaseURL}
	a.Live = &live.Service{DB: d, Store: live.Postgres{DB: d}, Catalog: a.Catalog, Bus: a.Bus,
		Storage: a.Storage, StreamDelay: cfg.StreamDelay}
	a.Catalog.OnChange(a.Live.ProductChanged)

	a.Payments = &payment.Service{DB: d, Provider: payment.Fake{Secret: cfg.FakePaymentSecret}}
	a.Orders = &order.Service{DB: d, Store: order.Postgres{DB: d}, Catalog: a.Catalog, Payments: a.Payments,
		Bus: a.Bus, MediaURL: a.Catalog.MediaURL}
	a.Payments.Orders = a.Orders
	a.Checkout = &checkout.Service{DB: d, Catalog: a.Catalog, Orders: a.Orders, Payments: a.Payments,
		HoldTTL: cfg.HoldTTL, VirtualAcctTTL: cfg.VirtualAcctTTL, ShippingFeeWon: cfg.ShippingFeeWon}

	a.Relay = &relay.Manager{Config: cfg.Relay, Live: a.Live}

	a.routes()
	a.jobs()
	return a
}

func (a *App) routes() {
	mux := http.NewServeMux()
	root := httpx.NewRouter(mux)

	api := root.Group("/api", a.Sessions.Load(auth.KindCustomer), httpx.CheckOrigin(a.Config.AllowedOrigin))
	studio := root.Group("/studio-api", a.Sessions.Load(auth.KindStaff), httpx.CheckOrigin(a.Config.AllowedOrigin))
	hooks := root.Group("/hooks")

	accounts := &account.Handler{Svc: a.Account, Sessions: a.Sessions}
	accounts.RegisterStudio(studio)
	accounts.RegisterAPI(api)

	products := &catalog.Handler{Svc: a.Catalog, Storage: a.Storage}
	products.RegisterStudio(studio)
	products.RegisterAPI(api)

	shows := &live.Handler{Svc: a.Live}
	shows.RegisterStudio(studio)
	shows.RegisterAPI(api)

	orders := &order.Handler{Svc: a.Orders, Storage: a.Storage}
	orders.RegisterStudio(studio)
	orders.RegisterAPI(api)

	(&checkout.Handler{Svc: a.Checkout}).RegisterAPI(api)
	(&relay.Handler{Manager: a.Relay}).RegisterStudio(studio)

	pay := &payment.Handler{Svc: a.Payments}
	pay.RegisterHooks(hooks)
	if a.Config.DevRoutes {
		pay.RegisterDev(api)
	}

	root.Handle("GET", "/media/{path...}", a.media)
	root.Handle("GET", "/healthz", a.health)

	a.Routes = root.Routes()
	a.Handler = httpx.Recover(httpx.Log(mux))
}

// media serves public files (product photos and product inspection media).
// Keys contain a UUIDv7 and never change, so they can be cached for a year.
func (a *App) media(w http.ResponseWriter, r *http.Request) error {
	key := storage.PublicPrefix + r.PathValue("path")
	f, err := a.Storage.Open(r.Context(), key)
	if err != nil {
		return apperr.NotFound("media")
	}
	defer f.Close()
	w.Header().Set("Content-Type", storage.ContentType(key))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, err = io.Copy(w, f)
	return err
}

func (a *App) health(w http.ResponseWriter, r *http.Request) error {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.DB.Pool.Ping(ctx); err != nil {
		return httpx.JSON(w, http.StatusServiceUnavailable, map[string]string{"status": "db_unavailable"})
	}
	return httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *App) jobs() {
	a.Jobs = []jobs.Job{
		{Name: "checkout-expiry", Every: 30 * time.Second, Run: func(ctx context.Context) error {
			_, err := a.Checkout.ExpireOpen(ctx)
			return err
		}},
		{Name: "unpaid-orders", Every: time.Minute, Run: func(ctx context.Context) error {
			_, err := a.Orders.ExpireUnpaid(ctx)
			return err
		}},
		{Name: "auto-hide", Every: time.Minute, Run: func(ctx context.Context) error {
			_, err := a.Live.HideDue(ctx)
			return err
		}},
		{Name: "prune", Every: time.Hour, Run: func(ctx context.Context) error {
			if err := a.Bus.Prune(ctx); err != nil {
				return err
			}
			return a.Sessions.Prune(ctx)
		}},
	}
}
