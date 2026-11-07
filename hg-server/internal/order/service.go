package order

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/payment"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/events"
)

// StaffTopic carries every order change to H-Global Studio.
const StaffTopic = "studio:orders"

// Event types on StaffTopic.
const (
	EvCreated = "order.created"
	EvUpdated = "order.updated"
)

// Service implements order use cases.
type Service struct {
	DB       *db.DB
	Store    Postgres
	Catalog  *catalog.Service
	Payments *payment.Service
	Bus      *events.Bus
	MediaURL func(key string) string
	Now      func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) decorate(o *Order, staff bool) {
	for i := range o.Lines {
		o.Lines[i].PhotoURL = s.MediaURL(o.Lines[i].PhotoKey)
	}
	if o.Lines == nil {
		o.Lines = []Line{}
	}
	if o.Media == nil {
		o.Media = []Media{}
	}
	for i := range o.Media {
		prefix := "/api"
		if staff {
			prefix = "/studio-api"
		}
		o.Media[i].URL = fmt.Sprintf("%s/orders/%s/media/%s", prefix, o.ID, o.Media[i].ID)
	}
}

// Create records a pending order. Checkout calls it inside its transaction;
// the stock is already held.
func (s *Service) Create(ctx context.Context, n NewOrder) (Order, error) {
	if len(n.Lines) == 0 {
		return Order{}, apperr.Invalid("empty_order", "an order needs at least one line")
	}
	var subtotal int64
	for _, l := range n.Lines {
		subtotal += l.UnitPriceWon * int64(l.Qty)
	}
	var out Order
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		id, err := s.Store.insert(ctx, n, subtotal)
		if err != nil {
			return err
		}
		if err := s.Store.addHistory(ctx, id, "", StatusPendingPayment, Actor{Kind: "customer", ID: &n.CustomerID}, ""); err != nil {
			return err
		}
		if out, err = s.Store.get(ctx, id, false); err != nil {
			return err
		}
		s.decorate(&out, true)
		return s.Bus.Publish(ctx, StaffTopic, EvCreated, map[string]any{"order": out})
	})
	return out, err
}

// Get returns an order for staff, with history.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Order, error) {
	o, err := s.Store.get(ctx, id, false)
	if err != nil {
		return o, err
	}
	if o.History, err = s.Store.history(ctx, id); err != nil {
		return o, err
	}
	s.decorate(&o, true)
	return o, nil
}

// GetForCustomer returns a customer's own order; others' are not found.
// Inspection media become visible once inspection is done.
func (s *Service) GetForCustomer(ctx context.Context, id, customerID uuid.UUID) (Order, error) {
	o, err := s.Store.get(ctx, id, false)
	if err != nil {
		return Order{}, err
	}
	if o.CustomerID != customerID {
		return Order{}, apperr.NotFound("order")
	}
	if o.InspectionDoneAt == nil {
		o.Media = nil
	}
	s.decorate(&o, false)
	return o, nil
}

// ListForCustomer lists a customer's orders, newest first.
func (s *Service) ListForCustomer(ctx context.Context, customerID uuid.UUID, limit int) ([]Order, error) {
	list, err := s.Store.list(ctx, `WHERE customer_id = $1 ORDER BY created_at DESC LIMIT $2`, customerID, limit)
	for i := range list {
		if list[i].InspectionDoneAt == nil {
			list[i].Media = nil
		}
		s.decorate(&list[i], false)
	}
	return list, err
}

// List lists orders for staff, newest first.
func (s *Service) List(ctx context.Context, statuses []string, limit, offset int) ([]Order, error) {
	var list []Order
	var err error
	if len(statuses) == 0 {
		list, err = s.Store.list(ctx, `ORDER BY created_at DESC LIMIT $1 OFFSET $2`, limit, offset)
	} else {
		list, err = s.Store.list(ctx, `WHERE status = ANY($1) ORDER BY created_at DESC LIMIT $2 OFFSET $3`, statuses, limit, offset)
	}
	for i := range list {
		s.decorate(&list[i], true)
	}
	return list, err
}

