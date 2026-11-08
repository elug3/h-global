package live

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/events"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
	"github.com/elug3/h-global/hg-server/internal/platform/upload"
)

// Event types on a show's topic.
const (
	EvShowStarted   = "live.show_started"
	EvShowEnded     = "live.show_ended"
	EvShowUpdated   = "live.show_updated"
	EvItemPublished = "live.item_published"
	EvItemUpdated   = "live.item_updated" // price, title, photos, stock, sold out
	EvItemPinned    = "live.item_pinned"
	EvItemUnpinned  = "live.item_unpinned"
)

// Topic is a show's event topic (public).
func Topic(showID uuid.UUID) string { return "show:" + showID.String() }

// Service implements live use cases.
type Service struct {
	DB          *db.DB
	Store       Postgres
	Catalog     *catalog.Service
	Bus         *events.Bus
	Storage     storage.Storage
	StreamDelay time.Duration
	Now         func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// CreateShow adds a show in draft or scheduled.
func (s *Service) CreateShow(ctx context.Context, staffID uuid.UUID, n NewShow) (Show, error) {
	var err error
	sh := Show{ID: ids.New(), Status: ShowDraft, DisplayDaysAfter: 3, ScheduledAt: n.ScheduledAt, HostName: n.HostName}
	if sh.Title, err = checkShowTitle(n.Title); err != nil {
		return Show{}, err
	}
	if sh.YouTubeVideoID, err = ParseYouTubeID(n.YouTubeURL); err != nil {
		return Show{}, err
	}
	if n.DisplayDaysAfter != nil {
		if err := checkDays(*n.DisplayDaysAfter); err != nil {
			return Show{}, err
		}
		sh.DisplayDaysAfter = *n.DisplayDaysAfter
	}
	switch n.Status {
	case "", ShowDraft:
	case ShowScheduled:
		sh.Status = ShowScheduled
	default:
		return Show{}, apperr.Invalid("invalid_status", "a new show is draft or scheduled")
	}
	if err := s.Store.insertShow(ctx, sh, staffID); err != nil {
		return Show{}, err
	}
	return s.Store.show(ctx, sh.ID, false)
}

// UpdateShow changes show settings. Changing display days on an ended show
// moves the hide time of items not yet hidden.
func (s *Service) UpdateShow(ctx context.Context, id uuid.UUID, p ShowPatch) (Show, error) {
	var out Show
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		sh, err := s.Store.show(ctx, id, true)
		if err != nil {
			return err
		}
		if p.Title != nil {
			if sh.Title, err = checkShowTitle(*p.Title); err != nil {
				return err
			}
		}
		if p.YouTubeURL != nil {
			if sh.YouTubeVideoID, err = ParseYouTubeID(*p.YouTubeURL); err != nil {
				return err
			}
		}
		if p.ScheduledAt != nil {
			sh.ScheduledAt = p.ScheduledAt
		}
		if p.HostName != nil {
			sh.HostName = *p.HostName
		}
		if p.Status != nil {
			ok := (sh.Status == ShowDraft || sh.Status == ShowScheduled) &&
				(*p.Status == ShowDraft || *p.Status == ShowScheduled)
			if !ok {
				return apperr.Conflict("invalid_transition", "use start and end to go live or end a show")
			}
			sh.Status = *p.Status
		}
		daysChanged := false
		if p.DisplayDaysAfter != nil {
			if err := checkDays(*p.DisplayDaysAfter); err != nil {
				return err
			}
			daysChanged = sh.DisplayDaysAfter != *p.DisplayDaysAfter
			sh.DisplayDaysAfter = *p.DisplayDaysAfter
		}
		if err := s.Store.saveShow(ctx, sh); err != nil {
			return err
		}
		if daysChanged && sh.Status == ShowEnded && sh.EndedAt != nil {
			if _, err := s.DB.Q(ctx).Exec(ctx,
				`UPDATE show_items SET hide_at = $2 WHERE show_id = $1 AND hidden_at IS NULL AND NOT keep_listed`,
				id, sh.EndedAt.AddDate(0, 0, sh.DisplayDaysAfter)); err != nil {
				return err
			}
		}
		if out, err = s.Store.show(ctx, id, false); err != nil {
			return err
		}
		return s.publishShow(ctx, EvShowUpdated, out)
	})
	return out, err
}

