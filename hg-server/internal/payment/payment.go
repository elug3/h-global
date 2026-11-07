// Package payment talks to the payment provider through one Provider
// interface. Until a provider is chosen (plan decision D2) the Fake provider
// stands in; a real adapter implements the same four methods.
package payment

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Statuses.
const (
	StatusPending   = "pending"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

// Methods (same values as order).
const (
	MethodCard           = "card"
	MethodVirtualAccount = "virtual_account"
)

// Webhook event types.
const (
	EventSucceeded = "payment.succeeded" // card captured or deposit received
	EventFailed    = "payment.failed"
)

// Payment is one payment attempt for an order.
type Payment struct {
	ID             uuid.UUID       `json:"id"`
	OrderID        uuid.UUID       `json:"order_id"`
	Provider       string          `json:"provider"`
	Method         string          `json:"method"`
	Status         string          `json:"status"`
	AmountWon      int64           `json:"amount_won"`
	RefundedWon    int64           `json:"refunded_won"`
	ProviderRef    string          `json:"provider_ref"`
	RedirectURL    string          `json:"redirect_url"`
	VirtualAccount *VirtualAccount `json:"virtual_account"`
	FailureReason  string          `json:"failure_reason"`
	SucceededAt    *time.Time      `json:"succeeded_at"`
	CreatedAt      time.Time       `json:"created_at"`
}

// VirtualAccount is where a customer deposits.
type VirtualAccount struct {
	Bank    string    `json:"bank"`
	Account string    `json:"account"`
	Holder  string    `json:"holder"`
	DueAt   time.Time `json:"due_at"`
}

// Created is what a provider returns for a new payment.
type Created struct {
	Ref            string
	RedirectURL    string
	VirtualAccount *VirtualAccount
}

// WebhookEvent is a verified provider notification.
type WebhookEvent struct {
	EventID   string
	Ref       string
	Type      string
	AmountWon int64
	Reason    string
}

// Provider is a payment gateway adapter.
type Provider interface {
	Name() string
	// Create starts a payment and returns where to send the customer.
	Create(ctx context.Context, p Payment, dueAt time.Time) (Created, error)
	// ParseWebhook verifies a webhook's signature and decodes it.
	ParseWebhook(h http.Header, body []byte) (WebhookEvent, error)
	// Refund returns money for a succeeded payment.
	Refund(ctx context.Context, p Payment, amountWon int64, reason string) (refundRef string, err error)
	// Cancel stops a pending payment that is no longer wanted.
	Cancel(ctx context.Context, p Payment) error
}

// OrderHook is how payment tells the order module about money. It is an
// interface so the two packages do not import each other.
type OrderHook interface {
	// MarkPaid is called inside the payment's transaction. It returns an
	// error with code "order_not_payable" when the order was canceled
	// meanwhile; the payment is then refunded.
	MarkPaid(ctx context.Context, orderID, paymentID uuid.UUID, amountWon int64) error
}
