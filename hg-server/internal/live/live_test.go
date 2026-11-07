package live_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/live"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
	"github.com/elug3/h-global/hg-server/internal/platform/events"
	"github.com/elug3/h-global/hg-server/internal/platform/imageutil"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
)

type fixture struct {
	db      *db.DB
	catalog *catalog.Service
	live    *live.Service
	bus     *events.Bus
	dir     string
	staff   uuid.UUID
	now     time.Time
}

func setup(t *testing.T) *fixture {
	d := dbtest.New(t)
	f := &fixture{db: d, dir: t.TempDir(), now: time.Now()}
	f.bus = events.NewBus(d)
	st := storage.Local{Dir: f.dir}
	f.catalog = &catalog.Service{DB: d, Store: catalog.Postgres{DB: d}, MediaBaseURL: "/media"}
	f.live = &live.Service{DB: d, Store: live.Postgres{DB: d}, Catalog: f.catalog, Bus: f.bus,
		Storage: st, StreamDelay: 10 * time.Second, Now: func() time.Time { return f.now }}
	f.catalog.OnChange(f.live.ProductChanged)
	if err := d.Pool.QueryRow(context.Background(),
		`INSERT INTO staff (email, name, role, password_hash) VALUES ('cs@x', 'CS', 'cs', 'h') RETURNING id`).Scan(&f.staff); err != nil {
		t.Fatal(err)
	}
	return f
}

