package app_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elug3/h-global/hg-server/internal/app"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
)

type client struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, http: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// do sends a JSON request and decodes the JSON answer into out, failing the
// test unless the status matches.
func (c *client) do(method, path string, body any, want int, out any) {
	c.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.send(req, want, out)
}

func (c *client) upload(path string, fields map[string]string, file []byte, want int, out any) {
	c.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = mw.WriteField(k, v)
	}
	fw, _ := mw.CreateFormFile("file", "frame.png")
	_, _ = fw.Write(file)
	_ = mw.Close()
	req, _ := http.NewRequest("POST", c.base+path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.send(req, want, out)
}

func (c *client) send(req *http.Request, want int, out any) {
	c.t.Helper()
	res, err := c.http.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", req.Method, req.URL.Path, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", req.Method, req.URL.Path, res.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode: %v: %s", req.Method, req.URL.Path, err, data)
		}
	}
}

type sseEvent struct {
	Type string
	Data map[string]any
}

// stream opens an event stream and delivers events on a channel.
func (c *client) stream(ctx context.Context, path string) <-chan sseEvent {
	c.t.Helper()
	req, _ := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	h := &http.Client{Jar: c.http.Jar}
	res, err := h.Do(req)
	if err != nil || res.StatusCode != 200 {
		c.t.Fatalf("stream %s: %v %v", path, err, res)
	}
	ch := make(chan sseEvent, 32)
	go func() {
		defer res.Body.Close()
		defer close(ch)
		sc := bufio.NewScanner(res.Body)
		var ev sseEvent
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				ev.Type = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &ev.Data)
			case line == "" && ev.Type != "":
				ch <- ev
				ev = sseEvent{}
			}
		}
	}()
	return ch
}

func waitFor(t *testing.T, ch <-chan sseEvent, typ string) sseEvent {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				t.Fatalf("stream closed before %s", typ)
			}
			if ev.Type == typ {
				return ev
			}
		case <-timeout:
			t.Fatalf("no %s event", typ)
		}
	}
}