// transition locks the order, checks its status, applies fn and records the
// change.
func (s *Service) transition(ctx context.Context, id uuid.UUID, actor Actor, note string, from []string, to string, fn func(ctx context.Context, o *Order) error) (Order, error) {
	var out Order
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.get(ctx, id, true)
		if err != nil {
			return err
		}
		ok := false
		for _, f := range from {
			ok = ok || o.Status == f
		}
		if !ok {
			return apperr.Conflict("invalid_transition", "order is %s; this needs %s", o.Status, strings.Join(from, " or "))
		}
		prev := o.Status
		if fn != nil {
			if err := fn(ctx, &o); err != nil {
				return err
			}
		}
		o.Status = to
		if err := s.Store.save(ctx, o); err != nil {
			return err
		}
		if err := s.Store.addHistory(ctx, id, prev, to, actor, note); err != nil {
			return err
		}
		out = o
		return s.publish(ctx, id)
	})
	if err != nil {
		return Order{}, err
	}
	return s.Get(ctx, out.ID)
}

func (s *Service) publish(ctx context.Context, id uuid.UUID) error {
	o, err := s.Store.get(ctx, id, false)
	if err != nil {
		return err
	}
	s.decorate(&o, true)
	return s.Bus.Publish(ctx, StaffTopic, EvUpdated, map[string]any{"order": o})
}

// MarkPaid implements payment.OrderHook: the held stock becomes sold.
func (s *Service) MarkPaid(ctx context.Context, orderID, paymentID uuid.UUID, amountWon int64) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.get(ctx, orderID, true)
		if err != nil {
			return err
		}
		switch o.Status {
		case StatusPendingPayment:
		case StatusCanceled:
			return apperr.Conflict("order_not_payable", "order %s was canceled", o.Number)
		default:
			return nil // already paid; a repeated notification
		}
		if amountWon != o.TotalWon {
			return apperr.Conflict("amount_mismatch", "paid %d won, order total is %d", amountWon, o.TotalWon)
		}
		for _, l := range o.Lines {
			if err := s.Catalog.CommitHold(ctx, l.VariantID, l.Qty); err != nil {
				return err
			}
		}
		now := s.now()
		o.Status, o.PaidAt = StatusPaid, &now
		if err := s.Store.save(ctx, o); err != nil {
			return err
		}
		pid := paymentID
		if err := s.Store.addHistory(ctx, orderID, StatusPendingPayment, StatusPaid, Actor{Kind: "payment", ID: &pid}, ""); err != nil {
			return err
		}
		return s.publish(ctx, orderID)
	})
}

// AddInspectionMedia records an inspection photo or video. The first one
// moves a paid order to inspecting.
func (s *Service) AddInspectionMedia(ctx context.Context, orderID, staffID uuid.UUID, m Media) (Order, error) {
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.get(ctx, orderID, true)
		if err != nil {
			return err
		}
		if o.Status != StatusPaid && o.Status != StatusInspecting {
			return apperr.Conflict("invalid_transition", "inspection media can be added while the order is paid or inspecting")
		}
		if err := s.Store.addMedia(ctx, orderID, m, staffID); err != nil {
			return err
		}
		if o.Status == StatusPaid {
			o.Status = StatusInspecting
			if err := s.Store.save(ctx, o); err != nil {
				return err
			}
			if err := s.Store.addHistory(ctx, orderID, StatusPaid, StatusInspecting, Actor{Kind: "staff", ID: &staffID}, ""); err != nil {
				return err
			}
		}
		return s.publish(ctx, orderID)
	})
	if err != nil {
		return Order{}, err
	}
	return s.Get(ctx, orderID)
}

// CompleteInspection moves an inspected order to the customs check.
func (s *Service) CompleteInspection(ctx context.Context, orderID, staffID uuid.UUID, note string) (Order, error) {
	return s.transition(ctx, orderID, Actor{Kind: "staff", ID: &staffID}, note,
		[]string{StatusInspecting}, StatusCustomsCheck, func(ctx context.Context, o *Order) error {
			if len(o.Media) == 0 {
				return apperr.Conflict("inspection_media_required", "add at least one inspection photo or video first")
			}
			now := s.now()
			o.InspectionDoneAt = &now
			return nil
		})
}

