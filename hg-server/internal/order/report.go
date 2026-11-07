package order

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
)

// Filter narrows the staff order list.
type Filter struct {
	Statuses []string
	// Q matches the order number (with or without hyphens) or recipient
	// (substring), or the phone number when it has at least three digits.
	Q      string
	Limit  int
	Offset int
}

// AllStatuses lists every order status in lifecycle order.
var AllStatuses = []string{StatusPendingPayment, StatusPaid, StatusInspecting, StatusCustomsCheck,
	StatusShipped, StatusDelivered, StatusCompleted, StatusCanceled}

func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func digitsOf(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// where builds the WHERE clause and arguments for f.
func (f Filter) where() (string, []any) {
	var conds []string
	var args []any
	arg := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}
	if len(f.Statuses) > 0 {
		conds = append(conds, "status = ANY("+arg(f.Statuses)+")")
	}
	if q := strings.TrimSpace(f.Q); q != "" {
		like := arg("%" + likeEscape(q) + "%")
		// Order numbers match with or without their hyphens (HG-261107-00042, 26110700042).
		num := arg("%" + likeEscape(strings.ReplaceAll(q, "-", "")) + "%")
		or := []string{"replace(number, '-', '') ILIKE " + num, "shipping->>'recipient' ILIKE " + like}
		if d := digitsOf(q); len(d) >= 3 {
			or = append(or, `regexp_replace(shipping->>'phone', '\D', '', 'g') LIKE `+arg("%"+d+"%"))
		}
		conds = append(conds, "("+strings.Join(or, " OR ")+")")
	}
	w := ""
	if len(conds) > 0 {
		w = "WHERE " + strings.Join(conds, " AND ") + " "
	}
	return w, args
}

// Counts returns how many orders are in each status (every status present).
func (s *Service) Counts(ctx context.Context) (map[string]int, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `SELECT status, count(*) FROM orders GROUP BY status`)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int, len(AllStatuses))
	for _, st := range AllStatuses {
		out[st] = 0
	}
	var st string
	var n int
	_, err = pgx.ForEachRow(rows, []any{&st, &n}, func() error {
		out[st] = n
		return nil
	})
	return out, err
}

// ShowSales sums the orders that came from one stream.
type ShowSales struct {
	ShowID uuid.UUID `json:"show_id"`
	// Paid orders that are not canceled, and their lines.
	Orders   int   `json:"orders"`
	Units    int   `json:"units"`
	SalesWon int64 `json:"sales_won"`
	// Orders still waiting for payment, and paid orders later canceled.
	PendingOrders  int         `json:"pending_orders"`
	CanceledOrders int         `json:"canceled_orders"`
	Items          []ItemSales `json:"items"`
}

// ItemSales is the part of ShowSales for one show item.
type ItemSales struct {
	ItemID   uuid.UUID `json:"item_id"`
	Orders   int       `json:"orders"`
	Units    int       `json:"units"`
	SalesWon int64     `json:"sales_won"`
}

// showSalesSQL attributes an order line to a show when the product is one of
// the show's items, the order was placed after the show started, and no later
// show featuring the same product had started by then. So a product shown
// again in a later stream counts for the later stream from its start.
// Amounts are line totals (shipping excluded).
const showSalesSQL = `
WITH s0 AS (SELECT id, started_at FROM shows WHERE id = $1 AND started_at IS NOT NULL),
lines AS (
  SELECT si.id AS item_id, o.id AS order_id, o.status, o.paid_at, l.qty, l.qty * l.unit_price_won AS amount
    FROM s0
    JOIN show_items si ON si.show_id = s0.id
    JOIN order_lines l ON l.product_id = si.product_id
    JOIN orders o ON o.id = l.order_id
   WHERE o.created_at >= s0.started_at
     AND NOT EXISTS (
       SELECT 1 FROM show_items si2 JOIN shows s2 ON s2.id = si2.show_id
        WHERE si2.product_id = si.product_id AND s2.id <> s0.id
          AND s2.started_at > s0.started_at AND s2.started_at <= o.created_at))
SELECT item_id,
       count(DISTINCT order_id) FILTER (WHERE paid_at IS NOT NULL AND status <> 'canceled'),
       coalesce(sum(qty) FILTER (WHERE paid_at IS NOT NULL AND status <> 'canceled'), 0),
       coalesce(sum(amount) FILTER (WHERE paid_at IS NOT NULL AND status <> 'canceled'), 0)::bigint,
       count(DISTINCT order_id) FILTER (WHERE status = 'pending_payment'),
       count(DISTINCT order_id) FILTER (WHERE status = 'canceled' AND paid_at IS NOT NULL)
  FROM lines
 GROUP BY GROUPING SETS ((item_id), ())`

// SalesForShow returns a stream's sales. Items without orders are left out.
func (s *Service) SalesForShow(ctx context.Context, showID uuid.UUID) (ShowSales, error) {
	out := ShowSales{ShowID: showID, Items: []ItemSales{}}
	q := s.DB.Q(ctx)
	var exists bool
	if err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shows WHERE id = $1)`, showID).Scan(&exists); err != nil {
		return out, err
	}
	if !exists {
		return out, apperr.NotFound("show")
	}
	rows, err := q.Query(ctx, showSalesSQL, showID)
	if err != nil {
		return out, err
	}
	var (
		item              *uuid.UUID
		orders, units     int
		sales             int64
		pending, canceled int
	)
	_, err = pgx.ForEachRow(rows, []any{&item, &orders, &units, &sales, &pending, &canceled}, func() error {
		if item == nil {
			out.Orders, out.Units, out.SalesWon = orders, units, sales
			out.PendingOrders, out.CanceledOrders = pending, canceled
			return nil
		}
		if orders > 0 {
			out.Items = append(out.Items, ItemSales{ItemID: *item, Orders: orders, Units: units, SalesWon: sales})
		}
		return nil
	})
	return out, err
}
