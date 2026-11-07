// Package checkout turns "Buy now" into an order. Starting a checkout holds
// the stock for HoldTTL, so a one-of-a-kind item cannot be sold to someone
// else while the buyer enters the address and customs code. Paying turns the
// checkout into a pending order (the holds move with it) and starts the
// payment.
package checkout

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/order"
	"github.com/elug3/h-global/hg-server/internal/payment"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

// Statuses.
const (
	StatusOpen     = "open"
	StatusOrdered  = "ordered"
	StatusExpired  = "expired"
	StatusCanceled = "canceled"
)

// MinCardWindow is the least time a card payment gets after Pay, even when
// the hold is about to run out.
const MinCardWindow = 5 * time.Minute

// Checkout is an open purchase.
type Checkout struct {
	ID             uuid.UUID       `json:"id"`
	CustomerID     uuid.UUID       `json:"customer_id"`
	Status         string          `json:"status"`
	ExpiresAt      time.Time       `json:"expires_at"`
	Shipping       *order.Shipping `json:"shipping"`
	OrderID        *uuid.UUID      `json:"order_id"`
	Lines          []order.Line    `json:"lines"`
	SubtotalWon    int64           `json:"subtotal_won"`
	ShippingFeeWon int64           `json:"shipping_fee_won"`
	TotalWon       int64           `json:"total_won"`
	CreatedAt      time.Time       `json:"created_at"`
}

// LineInput is one line of Buy now.
type LineInput struct {
	VariantID uuid.UUID `json:"variant_id"`
	Qty       int       `json:"qty"`
}

// PayResult is an order with its started payment.
type PayResult struct {
	Order   order.Order     `json:"order"`
	Payment payment.Payment `json:"payment"`
}

