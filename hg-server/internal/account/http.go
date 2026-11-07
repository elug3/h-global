package account

import (
	"net/http"

	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
)

// Handler serves account routes.
type Handler struct {
	Svc      *Service
	Sessions *auth.Sessions
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// RegisterStudio mounts staff routes under /studio-api.
func (h *Handler) RegisterStudio(rt *httpx.Router) {
	rt.Handle("POST", "/auth/login", h.staffLogin)
	rt.Handle("POST", "/auth/logout", h.staffLogout)
	rt.With(auth.RequireStaff(auth.RoleCS)).Handle("GET", "/auth/me", h.staffMe)

	owner := rt.With(auth.RequireStaff(auth.RoleOwner))
	owner.Handle("GET", "/staff", h.listStaff)
	owner.Handle("POST", "/staff", h.createStaff)
	owner.Handle("PATCH", "/staff/{id}", h.updateStaff)
}

// RegisterAPI mounts customer routes under /api.
func (h *Handler) RegisterAPI(rt *httpx.Router) {
	rt.Handle("POST", "/auth/register", h.register)
	rt.Handle("POST", "/auth/login", h.customerLogin)
	rt.Handle("POST", "/auth/logout", h.customerLogout)
	rt.With(auth.RequireCustomer).Handle("GET", "/me", h.customerMe)
}

func (h *Handler) staffLogin(w http.ResponseWriter, r *http.Request) error {
	var in loginRequest
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	st, err := h.Svc.StaffLogin(r.Context(), in.Email, in.Password)
	if err != nil {
		return err
	}
	if err := h.Sessions.Create(r.Context(), w, auth.KindStaff, st.ID); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) staffLogout(w http.ResponseWriter, r *http.Request) error {
	if err := h.Sessions.Destroy(r.Context(), w, r, auth.KindStaff); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h *Handler) staffMe(w http.ResponseWriter, r *http.Request) error {
	st, err := h.Svc.StaffByID(r.Context(), auth.MustPrincipal(r.Context()).ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) listStaff(w http.ResponseWriter, r *http.Request) error {
	list, err := h.Svc.ListStaff(r.Context())
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": nonNil(list)})
}

func (h *Handler) createStaff(w http.ResponseWriter, r *http.Request) error {
	var in NewStaff
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	st, err := h.Svc.CreateStaff(r.Context(), in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, st)
}

func (h *Handler) updateStaff(w http.ResponseWriter, r *http.Request) error {
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return err
	}
	var in StaffPatch
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	st, err := h.Svc.UpdateStaff(r.Context(), id, in)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, st)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) error {
	var in NewCustomer
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	c, err := h.Svc.Register(r.Context(), in)
	if err != nil {
		return err
	}
	if err := h.Sessions.Create(r.Context(), w, auth.KindCustomer, c.ID); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, c)
}

func (h *Handler) customerLogin(w http.ResponseWriter, r *http.Request) error {
	var in loginRequest
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	c, err := h.Svc.CustomerLogin(r.Context(), in.Email, in.Password)
	if err != nil {
		return err
	}
	if err := h.Sessions.Create(r.Context(), w, auth.KindCustomer, c.ID); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func (h *Handler) customerLogout(w http.ResponseWriter, r *http.Request) error {
	if err := h.Sessions.Destroy(r.Context(), w, r, auth.KindCustomer); err != nil {
		return err
	}
	return httpx.NoContent(w)
}

func (h *Handler) customerMe(w http.ResponseWriter, r *http.Request) error {
	c, err := h.Svc.CustomerByID(r.Context(), auth.MustPrincipal(r.Context()).ID)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, c)
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