func pngBytes(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 80, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (f *fixture) liveShow(t *testing.T) live.Show {
	ctx := context.Background()
	sh, err := f.live.CreateShow(ctx, f.staff, live.NewShow{Title: "Seoul warehouse tour", YouTubeURL: "https://youtu.be/dQw4w9WgXcQ"})
	if err != nil {
		t.Fatal(err)
	}
	if sh, err = f.live.Start(ctx, sh.ID); err != nil {
		t.Fatal(err)
	}
	return sh
}

func countFiles(t *testing.T, dir string) int {
	n := 0
	_ = filepath.Walk(dir, func(_ string, info os.FileInfo, _ error) error {
		if info != nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

func TestParseYouTubeID(t *testing.T) {
	for in, want := range map[string]string{
		"dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ": "dQw4w9WgXcQ",
		"https://youtu.be/dQw4w9WgXcQ":                "dQw4w9WgXcQ",
		"https://youtube.com/live/dQw4w9WgXcQ?si=abc": "dQw4w9WgXcQ",
		"https://m.youtube.com/watch?v=dQw4w9WgXcQ":   "dQw4w9WgXcQ",
		"": "",
	} {
		got, err := live.ParseYouTubeID(in)
		if err != nil || got != want {
			t.Errorf("%q: got %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"https://example.com/watch?v=dQw4w9WgXcQ", "https://youtu.be/short", "not a link"} {
		if _, err := live.ParseYouTubeID(bad); !apperr.Is(err, "invalid_youtube_url") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestPreselectedNumbersLockAtStart(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sh, _ := f.live.CreateShow(ctx, f.staff, live.NewShow{Title: "Show"})
	var items []live.Item
	for i := 0; i < 3; i++ {
		p, _ := f.catalog.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 1000, Stock: catalog.NewStock{Qty: 1}})
		it, err := f.live.AddPreselected(ctx, sh.ID, p.ID, f.staff)
		if err != nil {
			t.Fatal(err)
		}
		if it.Number != i+1 {
			t.Fatalf("number %d, want %d", it.Number, i+1)
		}
		items = append(items, it)
	}
	got, err := f.live.Reorder(ctx, sh.ID, []uuid.UUID{items[2].ID, items[0].ID, items[1].ID})
	if err != nil {
		t.Fatal(err)
	}
	if got[0].ID != items[2].ID || got[2].ID != items[1].ID {
		t.Fatal("reorder did not apply")
	}
	if err := f.live.RemoveItem(ctx, sh.ID, items[2].ID); err != nil {
		t.Fatal(err)
	}
	studio, _ := f.live.StudioShow(ctx, sh.ID)
	if len(studio.Items) != 2 || studio.Items[0].Number != 1 || studio.Items[1].Number != 2 || studio.NextNumber != 3 {
		t.Fatalf("after remove: %+v next=%d", studio.Items, studio.NextNumber)
	}
	if _, err := f.live.Start(ctx, sh.ID); err != nil {
		t.Fatal(err)
	}
	// Draft products went live with the show.
	studio, _ = f.live.StudioShow(ctx, sh.ID)
	if studio.Items[0].Product.Status != catalog.StatusLive {
		t.Fatalf("preselected product status %s", studio.Items[0].Product.Status)
	}
	if err := f.live.RemoveItem(ctx, sh.ID, items[0].ID); !apperr.Is(err, "show_started") {
		t.Fatalf("remove after start: %v", err)
	}
	other, _ := f.live.CreateShow(ctx, f.staff, live.NewShow{Title: "Second"})
	if _, err := f.live.Start(ctx, other.ID); !apperr.Is(err, "another_show_live") {
		t.Fatalf("second live show: %v", err)
	}
}

func TestQuickListPublishesInOneTransaction(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sh := f.liveShow(t)
	f.now = f.now.Add(42 * time.Second)

	c, err := f.live.SaveCapture(ctx, sh.ID, f.staff, pngBytes(t, 320, 180))
	if err != nil {
		t.Fatal(err)
	}
	if c.BroadcastOffsetSec == nil || *c.BroadcastOffsetSec != 32 {
		t.Fatalf("offset = %v, want 42s - 10s delay", c.BroadcastOffsetSec)
	}
	price := int64(149_000)
	in := live.QuickListInput{
		CaptureID: &c.ID, Crop: &imageutil.Rect{X: 40, Y: 20, W: 160, H: 140}, PriceWon: &price,
		Options:        []catalog.NewOption{{Name: "Color", Values: []catalog.NewOptionValue{{Label: "Brown", Hex1: "#4a3f35"}}}},
		Stock:          catalog.NewStock{Mode: catalog.StockUntilSoldOut},
		IdempotencyKey: "tap-1",
	}
	it, err := f.live.QuickList(ctx, sh.ID, f.staff, in)
	if err != nil {
		t.Fatal(err)
	}
	if it.Number != 1 || it.Source != live.SourceCaptured || it.Product.Title != "Live #1" || it.Product.Status != catalog.StatusLive {
		t.Fatalf("item: number=%d source=%s title=%q status=%s", it.Number, it.Source, it.Product.Title, it.Product.Status)
	}
	if len(it.Product.Media) != 1 || it.Product.Media[0].URL == "" {
		t.Fatalf("photo: %+v", it.Product.Media)
	}
	if it.ReplayURL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=32s" {
		t.Fatalf("replay url %q", it.ReplayURL)
	}

	// Same Idempotency-Key: the same item, no second product, no extra file.
	files := countFiles(t, f.dir)
	again, err := f.live.QuickList(ctx, sh.ID, f.staff, in)
	if err != nil || again.ID != it.ID {
		t.Fatalf("idempotent retry: %v %v", again.ID, err)
	}
	if countFiles(t, f.dir) != files {
		t.Fatal("retry left an extra file")
	}

	// A failing listing leaves nothing behind and does not use a number.
	bad := in
	bad.IdempotencyKey = "tap-2"
	bad.Options = []catalog.NewOption{{Name: "Color", Values: []catalog.NewOptionValue{{Label: "Red", Hex1: "nope"}}}}
	if _, err := f.live.QuickList(ctx, sh.ID, f.staff, bad); !apperr.Is(err, "invalid_color") {
		t.Fatalf("bad listing: %v", err)
	}
	if countFiles(t, f.dir) != files {
		t.Fatal("failed listing left its photo")
	}
	studio, _ := f.live.StudioShow(ctx, sh.ID)
	if len(studio.Items) != 1 || studio.NextNumber != 2 {
		t.Fatalf("after failure: items=%d next=%d", len(studio.Items), studio.NextNumber)
	}

	// Viewers got item_published.
	evs, _ := f.bus.Since(ctx, live.Topic(sh.ID), 0, 100)
	types := map[string]int{}
	for _, e := range evs {
		types[e.Type]++
	}
	if types[live.EvShowStarted] != 1 || types[live.EvItemPublished] != 1 {
		t.Fatalf("events: %v", types)
	}

	// Matched existing product, with a price change; listing it twice fails.
	p, _ := f.catalog.Create(ctx, catalog.NewProduct{Title: "Card wallet", PriceWon: 49_000, Stock: catalog.NewStock{Qty: 3}})
	newPrice := int64(45_000)
	m, err := f.live.QuickList(ctx, sh.ID, f.staff, live.QuickListInput{ProductID: &p.ID, PriceWon: &newPrice})
	if err != nil {
		t.Fatal(err)
	}
	if m.Number != 2 || m.Source != live.SourceMatched || m.Product.PriceWon != 45_000 || m.Product.Status != catalog.StatusLive {
		t.Fatalf("matched: %+v", m.Item)
	}
	if _, err := f.live.QuickList(ctx, sh.ID, f.staff, live.QuickListInput{ProductID: &p.ID}); !apperr.Is(err, "already_in_show") {
		t.Fatalf("listed twice: %v", err)
	}
}

func TestStockChangesReachViewersAndPin(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sh := f.liveShow(t)
	p, _ := f.catalog.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 1000, Status: catalog.StatusLive, Stock: catalog.NewStock{Qty: 2}})
	it, err := f.live.QuickList(ctx, sh.ID, f.staff, live.QuickListInput{ProductID: &p.ID})
	if err != nil {
		t.Fatal(err)
	}
	view, _ := f.live.PublicShow(ctx, sh.ID)
	last := view.LastEventID

	if err := f.catalog.Hold(ctx, p.Variants[0].ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := f.live.Pin(ctx, it.ID, true); err != nil {
		t.Fatal(err)
	}
	evs, _ := f.bus.Since(ctx, live.Topic(sh.ID), last, 100)
	if len(evs) != 2 || evs[0].Type != live.EvItemUpdated || evs[1].Type != live.EvItemPinned {
		t.Fatalf("events after snapshot: %+v", evs)
	}
	view, _ = f.live.PublicShow(ctx, sh.ID)
	if !view.Items[0].Pinned || *view.Items[0].Product.Variants[0].AvailableQty != 1 {
		t.Fatalf("public view: %+v", view.Items[0])
	}
}

func TestEndAndHideDue(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	sh := f.liveShow(t)
	var listed []live.StudioItem
	for i := 0; i < 2; i++ {
		p, _ := f.catalog.Create(ctx, catalog.NewProduct{Title: "Bag", PriceWon: 1000, Status: catalog.StatusLive, Stock: catalog.NewStock{Qty: 1}})
		it, err := f.live.QuickList(ctx, sh.ID, f.staff, live.QuickListInput{ProductID: &p.ID})
		if err != nil {
			t.Fatal(err)
		}
		listed = append(listed, it)
	}
	if _, err := f.live.SetKeepListed(ctx, listed[1].ID, true); err != nil {
		t.Fatal(err)
	}
	// End the show with zero display days, so items are due at once.
	zero := 0
	if _, err := f.live.UpdateShow(ctx, sh.ID, live.ShowPatch{DisplayDaysAfter: &zero}); err != nil {
		t.Fatal(err)
	}
	f.now = time.Now().Add(-time.Second)
	if _, err := f.live.End(ctx, sh.ID); err != nil {
		t.Fatal(err)
	}
	n, err := f.live.HideDue(ctx)
	if err != nil || n != 1 {
		t.Fatalf("hidden %d, %v", n, err)
	}
	if n, _ := f.live.HideDue(ctx); n != 0 {
		t.Fatal("HideDue is not idempotent")
	}
	p0, _ := f.catalog.Get(ctx, listed[0].ProductID)
	p1, _ := f.catalog.Get(ctx, listed[1].ProductID)
	if p0.Status != catalog.StatusHidden || p1.Status != catalog.StatusLive {
		t.Fatalf("statuses: %s %s", p0.Status, p1.Status)
	}
	b, _ := f.live.Broadcasts(ctx, listed[0].ProductID)
	if len(b) != 1 || b[0].Number != 1 {
		t.Fatalf("broadcasts: %+v", b)
	}
}
