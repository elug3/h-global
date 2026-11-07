// Package live runs streams (shows): numbered items, captured frames, quick
// listing, pinning "worn or held today", the viewer feed and hiding items
// after a stream.
package live

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/catalog"
	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/imageutil"
)

// Show statuses.
const (
	ShowDraft     = "draft"
	ShowScheduled = "scheduled"
	ShowLive      = "live"
	ShowEnded     = "ended"
)

// Item sources.
const (
	SourcePreselected = "preselected" // picked before the stream (No. 1-8)
	SourceCaptured    = "captured"    // listed on air from a captured frame
	SourceMatched     = "matched"     // an existing product added on air
)

// NewBadgeFor is how long an item listed on air shows as NEW.
const NewBadgeFor = 15 * time.Minute

// Show is one stream.
type Show struct {
	ID               uuid.UUID  `json:"id"`
	Title            string     `json:"title"`
	YouTubeVideoID   string     `json:"youtube_video_id"`
	Status           string     `json:"status"`
	ScheduledAt      *time.Time `json:"scheduled_at"`
	StartedAt        *time.Time `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
	DisplayDaysAfter int        `json:"display_days_after"`
	NextNumber       int        `json:"next_number"`
	HostName         string     `json:"host_name"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// IngestStartedAt is when the broadcast relay first received the stream.
	// YouTube's timeline starts at about the same moment, so relay frames are
	// placed on it relative to this time.
	IngestStartedAt  *time.Time `json:"ingest_started_at"`
	IngestKeyVersion int        `json:"-"`
}

// WatchURL is the stream's YouTube page.
func (s Show) WatchURL() string {
	if s.YouTubeVideoID == "" {
		return ""
	}
	return "https://www.youtube.com/watch?v=" + s.YouTubeVideoID
}

// ReplayURL links to a moment of the stream.
func (s Show) ReplayURL(offsetSec *int) string {
	if s.YouTubeVideoID == "" || offsetSec == nil {
		return ""
	}
	return fmt.Sprintf("https://www.youtube.com/watch?v=%s&t=%ds", s.YouTubeVideoID, *offsetSec)
}

// Item is one numbered product in a show.
type Item struct {
	ID                 uuid.UUID  `json:"id"`
	ShowID             uuid.UUID  `json:"show_id"`
	Number             int        `json:"number"`
	Source             string     `json:"source"`
	ProductID          uuid.UUID  `json:"product_id"`
	Pinned             bool       `json:"pinned"`
	CaptureID          *uuid.UUID `json:"capture_id"`
	BroadcastOffsetSec *int       `json:"broadcast_offset_sec"`
	KeepListed         bool       `json:"keep_listed"`
	HideAt             *time.Time `json:"hide_at"`
	HiddenAt           *time.Time `json:"hidden_at"`
	PublishedAt        time.Time  `json:"published_at"`
	PublishedBy        *uuid.UUID `json:"published_by"`
}

// Capture is a frame (or photo) taken during a show.
type Capture struct {
	ID                 uuid.UUID  `json:"id"`
	ShowID             uuid.UUID  `json:"show_id"`
	ContentType        string     `json:"content_type"`
	Width              int        `json:"width"`
	Height             int        `json:"height"`
	BroadcastOffsetSec *int       `json:"broadcast_offset_sec"`
	Source             string     `json:"source"`
	CreatedBy          *uuid.UUID `json:"created_by"`
	CreatedAt          time.Time  `json:"created_at"`
	ImageURL           string     `json:"image_url"` // staff-only route
	ObjectKey          string     `json:"-"`
}

// ItemView is the storefront view of an item: what the live page shows.
type ItemView struct {
	ID                 uuid.UUID             `json:"id"`
	Number             int                   `json:"number"`
	Source             string                `json:"source"`
	Pinned             bool                  `json:"pinned"`
	IsNew              bool                  `json:"is_new"`
	PublishedAt        time.Time             `json:"published_at"`
	BroadcastOffsetSec *int                  `json:"broadcast_offset_sec"`
	ReplayURL          string                `json:"replay_url"`
	Product            catalog.PublicProduct `json:"product"`
}