func frame(t *testing.T) []byte {
	img := image.NewRGBA(image.Rect(0, 0, 640, 360))
	for x := 0; x < 640; x++ {
		for y := 0; y < 360; y++ {
			img.Set(x, y, color.RGBA{uint8(x / 3), uint8(y / 2), 90, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func setup(t *testing.T) (*app.App, string) {
	d := dbtest.New(t)
	cfg := config.Config{
		StorageDir: t.TempDir(), MediaBaseURL: "/media", CookieSecure: false,
		HoldTTL: 10 * time.Minute, VirtualAcctTTL: 24 * time.Hour, StreamDelay: 0,
		StaffSessionTTL: time.Hour, CustomerSessionTTL: time.Hour, BcryptCost: 4,
		PaymentProvider: "fake", FakePaymentSecret: "test", DevRoutes: true,
	}
	a := app.New(cfg, d)
	if _, err := a.Account.EnsureOwner(context.Background(), "owner@hglobal.test", "owner-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(srv.Close)
	return a, srv.URL
}

type obj = map[string]any

func TestLiveStreamToDelivery(t *testing.T) {
	a, base := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	staff := newClient(t, base)
	staff.do("POST", "/studio-api/auth/login", obj{"email": "owner@hglobal.test", "password": "owner-password"}, 200, nil)

	// 1. Stream setup and start.
	var show obj
	staff.do("POST", "/studio-api/shows", obj{"title": "Seoul warehouse tour", "youtube_url": "https://youtu.be/dQw4w9WgXcQ", "status": "scheduled"}, 201, &show)
	showID := show["id"].(string)
	staff.do("POST", "/studio-api/shows/"+showID+"/start", nil, 200, nil)

	// 2. A viewer opens the live page, then its event stream from the snapshot.
	viewer := newClient(t, base)
	var snap obj
	viewer.do("GET", "/api/shows/current", nil, 200, &snap)
	if snap["show"].(obj)["id"] != showID {
		t.Fatalf("current show: %v", snap)
	}
	last := int64(snap["show"].(obj)["last_event_id"].(float64))
	feed := viewer.stream(ctx, fmt.Sprintf("/api/shows/%s/events?last_event_id=%d", showID, last))
	time.Sleep(50 * time.Millisecond) // let the stream subscribe

	// 3. The CS rep captures a frame and quick-lists a one-of-a-kind bag.
	var capture obj
	staff.upload("/studio-api/shows/"+showID+"/captures", nil, frame(t), 201, &capture)
	var item obj
	staff.do("POST", "/studio-api/shows/"+showID+"/items/quick", obj{
		"capture_id": capture["id"], "crop": obj{"x": 100, "y": 40, "w": 300, "h": 280},
		"price_won": 149000,
		"options":   []obj{{"name": "Color", "values": []obj{{"label": "Brown", "hex1": "#4a3f35"}, {"label": "Black", "hex1": "#111111"}}}},
		"stock":     obj{"mode": "counted", "qty": 1},
	}, 201, &item)
	if item["number"].(float64) != 1 {
		t.Fatalf("item number %v", item["number"])
	}

	ev := waitFor(t, feed, "live.item_published")
	pub := ev.Data["item"].(obj)
	product := pub["product"].(obj)
	if product["title"] != "Live #1" || product["price_won"].(float64) != 149000 || pub["is_new"] != true {
		t.Fatalf("published item: %v", pub)
	}
	photo := product["photos"].([]any)[0].(string)
	res, err := http.Get(base + photo)
	if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("photo %s: %v %v", photo, err, res)
	}
	res.Body.Close()
	brown := product["variants"].([]any)[0].(obj)["id"].(string)

	// 4. Two buyers press Buy now on the same last unit; one gets it.
	buyer := newClient(t, base)
	buyer.do("POST", "/api/auth/register", obj{"email": "mina@example.com", "password": "mina-password", "name": "Mina"}, 201, nil)
	rival := newClient(t, base)
	rival.do("POST", "/api/auth/register", obj{"email": "rival@example.com", "password": "rival-password"}, 201, nil)

	var co obj
	buyer.do("POST", "/api/checkout", obj{"lines": []obj{{"variant_id": brown, "qty": 1}}}, 201, &co)
	rival.do("POST", "/api/checkout", obj{"lines": []obj{{"variant_id": brown, "qty": 1}}}, 409, nil)
	waitFor(t, feed, "live.item_updated") // viewers see it is gone

	// 5. Address with customs code, pay by card, provider approves.
	coID := co["id"].(string)
	buyer.do("PUT", "/api/checkout/"+coID+"/shipping", obj{
		"recipient": "Mina Park", "phone": "010-1234-5678", "postcode": "04524",
		"address1": "110 Sejong-daero, Jung-gu, Seoul", "pccc": "p123456789012",
	}, 200, nil)
	var paid obj
	buyer.do("POST", "/api/checkout/"+coID+"/pay", obj{"method": "card"}, 201, &paid)
	ord := paid["order"].(obj)
	orderID := ord["id"].(string)
	if ord["status"] != "pending_payment" || ord["total_won"].(float64) != 149000 {
		t.Fatalf("order: %v", ord)
	}
	pay := paid["payment"].(obj)
	buyer.do("POST", pay["redirect_url"].(string), nil, 200, nil)
	buyer.do("GET", "/api/orders/"+orderID, nil, 200, &ord)
	if ord["status"] != "paid" {
		t.Fatalf("after payment: %v", ord["status"])
	}

	// 6. Staff inspect, check customs, ship and deliver.
	staffOrders := staff.stream(ctx, "/studio-api/orders/events")
	time.Sleep(50 * time.Millisecond)
	staff.upload("/studio-api/orders/"+orderID+"/inspection", obj2(map[string]string{"kind": "photo"}), frame(t), 201, nil)
	waitFor(t, staffOrders, "order.updated")
	staff.do("POST", "/studio-api/orders/"+orderID+"/ship", obj{"carrier": "cj", "tracking_number": "1234"}, 409, nil) // not yet
	staff.do("POST", "/studio-api/orders/"+orderID+"/inspection/complete", obj{"note": "matches the stream"}, 200, nil)
	staff.do("POST", "/studio-api/orders/"+orderID+"/customs-check", nil, 200, nil)
	staff.do("POST", "/studio-api/orders/"+orderID+"/ship", obj{"carrier": "cj", "tracking_number": "1234-5678"}, 200, nil)
	staff.do("POST", "/studio-api/orders/"+orderID+"/deliver", nil, 200, nil)

	// 7. The buyer sees the inspection photo and confirms receipt.
	buyer.do("GET", "/api/orders/"+orderID, nil, 200, &ord)
	media := ord["inspection_media"].([]any)
	if len(media) != 1 {
		t.Fatalf("inspection media: %v", media)
	}
	buyer.do("GET", media[0].(obj)["url"].(string), nil, 200, nil)
	rival.do("GET", "/api/orders/"+orderID, nil, 404, nil) // not theirs
	buyer.do("POST", "/api/orders/"+orderID+"/receipt", nil, 200, &ord)
	if ord["status"] != "completed" {
		t.Fatalf("final status %v", ord["status"])
	}

	var staffView obj
	staff.do("GET", "/studio-api/orders/"+orderID, nil, 200, &staffView)
	if n := len(staffView["history"].([]any)); n < 7 {
		t.Fatalf("history entries: %d", n)
	}

	// 8. After the stream, the product page links back to the moment.
	staff.do("POST", "/studio-api/shows/"+showID+"/end", nil, 200, nil)
	var bc obj
	viewer.do("GET", "/api/products/"+product["id"].(string)+"/broadcasts", nil, 200, &bc)
	if !strings.HasPrefix(bc["items"].([]any)[0].(obj)["replay_url"].(string), "https://www.youtube.com/watch?v=dQw4w9WgXcQ&t=") {
		t.Fatalf("broadcasts: %v", bc)
	}
	viewer.do("GET", "/api/shows/"+showID+"/recap", nil, 200, nil)
	_ = a
}

func obj2(m map[string]string) map[string]string { return m }

func TestPaidCancelRefundsAndReturnsStock(t *testing.T) {
	_, base := setup(t)
	staff := newClient(t, base)
	staff.do("POST", "/studio-api/auth/login", obj{"email": "owner@hglobal.test", "password": "owner-password"}, 200, nil)
	var p obj
	staff.do("POST", "/studio-api/products", obj{"title": "Card wallet", "price_won": 49000, "status": "live",
		"description": "Leather", "stock": obj{"mode": "counted", "qty": 2}}, 201, &p)
	variant := p["variants"].([]any)[0].(obj)["id"].(string)

	buyer := newClient(t, base)
	buyer.do("POST", "/api/auth/register", obj{"email": "b@example.com", "password": "b-password"}, 201, nil)
	var co obj
	buyer.do("POST", "/api/checkout", obj{"lines": []obj{{"variant_id": variant, "qty": 2}}}, 201, &co)
	buyer.do("PUT", "/api/checkout/"+co["id"].(string)+"/shipping", obj{
		"recipient": "B", "phone": "010", "postcode": "1", "address1": "Seoul"}, 200, nil)
	var paid obj
	buyer.do("POST", "/api/checkout/"+co["id"].(string)+"/pay", obj{"method": "card"}, 201, &paid)
	buyer.do("POST", paid["payment"].(obj)["redirect_url"].(string), nil, 200, nil)
	orderID := paid["order"].(obj)["id"].(string)

	var ord obj
	buyer.do("POST", "/api/orders/"+orderID+"/cancel", obj{"reason": "changed my mind"}, 200, &ord)
	if ord["status"] != "canceled" || ord["refunded_won"].(float64) != 98000 {
		t.Fatalf("canceled order: status=%v refunded=%v", ord["status"], ord["refunded_won"])
	}
	staff.do("GET", "/studio-api/products/"+p["id"].(string), nil, 200, &p)
	v := p["variants"].([]any)[0].(obj)
	if v["sold_qty"].(float64) != 0 || v["available_qty"].(float64) != 2 {
		t.Fatalf("stock after cancel: %v", v)
	}
	buyer.do("POST", "/api/orders/"+orderID+"/cancel", nil, 409, nil) // twice
}

func TestAccessRules(t *testing.T) {
	_, base := setup(t)
	anon := newClient(t, base)
	anon.do("GET", "/studio-api/products", nil, 401, nil)
	anon.do("POST", "/api/checkout", obj{"lines": []obj{}}, 401, nil)

	customer := newClient(t, base)
	customer.do("POST", "/api/auth/register", obj{"email": "c@example.com", "password": "c-password"}, 201, nil)
	customer.do("GET", "/studio-api/products", nil, 401, nil) // a customer session is not a staff session

	owner := newClient(t, base)
	owner.do("POST", "/studio-api/auth/login", obj{"email": "owner@hglobal.test", "password": "owner-password"}, 200, nil)
	owner.do("POST", "/studio-api/staff", obj{"email": "cs@hglobal.test", "name": "Jiwoo", "role": "cs", "password": "cs-password"}, 201, nil)
	cs := newClient(t, base)
	cs.do("POST", "/studio-api/auth/login", obj{"email": "cs@hglobal.test", "password": "cs-password"}, 200, nil)
	cs.do("GET", "/studio-api/products", nil, 200, nil)
	cs.do("POST", "/studio-api/shows", obj{"title": "x"}, 403, nil) // stream setup needs manager
	cs.do("GET", "/studio-api/staff", nil, 403, nil)
	anon.do("POST", "/studio-api/auth/login", obj{"email": "cs@hglobal.test", "password": "wrong-password"}, 401, nil)

	req, _ := http.NewRequest("POST", base+"/api/auth/login", strings.NewReader(`{"email":"c@example.com","password":"c-password"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	anon.send(req, 403, nil)
}
