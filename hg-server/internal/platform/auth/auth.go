// Package auth handles sessions, passwords and access checks. Sessions are
// opaque random tokens in HttpOnly cookies; only their SHA-256 is stored.
// Staff and customers use different cookies and never share a session.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
)

// Session kinds.
const (
	KindStaff    = "staff"
	KindCustomer = "customer"
)

// Cookie names, one per app.
const (
	StaffCookie    = "hg_staff_session"
	CustomerCookie = "hg_session"
)

// Staff roles, lowest to highest.
const (
	RoleCS      = "cs"
	RoleManager = "manager"
	RoleOwner   = "owner"
)

var roleRank = map[string]int{RoleCS: 1, RoleManager: 2, RoleOwner: 3}

// ValidRole reports whether r is a staff role.
func ValidRole(r string) bool { return roleRank[r] > 0 }

// Principal is who is calling.
type Principal struct {
	Kind string
	ID   uuid.UUID
	Role string // staff only
}

// AtLeast reports whether a staff principal has role min or higher.
func (p Principal) AtLeast(min string) bool {
	return p.Kind == KindStaff && roleRank[p.Role] >= roleRank[min]
}

type principalKey struct{}

// WithPrincipal stores p in ctx.
func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// FromContext returns the caller, if signed in.
func FromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// RoleLookup returns an active staff member's role, or ok=false.
type RoleLookup func(ctx context.Context, staffID uuid.UUID) (role string, ok bool, err error)

// Sessions stores sessions in PostgreSQL.
type Sessions struct {
	DB           *db.DB
	CookieSecure bool
	StaffTTL     time.Duration
	CustomerTTL  time.Duration
	StaffRole    RoleLookup
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Create starts a session and sets its cookie.
func (s *Sessions) Create(ctx context.Context, w http.ResponseWriter, kind string, subject uuid.UUID) error {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return err
	}
	token := base64.RawURLEncoding.EncodeToString(buf)
	ttl, cookie := s.CustomerTTL, CustomerCookie
	if kind == KindStaff {
		ttl, cookie = s.StaffTTL, StaffCookie
	}
	expires := time.Now().Add(ttl)
	if _, err := s.DB.Q(ctx).Exec(ctx,
		`INSERT INTO sessions (token_hash, kind, subject_id, expires_at) VALUES ($1, $2, $3, $4)`,
		hashToken(token), kind, subject, expires); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookie, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// Destroy ends the session in r (if any) and clears its cookie.
func (s *Sessions) Destroy(ctx context.Context, w http.ResponseWriter, r *http.Request, kind string) error {
	cookie := CustomerCookie
	if kind == KindStaff {
		cookie = StaffCookie
	}
	if c, err := r.Cookie(cookie); err == nil && c.Value != "" {
		if _, err := s.DB.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1 AND kind = $2`,
			hashToken(c.Value), kind); err != nil {
			return err
		}
	}
	http.SetCookie(w, &http.Cookie{Name: cookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.CookieSecure, SameSite: http.SameSiteLaxMode})
	return nil
}

// DestroyAll ends every session of a subject (e.g. a deactivated staff member).
func (s *Sessions) DestroyAll(ctx context.Context, kind string, subject uuid.UUID) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE kind = $1 AND subject_id = $2`, kind, subject)
	return err
}

// Load is middleware that puts the caller of the given kind into the context
// when its cookie holds a live session. It never rejects; Require* does.
func (s *Sessions) Load(kind string) func(http.Handler) http.Handler {
	cookie := CustomerCookie
	if kind == KindStaff {
		cookie = StaffCookie
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(cookie)
			if err != nil || c.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			ctx := r.Context()
			var subject uuid.UUID
			err = s.DB.Pool.QueryRow(ctx,
				`SELECT subject_id FROM sessions WHERE token_hash = $1 AND kind = $2 AND expires_at > now()`,
				hashToken(c.Value), kind).Scan(&subject)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			p := Principal{Kind: kind, ID: subject}
			if kind == KindStaff {
				role, ok, err := s.StaffRole(ctx, subject)
				if err != nil || !ok {
					next.ServeHTTP(w, r)
					return
				}
				p.Role = role
			}
			next.ServeHTTP(w, r.WithContext(WithPrincipal(ctx, p)))
		})
	}
}

// RequireStaff rejects callers that are not staff with at least role min.
func RequireStaff(min string) func(http.Handler) http.Handler {
	return require(func(p Principal, ok bool) error {
		if !ok || p.Kind != KindStaff {
			return apperr.Unauthorized("not_signed_in", "staff sign-in required")
		}
		if !p.AtLeast(min) {
			return apperr.Forbidden("role_required", "requires role %s or higher", min)
		}
		return nil
	})
}

// RequireCustomer rejects callers that are not signed-in customers.
func RequireCustomer(next http.Handler) http.Handler {
	return require(func(p Principal, ok bool) error {
		if !ok || p.Kind != KindCustomer {
			return apperr.Unauthorized("not_signed_in", "sign-in required")
		}
		return nil
	})(next)
}

func require(check func(Principal, bool) error) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, ok := FromContext(r.Context())
			if err := check(p, ok); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// HashPassword hashes a password with bcrypt.
func HashPassword(pw string, cost int) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(pw), cost)
	return string(h), err
}

// CheckPassword compares a password with its hash.
func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// MustPrincipal returns the caller; handlers behind Require* can rely on it.
func MustPrincipal(ctx context.Context) Principal {
	p, _ := FromContext(ctx)
	return p
}

// Prune deletes expired sessions.
func (s *Sessions) Prune(ctx context.Context) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	return err
}
