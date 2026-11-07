package catalog

import (
	"context"
	"net/http"
	"strings"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
	"github.com/elug3/h-global/hg-server/internal/platform/upload"
)

// Handler serves catalog routes.
type Handler struct {
	Svc     *Service
	Storage storage.Storage
}

// RegisterStudio mounts staff routes under /studio-api.
func (h *Handler) RegisterStudio(rt *httpx.Router) {
	cs := rt.With(auth.RequireStaff(auth.RoleCS))
	cs.Handle("GET", "/products", h.list(false))
	cs.Handle("POST", "/products", h.create)
	cs.Handle("GET", "/products/{id}", h.get(false))
	cs.Handle("PATCH", "/products/{id}", h.update)
	cs.Handle("PATCH", "/products/{id}/variants/{variant_id}", h.updateVariant)
	cs.Handle("POST", "/products/{id}/sold-out", h.soldOut)
	cs.Handle("POST", "/products/{id}/media", h.addMedia)
	cs.Handle("DELETE", "/products/{id}/media/{media_id}", h.deleteMedia)
	cs.Handle("GET", "/catalog/suggestions", h.suggestions)
}

// RegisterAPI mounts storefront routes under /api.
func (h *Handler) RegisterAPI(rt *httpx.Router) {
	rt.Handle("GET", "/products", h.list(true))
	rt.Handle("GET", "/products/{id}", h.get(true))
}

func (h *Handler) list(public bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		limit, err := httpx.QueryInt(r, "limit", 50, 1, 100)
		if err != nil {
			return err
		}
		offset, err := httpx.QueryInt(r, "offset", 0, 0, 1_000_000)
		if err != nil {
			return err
		}
		f := Filter{Query: strings.TrimSpace(r.URL.Query().Get("q")), Limit: limit, Offset: offset}
		if public {
			f.Statuses = []string{StatusLive}
		} else {
			if st := r.URL.Query().Get("status"); st != "" {
				f.Statuses = strings.Split(st, ",")
			}
			f.Incomplete = r.URL.Query().Get("incomplete") == "true"
		}
		list, err := h.Svc.List(r.Context(), f)
		if err != nil {
			return err
		}
		if !public {
			return httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
		}
		out := make([]PublicProduct, len(list))
		for i, p := range list {
			out[i] = p.Public()
		}
		return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
	}
}

func (h *Handler) get(public bool) httpx.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) error {
		id, err := httpx.PathID(r, "id")
		if err != nil {
			return err
		}
		p, err := h.Svc.Get(r.Context(), id)
		if err != nil {
			return err
		}
		if !public {
			return httpx.JSON(w, http.StatusOK, p)
		}
		// Hidden products keep their page (for past orders and shared links)
		// but cannot be bought; drafts and archived ones are not public.
		if p.Status != StatusLive && p.Status != StatusHidden {
			return apperr.NotFound("product")
		}
		return httpx.JSON(w, http.StatusOK, p.Public())
	}
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) error {
	var in NewProduct
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	staff := auth.MustPrincipal(r.Context()).ID
	in.Source, in.CreatedBy = SourceManual, &staff
	p, err := h.Svc.Create(r.Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, p)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in ProductPatch
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	p, err := h.Svc.Update(r.Context(), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) updateVariant(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	vid, err := httpx.PathID(r, "variant_id")
	if err != nil {
		return err
	}
	var in VariantPatch
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	p, err := h.Svc.UpdateVariant(r.Context(), id, vid, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) soldOut(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	p, err := h.Svc.MarkSoldOut(r.Context(), id)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, p)
}

func (h *Handler) addMedia(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	data, fields, err := upload.Read(r, upload.MaxVideoBytes)
	if err != nil {
		return err
	}
	kind := fields["kind"]
	if kind == "" {
		kind = MediaPhoto
	}
	ctx := r.Context()
	var f upload.File
	switch kind {
	case MediaPhoto, MediaInspectionPhoto:
		if len(data) > upload.MaxImageBytes {
			return apperr.Invalid("file_too_large", "images are up to %d MB", upload.MaxImageBytes>>20)
		}
		f, err = upload.StoreImage(ctx, h.Storage, storage.PublicPrefix, data, nil)
	case MediaInspectionVideo:
		f, err = upload.StoreVideo(ctx, h.Storage, storage.PublicPrefix, data)
	default:
		return apperr.Invalid("invalid_media_kind", "kind must be photo, inspection_photo or inspection_video")
	}
	if err != nil {
		return err
	}
	m, err := h.Svc.AddMedia(ctx, id, kind, f.Key, f.ContentType)
	if err != nil {
		_ = h.Storage.Delete(context.WithoutCancel(ctx), f.Key)
		return err
	}
	return httpx.JSON(w, http.StatusCreated, m)
}

func (h *Handler) deleteMedia(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	mid, err := httpx.PathID(r, "media_id")
	if err != nil {
		return err
	}
	ctx := r.Context()
	err = h.Svc.DB.InTx(ctx, func(ctx context.Context) error {
		key, err := h.Svc.DeleteMedia(ctx, id, mid)
		if err != nil {
			return err
		}
		db.AfterCommit(ctx, func() { _ = h.Storage.Delete(context.Background(), key) })
		return nil
	})
	if err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h *Handler) suggestions(w http.ResponseWriter, r *http.Request) error {
	list, err := h.Svc.Suggestions(r.Context(), r.URL.Query().Get("option"))
	if err != nil {
		return err
	}
	if list == nil {
		list = []Suggestion{}
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": list})
}
