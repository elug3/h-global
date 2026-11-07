package payment

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

// Service records payments and handles provider webhooks.
type Service struct {
	DB       *db.DB
	Provider Provider
	Orders   OrderHook
}

const paymentCols = `id, order_id, provider, method, status, amount_won, refunded_won, provider_ref,
	redirect_url, virtual_account, failure_reason, succeeded_at, created_at`

func scanPayment(row pgx.Row) (Payment, error) {
	var p Payment
	var va []byte
	err := row.Scan(&p.ID, &p.OrderID, &p.Provider, &p.Method, &p.Status, &p.AmountWon, &p.RefundedWon,
		&p.ProviderRef, &p.RedirectURL, &va, &p.FailureReason, &p.SucceededAt, &p.CreatedAt)
	if db.IsNoRows(err) {
		return p, apperr.NotFound("payment")
	}
	if err == nil && va != nil {
		p.VirtualAccount = &VirtualAccount{}
		err = json.Unmarshal(va, p.VirtualAccount)
	}
	return p, err
}

// Get returns a payment.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Payment, error) {
	return scanPayment(s.DB.Q(ctx).QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1`, id))
}

// ForOrder lists an order's payments, newest first.
func (s *Service) ForOrder(ctx context.Context, orderID uuid.UUID) ([]Payment, error) {
	rows, err := s.DB.Q(ctx).Query(ctx, `SELECT `+paymentCols+` FROM payments WHERE order_id = $1 ORDER BY created_at DESC`, orderID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Payment, error) { return scanPayment(r) })
}

// CreatePending records a pending payment. Call it inside the transaction
// that creates the order; call Begin after commit.
func (s *Service) CreatePending(ctx context.Context, orderID uuid.UUID, method string, amountWon int64) (Payment, error) {
	if method != MethodCard && method != MethodVirtualAccount {
		return Payment{}, apperr.Invalid("invalid_payment_method", "method must be card or virtual_account")
	}
	id := ids.New()
	if _, err := s.DB.Q(ctx).Exec(ctx,
		`INSERT INTO payments (id, order_id, provider, method, status, amount_won) VALUES ($1, $2, $3, $4, 'pending', $5)`,
		id, orderID, s.Provider.Name(), method, amountWon); err != nil {
		return Payment{}, err
	}
	return s.Get(ctx, id)
}

// Begin asks the provider to start a pending payment. It must run outside a
// database transaction, because it calls the provider over the network.
func (s *Service) Begin(ctx context.Context, paymentID uuid.UUID, dueAt time.Time) (Payment, error) {
	p, err := s.Get(ctx, paymentID)
	if err != nil {
		return p, err
	}
	if p.Status != StatusPending || p.ProviderRef != "" {
		return p, nil
	}
	c, err := s.Provider.Create(ctx, p, dueAt)
	if err != nil {
		slog.ErrorContext(ctx, "payment: provider create failed", "payment", p.ID, "err", err)
		_, _ = s.DB.Pool.Exec(ctx, `UPDATE payments SET status = 'failed', failure_reason = $2, updated_at = now() WHERE id = $1`,
			p.ID, "provider_unavailable")
		return p, apperr.Unavailable("payment_provider_unavailable", "the payment provider could not be reached; try again")
	}
	var va []byte
	if c.VirtualAccount != nil {
		va, _ = json.Marshal(c.VirtualAccount)
	}
	if _, err := s.DB.Pool.Exec(ctx,
		`UPDATE payments SET provider_ref = $2, redirect_url = $3, virtual_account = $4, updated_at = now() WHERE id = $1`,
		p.ID, c.Ref, c.RedirectURL, va); err != nil {
		return p, err
	}
	return s.Get(ctx, p.ID)
}

// HandleWebhook verifies and applies a provider notification. Repeated
// deliveries of the same event are ignored. A payment that arrives for an
// order canceled meanwhile is refunded.
func (s *Service) HandleWebhook(ctx context.Context, h http.Header, body []byte) error {
	ev, err := s.Provider.ParseWebhook(h, body)
	if err != nil {
		return err
	}
	var refund *Payment
	err = s.DB.InTx(ctx, func(ctx context.Context) error {
		tag, err := s.DB.Q(ctx).Exec(ctx,
			`INSERT INTO webhook_receipts (provider, event_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			s.Provider.Name(), ev.EventID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil // already handled
		}
		p, err := scanPayment(s.DB.Q(ctx).QueryRow(ctx,
			`SELECT `+paymentCols+` FROM payments WHERE provider = $1 AND provider_ref = $2 FOR UPDATE`,
			s.Provider.Name(), ev.Ref))
		if err != nil {
			return err
		}
		switch ev.Type {
		case EventSucceeded:
			if p.Status == StatusSucceeded {
				return nil
			}
			if ev.AmountWon != p.AmountWon {
				return apperr.Conflict("amount_mismatch", "paid %d won, expected %d", ev.AmountWon, p.AmountWon)
			}
			if _, err := s.DB.Q(ctx).Exec(ctx,
				`UPDATE payments SET status = 'succeeded', succeeded_at = now(), updated_at = now() WHERE id = $1`, p.ID); err != nil {
				return err
			}
			p.Status = StatusSucceeded
			err := s.Orders.MarkPaid(ctx, p.OrderID, p.ID, p.AmountWon)
			if apperr.Is(err, "order_not_payable") {
				refund = &p
				return nil
			}
			return err
		case EventFailed:
			if p.Status != StatusPending {
				return nil
			}
			_, err := s.DB.Q(ctx).Exec(ctx,
				`UPDATE payments SET status = 'failed', failure_reason = $2, updated_at = now() WHERE id = $1`, p.ID, ev.Reason)
			return err
		default:
			return apperr.Invalid("invalid_webhook", "unknown event type %q", ev.Type)
		}
	})
	if err != nil {
		return err
	}
	if refund != nil {
		slog.WarnContext(ctx, "payment: order no longer payable, refunding", "payment", refund.ID, "order", refund.OrderID)
		if _, err := s.refundPayment(ctx, refund.ID, refund.AmountWon, "order canceled before payment arrived"); err != nil {
			slog.ErrorContext(ctx, "payment: automatic refund failed", "payment", refund.ID, "err", err)
		}
	}
	return nil
}