func (s *Service) publishShow(ctx context.Context, typ string, sh Show) error {
	if sh.Status == ShowDraft {
		return nil
	}
	return s.Bus.Publish(ctx, Topic(sh.ID), typ, map[string]any{"show": sh, "watch_url": sh.WatchURL()})
}

// ListShows lists shows, newest first; statuses empty means all.
func (s *Service) ListShows(ctx context.Context, statuses []string, limit int) ([]Show, error) {
	if len(statuses) == 0 {
		return s.Store.shows(ctx, `ORDER BY created_at DESC LIMIT $1`, limit)
	}
	return s.Store.shows(ctx, `WHERE status = ANY($1) ORDER BY coalesce(started_at, scheduled_at, created_at) DESC LIMIT $2`, statuses, limit)
}

// Current returns the live show, else the next scheduled one, else nil.
func (s *Service) Current(ctx context.Context) (*ShowView, error) {
	list, err := s.Store.shows(ctx,
		`WHERE status = 'live' OR (status = 'scheduled' AND (scheduled_at IS NULL OR scheduled_at > now() - interval '6 hours'))
		 ORDER BY (status = 'live') DESC, scheduled_at NULLS LAST LIMIT 1`)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	v, err := s.PublicShow(ctx, list[0].ID)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// PublicShow is the storefront view. Items are listed once a show is live.
func (s *Service) PublicShow(ctx context.Context, id uuid.UUID) (ShowView, error) {
	last, err := s.Bus.LastID(ctx, Topic(id))
	if err != nil {
		return ShowView{}, err
	}
	sh, err := s.Store.show(ctx, id, false)
	if err != nil {
		return ShowView{}, err
	}
	if sh.Status == ShowDraft {
		return ShowView{}, apperr.NotFound("show")
	}
	v := ShowView{Show: sh, WatchURL: sh.WatchURL(), Items: []ItemView{}, LastEventID: last}
	if sh.Status == ShowScheduled {
		return v, nil
	}
	items, err := s.Store.items(ctx, `WHERE show_id = $1 ORDER BY number`, id)
	if err != nil {
		return ShowView{}, err
	}
	if v.Items, err = s.views(ctx, sh, items); err != nil {
		return ShowView{}, err
	}
	return v, nil
}

// StudioShow is the staff view with full product data.
func (s *Service) StudioShow(ctx context.Context, id uuid.UUID) (StudioShow, error) {
	last, err := s.Bus.LastID(ctx, Topic(id))
	if err != nil {
		return StudioShow{}, err
	}
	sh, err := s.Store.show(ctx, id, false)
	if err != nil {
		return StudioShow{}, err
	}
	items, err := s.Store.items(ctx, `WHERE show_id = $1 ORDER BY number`, id)
	if err != nil {
		return StudioShow{}, err
	}
	out := StudioShow{Show: sh, WatchURL: sh.WatchURL(), Items: []StudioItem{}, LastEventID: last}
	products, err := s.productsFor(ctx, items)
	if err != nil {
		return StudioShow{}, err
	}
	for _, it := range items {
		out.Items = append(out.Items, StudioItem{Item: it, ReplayURL: sh.ReplayURL(it.BroadcastOffsetSec), Product: products[it.ProductID]})
	}
	return out, nil
}

func (s *Service) productsFor(ctx context.Context, items []Item) (map[uuid.UUID]catalog.Product, error) {
	idList := make([]uuid.UUID, 0, len(items))
	for _, it := range items {
		idList = append(idList, it.ProductID)
	}
	list, err := s.Catalog.GetMany(ctx, idList)
	if err != nil {
		return nil, err
	}
	m := make(map[uuid.UUID]catalog.Product, len(list))
	for _, p := range list {
		m[p.ID] = p
	}
	return m, nil
}

func (s *Service) views(ctx context.Context, sh Show, items []Item) ([]ItemView, error) {
	products, err := s.productsFor(ctx, items)
	if err != nil {
		return nil, err
	}
	out := make([]ItemView, 0, len(items))
	for _, it := range items {
		out = append(out, s.view(sh, it, products[it.ProductID]))
	}
	return out, nil
}

func (s *Service) view(sh Show, it Item, p catalog.Product) ItemView {
	return ItemView{
		ID: it.ID, Number: it.Number, Source: it.Source, Pinned: it.Pinned,
		IsNew:       sh.Status == ShowLive && s.now().Sub(it.PublishedAt) < NewBadgeFor,
		PublishedAt: it.PublishedAt, BroadcastOffsetSec: it.BroadcastOffsetSec,
		ReplayURL: sh.ReplayURL(it.BroadcastOffsetSec), Product: p.Public(),
	}
}

// publishItem sends an item's current view to the show's viewers.
func (s *Service) publishItem(ctx context.Context, typ string, it Item) error {
	sh, err := s.Store.show(ctx, it.ShowID, false)
	if err != nil {
		return err
	}
	p, err := s.Catalog.Get(ctx, it.ProductID)
	if err != nil {
		return err
	}
	return s.Bus.Publish(ctx, Topic(sh.ID), typ, map[string]any{"item": s.view(sh, it, p)})
}

// ProductChanged is the catalog listener: viewers of live or ended shows
// that feature the product get its new state.
func (s *Service) ProductChanged(ctx context.Context, productID uuid.UUID) error {
	items, err := s.Store.items(ctx,
		`WHERE product_id = $1 AND hidden_at IS NULL
		   AND show_id IN (SELECT id FROM shows WHERE status IN ('live', 'ended'))`, productID)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := s.publishItem(ctx, EvItemUpdated, it); err != nil {
			return err
		}
	}
	return nil
}

