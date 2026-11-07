package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

// FakeSignatureHeader carries the fake provider's HMAC-SHA256 signature.
const FakeSignatureHeader = "X-Fake-Signature"

// Fake is a development provider. Payments stay pending until a signed
// webhook arrives; the dev routes (/api/dev/payments/{id}/approve and /fail)
// send one, standing in for the customer paying at the provider.
type Fake struct {
	Secret string
}

// Name implements Provider.
func (Fake) Name() string { return "fake" }

// Create implements Provider.
func (Fake) Create(_ context.Context, p Payment, dueAt time.Time) (Created, error) {
	c := Created{Ref: "fake_" + p.ID.String()}
	if p.Method == MethodVirtualAccount {
		c.VirtualAccount = &VirtualAccount{Bank: "Fake Bank", Account: "000-000-" + p.ID.String()[:8], Holder: "H-Global", DueAt: dueAt}
	} else {
		c.RedirectURL = "/api/dev/payments/" + p.ID.String() + "/approve"
	}
	return c, nil
}

type fakeWebhook struct {
	EventID   string `json:"event_id"`
	Ref       string `json:"ref"`
	Type      string `json:"type"`
	AmountWon int64  `json:"amount_won"`
	Reason    string `json:"reason"`
}

// Sign returns the signature for body.
func (f Fake) Sign(body []byte) string {
	m := hmac.New(sha256.New, []byte(f.Secret))
	m.Write(body)
	return hex.EncodeToString(m.Sum(nil))
}

// Webhook builds a signed webhook body for a payment (used by the dev routes).
func (f Fake) Webhook(p Payment, typ, reason string) ([]byte, string) {
	body, _ := json.Marshal(fakeWebhook{EventID: ids.New().String(), Ref: p.ProviderRef, Type: typ, AmountWon: p.AmountWon, Reason: reason})
	return body, f.Sign(body)
}

// ParseWebhook implements Provider.
func (f Fake) ParseWebhook(h http.Header, body []byte) (WebhookEvent, error) {
	got, err := hex.DecodeString(h.Get(FakeSignatureHeader))
	want, _ := hex.DecodeString(f.Sign(body))
	if err != nil || !hmac.Equal(got, want) {
		return WebhookEvent{}, apperr.Unauthorized("invalid_signature", "webhook signature does not match")
	}
	var w fakeWebhook
	if err := json.Unmarshal(body, &w); err != nil || w.EventID == "" || w.Ref == "" {
		return WebhookEvent{}, apperr.Invalid("invalid_webhook", "webhook body is not valid")
	}
	return WebhookEvent{EventID: w.EventID, Ref: w.Ref, Type: w.Type, AmountWon: w.AmountWon, Reason: w.Reason}, nil
}

// Refund implements Provider.
func (Fake) Refund(_ context.Context, _ Payment, _ int64, _ string) (string, error) {
	return "fake_refund_" + ids.New().String(), nil
}

// Cancel implements Provider.
func (Fake) Cancel(context.Context, Payment) error { return nil }
