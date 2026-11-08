package app_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elug3/h-global/hg-server/internal/app"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
	"github.com/elug3/h-global/hg-server/internal/platform/dbtest"
)

func udpPort(t *testing.T) int {
	c, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).Port
}

// TestRelayToQuickListing drives the broadcast relay through hg-server: a
// live show opens the SRT listener, a test broadcast arrives, Studio reads
// frames, captures one from the buffer and lists a product from it.
func TestRelayToQuickListing(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	d := dbtest.New(t)
	port := udpPort(t)
	cfg := config.Config{
		StorageDir: t.TempDir(), MediaBaseURL: "/media", HoldTTL: 10 * time.Minute, BcryptCost: 4,
		StaffSessionTTL: time.Hour, CustomerSessionTTL: time.Hour, FakePaymentSecret: "test",
		Relay: config.Relay{Enabled: true, Listen: fmt.Sprintf("127.0.0.1:%d", port),
			PublicHost: fmt.Sprintf("ingest.test:%d", port), Secret: "a-long-relay-secret", FFmpeg: "ffmpeg",
			LatencyMillis: 300, LocalCopy: fmt.Sprintf("127.0.0.1:%d", udpPort(t))},
	}
	a := app.New(cfg, d)
	if _, err := a.Account.EnsureOwner(context.Background(), "owner@hglobal.test", "owner-password"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	go a.Relay.Run(ctx)

	staff := newClient(t, srv.URL)
	staff.do("POST", "/studio-api/auth/login", obj{"email": "owner@hglobal.test", "password": "owner-password"}, 200, nil)
	var show obj
	staff.do("POST", "/studio-api/shows", obj{"title": "Relay test", "youtube_url": "dQw4w9WgXcQ"}, 201, &show)
	id := show["id"].(string)
	staff.do("GET", "/studio-api/shows/"+id+"/frames", nil, 409, nil) // nothing yet
	var ingest obj
	staff.do("GET", "/studio-api/shows/"+id+"/ingest", nil, 200, &ingest)
	if !strings.HasPrefix(ingest["url"].(string), fmt.Sprintf("srt://ingest.test:%d?passphrase=", port)) {
		t.Fatalf("ingest url %v", ingest["url"])
	}
	staff.do("POST", "/studio-api/shows/"+id+"/start", nil, 200, nil)

	// Wait for the manager to open the listener, then broadcast.
	waitStatus := func(cond func(obj) bool, what string) obj {
		t.Helper()
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			var st obj
			staff.do("GET", "/studio-api/shows/"+id+"/relay", nil, 200, &st)
			if cond(st) {
				return st
			}
			time.Sleep(200 * time.Millisecond)
		}
		t.Fatalf("relay never %s", what)
		return nil
	}
	waitStatus(func(s obj) bool { return s["listening"] == true }, "listened")
	pub := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", "testsrc2=size=1280x720:rate=30", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-t", "20", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", "30", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-f", "mpegts",
		fmt.Sprintf("srt://127.0.0.1:%d?passphrase=%s&latency=300000", port, ingest["passphrase"]))
	if err := pub.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = pub.Process.Kill(); _ = pub.Wait() }()
	st := waitStatus(func(s obj) bool { return s["receiving"] == true && s["frames_buffered"].(float64) >= 4 }, "received frames")
	if st["ingest_started_at"] == nil {
		t.Fatalf("status: %v", st)
	}

	var frames obj
	staff.do("GET", "/studio-api/shows/"+id+"/frames?limit=3", nil, 200, &frames)
	items := frames["items"].([]any)
	if len(items) != 3 || items[2].(obj)["offset_sec"] == nil {
		t.Fatalf("frames: %v", frames)
	}
	newest := items[2].(obj)
	res, err := staff.http.Get(srv.URL + newest["url"].(string))
	if err != nil || res.StatusCode != 200 || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Fatalf("frame image: %v %v", err, res)
	}
	res.Body.Close()

	// The live preview is an MJPEG stream.
	pctx, pcancel := context.WithTimeout(ctx, 2*time.Second)
	req, _ := http.NewRequestWithContext(pctx, "GET", srv.URL+"/studio-api/shows/"+id+"/preview", nil)
	res, err = staff.http.Do(req)
	if err != nil || !strings.HasPrefix(res.Header.Get("Content-Type"), "multipart/x-mixed-replace") {
		t.Fatalf("preview: %v %v", err, res)
	}
	res.Body.Close()
	pcancel()

	// Capture from the buffer, then quick-list it.
	var capture obj
	staff.do("POST", "/studio-api/shows/"+id+"/captures/relay", obj{"seq": newest["seq"]}, 201, &capture)
	if capture["source"] != "relay" || capture["broadcast_offset_sec"] == nil {
		t.Fatalf("capture: %v", capture)
	}
	var item obj
	staff.do("POST", "/studio-api/shows/"+id+"/items/quick", obj{"capture_id": capture["id"], "price_won": 59000,
		"crop": obj{"x": 320, "y": 180, "w": 640, "h": 360}}, 201, &item)
	if item["broadcast_offset_sec"] != capture["broadcast_offset_sec"] || !strings.Contains(item["replay_url"].(string), "&t=") {
		t.Fatalf("item: offset %v replay %v", item["broadcast_offset_sec"], item["replay_url"])
	}

	// Ending the show stops the relay.
	staff.do("POST", "/studio-api/shows/"+id+"/end", nil, 200, nil)
	waitStatus(func(s obj) bool { return s["listening"] != true }, "stopped")
}