// StudioItem is the staff view of an item, with stock bookkeeping.
type StudioItem struct {
	Item
	ReplayURL string          `json:"replay_url"`
	Product   catalog.Product `json:"product"`
}

// ShowView is a show with its items for the storefront. LastEventID is the
// point to open the event stream from.
type ShowView struct {
	Show
	WatchURL    string     `json:"watch_url"`
	Items       []ItemView `json:"items"`
	LastEventID int64      `json:"last_event_id"`
}

// StudioShow is a show with full items for H-Global Studio.
type StudioShow struct {
	Show
	WatchURL    string       `json:"watch_url"`
	Items       []StudioItem `json:"items"`
	LastEventID int64        `json:"last_event_id"`
}

// Broadcast is where a product was shown, for its product page.
type Broadcast struct {
	ShowID             uuid.UUID  `json:"show_id"`
	ShowTitle          string     `json:"show_title"`
	Number             int        `json:"number"`
	ShownAt            *time.Time `json:"shown_at"`
	BroadcastOffsetSec *int       `json:"broadcast_offset_sec"`
	ReplayURL          string     `json:"replay_url"`
}

// NewShow is the input for creating a show.
type NewShow struct {
	Title            string     `json:"title"`
	YouTubeURL       string     `json:"youtube_url"`
	ScheduledAt      *time.Time `json:"scheduled_at"`
	DisplayDaysAfter *int       `json:"display_days_after"`
	HostName         string     `json:"host_name"`
	Status           string     `json:"status"` // draft (default) or scheduled
}

// ShowPatch changes a show; nil fields are kept.
type ShowPatch struct {
	Title            *string    `json:"title"`
	YouTubeURL       *string    `json:"youtube_url"`
	ScheduledAt      *time.Time `json:"scheduled_at"`
	DisplayDaysAfter *int       `json:"display_days_after"`
	HostName         *string    `json:"host_name"`
	Status           *string    `json:"status"` // draft <-> scheduled only
}

// QuickListInput lists an item during a live show. For a new product,
// CaptureID and PriceWon are required. For an existing product, ProductID is
// set and PriceWon optionally changes its price.
type QuickListInput struct {
	CaptureID   *uuid.UUID          `json:"capture_id"`
	Crop        *imageutil.Rect     `json:"crop"`
	ProductID   *uuid.UUID          `json:"product_id"`
	Title       string              `json:"title"`
	Description string              `json:"description"`
	PriceWon    *int64              `json:"price_won"`
	Options     []catalog.NewOption `json:"options"`
	Stock       catalog.NewStock    `json:"stock"`

	IdempotencyKey string `json:"-"` // from the Idempotency-Key header
}

var youtubeIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// ParseYouTubeID accepts a video ID or a youtube.com / youtu.be link.
func ParseYouTubeID(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || youtubeIDRe.MatchString(s) {
		return s, nil
	}
	bad := apperr.Invalid("invalid_youtube_url", "not a YouTube video link")
	u, err := url.Parse(s)
	if err != nil {
		return "", bad
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	host = strings.TrimPrefix(host, "m.")
	var id string
	switch host {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com":
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		case strings.HasPrefix(u.Path, "/live/"), strings.HasPrefix(u.Path, "/shorts/"), strings.HasPrefix(u.Path, "/embed/"):
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if len(parts) == 2 {
				id = parts[1]
			}
		}
	}
	if !youtubeIDRe.MatchString(id) {
		return "", bad
	}
	return id, nil
}

func checkShowTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || len([]rune(t)) > 200 {
		return "", apperr.Invalid("invalid_title", "title must be 1 to 200 characters")
	}
	return t, nil
}

func checkDays(d int) error {
	if d < 0 || d > 365 {
		return apperr.Invalid("invalid_display_days", "display_days_after must be 0 to 365")
	}
	return nil
}
