package checkout_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/app"
	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/checkout"
	"github.com/elug3/h-global/hg-server/internal/order"
	"github.com/elug3/h-global/hg-server/internal/payment"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

type fx struct {
	a        *app.App
	now      time.Time
	customer uuid.UUID
	product  catalog.Product
}

func setup(t *testing.T, qty int) *fx {
	d := dbtest.New(t)
	f := &fx{now: time.Now()}
	f.a = app.New(config.Config{StorageDir: t.TempDir(), MediaBaseURL: "/media", HoldTTL: 10 * time.Minute,
		VirtualAcctTTL: 24 * time.Hour, BcryptCost: 4, FakePaymentSecret: "test"}, d)
	clock := func() time.Time { return f.now }
	f.a.Checkout.Now, f.a.Orders.Now = clock, clock
	ctx := context.Background()
	f.customer = ids.New()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO customers (id, email, password_hash) VALUES ($1, 'c@x', 'h')`, f.customer); err != nil {
		t.Fatal(err)
	}
	var err error
	f.product, err = f.a.Catalog.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 89_000, Status: catalog.StatusLive,
		Stock: catalog.NewStock{Mode: catalog.StockCounted, Qty: qty}})
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fx) available(t *testing.T) int {
	p, err := f.a.Catalog.Get(context.Background(), f.product.ID)
	if err != nil {
		t.Fatal(err)
	}
	return *p.Variants[0].AvailableQty
}

var addr = order.Shipping{Recipient: "Mina", Phone: "010", Postcode: "04524", Address1: "Seoul", PCCC: "P123456789012"}

func (f *fx) checkout(t *testing.T) checkout.Checkout {
	c, err := f.a.Checkout.Start(context.Background(), f.customer, []checkout.LineInput{{VariantID: f.product.Variants[0].ID, Qty: 1}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *fx) webhook(t *testing.T, p payment.Payment, typ string) ([]byte, http.Header) {
	fake := f.a.Payments.Provider.(payment.Fake)
	body, sig := fake.Webhook(p, typ, "")
	h := http.Header{}
	h.Set(payment.FakeSignatureHeader, sig)
	return body, h
}

func TestHoldExpires(t *testing.T) {
	f := setup(t, 1)
	ctx := context.Background()
	c := f.checkout(t)
	if f.available(t) != 0 {
		t.Fatal("hold not taken")
	}
	f.now = f.now.Add(11 * time.Minute)
	if _, err := f.a.Checkout.SetShipping(ctx, c.ID, f.customer, addr); !apperr.Is(err, "checkout_expired") {
		t.Fatalf("expired checkout accepted shipping: %v", err)
	}
	if f.available(t) != 1 {
		t.Fatal("expired hold not released")
	}
	if n, err := f.a.Checkout.ExpireOpen(ctx); err != nil || n != 0 {
		t.Fatalf("already expired: n=%d err=%v", n, err)
	}
	c2 := f.checkout(t)
	f.now = f.now.Add(11 * time.Minute)
	if n, err := f.a.Checkout.ExpireOpen(ctx); err != nil || n != 1 {
		t.Fatalf("job: n=%d err=%v", n, err)
	}
	if _, err := f.a.Checkout.Pay(ctx, c2.ID, f.customer, "card"); !apperr.Is(err, "checkout_closed") {
		t.Fatalf("pay after expiry: %v", err)
	}
}

func TestUnpaidOrderExpiresAndLatePaymentIsRefunded(t *testing.T) {
	f := setup(t, 1)
	ctx := context.Background()
	c := f.checkout(t)
	if _, err := f.a.Checkout.SetShipping(ctx, c.ID, f.customer, addr); err != nil {
		t.Fatal(err)
	}
	res, err := f.a.Checkout.Pay(ctx, c.ID, f.customer, "card")
	if err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(20 * time.Minute)
	if n, err := f.a.Orders.ExpireUnpaid(ctx); err != nil || n != 1 {
		t.Fatalf("expire unpaid: n=%d err=%v", n, err)
	}
	if f.available(t) != 1 {
		t.Fatal("canceled order kept its hold")
	}
	// The customer's card payment lands after the cancel: refunded automatically.
	body, h := f.webhook(t, res.Payment, payment.EventSucceeded)
	if err := f.a.Payments.HandleWebhook(ctx, h, body); err != nil {
		t.Fatal(err)
	}
	p, _ := f.a.Payments.Get(ctx, res.Payment.ID)
	if p.Status != payment.StatusSucceeded || p.RefundedWon != p.AmountWon {
		t.Fatalf("late payment: status=%s refunded=%d of %d", p.Status, p.RefundedWon, p.AmountWon)
	}
	o, _ := f.a.Orders.Get(ctx, res.Order.ID)
	if o.Status != order.StatusCanceled {
		t.Fatalf("order status %s", o.Status)
	}
}

func TestWebhooksAreVerifiedAndIdempotent(t *testing.T) {
	f := setup(t, 2)
	ctx := context.Background()
	c := f.checkout(t)
	_, _ = f.a.Checkout.SetShipping(ctx, c.ID, f.customer, addr)
	res, err := f.a.Checkout.Pay(ctx, c.ID, f.customer, "virtual_account")
	if err != nil {
		t.Fatal(err)
	}
	if res.Payment.VirtualAccount == nil || res.Payment.RedirectURL != "" {
		t.Fatalf("virtual account payment: %+v", res.Payment)
	}
	if got := res.Order.PaymentDeadline.Sub(f.now); got < 23*time.Hour {
		t.Fatalf("virtual account deadline in %s", got)
	}
	body, h := f.webhook(t, res.Payment, payment.EventSucceeded)
	bad := http.Header{}
	bad.Set(payment.FakeSignatureHeader, "00")
	if err := f.a.Payments.HandleWebhook(ctx, bad, body); !apperr.Is(err, "invalid_signature") {
		t.Fatalf("forged webhook: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := f.a.Payments.HandleWebhook(ctx, h, body); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}
	p, _ := f.a.Catalog.Get(ctx, f.product.ID)
	if v := p.Variants[0]; v.SoldQty != 1 || v.HeldQty != 0 {
		t.Fatalf("after repeated webhook: sold=%d held=%d", v.SoldQty, v.HeldQty)
	}
	o, _ := f.a.Orders.Get(ctx, res.Order.ID)
	if o.Status != order.StatusPaid {
		t.Fatalf("order %s", o.Status)
	}
}