// Start puts a show live. It has no items yet: every product is listed on air,
// from No.1.
func (s *Service) Start(ctx context.Context, showID uuid.UUID) (Show, error) {
	var out Show
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		sh, err := s.Store.show(ctx, showID, true)
		if err != nil {
			return err
		}
		if sh.Status != ShowDraft && sh.Status != ShowScheduled {
			return apperr.Conflict("invalid_transition", "only a draft or scheduled show can start")
		}
		now := s.now()
		sh.Status, sh.StartedAt = ShowLive, &now
		if err := s.Store.saveShow(ctx, sh); err != nil {
			return err
		}
		if err := s.publishShow(ctx, EvShowStarted, sh); err != nil {
			return err
		}
		out, err = s.Store.show(ctx, showID, false)
		return err
	})
	return out, err
}

// End ends a live show. Items stay on sale for the show's display days,
// unless kept listed.
func (s *Service) End(ctx context.Context, showID uuid.UUID) (Show, error) {
	var out Show
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		sh, err := s.Store.show(ctx, showID, true)
		if err != nil {
			return err
		}
		if sh.Status != ShowLive {
			return apperr.Conflict("invalid_transition", "only a live show can end")
		}
		now := s.now()
		sh.Status, sh.EndedAt = ShowEnded, &now
		if err := s.Store.saveShow(ctx, sh); err != nil {
			return err
		}
		if _, err := s.DB.Q(ctx).Exec(ctx,
			`UPDATE show_items SET hide_at = $2 WHERE show_id = $1 AND NOT keep_listed`,
			showID, now.AddDate(0, 0, sh.DisplayDaysAfter)); err != nil {
			return err
		}
		out = sh
		return s.publishShow(ctx, EvShowEnded, sh)
	})
	return out, err
}

func (s *Service) offset(sh Show, at time.Time) *int {
	if sh.Status != ShowLive || sh.StartedAt == nil {
		return nil
	}
	sec := int((at.Sub(*sh.StartedAt) - s.StreamDelay) / time.Second)
	if sec < 0 {
		sec = 0
	}
	return &sec
}

// SaveCapture stores a captured frame (or photo) for a show.
func (s *Service) SaveCapture(ctx context.Context, showID, staffID uuid.UUID, data []byte) (Capture, error) {
	sh, err := s.Store.show(ctx, showID, false)
	if err != nil {
		return Capture{}, err
	}
	if sh.Status == ShowEnded {
		return Capture{}, apperr.Conflict("show_ended", "the show has ended")
	}
	at := s.now()
	f, err := upload.StoreOriginalImage(ctx, s.Storage, storage.CapturePrefix, data)
	if err != nil {
		return Capture{}, err
	}
	c := Capture{ID: ids.New(), ShowID: showID, ObjectKey: f.Key, ContentType: f.ContentType,
		Width: f.Width, Height: f.Height, BroadcastOffsetSec: s.offset(sh, at), Source: "upload", CreatedBy: &staffID}
	if err := s.Store.insertCapture(ctx, c); err != nil {
		_ = s.Storage.Delete(context.WithoutCancel(ctx), f.Key)
		return Capture{}, err
	}
	return s.Capture(ctx, c.ID)
}

