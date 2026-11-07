// Package httpx holds the HTTP plumbing shared by every module: JSON in and
// out, error mapping, a small router that records its routes, and middleware.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
)

// MaxJSONBody limits JSON request bodies.
const MaxJSONBody = 1 << 20

// HandlerFunc is an http handler that returns an error instead of writing it.
type HandlerFunc func(w http.ResponseWriter, r *http.Request) error

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// JSON writes v with status.
func JSON(w http.ResponseWriter, status int, v any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return nil
	}
	return json.NewEncoder(w).Encode(v)
}

// NoContent writes 204.
func NoContent(w http.ResponseWriter) error {
	w.WriteHeader(http.StatusNoContent)
	return nil
}

type errorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Details any    `json:"details,omitempty"`
}

// WriteError maps err to a status and a JSON body. Unclassified errors are
// logged and answered as 500 without their text.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	if e, ok := apperr.As(err); ok {
		status := map[apperr.Kind]int{
			apperr.KindInvalid:      http.StatusBadRequest,
			apperr.KindUnauthorized: http.StatusUnauthorized,
			apperr.KindForbidden:    http.StatusForbidden,
			apperr.KindNotFound:     http.StatusNotFound,
			apperr.KindConflict:     http.StatusConflict,
			apperr.KindUnavailable:  http.StatusBadGateway,
		}[e.Kind]
		if status == 0 {
			status = http.StatusInternalServerError
		}
		_ = JSON(w, status, errorBody{Error: e.Code, Message: e.Message, Details: e.Details})
		return
	}
	slog.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	_ = JSON(w, http.StatusInternalServerError, errorBody{Error: "internal_error"})
}

// Decode reads a JSON body into dst, rejecting unknown fields.
func Decode(r *http.Request, dst any) error {
	ct := r.Header.Get("Content-Type")
	if ct != "" && !strings.HasPrefix(ct, "application/json") {
		return apperr.Invalid("unsupported_media_type", "expected application/json")
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, MaxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return apperr.Invalid("invalid_json", "request body is empty")
		}
		return apperr.Invalid("invalid_json", "%s", err.Error())
	}
	return nil
}

// PathID parses a UUID path value.
func PathID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		return uuid.Nil, apperr.Invalid("invalid_id", "%s is not a valid id", name)
	}
	return id, nil
}

// QueryInt reads an integer query parameter with a default and bounds.
func QueryInt(r *http.Request, name string, def, min, max int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min || n > max {
		return 0, apperr.Invalid("invalid_query", "%s must be between %d and %d", name, min, max)
	}
	return n, nil
}

// Router registers handlers on a ServeMux under a prefix with middleware and
// records every route, so tests can compare them with api/openapi.yaml.
type Router struct {
	mux    *http.ServeMux
	prefix string
	mw     []Middleware
	routes *[]string
}

// NewRouter returns a root router.
func NewRouter(mux *http.ServeMux) *Router {
	return &Router{mux: mux, routes: &[]string{}}
}

// Group returns a child router with an added prefix and middleware.
func (rt *Router) Group(prefix string, mw ...Middleware) *Router {
	all := append(append([]Middleware{}, rt.mw...), mw...)
	return &Router{mux: rt.mux, prefix: rt.prefix + prefix, mw: all, routes: rt.routes}
}

// With returns a router with extra middleware and the same prefix.
func (rt *Router) With(mw ...Middleware) *Router { return rt.Group("", mw...) }

// Handle registers fn for method and path (Go 1.22 patterns, e.g. "/shows/{id}").
func (rt *Router) Handle(method, path string, fn HandlerFunc) {
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			WriteError(w, r, err)
		}
	})
	for i := len(rt.mw) - 1; i >= 0; i-- {
		h = rt.mw[i](h)
	}
	full := rt.prefix + path
	rt.mux.Handle(method+" "+full, h)
	*rt.routes = append(*rt.routes, method+" "+full)
}

// Routes lists every registered "METHOD /path".
func (rt *Router) Routes() []string { return append([]string(nil), *rt.routes...) }
