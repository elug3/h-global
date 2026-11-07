package checkout

import (
	"net/http"

	"github.com/elug3/h-global/hg-server/internal/order"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
)

// Handler serves checkout routes.
type Handler struct {
	Svc *Service
}

// RegisterAPI mounts customer routes under /api.
func (h *Handler) RegisterAPI(rt *httpx.Router) {
	c := rt.With(auth.RequireCustomer)
	c.Handle("POST", "/checkout", h.start)
	c.Handle("GET", "/checkout/{id}", h.get)
	c.Handle("PUT", "/checkout/{id}/shipping", h.shipping)
	c.Handle("POST", "/checkout/{id}/pay", h.pay)
	c.Handle("DELETE", "/checkout/{id}", h.cancel)
	c.Handle("POST", "/orders/{id}/pay", h.retry)
}

func me(r *http.Request) auth.Principal { return auth.MustPrincipal(r.Context()) }

func (h *Handler) start(w http.ResponseWriter, r *http.Request) error {
	var in struct {
		Lines []LineInput `json:"lines"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	c, err := h.Svc.Start(r.Context(), me(r).ID, in.Lines)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, c)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	c, err := h.Svc.Get(r.Context(), id, me(r).ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) shipping(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in order.Shipping
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	c, err := h.Svc.SetShipping(r.Context(), id, me(r).ID, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) pay(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Method string `json:"method"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	res, err := h.Svc.Pay(r.Context(), id, me(r).ID, in.Method)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, res)
}

func (h *Handler) cancel(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	if err := h.Svc.Cancel(r.Context(), id, me(r).ID); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	res, err := h.Svc.RetryPayment(r.Context(), id, me(r).ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, res)
}
