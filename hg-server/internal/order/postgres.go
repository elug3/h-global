package order

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

// Postgres stores orders.
type Postgres struct{ DB *db.DB }

const orderCols = `id, number, customer_id, status, subtotal_won, shipping_fee_won, total_won, refunded_won,
	shipping, payment_method, payment_deadline, paid_at, inspection_done_at, customs_checked_at, shipped_at,
	carrier, tracking_number, delivered_at, completed_at, canceled_at, cancel_reason, created_at, updated_at`

func scanOrder(row pgx.Row) (Order, error) {
	var o Order
	var ship []byte
	err := row.Scan(&o.ID, &o.Number, &o.CustomerID, &o.Status, &o.SubtotalWon, &o.ShippingFeeWon, &o.TotalWon,
		&o.RefundedWon, &ship, &o.PaymentMethod, &o.PaymentDeadline, &o.PaidAt, &o.InspectionDoneAt,
		&o.CustomsCheckedAt, &o.ShippedAt, &o.Carrier, &o.TrackingNumber, &o.DeliveredAt, &o.CompletedAt,
		&o.CanceledAt, &o.CancelReason, &o.CreatedAt, &o.UpdatedAt)
	if db.IsNoRows(err) {
		return o, apperr.NotFound("order")
	}
	if err == nil {
		err = json.Unmarshal(ship, &o.Shipping)
	}
	return o, err
}

// nextNumber returns the next order number, HG-YYMMDD-XXXXX: today's date in
// Korea time and the day's count from 00001 (more than 99,999 orders in a day
// would give six digits). It must run in the order's transaction: the day's
// row stays locked until commit, and a rollback gives the count back.
func (p Postgres) nextNumber(ctx context.Context) (string, error) {
	var day time.Time
	var n int
	err := p.DB.Q(ctx).QueryRow(ctx,
		`INSERT INTO order_number_days (day, last) VALUES ((now() AT TIME ZONE 'Asia/Seoul')::date, 1)
		 ON CONFLICT (day) DO UPDATE SET last = order_number_days.last + 1
		 RETURNING day, last`).Scan(&day, &n)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("HG-%s-%05d", day.Format("060102"), n), nil
}

func (p Postgres) insert(ctx context.Context, n NewOrder, subtotal int64) (uuid.UUID, error) {
	q := p.DB.Q(ctx)
	id := ids.New()
	ship, _ := json.Marshal(n.Shipping)
	number, err := p.nextNumber(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	if _, err := q.Exec(ctx,
		`INSERT INTO orders (id, number, customer_id, status, subtotal_won, shipping_fee_won, total_won, shipping,
		                     payment_method, payment_deadline)
		 VALUES ($1, $2, $3, 'pending_payment', $4, $5, $6, $7, $8, $9)`,
		id, number, n.CustomerID, subtotal, n.ShippingFeeWon, subtotal+n.ShippingFeeWon, ship, n.PaymentMethod, n.Deadline); err != nil {
		return uuid.Nil, err
	}
	for i, l := range n.Lines {
		if _, err := q.Exec(ctx,
			`INSERT INTO order_lines (id, order_id, position, product_id, variant_id, title, variant_label, photo_key, unit_price_won, qty)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			ids.New(), id, i, l.ProductID, l.VariantID, l.Title, l.VariantLabel, l.PhotoKey, l.UnitPriceWon, l.Qty); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

func (p Postgres) get(ctx context.Context, id uuid.UUID, forUpdate bool) (Order, error) {
	sql := `SELECT ` + orderCols + ` FROM orders WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	o, err := scanOrder(p.DB.Q(ctx).QueryRow(ctx, sql, id))
	if err != nil {
		return o, err
	}
	return o, p.fill(ctx, &o)
}

func (p Postgres) fill(ctx context.Context, o *Order) error {
	q := p.DB.Q(ctx)
	rows, err := q.Query(ctx,
		`SELECT id, product_id, variant_id, title, variant_label, photo_key, unit_price_won, qty
		 FROM order_lines WHERE order_id = $1 ORDER BY position`, o.ID)
	if err != nil {
		return err
	}
	o.Lines, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Line, error) {
		var l Line
		err := r.Scan(&l.ID, &l.ProductID, &l.VariantID, &l.Title, &l.VariantLabel, &l.PhotoKey, &l.UnitPriceWon, &l.Qty)
		return l, err
	})
	if err != nil {
		return err
	}
	rows, err = q.Query(ctx,
		`SELECT id, kind, content_type, object_key, created_at FROM order_media WHERE order_id = $1 ORDER BY created_at`, o.ID)
	if err != nil {
		return err
	}
	o.Media, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (Media, error) {
		var m Media
		err := r.Scan(&m.ID, &m.Kind, &m.ContentType, &m.ObjectKey, &m.CreatedAt)
		return m, err
	})
	return err
}

func (p Postgres) history(ctx context.Context, id uuid.UUID) ([]History, error) {
	rows, err := p.DB.Q(ctx).Query(ctx,
		`SELECT from_status, to_status, actor_kind, actor_id, note, created_at FROM order_history WHERE order_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (History, error) {
		var h History
		err := r.Scan(&h.From, &h.To, &h.ActorKind, &h.ActorID, &h.Note, &h.At)
		return h, err
	})
}

func (p Postgres) list(ctx context.Context, where string, args ...any) ([]Order, error) {
	rows, err := p.DB.Q(ctx).Query(ctx, `SELECT `+orderCols+` FROM orders `+where, args...)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Order, error) { return scanOrder(r) })
	if err != nil {
		return nil, err
	}
	for i := range list {
		if err := p.fill(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

func (p Postgres) save(ctx context.Context, o Order) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`UPDATE orders SET status = $2, refunded_won = $3, paid_at = $4, inspection_done_at = $5,
		        customs_checked_at = $6, shipped_at = $7, carrier = $8, tracking_number = $9, delivered_at = $10,
		        completed_at = $11, canceled_at = $12, cancel_reason = $13, updated_at = now()
		 WHERE id = $1`,
		o.ID, o.Status, o.RefundedWon, o.PaidAt, o.InspectionDoneAt, o.CustomsCheckedAt, o.ShippedAt,
		o.Carrier, o.TrackingNumber, o.DeliveredAt, o.CompletedAt, o.CanceledAt, o.CancelReason)
	return err
}

func (p Postgres) addHistory(ctx context.Context, id uuid.UUID, from, to string, a Actor, note string) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO order_history (order_id, from_status, to_status, actor_kind, actor_id, note) VALUES ($1, $2, $3, $4, $5, $6)`,
		id, from, to, a.Kind, a.ID, note)
	return err
}

func (p Postgres) setCustomsChecker(ctx context.Context, id, staffID uuid.UUID) error {
	_, err := p.DB.Q(ctx).Exec(ctx, `UPDATE orders SET customs_checked_by = $2 WHERE id = $1`, id, staffID)
	return err
}

func (p Postgres) addMedia(ctx context.Context, orderID uuid.UUID, m Media, staffID uuid.UUID) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO order_media (id, order_id, kind, object_key, content_type, created_by) VALUES ($1, $2, $3, $4, $5, $6)`,
		m.ID, orderID, m.Kind, m.ObjectKey, m.ContentType, staffID)
	return err
}