// CheckCustoms records that the customs code was checked. Shipping needs it.
func (s *Service) CheckCustoms(ctx context.Context, orderID, staffID uuid.UUID) (Order, error) {
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.get(ctx, orderID, true)
		if err != nil {
			return err
		}
		if o.Status != StatusCustomsCheck {
			return apperr.Conflict("invalid_transition", "order is %s; the customs check comes after inspection", o.Status)
		}
		if !ValidPCCC(o.Shipping.PCCC) {
			return apperr.Conflict("pccc_missing", "the order has no valid customs code; ask the customer for it")
		}
		now := s.now()
		o.CustomsCheckedAt = &now
		if err := s.Store.save(ctx, o); err != nil {
			return err
		}
		if err := s.Store.setCustomsChecker(ctx, orderID, staffID); err != nil {
			return err
		}
		if err := s.Store.addHistory(ctx, orderID, o.Status, o.Status, Actor{Kind: "staff", ID: &staffID}, "customs code checked"); err != nil {
			return err
		}
		return s.publish(ctx, orderID)
	})
	if err != nil {
		return Order{}, err
	}
	return s.Get(ctx, orderID)
}

// UpdateCustomsCode lets the customer correct the customs code before the
// check is done.
func (s *Service) UpdateCustomsCode(ctx context.Context, orderID, customerID uuid.UUID, pccc string) (Order, error) {
	pccc = strings.ToUpper(strings.TrimSpace(pccc))
	if !ValidPCCC(pccc) {
		return Order{}, apperr.Invalid("invalid_pccc", "the customs code is P followed by 12 digits")
	}
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		o, err := s.Store.get(ctx, orderID, true)
		if err != nil {
			return err
		}
		if o.CustomerID != customerID {
			return apperr.NotFound("order")
		}
		if o.CustomsCheckedAt != nil || o.Status == StatusShipped || o.Status == StatusDelivered ||
			o.Status == StatusCompleted || o.Status == StatusCanceled {
			return apperr.Conflict("customs_locked", "the customs code can no longer change")
		}
		o.Shipping.PCCC = pccc
		if _, err := s.DB.Q(ctx).Exec(ctx, `UPDATE orders SET shipping = jsonb_set(shipping, '{pccc}', to_jsonb($2::text)), updated_at = now() WHERE id = $1`,
			orderID, pccc); err != nil {
			return err
		}
		return s.publish(ctx, orderID)
	})
	if err != nil {
		return Order{}, err
	}
	return s.GetForCustomer(ctx, orderID, customerID)
}

// Ship records the carrier and tracking number.
func (s *Service) Ship(ctx context.Context, orderID, staffID uuid.UUID, carrier, tracking string) (Order, error) {
	carrier, tracking = strings.TrimSpace(carrier), strings.TrimSpace(tracking)
	if carrier == "" || len(carrier) > 50 || tracking == "" || len(tracking) > 100 {
		return Order{}, apperr.Invalid("invalid_shipment", "carrier (up to 50) and tracking_number (up to 100) are required")
	}
	return s.transition(ctx, orderID, Actor{Kind: "staff", ID: &staffID}, carrier+" "+tracking,
		[]string{StatusCustomsCheck}, StatusShipped, func(ctx context.Context, o *Order) error {
			if o.CustomsCheckedAt == nil {
				return apperr.Conflict("customs_not_checked", "check the customs code before shipping")
			}
			now := s.now()
			o.ShippedAt, o.Carrier, o.TrackingNumber = &now, carrier, tracking
			return nil
		})
}

// Deliver records delivery.
func (s *Service) Deliver(ctx context.Context, orderID, staffID uuid.UUID) (Order, error) {
	return s.transition(ctx, orderID, Actor{Kind: "staff", ID: &staffID}, "",
		[]string{StatusShipped}, StatusDelivered, func(ctx context.Context, o *Order) error {
			now := s.now()
			o.DeliveredAt = &now
			return nil
		})
}