// Show returns a show.
func (s *Service) Show(ctx context.Context, id uuid.UUID) (Show, error) {
	return s.Store.show(ctx, id, false)
}

// LiveShow returns the show that is live now, if any.
func (s *Service) LiveShow(ctx context.Context) (*Show, error) {
	list, err := s.Store.shows(ctx, `WHERE status = 'live' LIMIT 1`)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return &list[0], nil
}

// MarkIngestStarted records when the relay first received a show's stream.
// Later calls keep the first time.
func (s *Service) MarkIngestStarted(ctx context.Context, showID uuid.UUID, at time.Time) error {
	_, err := s.DB.Q(ctx).Exec(ctx,
		`UPDATE shows SET ingest_started_at = $2 WHERE id = $1 AND ingest_started_at IS NULL`, showID, at)
	return err
}

// RotateIngestKey invalidates a show's ingest key by bumping its version.
func (s *Service) RotateIngestKey(ctx context.Context, showID uuid.UUID) (Show, error) {
	tag, err := s.DB.Q(ctx).Exec(ctx,
		`UPDATE shows SET ingest_key_version = ingest_key_version + 1, updated_at = now() WHERE id = $1`, showID)
	if err != nil {
		return Show{}, err
	}
	if tag.RowsAffected() == 0 {
		return Show{}, apperr.NotFound("show")
	}
	return s.Store.show(ctx, showID, false)
}

// SaveRelayCapture keeps a frame from the broadcast relay as a capture. The
// frame's place in the stream is its arrival time minus the time the relay
// first received the stream (no stream delay: relay frames arrive before
// YouTube shows them).
func (s *Service) SaveRelayCapture(ctx context.Context, showID, staffID uuid.UUID, jpeg []byte, at time.Time) (Capture, error) {
	sh, err := s.Store.show(ctx, showID, false)
	if err != nil {
		return Capture{}, err
	}
	if sh.Status != ShowLive {
		return Capture{}, apperr.Conflict("show_not_live", "frames can be captured while the show is live")
	}
	f, err := upload.StoreOriginalImage(ctx, s.Storage, storage.CapturePrefix, jpeg)
	if err != nil {
		return Capture{}, err
	}
	var offset *int
	if start := sh.IngestStartedAt; start != nil || sh.StartedAt != nil {
		if start == nil {
			start = sh.StartedAt
		}
		sec := int(at.Sub(*start) / time.Second)
		if sec < 0 {
			sec = 0
		}
		offset = &sec
	}
	c := Capture{ID: ids.New(), ShowID: showID, ObjectKey: f.Key, ContentType: f.ContentType,
		Width: f.Width, Height: f.Height, BroadcastOffsetSec: offset, Source: "relay", CreatedBy: &staffID}
	if err := s.Store.insertCapture(ctx, c); err != nil {
		_ = s.Storage.Delete(context.WithoutCancel(ctx), f.Key)
		return Capture{}, err
	}
	return s.Capture(ctx, c.ID)
}

// Capture returns a capture.
func (s *Service) Capture(ctx context.Context, id uuid.UUID) (Capture, error) {
	c, err := s.Store.capture(ctx, id)
	if err != nil {
		return c, err
	}
	c.ImageURL = fmt.Sprintf("/studio-api/captures/%s/image", c.ID)
	return c, nil
}

