package order

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
	"github.com/elug3/h-global/hg-server/internal/platform/upload"
)

// Handler serves order routes.
type Handler struct {
	Svc     *Service
	Storage storage.Storage
}

// RegisterStudio mounts staff routes under /studio-api.
func (h *Handler) RegisterStudio(rt *httpx.Router) {
	cs := rt.With(auth.RequireStaff(auth.RoleCS))
	mgr := rt.With(auth.RequireStaff(auth.RoleManager))
	cs.Handle("GET", "/orders", h.list)
	cs.Handle("GET", "/orders/counts", h.counts)
	cs.Handle("GET", "/shows/{id}/sales", h.showSales)
	cs.Handle("GET", "/orders/events", h.events)
	cs.Handle("GET", "/orders/{id}", h.get)
	cs.Handle("GET", "/orders/{id}/media/{media_id}", h.media(false))
	cs.Handle("POST", "/orders/{id}/inspection", h.addInspection)
	cs.Handle("POST", "/orders/{id}/inspection/complete", h.completeInspection)
	cs.Handle("POST", "/orders/{id}/customs-check", h.customsCheck)
	cs.Handle("POST", "/orders/{id}/ship", h.ship)
	cs.Handle("POST", "/orders/{id}/deliver", h.deliver)
	cs.Handle("POST", "/orders/{id}/complete", h.staffComplete)
	mgr.Handle("POST", "/orders/{id}/cancel", h.staffCancel)
}

// RegisterAPI mounts customer routes under /api.
func (h *Handler) RegisterAPI(rt *httpx.Router) {
	c := rt.With(auth.RequireCustomer)
	c.Handle("GET", "/orders", h.customerList)
	c.Handle("GET", "/orders/{id}", h.customerGet)
	c.Handle("GET", "/orders/{id}/media/{media_id}", h.media(true))
	c.Handle("POST", "/orders/{id}/cancel", h.customerCancel)
	c.Handle("POST", "/orders/{id}/receipt", h.confirmReceipt)
	c.Handle("PUT", "/orders/{id}/customs-code", h.customsCode)
}

func staffActor(r *http.Request) (Actor, uuid.UUID) {
	id := auth.MustPrincipal(r.Context()).ID
	return Actor{Kind: "staff", ID: &id}, id
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) error {
	var statuses []string
	if v := r.URL.Query().Get("status"); v != "" {
		statuses = strings.Split(v, ",")
	}
	limit, err := httpx.QueryInt(r, "limit", 50, 1, 200)
	if err != nil {
		return err
	}
	offset, err := httpx.QueryInt(r, "offset", 0, 0, 1_000_000)
	if err != nil {
		return err
	}
	q := r.URL.Query().Get("q")
	if len([]rune(q)) > 100 {
		return apperr.Invalid("invalid_query", "q is up to 100 characters")
	}
	list, err := h.Svc.List(r.Context(), Filter{Statuses: statuses, Q: q, Limit: limit, Offset: offset})
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(list)})
}

func (h *Handler) counts(w http.ResponseWriter, r *http.Request) error {
	c, err := h.Svc.Counts(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"counts": c})
}

func (h *Handler) showSales(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	s, err := h.Svc.SalesForShow(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, s)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) error {
	return h.Svc.Bus.ServeSSE(w, r, StaffTopic)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	o, err := h.Svc.Get(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) media(customer bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathID(r, "id")
		if err != nil {
			return err
		}
		mid, err := httpx.PathID(r, "media_id")
		if err != nil {
			return err
		}
		var cust *uuid.UUID
		if customer {
			c := auth.MustPrincipal(r.Context()).ID
			cust = &c
		}
		m, err := h.Svc.MediaKey(r.Context(), id, mid, cust)
		if err != nil {
			return err
		}
		f, err := h.Storage.Open(r.Context(), m.ObjectKey)
		if err != nil {
			return apperr.NotFound("media")
		}
		defer f.Close()
		w.Header().Set("Content-Type", m.ContentType)
		w.Header().Set("Cache-Control", "private, max-age=3600")
		_, err = io.Copy(w, f)
		return err
	}
}