// Complete closes a delivered order (customer confirms receipt, or staff).
func (s *Service) Complete(ctx context.Context, orderID uuid.UUID, actor Actor) (Order, error) {
	if actor.Kind == "customer" {
		o, err := s.Store.get(ctx, orderID, false)
		if err != nil {
			return Order{}, err
		}
		if o.CustomerID != *actor.ID {
			return Order{}, apperr.NotFound("order")
		}
	}
	return s.transition(ctx, orderID, actor, "", []string{StatusDelivered}, StatusCompleted, func(ctx context.Context, o *Order) error {
		now := s.now()
		o.CompletedAt = &now
		return nil
	})
}

// Cancel cancels an order. Unpaid: holds are released. Paid: the provider
// refunds first, then the order changes; stock returns only if the order had
// not shipped. Customers may cancel only while unpaid or paid (not yet
// inspected).
func (s *Service) Cancel(ctx context.Context, orderID uuid.UUID, actor Actor, reason string) (Order, error) {
	o, err := s.Store.get(ctx, orderID, false)
	if err != nil {
		return Order{}, err
	}
	if actor.Kind == "customer" {
		if o.CustomerID != *actor.ID {
			return Order{}, apperr.NotFound("order")
		}
		if !o.CustomerCancelable() {
			return Order{}, apperr.Conflict("not_cancelable", "the order is being prepared; contact us to cancel")
		}
	}
	if o.Status == StatusCompleted || o.Status == StatusCanceled {
		return Order{}, apperr.Conflict("not_cancelable", "the order is %s", o.Status)
	}
	var refunded int64
	if o.Status != StatusPendingPayment {
		refunded, err = s.Payments.Refund(ctx, orderID, o.TotalWon-o.RefundedWon, reason)
		if err != nil && !apperr.Is(err, "already_refunded") {
			return Order{}, err
		}
	}
	from := []string{StatusPendingPayment, StatusPaid, StatusInspecting, StatusCustomsCheck, StatusShipped, StatusDelivered}
	return s.transition(ctx, orderID, actor, reason, from, StatusCanceled, func(ctx context.Context, o *Order) error {
		switch {
		case o.Status == StatusPendingPayment:
			for _, l := range o.Lines {
				if err := s.Catalog.Release(ctx, l.VariantID, l.Qty); err != nil {
					return err
				}
			}
			if err := s.Payments.CancelPending(ctx, orderID); err != nil {
				return err
			}
		case stockReturnable(o.Status):
			for _, l := range o.Lines {
				if err := s.Catalog.ReturnSold(ctx, l.VariantID, l.Qty); err != nil {
					return err
				}
			}
		}
		now := s.now()
		o.CanceledAt, o.CancelReason = &now, reason
		o.RefundedWon += refunded
		return nil
	})
}

// ExpireUnpaid cancels pending orders past their payment deadline. It is a
// job and safe to repeat.
func (s *Service) ExpireUnpaid(ctx context.Context) (int, error) {
	list, err := s.Store.list(ctx, `WHERE status = 'pending_payment' AND payment_deadline < $1 ORDER BY payment_deadline LIMIT 200`, s.now())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, o := range list {
		_, err := s.Cancel(ctx, o.ID, Actor{Kind: "system"}, "payment deadline passed")
		if apperr.Is(err, "invalid_transition") || apperr.Is(err, "not_cancelable") {
			continue // paid meanwhile
		}
		if err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// MediaKey returns the storage key of an order's media, checking access.
func (s *Service) MediaKey(ctx context.Context, orderID, mediaID uuid.UUID, customerID *uuid.UUID) (Media, error) {
	var o Order
	var err error
	if customerID != nil {
		o, err = s.GetForCustomer(ctx, orderID, *customerID)
	} else {
		o, err = s.Get(ctx, orderID)
	}
	if err != nil {
		return Media{}, err
	}
	for _, m := range o.Media {
		if m.ID == mediaID {
			return m, nil
		}
	}
	return Media{}, apperr.NotFound("media")
}