// Service implements checkout.
type Service struct {
	DB             *db.DB
	Catalog        *catalog.Service
	Orders         *order.Service
	Payments       *payment.Service
	HoldTTL        time.Duration
	VirtualAcctTTL time.Duration
	ShippingFeeWon int64
	Now            func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Start holds stock for the lines and opens a checkout.
func (s *Service) Start(ctx context.Context, customerID uuid.UUID, in []LineInput) (Checkout, error) {
	if len(in) == 0 || len(in) > 20 {
		return Checkout{}, apperr.Invalid("invalid_lines", "a checkout has 1 to 20 lines")
	}
	merged := map[uuid.UUID]int{}
	var seq []uuid.UUID
	for _, l := range in {
		if l.Qty <= 0 || l.Qty > 100 {
			return Checkout{}, apperr.Invalid("invalid_qty", "quantity must be 1 to 100")
		}
		if _, ok := merged[l.VariantID]; !ok {
			seq = append(seq, l.VariantID)
		}
		merged[l.VariantID] += l.Qty
	}
	id := ids.New()
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.DB.Q(ctx).Exec(ctx,
			`INSERT INTO checkouts (id, customer_id, status, expires_at) VALUES ($1, $2, 'open', $3)`,
			id, customerID, s.now().Add(s.HoldTTL)); err != nil {
			return err
		}
		for pos, vid := range seq {
			qty := merged[vid]
			if err := s.Catalog.Hold(ctx, vid, qty); err != nil {
				return err
			}
			p, v, err := s.Catalog.ProductOfVariant(ctx, vid)
			if err != nil {
				return err
			}
			if _, err := s.DB.Q(ctx).Exec(ctx,
				`INSERT INTO checkout_lines (checkout_id, position, product_id, variant_id, title, variant_label, photo_key, unit_price_won, qty)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
				id, pos, p.ID, vid, p.Title, v.Label, p.FirstPhotoKey(), v.PriceWon, qty); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Checkout{}, err
	}
	return s.Get(ctx, id, customerID)
}

// Get returns a customer's checkout.
func (s *Service) Get(ctx context.Context, id, customerID uuid.UUID) (Checkout, error) {
	c, err := s.load(ctx, id, false)
	if err != nil {
		return Checkout{}, err
	}
	if c.CustomerID != customerID {
		return Checkout{}, apperr.NotFound("checkout")
	}
	return c, nil
}

func (s *Service) load(ctx context.Context, id uuid.UUID, forUpdate bool) (Checkout, error) {
	var c Checkout
	var ship []byte
	sql := `SELECT id, customer_id, status, expires_at, shipping, order_id, created_at FROM checkouts WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	err := s.DB.Q(ctx).QueryRow(ctx, sql, id).Scan(&c.ID, &c.CustomerID, &c.Status, &c.ExpiresAt, &ship, &c.OrderID, &c.CreatedAt)
	if db.IsNoRows(err) {
		return c, apperr.NotFound("checkout")
	}
	if err != nil {
		return c, err
	}
	if ship != nil {
		c.Shipping = &order.Shipping{}
		if err := json.Unmarshal(ship, c.Shipping); err != nil {
			return c, err
		}
	}
	rows, err := s.DB.Q(ctx).Query(ctx,
		`SELECT product_id, variant_id, title, variant_label, photo_key, unit_price_won, qty
		 FROM checkout_lines WHERE checkout_id = $1 ORDER BY position`, id)
	if err != nil {
		return c, err
	}
	c.Lines, err = pgx.CollectRows(rows, func(r pgx.CollectableRow) (order.Line, error) {
		var l order.Line
		err := r.Scan(&l.ProductID, &l.VariantID, &l.Title, &l.VariantLabel, &l.PhotoKey, &l.UnitPriceWon, &l.Qty)
		l.PhotoURL = s.Catalog.MediaURL(l.PhotoKey)
		return l, err
	})
	for _, l := range c.Lines {
		c.SubtotalWon += l.UnitPriceWon * int64(l.Qty)
	}
	c.ShippingFeeWon = s.ShippingFeeWon
	c.TotalWon = c.SubtotalWon + c.ShippingFeeWon
	return c, err
}

// open locks a customer's checkout and checks it can still be used. An
// expired one releases its holds; the caller must then commit that.
func (s *Service) open(ctx context.Context, id, customerID uuid.UUID) (Checkout, error) {
	c, err := s.load(ctx, id, true)
	if err != nil {
		return Checkout{}, err
	}
	if c.CustomerID != customerID {
		return Checkout{}, apperr.NotFound("checkout")
	}
	if c.Status != StatusOpen {
		return c, apperr.Conflict("checkout_closed", "the checkout is %s", c.Status)
	}
	if !s.now().Before(c.ExpiresAt) {
		return c, errExpired
	}
	return c, nil
}

var errExpired = apperr.Conflict("checkout_expired", "the hold ran out; start again")

// SetShipping stores the address and customs code.
func (s *Service) SetShipping(ctx context.Context, id, customerID uuid.UUID, sh order.Shipping) (Checkout, error) {
	if err := sh.Normalize(); err != nil {
		return Checkout{}, err
	}
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		if _, err := s.open(ctx, id, customerID); err != nil {
			return err
		}
		raw, _ := json.Marshal(sh)
		_, err := s.DB.Q(ctx).Exec(ctx, `UPDATE checkouts SET shipping = $2, updated_at = now() WHERE id = $1`, id, raw)
		return err
	})
	if err == errExpired {
		_ = s.expire(ctx, id)
	}
	if err != nil {
		return Checkout{}, err
	}
	return s.Get(ctx, id, customerID)
}

// Pay turns the checkout into a pending order and starts the payment. The
// holds stay in place until the order is paid or its deadline passes.
func (s *Service) Pay(ctx context.Context, id, customerID uuid.UUID, method string) (PayResult, error) {
	if method != order.MethodCard && method != order.MethodVirtualAccount {
		return PayResult{}, apperr.Invalid("invalid_payment_method", "method must be card or virtual_account")
	}
	var o order.Order
	var p payment.Payment
	var deadline time.Time
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		c, err := s.open(ctx, id, customerID)
		if err != nil {
			return err
		}
		if c.Shipping == nil {
			return apperr.Conflict("shipping_required", "set the shipping address first")
		}
		deadline = c.ExpiresAt
		if min := s.now().Add(MinCardWindow); deadline.Before(min) {
			deadline = min
		}
		if method == order.MethodVirtualAccount {
			deadline = s.now().Add(s.VirtualAcctTTL)
		}
		if o, err = s.Orders.Create(ctx, order.NewOrder{
			CustomerID: customerID, Lines: c.Lines, Shipping: *c.Shipping,
			PaymentMethod: method, Deadline: deadline, ShippingFeeWon: s.ShippingFeeWon,
		}); err != nil {
			return err
		}
		if p, err = s.Payments.CreatePending(ctx, o.ID, method, o.TotalWon); err != nil {
			return err
		}
		_, err = s.DB.Q(ctx).Exec(ctx, `UPDATE checkouts SET status = 'ordered', order_id = $2, updated_at = now() WHERE id = $1`, id, o.ID)
		return err
	})
	if err == errExpired {
		_ = s.expire(ctx, id)
	}
	if err != nil {
		return PayResult{}, err
	}
	// Outside the transaction: talk to the provider.
	p, err = s.Payments.Begin(ctx, p.ID, deadline)
	if err != nil {
		return PayResult{Order: o, Payment: p}, err
	}
	return PayResult{Order: o, Payment: p}, nil
}

