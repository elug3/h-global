package account

import (
	"context"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
)

// Postgres implements Store.
type Postgres struct{ DB *db.DB }

const staffCols = `id, email, name, role, active, created_at, password_hash`

func scanStaff(row interface{ Scan(...any) error }) (Staff, error) {
	var s Staff
	err := row.Scan(&s.ID, &s.Email, &s.Name, &s.Role, &s.Active, &s.CreatedAt, &s.passwordHash)
	if db.IsNoRows(err) {
		return s, apperr.NotFound("staff")
	}
	return s, err
}

func (p Postgres) CountStaff(ctx context.Context) (int, error) {
	var n int
	err := p.DB.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM staff`).Scan(&n)
	return n, err
}

func (p Postgres) CountActiveOwners(ctx context.Context) (int, error) {
	var n int
	err := p.DB.Q(ctx).QueryRow(ctx, `SELECT count(*) FROM staff WHERE role = 'owner' AND active`).Scan(&n)
	return n, err
}

func (p Postgres) InsertStaff(ctx context.Context, s Staff) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO staff (id, email, name, role, password_hash, active) VALUES ($1, $2, $3, $4, $5, $6)`,
		s.ID, s.Email, s.Name, s.Role, s.passwordHash, s.Active)
	if db.ConstraintViolated(err) == "staff_email" {
		return apperr.Conflict("email_taken", "a staff member with this email exists")
	}
	return err
}

func (p Postgres) StaffByID(ctx context.Context, id uuid.UUID) (Staff, error) {
	return scanStaff(p.DB.Q(ctx).QueryRow(ctx, `SELECT `+staffCols+` FROM staff WHERE id = $1`, id))
}

func (p Postgres) StaffByEmail(ctx context.Context, email string) (Staff, error) {
	return scanStaff(p.DB.Q(ctx).QueryRow(ctx, `SELECT `+staffCols+` FROM staff WHERE lower(email) = lower($1)`, email))
}

func (p Postgres) ListStaff(ctx context.Context) ([]Staff, error) {
	rows, err := p.DB.Q(ctx).Query(ctx, `SELECT `+staffCols+` FROM staff ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Staff
	for rows.Next() {
		s, err := scanStaff(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p Postgres) UpdateStaff(ctx context.Context, s Staff) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`UPDATE staff SET name = $2, role = $3, active = $4, password_hash = $5 WHERE id = $1`,
		s.ID, s.Name, s.Role, s.Active, s.passwordHash)
	return err
}

const customerCols = `id, email, name, created_at, password_hash`

func scanCustomer(row interface{ Scan(...any) error }) (Customer, error) {
	var c Customer
	err := row.Scan(&c.ID, &c.Email, &c.Name, &c.CreatedAt, &c.passwordHash)
	if db.IsNoRows(err) {
		return c, apperr.NotFound("customer")
	}
	return c, err
}

func (p Postgres) InsertCustomer(ctx context.Context, c Customer) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO customers (id, email, name, password_hash) VALUES ($1, $2, $3, $4)`,
		c.ID, c.Email, c.Name, c.passwordHash)
	if db.ConstraintViolated(err) == "customers_email" {
		return apperr.Conflict("email_taken", "an account with this email exists")
	}
	return err
}

func (p Postgres) CustomerByID(ctx context.Context, id uuid.UUID) (Customer, error) {
	return scanCustomer(p.DB.Q(ctx).QueryRow(ctx, `SELECT `+customerCols+` FROM customers WHERE id = $1`, id))
}

func (p Postgres) CustomerByEmail(ctx context.Context, email string) (Customer, error) {
	return scanCustomer(p.DB.Q(ctx).QueryRow(ctx, `SELECT `+customerCols+` FROM customers WHERE lower(email) = lower($1)`, email))
}
