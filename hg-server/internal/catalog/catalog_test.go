package catalog_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
)

func newService(t *testing.T) *catalog.Service {
	d := dbtest.New(t)
	return &catalog.Service{DB: d, Store: catalog.Postgres{DB: d}, MediaBaseURL: "/media"}
}

func TestCreateMakesEveryCombination(t *testing.T) {
	svc := newService(t)
	p, err := svc.Create(context.Background(), catalog.NewProduct{
		Title: "Woven shoulder bag", PriceWon: 149_000, Status: catalog.StatusLive,
		Options: []catalog.NewOption{
			{Name: "Color", Values: []catalog.NewOptionValue{{Label: "Brown", Hex1: "#4A3F35"}, {Label: "Ivory / Black", Hex1: "#f3ede1", Hex2: "#111111"}}},
			{Name: "Size", Values: []catalog.NewOptionValue{{Label: "Mini"}, {Label: "Small"}, {Label: "Medium"}}},
		},
		Stock: catalog.NewStock{Mode: catalog.StockCounted, Qty: 2},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Variants) != 6 {
		t.Fatalf("variants = %d, want 6", len(p.Variants))
	}
	if p.Variants[0].Label != "Brown / Mini" || p.Variants[5].Label != "Ivory / Black / Medium" {
		t.Fatalf("labels: %q .. %q", p.Variants[0].Label, p.Variants[5].Label)
	}
	if h := p.Options[0].Values[0].Hex1; h == nil || *h != "#4a3f35" {
		t.Fatalf("hex not normalized: %v", h)
	}
	for _, v := range p.Variants {
		if v.PriceWon != 149_000 || !v.Buyable || v.AvailableQty == nil || *v.AvailableQty != 2 {
			t.Fatalf("variant %+v", v)
		}
	}
	if p.DetailsComplete {
		t.Fatal("no description yet, details should be incomplete")
	}
}

func TestCreateValidation(t *testing.T) {
	svc := newService(t)
	cases := map[string]catalog.NewProduct{
		"invalid_title":     {PriceWon: 1},
		"invalid_price":     {Title: "x", PriceWon: -1},
		"invalid_color":     {Title: "x", Options: []catalog.NewOption{{Name: "Color", Values: []catalog.NewOptionValue{{Label: "Red", Hex1: "red"}}}}},
		"invalid_options":   {Title: "x", Options: []catalog.NewOption{{Name: "Size", Values: []catalog.NewOptionValue{{Label: "S"}, {Label: "s"}}}}},
		"too_many_variants": {Title: "x", Options: []catalog.NewOption{{Name: "A", Values: vals(11)}, {Name: "B", Values: vals(10)}}},
	}
	for code, in := range cases {
		if _, err := svc.Create(context.Background(), in); !apperr.Is(err, code) {
			t.Errorf("%s: got %v", code, err)
		}
	}
}

func vals(n int) []catalog.NewOptionValue {
	out := make([]catalog.NewOptionValue, n)
	for i := range out {
		out[i].Label = string(rune('a' + i))
	}
	return out
}

func TestHoldNeverOversells(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	p, err := svc.Create(ctx, catalog.NewProduct{Title: "One-of-a-kind bag", PriceWon: 89_000,
		Status: catalog.StatusLive, Stock: catalog.NewStock{Mode: catalog.StockCounted, Qty: 1}})
	if err != nil {
		t.Fatal(err)
	}
	vid := p.Variants[0].ID

	var wins, soldOut atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := svc.Hold(ctx, vid, 1)
			switch {
			case err == nil:
				wins.Add(1)
			case apperr.Is(err, "sold_out"):
				soldOut.Add(1)
			default:
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if wins.Load() != 1 || soldOut.Load() != 19 {
		t.Fatalf("wins=%d sold_out=%d, want 1 and 19", wins.Load(), soldOut.Load())
	}
	got, _ := svc.Get(ctx, p.ID)
	if v := got.Variants[0]; v.HeldQty != 1 || *v.AvailableQty != 0 || v.Buyable {
		t.Fatalf("after hold: %+v", v)
	}
}

func TestStockBookkeeping(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, catalog.NewProduct{Title: "Toner", PriceWon: 28_000, Status: catalog.StatusLive,
		Stock: catalog.NewStock{Mode: catalog.StockCounted, Qty: 3}})
	vid := p.Variants[0].ID
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(svc.Hold(ctx, vid, 2))
	must(svc.CommitHold(ctx, vid, 2))
	must(svc.Hold(ctx, vid, 1))
	must(svc.Release(ctx, vid, 1))
	must(svc.ReturnSold(ctx, vid, 1))
	got, _ := svc.Get(ctx, p.ID)
	v := got.Variants[0]
	if v.HeldQty != 0 || v.SoldQty != 1 || *v.AvailableQty != 2 {
		t.Fatalf("held=%d sold=%d available=%d", v.HeldQty, v.SoldQty, *v.AvailableQty)
	}
	if err := svc.Release(ctx, vid, 1); !apperr.Is(err, "stock_mismatch") {
		t.Fatalf("releasing more than held: %v", err)
	}
	low := 0
	if _, err := svc.UpdateVariant(ctx, p.ID, vid, catalog.VariantPatch{StockQty: &low}); !apperr.Is(err, "stock_below_committed") {
		t.Fatalf("stock below sold: %v", err)
	}
}

func TestUntilSoldOutAndHiddenProducts(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	p, _ := svc.Create(ctx, catalog.NewProduct{Title: "Night serum", PriceWon: 35_000, Status: catalog.StatusLive,
		Stock: catalog.NewStock{Mode: catalog.StockUntilSoldOut}})
	vid := p.Variants[0].ID
	for i := 0; i < 5; i++ {
		if err := svc.Hold(ctx, vid, 3); err != nil {
			t.Fatalf("hold %d: %v", i, err)
		}
	}
	if _, err := svc.MarkSoldOut(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Hold(ctx, vid, 1); !apperr.Is(err, "sold_out") {
		t.Fatalf("after sold out: %v", err)
	}
	other, _ := svc.Create(ctx, catalog.NewProduct{Title: "Draft bag", PriceWon: 1, Stock: catalog.NewStock{Qty: 5}})
	if err := svc.Hold(ctx, other.Variants[0].ID, 1); !apperr.Is(err, "sold_out") {
		t.Fatalf("draft product must not be buyable: %v", err)
	}
	if err := svc.Hold(ctx, uuid.New(), 1); !apperr.Is(err, "variant_not_found") {
		t.Fatalf("unknown variant: %v", err)
	}
}

func TestSuggestionsAndListeners(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	var changed []uuid.UUID
	svc.OnChange(func(_ context.Context, id uuid.UUID) error { changed = append(changed, id); return nil })
	for i := 0; i < 3; i++ {
		_, err := svc.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 1,
			Options: []catalog.NewOption{{Name: "Color", Values: []catalog.NewOptionValue{{Label: "Brown", Hex1: "#4a3f35"}, {Label: "Black"}}}}})
		if err != nil {
			t.Fatal(err)
		}
	}
	_, _ = svc.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 1,
		Options: []catalog.NewOption{{Name: "color", Values: []catalog.NewOptionValue{{Label: "brown", Hex1: "#4a3f36"}}}}})
	list, err := svc.Suggestions(ctx, "COLOR")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].Uses != 4 || list[0].Label != "brown" {
		t.Fatalf("suggestions: %+v", list)
	}
	if len(changed) != 4 {
		t.Fatalf("listener calls = %d, want 4", len(changed))
	}
}
