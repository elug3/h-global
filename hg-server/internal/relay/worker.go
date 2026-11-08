package relay

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// FPS is how many frames a second the tap decodes.
const FPS = 2

// BufferWindow is how much recent video the ring buffer keeps.
const BufferWindow = 120 * time.Second

// WorkerConfig is what one show's relay needs.
type WorkerConfig struct {
	FFmpeg        string
	Listen        string // host:port for the SRT listener
	Key           string // SRT passphrase
	LatencyMillis int
	YouTubeTarget string // full RTMP(S) URL including the stream key; empty = no push
	LocalCopy     string // host:port of the local UDP copy the YouTube pusher reads
	FPS           int    // frames a second into the buffer (0 = FPS)
}

// ListenURL is the SRT URL ffmpeg listens on.
func (c WorkerConfig) ListenURL() string {
	return fmt.Sprintf("srt://%s?mode=listener&passphrase=%s&enforced_encryption=1&latency=%d",
		c.Listen, c.Key, c.LatencyMillis*1000)
}

// Args builds the ingest command: one SRT input, an unchanged copy to a local
// UDP port for the YouTube pusher (UDP never blocks or fails when nobody
// reads it), and JPEG frames on stdout.
//
// The copy goes through a second process rather than ffmpeg's tee muxer:
// tee cannot write an FLV header for H.264 copied from MPEG-TS, and a
// separate pusher can fail and restart without interrupting the frames.
func (c WorkerConfig) Args() []string {
	a := []string{"-hide_banner", "-loglevel", "warning", "-nostats",
		"-fflags", "nobuffer", "-analyzeduration", "1000000", "-i", c.ListenURL()}
	if c.YouTubeTarget != "" {
		a = append(a, "-map", "0:v:0", "-map", "0:a?", "-c", "copy", "-f", "mpegts",
			"udp://"+c.LocalCopy+"?pkt_size=1316")
	}
	fps := c.FPS
	if fps <= 0 {
		fps = FPS
	}
	return append(a, "-map", "0:v:0", "-vf", fmt.Sprintf("fps=%d", fps),
		"-c:v", "mjpeg", "-q:v", "3", "-f", "image2pipe", "pipe:1")
}

// PushArgs builds the YouTube pusher: the local copy, unchanged, as FLV.
func (c WorkerConfig) PushArgs() []string {
	return []string{"-hide_banner", "-loglevel", "warning", "-nostats",
		"-fflags", "nobuffer", "-analyzeduration", "1000000", "-probesize", "1000000",
		"-i", "udp://" + c.LocalCopy + "?fifo_size=100000&overrun_nonfatal=1",
		"-c", "copy", "-f", "flv", c.YouTubeTarget}
}

// Status reports a worker's state.
type Status struct {
	ShowID          uuid.UUID  `json:"show_id"`
	Enabled         bool       `json:"enabled"`
	Listening       bool       `json:"listening"`
	Receiving       bool       `json:"receiving"`
	IngestStartedAt *time.Time `json:"ingest_started_at"`
	LastFrameAt     *time.Time `json:"last_frame_at"`
	FramesBuffered  int        `json:"frames_buffered"`
	Sessions        int        `json:"sessions"`
	YouTubePush     bool       `json:"youtube_push"`
	PushRunning     bool       `json:"push_running"`
	PushRestarts    int        `json:"push_restarts"`
	PushLastError   string     `json:"push_last_error"`
	LastError       string     `json:"last_error"`
}

// Worker relays one show while its context lives.
type Worker struct {
	ShowID  uuid.UUID
	Config  WorkerConfig
	Buffer  *Buffer
	OnFirst func(at time.Time) // first frame ever received for the show

	mu           sync.Mutex
	listening    bool
	sessions     int
	lastErr      string
	first        *time.Time
	pushRunning  bool
	pushRestarts int
	pushErr      string
}

// Run listens, relays and restarts ffmpeg when the broadcaster disconnects,
// until ctx ends. The YouTube pusher runs and restarts on its own.
func (w *Worker) Run(ctx context.Context) {
	if w.Config.YouTubeTarget != "" {
		go w.push(ctx)
	}
	for ctx.Err() == nil {
		err := w.session(ctx)
		if ctx.Err() != nil {
			return
		}
		w.mu.Lock()
		w.listening = false
		if err != nil {
			w.lastErr = err.Error()
		}
		w.mu.Unlock()
		if err != nil {
			slog.Warn("relay: ffmpeg ended", "show", w.ShowID, "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// push keeps the YouTube pusher running.
func (w *Worker) push(ctx context.Context) {
	for ctx.Err() == nil {
		cmd := exec.CommandContext(ctx, w.Config.FFmpeg, w.Config.PushArgs()...)
		cmd.WaitDelay = 2 * time.Second
		var stderr strings.Builder
		cmd.Stderr = &limitedWriter{w: &stderr, n: 4096}
		w.mu.Lock()
		w.pushRunning = true
		w.mu.Unlock()
		err := cmd.Run()
		w.mu.Lock()
		w.pushRunning = false
		if ctx.Err() == nil {
			w.pushRestarts++
			if msg := lastLine(stderr.String()); msg != "" {
				w.pushErr = msg
			} else if err != nil {
				w.pushErr = err.Error()
			}
		}
		w.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n <= 0 {
		return len(p), nil
	}
	k := len(p)
	if k > l.n {
		k = l.n
	}
	l.n -= k
	_, err := l.w.Write(p[:k])
	return len(p), err
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func (w *Worker) session(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, w.Config.FFmpeg, w.Config.Args()...)
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}
	w.mu.Lock()
	w.listening = true
	w.mu.Unlock()

	var lastLine string
	var stderrDone sync.WaitGroup
	stderrDone.Add(1)
	go func() {
		defer stderrDone.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			lastLine = sc.Text()
		}
	}()

	frames := NewJPEGReader(stdout)
	counted := false
	for {
		img, err := frames.Next()
		if err != nil {
			if err != io.EOF && err != io.ErrUnexpectedEOF && ctx.Err() == nil {
				slog.Warn("relay: frame read", "show", w.ShowID, "err", err)
			}
			break
		}
		now := time.Now()
		w.Buffer.Add(now, img)
		if !counted {
			counted = true
			w.mu.Lock()
			w.sessions++
			first := w.first == nil
			if first {
				w.first = &now
			}
			w.mu.Unlock()
			if first && w.OnFirst != nil {
				w.OnFirst(now)
			}
		}
	}
	err = cmd.Wait()
	stderrDone.Wait()
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		if lastLine != "" {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(lastLine))
		}
		return err
	}
	return nil
}

// Status returns the worker's state.
func (w *Worker) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	s := Status{ShowID: w.ShowID, Enabled: true, Listening: w.listening, Sessions: w.sessions,
		YouTubePush: w.Config.YouTubeTarget != "", PushRunning: w.pushRunning, PushRestarts: w.pushRestarts,
		PushLastError: w.pushErr, LastError: w.lastErr, IngestStartedAt: w.first, FramesBuffered: w.Buffer.Len()}
	if f, ok := w.Buffer.Latest(); ok {
		at := f.At
		s.LastFrameAt = &at
		s.Receiving = time.Since(at) < 3*time.Second
	}
	return s
}
