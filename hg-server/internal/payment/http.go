package payment

import (
	"bytes"
	"io"
	"net/http"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
)

// Handler serves the webhook and, in development, the fake provider's
// approve and fail routes.
type Handler struct {
	Svc *Service
}

// RegisterHooks mounts provider webhooks under /hooks.
func (h *Handler) RegisterHooks(rt *httpx.Router) {
	rt.Handle("POST", "/payments/{provider}", h.webhook)
}

// RegisterDev mounts /api/dev routes. Only wired when HG_DEV_ROUTES=true and
// the provider is Fake.
func (h *Handler) RegisterDev(rt *httpx.Router) {
	rt.Handle("POST", "/dev/payments/{id}/approve", h.devSend(EventSucceeded))
	rt.Handle("POST", "/dev/payments/{id}/fail", h.devSend(EventFailed))
}

func (h *Handler) webhook(w http.ResponseWriter, r *http.Request) error {
	if r.PathValue("provider") != h.Svc.Provider.Name() {
		return apperr.NotFound("provider")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if err := h.Svc.HandleWebhook(r.Context(), r.Header, body); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) devSend(typ string) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		fake, ok := h.Svc.Provider.(Fake)
		if !ok {
			return apperr.NotFound("route")
		}
		id, err := httpx.PathID(r, "id")
		if err != nil {
			return err
		}
		p, err := h.Svc.Get(r.Context(), id)
		if err != nil {
			return err
		}
		if p.ProviderRef == "" {
			return apperr.Conflict("payment_not_started", "the payment was not started with the provider")
		}
		body, sig := fake.Webhook(p, typ, "declined in development")
		hdr := http.Header{}
		hdr.Set(FakeSignatureHeader, sig)
		if err := h.Svc.HandleWebhook(r.Context(), hdr, bytes.Clone(body)); err != nil {
			return err
		}
		p, err = h.Svc.Get(r.Context(), id)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, p)
	}
}
