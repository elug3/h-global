package relay_test

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"net"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/relay"
)

func solidJPEG(t *testing.T, c color.Gray) []byte {
	img := image.NewGray(image.Rect(0, 0, 32, 32))
	for i := range img.Pix {
		img.Pix[i] = c.Y
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestJPEGReaderSplitsAStream(t *testing.T) {
	a, b := solidJPEG(t, color.Gray{10}), solidJPEG(t, color.Gray{250})
	stream := append(append(append([]byte("noise"), a...), b...), a...)
	r := relay.NewJPEGReader(bytes.NewReader(stream))
	for i, want := range [][]byte{a, b, a} {
		got, err := r.Next()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("frame %d: err=%v equal=%v", i, err, bytes.Equal(got, want))
		}
	}
	if _, err := r.Next(); err != io.EOF {
		t.Fatalf("end: %v", err)
	}
}

func TestBufferKeepsTheWindow(t *testing.T) {
	b := relay.NewBuffer(10 * time.Second)
	start := time.Now()
	for i := 0; i < 30; i++ {
		b.Add(start.Add(time.Duration(i)*time.Second), []byte{byte(i)})
	}
	if b.Len() != 11 { // seconds 19..29
		t.Fatalf("len = %d, want 11", b.Len())
	}
	if _, ok := b.Get(5); ok {
		t.Fatal("old frame kept")
	}
	if got := b.After(0, 3); len(got) != 3 || got[2].Seq != 30 {
		t.Fatalf("latest: %+v", got)
	}
	if got := b.After(25, 100); len(got) != 5 || got[0].Seq != 26 {
		t.Fatalf("after 25: %d frames", len(got))
	}
}

func TestIngestKeyAndArgs(t *testing.T) {
	id := uuid.New()
	k1, k2 := relay.IngestKey("a-long-server-secret", id, 1), relay.IngestKey("a-long-server-secret", id, 2)
	if len(k1) != 32 || k1 == k2 || k1 != relay.IngestKey("a-long-server-secret", id, 1) {
		t.Fatalf("keys: %q %q", k1, k2)
	}
	c := relay.WorkerConfig{FFmpeg: "ffmpeg", Listen: "0.0.0.0:9000", Key: k1, LatencyMillis: 300}
	if slices.Contains(c.Args(), "mpegts") {
		t.Fatal("local copy without a YouTube target")
	}
	c.YouTubeTarget = "rtmps://a.rtmp.youtube.com/live2/xyz"
	c.LocalCopy = "127.0.0.1:19400"
	args := strings.Join(c.Args(), " ")
	if push := strings.Join(c.PushArgs(), " "); !strings.Contains(push, "udp://127.0.0.1:19400") || !strings.Contains(push, "-c copy -f flv rtmps://") {
		t.Errorf("push args: %s", push)
	}
	for _, want := range []string{"mode=listener", "passphrase=" + k1, "latency=300000", "-c copy -f mpegts udp://127.0.0.1:19400", "fps=2"} {
		if !strings.Contains(args, want) {
			t.Errorf("args missing %q: %s", want, args)
		}
	}
}

// --- With ffmpeg: measure how fast frames arrive. ---

func needFFmpeg(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
}

func freePort(t *testing.T, network string) int {
	if network == "udp" {
		c, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		return c.LocalAddr().(*net.UDPAddr).Port
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func white(img []byte) bool {
	m, err := jpeg.Decode(bytes.NewReader(img))
	if err != nil {
		return false
	}
	b := m.Bounds()
	r, g, bl, _ := m.At(b.Dx()/2, b.Dy()/2).RGBA()
	return (r+g+bl)/3>>8 > 200
}

// publish streams black for blackFor, then white, over SRT like OBS would.
func publish(ctx context.Context, srtURL string, blackFor time.Duration) *exec.Cmd {
	graph := fmt.Sprintf("color=c=black:s=1280x720:r=30:d=%g[b];color=c=white:s=1280x720:r=30:d=8[w];[b][w]concat=n=2:v=1:a=0[out0]",
		blackFor.Seconds())
	cmd := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-re",
		"-f", "lavfi", "-i", graph, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000",
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-g", "30", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest", "-f", "mpegts", srtURL)
	return cmd
}

func TestRelayLatency(t *testing.T) {
	needFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()

	// A local stand-in for YouTube: an RTMP listener that decodes what the
	// relay copies to it.
	rtmpPort := freePort(t, "tcp")
	sinkURL := fmt.Sprintf("rtmp://127.0.0.1:%d/live2/test", rtmpPort)
	sink := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-loglevel", "error", "-fflags", "nobuffer",
		"-listen", "1", "-i", sinkURL, "-map", "0:v:0", "-vf", "fps=10", "-c:v", "mjpeg", "-f", "image2pipe", "pipe:1")
	sinkOut, _ := sink.StdoutPipe()
	if err := sink.Start(); err != nil {
		t.Fatal(err)
	}
	sinkWhite := make(chan time.Time, 1)
	go func() {
		r := relay.NewJPEGReader(sinkOut)
		for {
			img, err := r.Next()
			if err != nil {
				return
			}
			if white(img) {
				sinkWhite <- time.Now()
				return
			}
		}
	}()

	srtPort := freePort(t, "udp")
	key := relay.IngestKey("a-long-server-secret", uuid.New(), 1)
	buf := relay.NewBuffer(relay.BufferWindow)
	w := &relay.Worker{Buffer: buf, Config: relay.WorkerConfig{FFmpeg: "ffmpeg",
		Listen: fmt.Sprintf("127.0.0.1:%d", srtPort), Key: key, LatencyMillis: 300,
		YouTubeTarget: sinkURL, LocalCopy: fmt.Sprintf("127.0.0.1:%d", freePort(t, "udp")), FPS: 10}}
	frames, unsub := buf.Subscribe()
	defer unsub()
	go w.Run(ctx)
	time.Sleep(500 * time.Millisecond) // let the listener open

	const blackFor = 4 * time.Second
	pub := publish(ctx, fmt.Sprintf("srt://127.0.0.1:%d?passphrase=%s&latency=300000", srtPort, key), blackFor)
	if err := pub.Start(); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	turnsWhite := started.Add(blackFor)

	var first, tapWhite time.Time
	for tapWhite.IsZero() {
		select {
		case f := <-frames:
			if first.IsZero() {
				first = f.At
			}
			if white(f.JPEG) {
				tapWhite = f.At
			}
		case <-ctx.Done():
			t.Fatal("no white frame reached the relay buffer")
		}
	}
	var ytWhite time.Time
	select {
	case ytWhite = <-sinkWhite:
	case <-time.After(10 * time.Second):
		t.Fatal("the copy to the YouTube stand-in never turned white")
	}

	tapDelay := tapWhite.Sub(turnsWhite)
	copyDelay := ytWhite.Sub(turnsWhite)
	t.Logf("first frame %v after the publisher started (connect + SRT handshake + first keyframe)", first.Sub(started).Round(time.Millisecond))
	t.Logf("relay frame tap: white seen %v after the publisher switched (upper bound, includes publisher start-up)", tapDelay.Round(time.Millisecond))
	t.Logf("copy to YouTube stand-in: white seen %v after the switch, %v later than the tap", copyDelay.Round(time.Millisecond), ytWhite.Sub(tapWhite).Round(time.Millisecond))
	if tapDelay > 2500*time.Millisecond {
		t.Errorf("frame tap too slow: %v", tapDelay)
	}
	if ytWhite.Sub(tapWhite) > 1500*time.Millisecond {
		t.Errorf("the relay hop to YouTube adds too much: %v", ytWhite.Sub(tapWhite))
	}
	st := w.Status()
	if st.Sessions != 1 || !st.YouTubePush || !st.PushRunning || st.FramesBuffered == 0 {
		t.Errorf("status: %+v", st)
	}
}

func TestRelayRejectsAWrongPassphrase(t *testing.T) {
	needFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	srtPort := freePort(t, "udp")
	buf := relay.NewBuffer(relay.BufferWindow)
	w := &relay.Worker{Buffer: buf, Config: relay.WorkerConfig{FFmpeg: "ffmpeg",
		Listen: fmt.Sprintf("127.0.0.1:%d", srtPort), Key: relay.IngestKey("a-long-server-secret", uuid.New(), 1), LatencyMillis: 300}}
	go w.Run(ctx)
	time.Sleep(500 * time.Millisecond)
	wrong := relay.IngestKey("a-long-server-secret", uuid.New(), 9)
	pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
	defer pcancel()
	_ = publish(pctx, fmt.Sprintf("srt://127.0.0.1:%d?passphrase=%s&latency=300000", srtPort, wrong), time.Second).Run()
	if buf.Len() != 0 {
		t.Fatalf("a publisher with the wrong passphrase got %d frames in", buf.Len())
	}
}
