// Package events records changes and streams them to browsers.
//
// Publish writes an event row in the caller's transaction, so an event exists
// exactly when the change it describes was committed. After commit the event
// is handed to the in-process Hub, which forwards it to every open
// Server-Sent Events stream on that topic. A client that reconnects with
// Last-Event-ID is replayed from the table.
package events

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/elug3/h-global/hg-server/internal/platform/db"
)

// Retention is how long events are kept for replay.
const Retention = 24 * time.Hour

// Event is one recorded change.
type Event struct {
	ID        int64           `json:"id"`
	Topic     string          `json:"topic"`
	Type      string          `json:"type"`
	Data      json.RawMessage `json:"data"`
	CreatedAt time.Time       `json:"created_at"`
}

// Bus publishes events and serves replays.
type Bus struct {
	db  *db.DB
	hub *Hub
}

// NewBus returns a bus with its own hub.
func NewBus(d *db.DB) *Bus { return &Bus{db: d, hub: NewHub()} }

// Hub returns the bus's hub.
func (b *Bus) Hub() *Hub { return b.hub }

// Publish records an event on topic and broadcasts it after commit.
func (b *Bus) Publish(ctx context.Context, topic, typ string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("event %s: %w", typ, err)
	}
	ev := Event{Topic: topic, Type: typ, Data: raw}
	if err := b.db.Q(ctx).QueryRow(ctx,
		`INSERT INTO events (topic, type, payload) VALUES ($1, $2, $3) RETURNING id, created_at`,
		topic, typ, raw).Scan(&ev.ID, &ev.CreatedAt); err != nil {
		return fmt.Errorf("event %s: %w", typ, err)
	}
	db.AfterCommit(ctx, func() { b.hub.Broadcast(ev) })
	return nil
}

// Since returns events on topic with id > after, oldest first.
func (b *Bus) Since(ctx context.Context, topic string, after int64, limit int) ([]Event, error) {
	rows, err := b.db.Q(ctx).Query(ctx,
		`SELECT id, topic, type, payload, created_at FROM events
		 WHERE topic = $1 AND id > $2 ORDER BY id LIMIT $3`, topic, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Topic, &e.Type, &e.Data, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// LastID returns the newest event id on topic (0 if none). Snapshots return it
// so a client can open the stream from exactly that point.
func (b *Bus) LastID(ctx context.Context, topic string) (int64, error) {
	var id int64
	err := b.db.Q(ctx).QueryRow(ctx,
		`SELECT coalesce(max(id), 0) FROM events WHERE topic = $1`, topic).Scan(&id)
	return id, err
}

// covers reports whether replay from after is complete. Ids are shared by all
// topics and pruning deletes the oldest rows first, so replay is complete
// when nothing older than after+1 has been deleted, i.e. after+1 is at or
// past the oldest retained id. When in doubt it answers false: the client
// then reloads its snapshot, which is always safe.
func (b *Bus) covers(ctx context.Context, _ string, after int64) (bool, error) {
	var oldest int64
	if err := b.db.Q(ctx).QueryRow(ctx,
		`SELECT coalesce(min(id), 0) FROM events`).Scan(&oldest); err != nil {
		return false, err
	}
	if oldest == 0 {
		return false, nil
	}
	return after+1 >= oldest, nil
}

// Prune deletes events older than Retention.
func (b *Bus) Prune(ctx context.Context) error {
	_, err := b.db.Q(ctx).Exec(ctx, `DELETE FROM events WHERE created_at < now() - $1::interval`,
		Retention.String())
	return err
}

// Hub fans events out to subscribers in this process.
type Hub struct {
	mu   sync.Mutex
	subs map[string]map[chan Event]struct{}
}

// NewHub returns an empty hub.
func NewHub() *Hub { return &Hub{subs: map[string]map[chan Event]struct{}{}} }

// subscriberBuffer is how many events a slow subscriber may fall behind before
// it is dropped. A dropped client reconnects and is replayed from the table.
const subscriberBuffer = 256

// Subscribe returns a channel of events on topic and a cancel func. The
// channel is closed when the subscriber falls too far behind.
func (h *Hub) Subscribe(topic string) (<-chan Event, func()) {
	ch := make(chan Event, subscriberBuffer)
	h.mu.Lock()
	if h.subs[topic] == nil {
		h.subs[topic] = map[chan Event]struct{}{}
	}
	h.subs[topic][ch] = struct{}{}
	h.mu.Unlock()
	return ch, func() { h.remove(topic, ch) }
}

func (h *Hub) remove(topic string, ch chan Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[topic]; ok {
		if _, ok := set[ch]; ok {
			delete(set, ch)
			close(ch)
		}
		if len(set) == 0 {
			delete(h.subs, topic)
		}
	}
}

// Broadcast sends ev to every subscriber of its topic without blocking.
func (h *Hub) Broadcast(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for ch := range h.subs[ev.Topic] {
		select {
		case ch <- ev:
		default:
			delete(h.subs[ev.Topic], ch)
			close(ch)
		}
	}
}

// Subscribers returns the number of subscribers on topic.
func (h *Hub) Subscribers(topic string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[topic])
}
