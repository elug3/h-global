// Package order runs the order lifecycle from the meeting's flow:
// order -> inspection -> customs check -> shipping.
//
//	pending_payment --paid--> paid --> inspecting --> customs_check --> shipped --> delivered --> completed
//	      |                     |           |               |              |            |
//	      +-- unpaid deadline   +-----------+---------------+-- cancel ----+------------+
//	          or cancel             (refund; stock returned before shipping, not after)
package order

import (
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
)

// Statuses.
const (
	StatusPendingPayment = "pending_payment"
	StatusPaid           = "paid"
	StatusInspecting     = "inspecting"
	StatusCustomsCheck   = "customs_check"
	StatusShipped        = "shipped"
	StatusDelivered      = "delivered"
	StatusCompleted      = "completed"
	StatusCanceled       = "canceled"
)

// Payment methods.
const (
	MethodCard           = "card"
	MethodVirtualAccount = "virtual_account"
)

// Order is one order.
type Order struct {
	ID               uuid.UUID  `json:"id"`
	Number           string     `json:"number"`
	CustomerID       uuid.UUID  `json:"customer_id"`
	Status           string     `json:"status"`
	SubtotalWon      int64      `json:"subtotal_won"`
	ShippingFeeWon   int64      `json:"shipping_fee_won"`
	TotalWon         int64      `json:"total_won"`
	RefundedWon      int64      `json:"refunded_won"`
	Shipping         Shipping   `json:"shipping"`
	PaymentMethod    string     `json:"payment_method"`
	PaymentDeadline  time.Time  `json:"payment_deadline"`
	PaidAt           *time.Time `json:"paid_at"`
	InspectionDoneAt *time.Time `json:"inspection_done_at"`
	CustomsCheckedAt *time.Time `json:"customs_checked_at"`
	ShippedAt        *time.Time `json:"shipped_at"`
	Carrier          string     `json:"carrier"`
	TrackingNumber   string     `json:"tracking_number"`
	DeliveredAt      *time.Time `json:"delivered_at"`
	CompletedAt      *time.Time `json:"completed_at"`
	CanceledAt       *time.Time `json:"canceled_at"`
	CancelReason     string     `json:"cancel_reason"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	Lines            []Line     `json:"lines"`
	Media            []Media    `json:"inspection_media"`
	History          []History  `json:"history,omitempty"` // staff view only
}

// Shipping is the delivery address with the customs ID.
type Shipping struct {
	Recipient string `json:"recipient"`
	Phone     string `json:"phone"`
	Postcode  string `json:"postcode"`
	Address1  string `json:"address1"`
	Address2  string `json:"address2"`
	Country   string `json:"country"`
	// PCCC is Korea's personal customs clearance code (P + 12 digits), needed
	// to clear goods bought from abroad.
	PCCC string `json:"pccc"`
}

// Line is a snapshot of one bought item.
type Line struct {
	ID           uuid.UUID `json:"id"`
	ProductID    uuid.UUID `json:"product_id"`
	VariantID    uuid.UUID `json:"variant_id"`
	Title        string    `json:"title"`
	VariantLabel string    `json:"variant_label"`
	PhotoKey     string    `json:"-"`
	PhotoURL     string    `json:"photo_url"`
	UnitPriceWon int64     `json:"unit_price_won"`
	Qty          int       `json:"qty"`
}

// Media is an inspection photo or video of this order.
type Media struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	ContentType string    `json:"content_type"`
	URL         string    `json:"url"`
	CreatedAt   time.Time `json:"created_at"`
	ObjectKey   string    `json:"-"`
}

// History is one status change.
type History struct {
	From      string     `json:"from"`
	To        string     `json:"to"`
	ActorKind string     `json:"actor_kind"`
	ActorID   *uuid.UUID `json:"actor_id"`
	Note      string     `json:"note"`
	At        time.Time  `json:"at"`
}

// Actor is who changed an order.
type Actor struct {
	Kind string // customer, staff, system, payment
	ID   *uuid.UUID
}

// NewOrder is the input from checkout.
type NewOrder struct {
	CustomerID     uuid.UUID
	Lines          []Line
	Shipping       Shipping
	PaymentMethod  string
	Deadline       time.Time
	ShippingFeeWon int64
}

var pcccRe = regexp.MustCompile(`^P[0-9]{12}$`)

// ValidPCCC reports whether s is a well-formed customs code.
func ValidPCCC(s string) bool { return pcccRe.MatchString(s) }

// Normalize validates the address.
func (s *Shipping) Normalize() error {
	trim := func(v *string, max int, name string, required bool) error {
		*v = strings.TrimSpace(*v)
		if (required && *v == "") || len([]rune(*v)) > max {
			return apperr.Invalid("invalid_shipping", "%s is required (up to %d characters)", name, max)
		}
		return nil
	}
	for _, f := range []struct {
		v        *string
		max      int
		name     string
		required bool
	}{
		{&s.Recipient, 100, "recipient", true},
		{&s.Phone, 30, "phone", true},
		{&s.Postcode, 20, "postcode", true},
		{&s.Address1, 200, "address1", true},
		{&s.Address2, 200, "address2", false},
		{&s.Country, 2, "country", false},
		{&s.PCCC, 13, "pccc", false},
	} {
		if err := trim(f.v, f.max, f.name, f.required); err != nil {
			return err
		}
	}
	s.Country = strings.ToUpper(s.Country)
	if s.Country == "" {
		s.Country = "KR"
	}
	s.PCCC = strings.ToUpper(s.PCCC)
	if s.PCCC != "" && !ValidPCCC(s.PCCC) {
		return apperr.Invalid("invalid_pccc", "the customs code is P followed by 12 digits")
	}
	return nil
}

// CustomerCancelable reports whether the customer may cancel on their own.
func (o Order) CustomerCancelable() bool {
	return o.Status == StatusPendingPayment || o.Status == StatusPaid
}

// stockReturnable reports whether canceling puts sold stock back.
func stockReturnable(status string) bool {
	switch status {
	case StatusPaid, StatusInspecting, StatusCustomsCheck:
		return true
	}
	return false
}
