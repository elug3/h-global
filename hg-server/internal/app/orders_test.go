package app_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/elug3/h-global/hg-server/internal/account"
	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/order"
)

var buyers int

// placeOrder checks out one line as a new customer and, if pay, approves the
// card payment. It returns the order.
func placeOrder(t *testing.T, base, variant string, qty int, recipient, phone string, pay bool) obj {
	t.Helper()
	buyer := newClient(t, base)
	buyers++
	buyer.do("POST", "/api/auth/register", obj{"email": fmt.Sprintf("buyer%d@example.com", buyers), "password": "buyer-password"}, 201, nil)
	var co obj
	buyer.do("POST", "/api/checkout", obj{"lines": []obj{{"variant_id": variant, "qty": qty}}}, 201, &co)
	buyer.do("PUT", "/api/checkout/"+co["id"].(string)+"/shipping", obj{
		"recipient": recipient, "phone": phone, "postcode": "04524", "address1": "Seoul", "pccc": "P123456789012"}, 200, nil)
	var res obj
	buyer.do("POST", "/api/checkout/"+co["id"].(string)+"/pay", obj{"method": "card"}, 201, &res)
	if pay {
		buyer.do("POST", res["payment"].(obj)["redirect_url"].(string), nil, 200, nil)
	}
	return res["order"].(obj)
}

func TestOrderSearchCountsAndShowSales(t *testing.T) {
	_, base := setup(t)
	staff := newClient(t, base)
	staff.do("POST", "/studio-api/auth/login", obj{"email": "owner@hglobal.test", "password": "owner-password"}, 200, nil)

	var bag, scarf obj
	staff.do("POST", "/studio-api/products", obj{"title": "Bag", "price_won": 100000, "status": "live", "stock": obj{"mode": "counted", "qty": 20}}, 201, &bag)
	staff.do("POST", "/studio-api/products", obj{"title": "Scarf", "price_won": 30000, "status": "live", "stock": obj{"mode": "counted", "qty": 20}}, 201, &scarf)
	bagV := bag["variants"].([]any)[0].(obj)["id"].(string)
	scarfV := scarf["variants"].([]any)[0].(obj)["id"].(string)

	newShow := func(title string, products ...obj) (string, []string) {
		var sh obj
		staff.do("POST", "/studio-api/shows", obj{"title": title}, 201, &sh)
		id := sh["id"].(string)
		var items []string
		for _, p := range products {
			var it obj
			staff.do("POST", "/studio-api/shows/"+id+"/items", obj{"product_id": p["id"]}, 201, &it)
			items = append(items, it["id"].(string))
		}
		return id, items
	}

	// Before the first stream starts: does not count for it.
	placeOrder(t, base, bagV, 1, "Early", "010-0000-0001", true)

	show1, items1 := newShow("First", bag, scarf)
	staff.do("POST", "/studio-api/shows/"+show1+"/start", nil, 200, nil)
	mina := placeOrder(t, base, bagV, 2, "Kim Mina", "010-1234-5678", true)
	placeOrder(t, base, scarfV, 1, "Lee Jun", "010-2222-3333", true)
	placeOrder(t, base, bagV, 1, "Unpaid", "010-4444-5555", false)
	refunded := placeOrder(t, base, scarfV, 1, "Refund", "010-6666-7777", true)
	staff.do("POST", "/studio-api/orders/"+refunded["id"].(string)+"/cancel", obj{"reason": "test"}, 200, nil)
	staff.do("POST", "/studio-api/shows/"+show1+"/end", nil, 200, nil)

	// The bag is shown again: its later orders count for the second stream.
	show2, _ := newShow("Second", bag)
	staff.do("POST", "/studio-api/shows/"+show2+"/start", nil, 200, nil)
	placeOrder(t, base, bagV, 1, "Later", "010-8888-9999", true)
	placeOrder(t, base, scarfV, 3, "Scarf later", "010-1111-2222", true) // scarf still counts for the first

	var s1 obj
	staff.do("GET", "/studio-api/shows/"+show1+"/sales", nil, 200, &s1)
	want := obj{"orders": 3.0, "units": 6.0, "sales_won": 320000.0, "pending_orders": 1.0, "canceled_orders": 1.0}
	for k, v := range want {
		if s1[k] != v {
			t.Errorf("show 1 %s = %v, want %v (%v)", k, s1[k], v, s1)
		}
	}
	per := map[string]obj{}
	for _, it := range s1["items"].([]any) {
		per[it.(obj)["item_id"].(string)] = it.(obj)
	}
	if b := per[items1[0]]; b == nil || b["units"] != 2.0 || b["sales_won"] != 200000.0 || b["orders"] != 1.0 {
		t.Errorf("bag in show 1: %v", b)
	}
	if sc := per[items1[1]]; sc == nil || sc["units"] != 4.0 || sc["orders"] != 2.0 || sc["sales_won"] != 120000.0 {
		t.Errorf("scarf in show 1: %v", sc)
	}
	var s2 obj
	staff.do("GET", "/studio-api/shows/"+show2+"/sales", nil, 200, &s2)
	if s2["orders"] != 1.0 || s2["sales_won"] != 100000.0 {
		t.Errorf("show 2: %v", s2)
	}
	staff.do("GET", "/studio-api/shows/00000000-0000-7000-8000-000000000000/sales", nil, 404, nil)

	var counts struct {
		Counts map[string]float64 `json:"counts"`
	}
	staff.do("GET", "/studio-api/orders/counts", nil, 200, &counts)
	if counts.Counts["paid"] != 5 || counts.Counts["pending_payment"] != 1 || counts.Counts["canceled"] != 1 || counts.Counts["shipped"] != 0 {
		t.Errorf("counts: %v", counts.Counts)
	}

	search := func(q string, status string) []any {
		var res obj
		staff.do("GET", "/studio-api/orders?q="+url.QueryEscape(q)+"&status="+status, nil, 200, &res)
		return res["items"].([]any)
	}
	if r := search(mina["number"].(string), ""); len(r) != 1 || r[0].(obj)["id"] != mina["id"] {
		t.Errorf("by number: %v", r)
	}
	if r := search("mina", ""); len(r) != 1 {
		t.Errorf("by recipient: %d", len(r))
	}
	if r := search("12345678", ""); len(r) != 1 {
		t.Errorf("by phone digits: %d", len(r))
	}
	if r := search("mina", "canceled"); len(r) != 0 {
		t.Errorf("status and q: %d", len(r))
	}
	if r := search("%", ""); len(r) != 0 {
		t.Errorf("%% must be literal: %d", len(r))
	}
	if r := search("", "paid,pending_payment"); len(r) != 6 {
		t.Errorf("by status: %d", len(r))
	}
}