// RetryPayment starts a new payment for a pending order whose previous
// attempt failed.
func (s *Service) RetryPayment(ctx context.Context, orderID, customerID uuid.UUID) (PayResult, error) {
	o, err := s.Orders.GetForCustomer(ctx, orderID, customerID)
	if err != nil {
		return PayResult{}, err
	}
	if o.Status != order.StatusPendingPayment || !s.now().Before(o.PaymentDeadline) {
		return PayResult{}, apperr.Conflict("not_payable", "the order is not waiting for payment")
	}
	list, err := s.Payments.ForOrder(ctx, orderID)
	if err != nil {
		return PayResult{}, err
	}
	for _, p := range list {
		if p.Status == payment.StatusPending && p.ProviderRef != "" {
			return PayResult{Order: o, Payment: p}, nil // still usable
		}
	}
	p, err := s.Payments.CreatePending(ctx, orderID, o.PaymentMethod, o.TotalWon)
	if err != nil {
		return PayResult{}, err
	}
	p, err = s.Payments.Begin(ctx, p.ID, o.PaymentDeadline)
	return PayResult{Order: o, Payment: p}, err
}

// Cancel abandons an open checkout and releases its holds.
func (s *Service) Cancel(ctx context.Context, id, customerID uuid.UUID) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		c, err := s.load(ctx, id, true)
		if err != nil {
			return err
		}
		if c.CustomerID != customerID {
			return apperr.NotFound("checkout")
		}
		if c.Status != StatusOpen {
			return apperr.Conflict("checkout_closed", "the checkout is %s", c.Status)
		}
		return s.close(ctx, c, StatusCanceled)
	})
}

func (s *Service) close(ctx context.Context, c Checkout, status string) error {
	for _, l := range c.Lines {
		if err := s.Catalog.Release(ctx, l.VariantID, l.Qty); err != nil {
			return err
		}
	}
	_, err := s.DB.Q(ctx).Exec(ctx, `UPDATE checkouts SET status = $2, updated_at = now() WHERE id = $1`, c.ID, status)
	return err
}

func (s *Service) expire(ctx context.Context, id uuid.UUID) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		c, err := s.load(ctx, id, true)
		if err != nil || c.Status != StatusOpen {
			return err
		}
		return s.close(ctx, c, StatusExpired)
	})
}

// ExpireOpen releases holds of checkouts past their expiry. It is a job and
// safe to repeat.
func (s *Service) ExpireOpen(ctx context.Context) (int, error) {
	rows, err := s.DB.Q(ctx).Query(ctx,
		`SELECT id FROM checkouts WHERE status = 'open' AND expires_at <= $1 ORDER BY expires_at LIMIT 200`, s.now())
	if err != nil {
		return 0, err
	}
	idList, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	for i, id := range idList {
		if err := s.expire(ctx, id); err != nil {
			return i, err
		}
	}
	return len(idList), nil
}