// Refund returns up to amountWon of an order's succeeded payment and reports
// how much was refunded. The provider is called while the payment row is
// locked, so two refunds cannot both go through.
func (s *Service) Refund(ctx context.Context, orderID uuid.UUID, amountWon int64, reason string) (int64, error) {
	var pid uuid.UUID
	err := s.DB.Q(ctx).QueryRow(ctx,
		`SELECT id FROM payments WHERE order_id = $1 AND status = 'succeeded' ORDER BY created_at DESC LIMIT 1`, orderID).Scan(&pid)
	if db.IsNoRows(err) {
		return 0, apperr.Conflict("nothing_to_refund", "the order has no succeeded payment")
	}
	if err != nil {
		return 0, err
	}
	return s.refundPayment(ctx, pid, amountWon, reason)
}

func (s *Service) refundPayment(ctx context.Context, paymentID uuid.UUID, amountWon int64, reason string) (int64, error) {
	var refunded int64
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		p, err := scanPayment(s.DB.Q(ctx).QueryRow(ctx, `SELECT `+paymentCols+` FROM payments WHERE id = $1 FOR UPDATE`, paymentID))
		if err != nil {
			return err
		}
		remaining := p.AmountWon - p.RefundedWon
		if amountWon > remaining {
			amountWon = remaining
		}
		if amountWon <= 0 {
			return apperr.Conflict("already_refunded", "the payment is fully refunded")
		}
		ref, err := s.Provider.Refund(ctx, p, amountWon, reason)
		if err != nil {
			return apperr.Unavailable("refund_failed", "the payment provider refused the refund: %v", err)
		}
		if _, err := s.DB.Q(ctx).Exec(ctx,
			`INSERT INTO refunds (id, payment_id, amount_won, reason, provider_ref) VALUES ($1, $2, $3, $4, $5)`,
			ids.New(), p.ID, amountWon, reason, ref); err != nil {
			return err
		}
		refunded = amountWon
		_, err = s.DB.Q(ctx).Exec(ctx,
			`UPDATE payments SET refunded_won = refunded_won + $2, updated_at = now() WHERE id = $1`, p.ID, amountWon)
		return err
	})
	return refunded, err
}

// CancelPending stops an order's pending payments (unpaid order canceled).
func (s *Service) CancelPending(ctx context.Context, orderID uuid.UUID) error {
	rows, err := s.DB.Q(ctx).Query(ctx,
		`UPDATE payments SET status = 'canceled', updated_at = now() WHERE order_id = $1 AND status = 'pending' RETURNING `+paymentCols, orderID)
	if err != nil {
		return err
	}
	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (Payment, error) { return scanPayment(r) })
	if err != nil {
		return err
	}
	db.AfterCommit(ctx, func() {
		for _, p := range list {
			if p.ProviderRef == "" {
				continue
			}
			if err := s.Provider.Cancel(context.Background(), p); err != nil {
				slog.Error("payment: provider cancel failed", "payment", p.ID, "err", err)
			}
		}
	})
	return nil
}
