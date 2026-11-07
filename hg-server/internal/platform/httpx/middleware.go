package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"slices"
	"time"
)

// Recover turns a panic into a 500.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if p := recover(); p != nil {
				slog.ErrorContext(r.Context(), "panic", "panic", p, "stack", string(debug.Stack()))
				_ = JSON(w, http.StatusInternalServerError, errorBody{Error: "internal_error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

// Flush keeps SSE working through the wrapper.
func (s *statusWriter) Flush() {
	if f, ok := s.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Unwrap lets http.ResponseController reach the original writer.
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Log writes one line per request.
func Log(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		slog.InfoContext(r.Context(), "http", "method", r.Method, "path", r.URL.Path,
			"status", sw.status, "ms", time.Since(start).Milliseconds())
	})
}

// CheckOrigin rejects state-changing requests whose Origin header is present
// and not in allowed. Together with SameSite=Lax session cookies this blocks
// cross-site request forgery. Requests without Origin (server-to-server, the
// apps' own proxies) pass.
func CheckOrigin(allowed []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if o := r.Header.Get("Origin"); o != "" && !slices.Contains(allowed, o) {
					_ = JSON(w, http.StatusForbidden, errorBody{Error: "origin_not_allowed"})
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