func TestOrderNumbers(t *testing.T) {
	a, base := setup(t)
	ctx := context.Background()
	cust, err := a.Account.Register(ctx, account.NewCustomer{Email: "n@example.com", Password: "n-password"})
	if err != nil {
		t.Fatal(err)
	}
	p, err := a.Catalog.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 1000, Stock: catalog.NewStock{Mode: "counted", Qty: 100}})
	if err != nil {
		t.Fatal(err)
	}
	newOrder := func() order.NewOrder {
		return order.NewOrder{
			CustomerID:    cust.ID,
			Lines:         []order.Line{{ProductID: p.ID, VariantID: p.Variants[0].ID, Title: "Bag", UnitPriceWon: 1000, Qty: 1}},
			Shipping:      order.Shipping{Recipient: "N", Phone: "010", Postcode: "1", Address1: "Seoul"},
			PaymentMethod: "card", Deadline: time.Now().Add(time.Hour),
		}
	}
	today := time.Now().In(time.FixedZone("KST", 9*3600)).Format("060102") // Korea has no daylight saving
	format := regexp.MustCompile(`^HG-` + today + `-[23456789BCDFGHJKMNPQRSTVWXZ]{5}$`)

	// Twenty orders at once: Korean date, five random characters, all different.
	const n = 20
	numbers := make([]string, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			o, err := a.Orders.Create(ctx, newOrder())
			numbers[i], errs[i] = o.Number, err
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, num := range numbers {
		if !format.MatchString(num) || seen[num] {
			t.Fatalf("number %q: bad format or repeated (all: %v)", num, numbers)
		}
		seen[num] = true
	}

	// A taken code is retried with a new one; if every try is taken, the order fails.
	codes := []string{"BBBBB", "BBBBB", "CCCCC"}
	a.Orders.Store.Code = func() string { c := codes[0]; codes = codes[1:]; return c }
	first, err := a.Orders.Create(ctx, newOrder())
	if err != nil || first.Number != "HG-"+today+"-BBBBB" {
		t.Fatalf("first: %s %v", first.Number, err)
	}
	second, err := a.Orders.Create(ctx, newOrder())
	if err != nil || second.Number != "HG-"+today+"-CCCCC" {
		t.Fatalf("after a clash: %s %v", second.Number, err)
	}
	a.Orders.Store.Code = func() string { return "BBBBB" }
	if _, err := a.Orders.Create(ctx, newOrder()); err == nil {
		t.Fatal("every code taken: want an error")
	}
	a.Orders.Store.Code = nil

	// Staff find an order with or without the hyphens, in any case.
	staff := newClient(t, base)
	staff.do("POST", "/studio-api/auth/login", obj{"email": "owner@hglobal.test", "password": "owner-password"}, 200, nil)
	for _, q := range []string{second.Number, strings.ReplaceAll(strings.TrimPrefix(second.Number, "HG-"), "-", ""), "ccccc"} {
		var res obj
		staff.do("GET", "/studio-api/orders?q="+url.QueryEscape(q), nil, 200, &res)
		items := res["items"].([]any)
		if len(items) != 1 || items[0].(obj)["number"] != second.Number {
			t.Errorf("search %q: want only %s, got %d results", q, second.Number, len(items))
		}
	}
}
