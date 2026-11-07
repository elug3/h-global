package relay

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/live"
	"github.com/elug3/h-global/hg-server/internal/platform/config"
)

// Manager runs a worker for the show that is live and stops it when the show
// ends. One show is live at a time (shows_one_live), so there is at most one
// worker. It must run in the HTTP process, because the frame buffer it fills
// is served by the Studio routes.
type Manager struct {
	Config config.Relay
	Live   *live.Service

	mu      sync.Mutex
	current *running
	last    map[uuid.UUID]*Buffer // buffer of the most recent run per show
}

type running struct {
	showID uuid.UUID
	key    string
	worker *Worker
	cancel context.CancelFunc
	done   chan struct{}
}

// Run checks every 2 seconds which show is live and reconciles.
func (m *Manager) Run(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		m.reconcile(ctx)
		select {
		case <-ctx.Done():
			m.stop()
			return
		case <-t.C:
		}
	}
}

// Key returns a show's current ingest key.
func (m *Manager) Key(sh live.Show) string {
	return IngestKey(m.Config.Secret, sh.ID, sh.IngestKeyVersion)
}

func (m *Manager) reconcile(ctx context.Context) {
	sh, err := m.Live.LiveShow(ctx)
	if err != nil {
		if ctx.Err() == nil {
			slog.Error("relay: live show lookup", "err", err)
		}
		return
	}
	m.mu.Lock()
	cur := m.current
	m.mu.Unlock()
	if sh == nil {
		if cur != nil {
			m.stop()
		}
		return
	}
	key := m.Key(*sh)
	if cur != nil && cur.showID == sh.ID && cur.key == key {
		return
	}
	if cur != nil {
		m.stop() // another show went live, or the key was rotated
	}
	m.start(ctx, *sh, key)
}

func (m *Manager) start(parent context.Context, sh live.Show, key string) {
	target := ""
	if m.Config.YouTubeKey != "" {
		target = m.Config.YouTubeURL + "/" + m.Config.YouTubeKey
	}
	buf := m.bufferFor(sh.ID)
	showID := sh.ID
	w := &Worker{
		ShowID: showID,
		Config: WorkerConfig{FFmpeg: m.Config.FFmpeg, Listen: m.Config.Listen, Key: key,
			LatencyMillis: m.Config.LatencyMillis, YouTubeTarget: target, LocalCopy: m.Config.LocalCopy},
		Buffer: buf,
		OnFirst: func(at time.Time) {
			if err := m.Live.MarkIngestStarted(context.Background(), showID, at); err != nil {
				slog.Error("relay: mark ingest started", "show", showID, "err", err)
			}
		},
	}
	ctx, cancel := context.WithCancel(parent)
	r := &running{showID: sh.ID, key: key, worker: w, cancel: cancel, done: make(chan struct{})}
	go func() { defer close(r.done); w.Run(ctx) }()
	m.mu.Lock()
	m.current = r
	m.mu.Unlock()
	slog.Info("relay: listening", "show", sh.ID, "listen", m.Config.Listen, "youtube_push", target != "")
}

func (m *Manager) stop() {
	m.mu.Lock()
	r := m.current
	m.current = nil
	m.mu.Unlock()
	if r == nil {
		return
	}
	r.cancel()
	<-r.done
	slog.Info("relay: stopped", "show", r.showID)
}

// bufferFor reuses a show's buffer across worker restarts (key rotation).
func (m *Manager) bufferFor(showID uuid.UUID) *Buffer {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last == nil {
		m.last = map[uuid.UUID]*Buffer{}
	}
	if b, ok := m.last[showID]; ok {
		return b
	}
	for id := range m.last { // keep only the latest show's frames
		delete(m.last, id)
	}
	b := NewBuffer(BufferWindow)
	m.last[showID] = b
	return b
}

// Buffer returns a show's frame buffer, if the relay has one.
func (m *Manager) Buffer(showID uuid.UUID) (*Buffer, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.last[showID]
	return b, ok
}

// Status returns the relay state for a show.
func (m *Manager) Status(showID uuid.UUID) Status {
	m.mu.Lock()
	r := m.current
	m.mu.Unlock()
	if r != nil && r.showID == showID {
		return r.worker.Status()
	}
	s := Status{ShowID: showID, Enabled: m.Config.Enabled, YouTubePush: m.Config.YouTubeKey != ""}
	if b, ok := m.Buffer(showID); ok {
		s.FramesBuffered = b.Len()
	}
	return s
}
