package live

import (
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
	"github.com/elug3/h-global/hg-server/internal/platform/upload"
)

// Handler serves live routes.
type Handler struct {
	Svc *Service
}

// RegisterStudio mounts staff routes under /studio-api.
func (h *Handler) RegisterStudio(rt *httpx.Router) {
	cs := rt.With(auth.RequireStaff(auth.RoleCS))
	mgr := rt.With(auth.RequireStaff(auth.RoleManager))

	cs.Handle("GET", "/shows", h.listShows)
	mgr.Handle("POST", "/shows", h.createShow)
	cs.Handle("GET", "/shows/{id}", h.studioShow)
	mgr.Handle("PATCH", "/shows/{id}", h.updateShow)
	cs.Handle("POST", "/shows/{id}/start", h.start)
	cs.Handle("POST", "/shows/{id}/end", h.end)
	cs.Handle("GET", "/shows/{id}/events", h.events)

	mgr.Handle("POST", "/shows/{id}/items", h.addPreselected)
	mgr.Handle("DELETE", "/shows/{id}/items/{item_id}", h.removeItem)
	mgr.Handle("PUT", "/shows/{id}/items/order", h.reorder)

	cs.Handle("POST", "/shows/{id}/captures", h.saveCapture)
	cs.Handle("GET", "/captures/{id}", h.capture)
	cs.Handle("GET", "/captures/{id}/image", h.captureImage)
	cs.Handle("POST", "/shows/{id}/items/quick", h.quickList)

	cs.Handle("POST", "/items/{id}/pin", h.pin(true))
	cs.Handle("POST", "/items/{id}/unpin", h.pin(false))
	cs.Handle("POST", "/items/{id}/sold-out", h.soldOut)
	mgr.Handle("PATCH", "/items/{id}", h.updateItem)
}

// RegisterAPI mounts storefront routes under /api.
func (h *Handler) RegisterAPI(rt *httpx.Router) {
	rt.Handle("GET", "/shows/current", h.current)
	rt.Handle("GET", "/shows", h.endedShows)
	rt.Handle("GET", "/shows/{id}", h.publicShow)
	rt.Handle("GET", "/shows/{id}/events", h.events)
	rt.Handle("GET", "/shows/{id}/recap", h.recap)
	rt.Handle("GET", "/products/{id}/broadcasts", h.broadcasts)
}

func (h *Handler) listShows(w http.ResponseWriter, r *http.Request) error {
	var statuses []string
	if v := r.URL.Query().Get("status"); v != "" {
		statuses = strings.Split(v, ",")
	}
	limit, err := httpx.QueryInt(r, "limit", 50, 1, 200)
	if err != nil {
		return err
	}
	list, err := h.Svc.ListShows(r.Context(), statuses, limit)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(list)})
}

func (h *Handler) createShow(w http.ResponseWriter, r *http.Request) error {
	var in NewShow
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	sh, err := h.Svc.CreateShow(r.Context(), auth.MustPrincipal(r.Context()).ID, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, sh)
}

func (h *Handler) studioShow(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	v, err := h.Svc.StudioShow(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, v)
}

func (h *Handler) updateShow(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in ShowPatch
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	sh, err := h.Svc.UpdateShow(r.Context(), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, sh)
}

func (h *Handler) start(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	sh, err := h.Svc.Start(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, sh)
}

func (h *Handler) end(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	sh, err := h.Svc.End(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, sh)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	sh, err := h.Svc.Store.show(r.Context(), id, false)
	if err != nil {
		return err
	}
	if sh.Status == ShowDraft && !isStaff(r) {
		return apperr.NotFound("show")
	}
	return h.Svc.Bus.ServeSSE(w, r, Topic(id))
}

func isStaff(r *http.Request) bool {
	p, ok := auth.FromContext(r.Context())
	return ok && p.Kind == auth.KindStaff
}

func (h *Handler) addPreselected(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		ProductID uuid.UUID `json:"product_id"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	it, err := h.Svc.AddPreselected(r.Context(), id, in.ProductID, auth.MustPrincipal(r.Context()).ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, it)
}

func (h *Handler) removeItem(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	itemID, err := httpx.PathID(r, "item_id")
	if err != nil {
		return err
	}
	if err := h.Svc.RemoveItem(r.Context(), id, itemID); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h *Handler) reorder(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		ItemIDs []uuid.UUID `json:"item_ids"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	items, err := h.Svc.Reorder(r.Context(), id, in.ItemIDs)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *Handler) saveCapture(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	data, _, err := upload.Read(r, upload.MaxImageBytes)
	if err != nil {
		return err
	}
	c, err := h.Svc.SaveCapture(r.Context(), id, auth.MustPrincipal(r.Context()).ID, data)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, c)
}

func (h *Handler) capture(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	c, err := h.Svc.Capture(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) captureImage(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	c, err := h.Svc.Capture(r.Context(), id)
	if err != nil {
		return err
	}
	f, err := h.Svc.Storage.Open(r.Context(), c.ObjectKey)
	if err != nil {
		return apperr.NotFound("capture")
	}
	defer f.Close()
	w.Header().Set("Content-Type", c.ContentType)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, err = io.Copy(w, f)
	return err
}

func (h *Handler) quickList(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in QuickListInput
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	in.IdempotencyKey = strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(in.IdempotencyKey) > 100 {
		return apperr.Invalid("invalid_idempotency_key", "Idempotency-Key is up to 100 characters")
	}
	it, err := h.Svc.QuickList(r.Context(), id, auth.MustPrincipal(r.Context()).ID, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, it)
}

func (h *Handler) pin(pinned bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathID(r, "id")
		if err != nil {
			return err
		}
		it, err := h.Svc.Pin(r.Context(), id, pinned)
		if err != nil {
			return err
		}
		return httpx.JSON(w, http.StatusOK, it)
	}
}

func (h *Handler) soldOut(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	it, err := h.Svc.SoldOut(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) updateItem(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		KeepListed *bool `json:"keep_listed"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	if in.KeepListed == nil {
		return apperr.Invalid("nothing_to_change", "send keep_listed")
	}
	it, err := h.Svc.SetKeepListed(r.Context(), id, *in.KeepListed)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, it)
}

func (h *Handler) current(w http.ResponseWriter, r *http.Request) error {
	v, err := h.Svc.Current(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"show": v})
}

func (h *Handler) endedShows(w http.ResponseWriter, r *http.Request) error {
	if st := r.URL.Query().Get("status"); st != "" && st != ShowEnded {
		return apperr.Invalid("invalid_status", "only status=ended can be listed")
	}
	limit, err := httpx.QueryInt(r, "limit", 20, 1, 100)
	if err != nil {
		return err
	}
	list, err := h.Svc.ListShows(r.Context(), []string{ShowEnded}, limit)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(list)})
}

func (h *Handler) publicShow(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	v, err := h.Svc.PublicShow(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, v)
}

func (h *Handler) recap(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	v, err := h.Svc.PublicShow(r.Context(), id)
	if err != nil {
		return err
	}
	if v.Status != ShowEnded {
		return apperr.Conflict("show_not_ended", "the recap is available after the show ends")
	}
	return httpx.JSON(w, http.StatusOK, v)
}

func (h *Handler) broadcasts(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	list, err := h.Svc.Broadcasts(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