// QuickList publishes an item during a live show in one transaction: the
// product (with options, variants, stock and photo), the numbered item and
// the viewer event commit together or not at all.
func (s *Service) QuickList(ctx context.Context, showID, staffID uuid.UUID, in QuickListInput) (StudioItem, error) {
	if in.IdempotencyKey != "" {
		if it, ok, err := s.Store.itemByKey(ctx, showID, in.IdempotencyKey); err != nil || ok {
			if err != nil {
				return StudioItem{}, err
			}
			return s.studioItem(ctx, it)
		}
	}
	var capture *Capture
	if in.CaptureID != nil {
		c, err := s.Store.capture(ctx, *in.CaptureID)
		if err != nil {
			return StudioItem{}, err
		}
		if c.ShowID != showID {
			return StudioItem{}, apperr.NotFound("capture")
		}
		capture = &c
	}
	var photoKey string
	if in.ProductID == nil {
		if capture == nil {
			return StudioItem{}, apperr.Invalid("capture_required", "a new item needs capture_id")
		}
		if in.PriceWon == nil {
			return StudioItem{}, apperr.Invalid("price_required", "a new item needs price_won")
		}
		// Store the cropped photo before the transaction; remove it if the
		// transaction fails.
		data, err := s.readObject(ctx, capture.ObjectKey)
		if err != nil {
			return StudioItem{}, err
		}
		f, err := upload.StoreImage(ctx, s.Storage, storage.PublicPrefix, data, in.Crop)
		if err != nil {
			return StudioItem{}, err
		}
		photoKey = f.Key
	}

	var it Item
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		sh, err := s.Store.show(ctx, showID, true) // serializes listings in this show
		if err != nil {
			return err
		}
		if sh.Status != ShowLive {
			return apperr.Conflict("show_not_live", "items can be listed on air only while the show is live")
		}
		if in.IdempotencyKey != "" {
			if prev, ok, err := s.Store.itemByKey(ctx, showID, in.IdempotencyKey); err != nil || ok {
				it = prev
				if ok && photoKey != "" {
					k := photoKey
					db.AfterCommit(ctx, func() { _ = s.Storage.Delete(context.Background(), k) })
				}
				return err
			}
		}
		number := sh.NextNumber
		it = Item{ID: ids.New(), ShowID: showID, Number: number, PublishedBy: &staffID}
		if capture != nil {
			it.CaptureID = &capture.ID
			it.BroadcastOffsetSec = capture.BroadcastOffsetSec
		} else {
			it.BroadcastOffsetSec = s.offset(sh, s.now())
		}
		if in.ProductID == nil {
			title := in.Title
			if title == "" {
				title = fmt.Sprintf("%s%d", catalog.PlaceholderTitle, number)
			}
			p, err := s.Catalog.Create(ctx, catalog.NewProduct{
				Title: title, Description: in.Description, PriceWon: *in.PriceWon, Status: catalog.StatusLive,
				Options: in.Options, Stock: in.Stock, Source: catalog.SourceLiveCapture,
				CreatedBy: &staffID, PhotoKeys: []string{photoKey},
			})
			if err != nil {
				return err
			}
			it.ProductID, it.Source = p.ID, SourceCaptured
		} else {
			p, err := s.Catalog.Get(ctx, *in.ProductID)
			if err != nil {
				return err
			}
			if p.Status == catalog.StatusArchived {
				return apperr.Conflict("product_archived", "archived products cannot be listed")
			}
			patch := catalog.ProductPatch{}
			live := catalog.StatusLive
			if p.Status != catalog.StatusLive {
				patch.Status = &live
			}
			if in.PriceWon != nil {
				patch.PriceWon = in.PriceWon
			}
			if patch.Status != nil || patch.PriceWon != nil {
				if _, err := s.Catalog.Update(ctx, p.ID, patch); err != nil {
					return err
				}
			}
			it.ProductID, it.Source = p.ID, SourceMatched
		}
		if err := s.Store.insertItem(ctx, it, in.IdempotencyKey); err != nil {
			return err
		}
		sh.NextNumber++
		if err := s.Store.saveShow(ctx, sh); err != nil {
			return err
		}
		if it, err = s.Store.item(ctx, it.ID); err != nil {
			return err
		}
		return s.publishItem(ctx, EvItemPublished, it)
	})
	if err != nil {
		if photoKey != "" {
			_ = s.Storage.Delete(context.WithoutCancel(ctx), photoKey)
		}
		return StudioItem{}, err
	}
	return s.studioItem(ctx, it)
}

func (s *Service) readObject(ctx context.Context, key string) ([]byte, error) {
	r, err := s.Storage.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, upload.MaxImageBytes+1))
}

func (s *Service) studioItem(ctx context.Context, it Item) (StudioItem, error) {
	sh, err := s.Store.show(ctx, it.ShowID, false)
	if err != nil {
		return StudioItem{}, err
	}
	p, err := s.Catalog.Get(ctx, it.ProductID)
	if err != nil {
		return StudioItem{}, err
	}
	return StudioItem{Item: it, ReplayURL: sh.ReplayURL(it.BroadcastOffsetSec), Product: p}, nil
}

