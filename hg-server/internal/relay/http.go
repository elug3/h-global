package relay

import (
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"time"

	"github.com/elug3/h-global/hg-server/internal/live"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/auth"
	"github.com/elug3/h-global/hg-server/internal/platform/httpx"
)

// Handler serves the relay's Studio routes.
type Handler struct {
	Manager *Manager
}

// RegisterStudio mounts routes under /studio-api. They exist even when the
// relay is disabled and then answer 409 relay_disabled.
func (h *Handler) RegisterStudio(rt *httpx.Router) {
	cs := rt.With(auth.RequireStaff(auth.RoleCS))
	mgr := rt.With(auth.RequireStaff(auth.RoleManager))
	mgr.Handle("GET", "/shows/{id}/ingest", h.ingest)
	mgr.Handle("POST", "/shows/{id}/ingest/rotate", h.rotate)
	cs.Handle("GET", "/shows/{id}/relay", h.status)
	cs.Handle("GET", "/shows/{id}/frames", h.frames)
	cs.Handle("GET", "/shows/{id}/frames/{seq}", h.frame)
	cs.Handle("GET", "/shows/{id}/preview", h.preview)
	cs.Handle("POST", "/shows/{id}/captures/relay", h.capture)
}

var errDisabled = apperr.Conflict("relay_disabled", "the broadcast relay is not enabled on this server")

// Ingest is what the broadcaster enters in OBS.
type Ingest struct {
	URL        string `json:"url"` // full SRT URL with passphrase, for OBS "Server"
	Host       string `json:"host"`
	Passphrase string `json:"passphrase"`
	LatencyMs  int    `json:"latency_ms"`
	Note       string `json:"note"`
}

func (h *Handler) show(r *http.Request) (live.Show, error) {
	if !h.Manager.Config.Enabled {
		return live.Show{}, errDisabled
	}
	id, err := httpx.PathID(r, "id")
	if err != nil {
		return live.Show{}, err
	}
	return h.Manager.Live.Show(r.Context(), id)
}

func (h *Handler) ingestFor(sh live.Show) Ingest {
	key := h.Manager.Key(sh)
	lat := h.Manager.Config.LatencyMillis
	return Ingest{
		URL:        fmt.Sprintf("srt://%s?passphrase=%s&latency=%d", h.Manager.Config.PublicHost, key, lat*1000),
		Host:       h.Manager.Config.PublicHost,
		Passphrase: key, LatencyMs: lat,
		Note: "OBS: Settings > Stream > Service: Custom, Server: the url above, Stream key: empty. " +
			"The relay listens while the show is live.",
	}
}

func (h *Handler) ingest(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, h.ingestFor(sh))
}

func (h *Handler) rotate(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	if sh, err = h.Manager.Live.RotateIngestKey(r.Context(), sh.ID); err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, h.ingestFor(sh))
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusOK, h.Manager.Status(sh.ID))
}

// FrameInfo describes a buffered frame.
type FrameInfo struct {
	Seq       int64     `json:"seq"`
	At        time.Time `json:"at"`
	OffsetSec *int      `json:"offset_sec"` // place in the stream, from ingest start
	URL       string    `json:"url"`
}

func offset(sh live.Show, at time.Time) *int {
	start := sh.IngestStartedAt
	if start == nil {
		return nil
	}
	sec := int(at.Sub(*start) / time.Second)
	if sec < 0 {
		sec = 0
	}
	return &sec
}

func (h *Handler) buffer(sh live.Show) (*Buffer, error) {
	b, ok := h.Manager.Buffer(sh.ID)
	if !ok {
		return nil, apperr.Conflict("no_frames", "the relay has not received this show's stream")
	}
	return b, nil
}

func (h *Handler) frames(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	b, err := h.buffer(sh)
	if err != nil {
		return err
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
	limit, err := httpx.QueryInt(r, "limit", 20, 1, int(BufferWindow/time.Second)*FPS)
	if err != nil {
		return err
	}
	if sh, err = h.Manager.Live.Show(r.Context(), sh.ID); err != nil { // fresh ingest_started_at
		return err
	}
	out := []FrameInfo{}
	for _, f := range b.After(after, limit) {
		out = append(out, FrameInfo{Seq: f.Seq, At: f.At, OffsetSec: offset(sh, f.At),
			URL: fmt.Sprintf("/studio-api/shows/%s/frames/%d", sh.ID, f.Seq)})
	}
	return httpx.JSON(w, http.StatusOK, map[string]any{"items": out})
}

func (h *Handler) frame(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	b, err := h.buffer(sh)
	if err != nil {
		return err
	}
	seq, err := strconv.ParseInt(r.PathValue("seq"), 10, 64)
	if err != nil {
		return apperr.Invalid("invalid_seq", "seq must be a number")
	}
	f, ok := b.Get(seq)
	if !ok {
		return apperr.NotFound("frame")
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=300")
	_, err = w.Write(f.JPEG)
	return err
}

// preview streams new frames as multipart/x-mixed-replace (MJPEG), which an
// <img> element shows as moving video. It is the prototype's live preview.
func (h *Handler) preview(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	b, err := h.buffer(sh)
	if err != nil {
		return err
	}
	frames, cancel := b.Subscribe()
	defer cancel()
	mw := multipart.NewWriter(w)
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary="+mw.Boundary())
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	send := func(f Frame) error {
		part, err := mw.CreatePart(textproto.MIMEHeader{
			"Content-Type":   {"image/jpeg"},
			"Content-Length": {strconv.Itoa(len(f.JPEG))},
			"X-Frame-Seq":    {strconv.FormatInt(f.Seq, 10)},
		})
		if err != nil {
			return err
		}
		if _, err := part.Write(f.JPEG); err != nil {
			return err
		}
		if flusher != nil {
			flusher.Flush()
		}
		return nil
	}
	if f, ok := b.Latest(); ok {
		if err := send(f); err != nil {
			return nil
		}
	}
	for {
		select {
		case <-r.Context().Done():
			return nil
		case f := <-frames:
			if err := send(f); err != nil {
				return nil
			}
		}
	}
}

func (h *Handler) capture(w http.ResponseWriter, r *http.Request) error {
	sh, err := h.show(r)
	if err != nil {
		return err
	}
	b, err := h.buffer(sh)
	if err != nil {
		return err
	}
	var in struct {
		Seq int64 `json:"seq"`
	}
	if err := httpx.Decode(r, &in); err != nil {
		return err
	}
	f, ok := b.Get(in.Seq)
	if !ok {
		return apperr.NotFound("frame")
	}
	c, err := h.Manager.Live.SaveRelayCapture(r.Context(), sh.ID, auth.MustPrincipal(r.Context()).ID, f.JPEG, f.At)
	if err != nil {
		return err
	}
	return httpx.JSON(w, http.StatusCreated, c)
}
