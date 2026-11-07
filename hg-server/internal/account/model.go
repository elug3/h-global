// Package account holds staff (H-Global Studio users) and customers
// (storefront users). Sessions themselves live in platform/auth.
package account

import (
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
)

// Staff is an H-Global Studio user.
type Staff struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	Role         string    `json:"role"`
	Active       bool      `json:"active"`
	CreatedAt    time.Time `json:"created_at"`
	passwordHash string
}

// Customer is a storefront user.
type Customer struct {
	ID           uuid.UUID `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	CreatedAt    time.Time `json:"created_at"`
	passwordHash string
}

// NewStaff is the input for creating a staff member.
type NewStaff struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

// StaffPatch changes a staff member; nil fields are kept.
type StaffPatch struct {
	Name     *string `json:"name"`
	Role     *string `json:"role"`
	Active   *bool   `json:"active"`
	Password *string `json:"password"`
}

// NewCustomer is the input for registering.
type NewCustomer struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

func normalizeEmail(s string) (string, error) {
	s = strings.TrimSpace(s)
	a, err := mail.ParseAddress(s)
	if err != nil || a.Address != s || len(s) > 254 {
		return "", apperr.Invalid("invalid_email", "email address is not valid")
	}
	return strings.ToLower(s), nil
}

func checkPassword(pw string) error {
	if len(pw) < 8 || len(pw) > 72 {
		return apperr.Invalid("invalid_password", "password must be 8 to 72 bytes")
	}
	return nil
}

func checkName(name string, required bool) (string, error) {
	name = strings.TrimSpace(name)
	if (required && name == "") || len([]rune(name)) > 100 {
		return "", apperr.Invalid("invalid_name", "name must be 1 to 100 characters")
	}
	return name, nil
}

func (n *NewStaff) validate() error {
	var err error
	if n.Email, err = normalizeEmail(n.Email); err != nil {
		return err
	}
	if n.Name, err = checkName(n.Name, true); err != nil {
		return err
	}
	if !auth.ValidRole(n.Role) {
		return apperr.Invalid("invalid_role", "role must be owner, manager or cs")
	}
	return checkPassword(n.Password)
}

func (n *NewCustomer) validate() error {
	var err error
	if n.Email, err = normalizeEmail(n.Email); err != nil {
		return err
	}
	if n.Name, err = checkName(n.Name, false); err != nil {
		return err
	}
	return checkPassword(n.Password)
}