// Pin marks or unmarks an item as worn or held today.
func (s *Service) Pin(ctx context.Context, itemID uuid.UUID, pinned bool) (StudioItem, error) {
	var it Item
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		tag, err := s.DB.Q(ctx).Exec(ctx, `UPDATE show_items SET pinned = $2 WHERE id = $1`, itemID, pinned)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("item")
		}
		if it, err = s.Store.item(ctx, itemID); err != nil {
			return err
		}
		typ := EvItemUnpinned
		if pinned {
			typ = EvItemPinned
		}
		return s.publishItem(ctx, typ, it)
	})
	if err != nil {
		return StudioItem{}, err
	}
	return s.studioItem(ctx, it)
}

// SetKeepListed keeps an item on sale after the stream (or undoes that).
func (s *Service) SetKeepListed(ctx context.Context, itemID uuid.UUID, keep bool) (StudioItem, error) {
	var it Item
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		cur, err := s.Store.item(ctx, itemID)
		if err != nil {
			return err
		}
		sh, err := s.Store.show(ctx, cur.ShowID, false)
		if err != nil {
			return err
		}
		var hideAt *time.Time
		if !keep && sh.EndedAt != nil {
			t := sh.EndedAt.AddDate(0, 0, sh.DisplayDaysAfter)
			hideAt = &t
		}
		if _, err := s.DB.Q(ctx).Exec(ctx,
			`UPDATE show_items SET keep_listed = $2, hide_at = $3 WHERE id = $1 AND hidden_at IS NULL`,
			itemID, keep, hideAt); err != nil {
			return err
		}
		it, err = s.Store.item(ctx, itemID)
		return err
	})
	if err != nil {
		return StudioItem{}, err
	}
	return s.studioItem(ctx, it)
}

// SoldOut marks an item's product sold out.
func (s *Service) SoldOut(ctx context.Context, itemID uuid.UUID) (StudioItem, error) {
	it, err := s.Store.item(ctx, itemID)
	if err != nil {
		return StudioItem{}, err
	}
	if _, err := s.Catalog.MarkSoldOut(ctx, it.ProductID); err != nil {
		return StudioItem{}, err
	}
	return s.studioItem(ctx, it)
}

// Broadcasts lists where a product was shown, for its product page.
func (s *Service) Broadcasts(ctx context.Context, productID uuid.UUID) ([]Broadcast, error) {
	items, err := s.Store.items(ctx,
		`WHERE product_id = $1 AND show_id IN (SELECT id FROM shows WHERE status IN ('live', 'ended')) ORDER BY published_at DESC`, productID)
	if err != nil {
		return nil, err
	}
	out := []Broadcast{}
	for _, it := range items {
		sh, err := s.Store.show(ctx, it.ShowID, false)
		if err != nil {
			return nil, err
		}
		b := Broadcast{ShowID: sh.ID, ShowTitle: sh.Title, Number: it.Number,
			BroadcastOffsetSec: it.BroadcastOffsetSec, ReplayURL: sh.ReplayURL(it.BroadcastOffsetSec)}
		if sh.StartedAt != nil && it.BroadcastOffsetSec != nil {
			t := sh.StartedAt.Add(time.Duration(*it.BroadcastOffsetSec) * time.Second)
			b.ShownAt = &t
		}
		out = append(out, b)
	}
	return out, nil
}

// HideDue hides products whose items passed their hide time, unless another
// show still features them. It is the auto-hide job and safe to repeat.
func (s *Service) HideDue(ctx context.Context) (int, error) {
	due, err := s.Store.dueForHiding(ctx, 200)
	if err != nil {
		return 0, err
	}
	hidden := 0
	for _, it := range due {
		err := s.DB.InTx(ctx, func(ctx context.Context) error {
			tag, err := s.DB.Q(ctx).Exec(ctx,
				`UPDATE show_items SET hidden_at = now() WHERE id = $1 AND hidden_at IS NULL AND NOT keep_listed AND hide_at <= now()`, it.ID)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			featured, err := s.Store.stillFeatured(ctx, it.ProductID, it.ID)
			if err != nil || featured {
				return err
			}
			p, err := s.Catalog.Get(ctx, it.ProductID)
			if err != nil {
				return err
			}
			if p.Status == catalog.StatusLive {
				if err := s.Catalog.SetStatus(ctx, p.ID, catalog.StatusHidden); err != nil {
					return err
				}
				hidden++
			}
			return nil
		})
		if err != nil {
			return hidden, err
		}
	}
	return hidden, nil
}
