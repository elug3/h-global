package account

import (
	"context"
	"strings"
	"sync"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

// Service implements account use cases.
type Service struct {
	DB         *db.DB
	Store      Postgres
	BcryptCost int
	// OnStaffDeactivated ends a staff member's sessions (wired to auth.Sessions).
	OnStaffDeactivated func(ctx context.Context, id uuid.UUID) error

	dummyOnce sync.Once
	dummy     string
}

var errBadLogin = apperr.Unauthorized("invalid_credentials", "email or password is wrong")

// dummyHash is compared against when an email is unknown, so a login takes
// the same time whether or not the account exists.
func (s *Service) dummyHash() string {
	s.dummyOnce.Do(func() {
		s.dummy, _ = auth.HashPassword("not-a-real-password", s.BcryptCost)
	})
	return s.dummy
}

// EnsureOwner creates the first owner when there are no staff at all.
func (s *Service) EnsureOwner(ctx context.Context, email, password string) (bool, error) {
	if email == "" || password == "" {
		return false, nil
	}
	created := false
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		n, err := s.Store.CountStaff(ctx)
		if err != nil || n > 0 {
			return err
		}
		_, err = s.CreateStaff(ctx, NewStaff{Email: email, Name: "Owner", Role: auth.RoleOwner, Password: password})
		created = err == nil
		return err
	})
	return created, err
}

// StaffLogin checks a staff member's password.
func (s *Service) StaffLogin(ctx context.Context, email, password string) (Staff, error) {
	st, err := s.Store.StaffByEmail(ctx, strings.TrimSpace(email))
	if apperr.Is(err, "staff_not_found") {
		auth.CheckPassword(s.dummyHash(), password)
		return Staff{}, errBadLogin
	}
	if err != nil {
		return Staff{}, err
	}
	if !auth.CheckPassword(st.passwordHash, password) || !st.Active {
		return Staff{}, errBadLogin
	}
	return st, nil
}

// StaffByID returns a staff member.
func (s *Service) StaffByID(ctx context.Context, id uuid.UUID) (Staff, error) {
	return s.Store.StaffByID(ctx, id)
}

// StaffRole is the auth.RoleLookup for staff sessions.
func (s *Service) StaffRole(ctx context.Context, id uuid.UUID) (string, bool, error) {
	st, err := s.Store.StaffByID(ctx, id)
	if apperr.Is(err, "staff_not_found") {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return st.Role, st.Active, nil
}

// ListStaff lists all staff.
func (s *Service) ListStaff(ctx context.Context) ([]Staff, error) { return s.Store.ListStaff(ctx) }

// CreateStaff adds a staff member.
func (s *Service) CreateStaff(ctx context.Context, n NewStaff) (Staff, error) {
	if err := n.validate(); err != nil {
		return Staff{}, err
	}
	hash, err := auth.HashPassword(n.Password, s.BcryptCost)
	if err != nil {
		return Staff{}, err
	}
	st := Staff{ID: ids.New(), Email: n.Email, Name: n.Name, Role: n.Role, Active: true, passwordHash: hash}
	if err := s.Store.InsertStaff(ctx, st); err != nil {
		return Staff{}, err
	}
	return s.Store.StaffByID(ctx, st.ID)
}

// UpdateStaff changes name, role, active state or password. The last active
// owner cannot be demoted or deactivated.
func (s *Service) UpdateStaff(ctx context.Context, id uuid.UUID, p StaffPatch) (Staff, error) {
	var out Staff
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		st, err := s.Store.StaffByID(ctx, id)
		if err != nil {
			return err
		}
		wasOwner := st.Role == auth.RoleOwner && st.Active
		if p.Name != nil {
			if st.Name, err = checkName(*p.Name, true); err != nil {
				return err
			}
		}
		if p.Role != nil {
			if !auth.ValidRole(*p.Role) {
				return apperr.Invalid("invalid_role", "role must be owner, manager or cs")
			}
			st.Role = *p.Role
		}
		if p.Active != nil {
			st.Active = *p.Active
		}
		if p.Password != nil {
			if err := checkPassword(*p.Password); err != nil {
				return err
			}
			if st.passwordHash, err = auth.HashPassword(*p.Password, s.BcryptCost); err != nil {
				return err
			}
		}
		if wasOwner && (st.Role != auth.RoleOwner || !st.Active) {
			n, err := s.Store.CountActiveOwners(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return apperr.Conflict("last_owner", "the last active owner cannot be demoted or deactivated")
			}
		}
		if err := s.Store.UpdateStaff(ctx, st); err != nil {
			return err
		}
		if (!st.Active || p.Password != nil) && s.OnStaffDeactivated != nil {
			if err := s.OnStaffDeactivated(ctx, st.ID); err != nil {
				return err
			}
		}
		out = st
		return nil
	})
	return out, err
}

// Register creates a customer.
func (s *Service) Register(ctx context.Context, n NewCustomer) (Customer, error) {
	if err := n.validate(); err != nil {
		return Customer{}, err
	}
	hash, err := auth.HashPassword(n.Password, s.BcryptCost)
	if err != nil {
		return Customer{}, err
	}
	c := Customer{ID: ids.New(), Email: n.Email, Name: n.Name, passwordHash: hash}
	if err := s.Store.InsertCustomer(ctx, c); err != nil {
		return Customer{}, err
	}
	return s.Store.CustomerByID(ctx, c.ID)
}

// CustomerLogin checks a customer's password.
func (s *Service) CustomerLogin(ctx context.Context, email, password string) (Customer, error) {
	c, err := s.Store.CustomerByEmail(ctx, strings.TrimSpace(email))
	if apperr.Is(err, "customer_not_found") {
		auth.CheckPassword(s.dummyHash(), password)
		return Customer{}, errBadLogin
	}
	if err != nil {
		return Customer{}, err
	}
	if !auth.CheckPassword(c.passwordHash, password) {
		return Customer{}, errBadLogin
	}
	return c, nil
}

// CustomerByID returns a customer.
func (s *Service) CustomerByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return s.Store.CustomerByID(ctx, id)
}
