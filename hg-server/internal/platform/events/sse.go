package events

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// Heartbeat is how often an idle stream sends a comment line, so proxies
// keep the connection open.
var Heartbeat = 15 * time.Second

// ServeSSE streams topic to the client.
//
// The client first reads a snapshot (which carries last_event_id), then opens
// this stream with that id as Last-Event-ID (header) or last_event_id (query).
// Events after it are replayed from the table, then live events follow. If
// the requested history is gone, the stream sends `event: reset` and the
// client must reload its snapshot.
func (b *Bus) ServeSSE(w http.ResponseWriter, r *http.Request, topic string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}
	ctx := r.Context()
	last := int64(0)
	if v := r.Header.Get("Last-Event-ID"); v != "" {
		last, _ = strconv.ParseInt(v, 10, 64)
	} else if v := r.URL.Query().Get("last_event_id"); v != "" {
		last, _ = strconv.ParseInt(v, 10, 64)
	}

	// Subscribe before replaying, so nothing committed in between is lost;
	// duplicates are skipped by id below.
	live, cancel := b.hub.Subscribe(topic)
	defer cancel()

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "retry: 3000\n\n")

	if last > 0 {
		ok, err := b.covers(ctx, topic, last)
		if err != nil {
			return nil
		}
		if !ok {
			fmt.Fprint(w, "event: reset\ndata: {}\n\n")
			flusher.Flush()
			return nil
		}
		for {
			evs, err := b.Since(ctx, topic, last, 500)
			if err != nil {
				return nil
			}
			for _, ev := range evs {
				writeEvent(w, ev)
				last = ev.ID
			}
			if len(evs) < 500 {
				break
			}
		}
	}
	flusher.Flush()

	tick := time.NewTicker(Heartbeat)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-tick.C:
			fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case ev, open := <-live:
			if !open {
				// Fell behind; end the stream so the client reconnects and replays.
				return nil
			}
			if ev.ID <= last {
				continue
			}
			writeEvent(w, ev)
			last = ev.ID
			flusher.Flush()
		}
	}
}

func writeEvent(w http.ResponseWriter, ev Event) {
	fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", ev.ID, ev.Type, ev.Data)
}