func (h *Handler) addInspection(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	_, staff := staffActor(r)
	data, fields, err := upload.Read(r, upload.MaxVideoBytes)
	if err != nil {
		return err
	}
	ctx := r.Context()
	var f upload.File
	kind := fields["kind"]
	switch kind {
	case "", "photo":
		kind = "photo"
		if len(data) > upload.MaxImageBytes {
			return apperr.Invalid("file_too_large", "images are up to %d MB", upload.MaxImageBytes>>20)
		}
		f, err = upload.StoreImage(ctx, h.Storage, storage.OrderPrefix, data, nil)
	case "video":
		f, err = upload.StoreVideo(ctx, h.Storage, storage.OrderPrefix, data)
	default:
		return apperr.Invalid("invalid_media_kind", "kind must be photo or video")
	}
	if err != nil {
		return err
	}
	o, err := h.Svc.AddInspectionMedia(ctx, id, staff, Media{ID: ids.New(), Kind: kind, ContentType: f.ContentType, ObjectKey: f.Key})
	if err != nil {
		_ = h.Storage.Delete(context.WithoutCancel(ctx), f.Key)
		return err
	}
	return httpx.JSON(w, http.StatusCreated, o)
}

type noteBody struct {
	Note string `json:"note"`
}

func decodeOptional(r *http.Request, dst any) error {
	if r.ContentLength == 0 {
		return nil
	}
	return httpx.Decode(r, dst)
}

func (h *Handler) completeInspection(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in noteBody
	if err := decodeOptional(r, &in); err != nil {
		return err
	}
	_, staff := staffActor(r)
	o, err := h.Svc.CompleteInspection(r.Context(), id, staff, in.Note)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) customsCheck(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	_, staff := staffActor(r)
	o, err := h.Svc.CheckCustoms(r.Context(), id, staff)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) ship(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		Carrier        string `json:"carrier"`
		TrackingNumber string `json:"tracking_number"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	_, staff := staffActor(r)
	o, err := h.Svc.Ship(r.Context(), id, staff, in.Carrier, in.TrackingNumber)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) deliver(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	_, staff := staffActor(r)
	o, err := h.Svc.Deliver(r.Context(), id, staff)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) staffComplete(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	actor, _ := staffActor(r)
	o, err := h.Svc.Complete(r.Context(), id, actor)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

type cancelBody struct {
	Reason string `json:"reason"`
}

func (h *Handler) staffCancel(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in cancelBody
	if err := decodeOptional(r, &in); err != nil {
		return err
	}
	actor, _ := staffActor(r)
	o, err := h.Svc.Cancel(r.Context(), id, actor, strings.TrimSpace(in.Reason))
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func customer(r *http.Request) (Actor, uuid.UUID) {
	id := auth.MustPrincipal(r.Context()).ID
	return Actor{Kind: "customer", ID: &id}, id
}

func (h *Handler) customerList(w http.ResponseWriter, r *http.Request) error {
	_, cid := customer(r)
	limit, err := httpx.QueryInt(r, "limit", 50, 1, 100)
	if err != nil {
		return err
	}
	list, err := h.Svc.ListForCustomer(r.Context(), cid, limit)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(list)})
}

func (h *Handler) customerGet(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	_, cid := customer(r)
	o, err := h.Svc.GetForCustomer(r.Context(), id, cid)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) customerCancel(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in cancelBody
	if err := decodeOptional(r, &in); err != nil {
		return err
	}
	actor, cid := customer(r)
	if _, err := h.Svc.Cancel(r.Context(), id, actor, strings.TrimSpace(in.Reason)); err != nil {
		return err
	}
	o, err := h.Svc.GetForCustomer(r.Context(), id, cid)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) confirmReceipt(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	actor, cid := customer(r)
	if _, err := h.Svc.Complete(r.Context(), id, actor); err != nil {
		return err
	}
	o, err := h.Svc.GetForCustomer(r.Context(), id, cid)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func (h *Handler) customsCode(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in struct {
		PCCC string `json:"pccc"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	_, cid := customer(r)
	o, err := h.Svc.UpdateCustomsCode(r.Context(), id, cid, in.PCCC)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, o)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
