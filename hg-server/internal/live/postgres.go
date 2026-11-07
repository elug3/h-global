package live

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
)

// Postgres stores shows, items and captures.
type Postgres struct{ DB *db.DB }

const showCols = `id, title, youtube_video_id, status, scheduled_at, started_at, ended_at,
	display_days_after, next_number, host_name, created_at, updated_at, ingest_key_version, ingest_started_at`

func scanShow(row pgx.Row) (Show, error) {
	var s Show
	err := row.Scan(&s.ID, &s.Title, &s.YouTubeVideoID, &s.Status, &s.ScheduledAt, &s.StartedAt, &s.EndedAt,
		&s.DisplayDaysAfter, &s.NextNumber, &s.HostName, &s.CreatedAt, &s.UpdatedAt, &s.IngestKeyVersion, &s.IngestStartedAt)
	if db.IsNoRows(err) {
		return s, apperr.NotFound("show")
	}
	return s, err
}

func (p Postgres) show(ctx context.Context, id uuid.UUID, forUpdate bool) (Show, error) {
	sql := `SELECT ` + showCols + ` FROM shows WHERE id = $1`
	if forUpdate {
		sql += ` FOR UPDATE`
	}
	return scanShow(p.DB.Q(ctx).QueryRow(ctx, sql, id))
}

func (p Postgres) shows(ctx context.Context, sql string, args ...any) ([]Show, error) {
	rows, err := p.DB.Q(ctx).Query(ctx, `SELECT `+showCols+` FROM shows `+sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Show, error) { return scanShow(r) })
}

func (p Postgres) insertShow(ctx context.Context, s Show, createdBy uuid.UUID) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO shows (id, title, youtube_video_id, status, scheduled_at, display_days_after, host_name, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		s.ID, s.Title, s.YouTubeVideoID, s.Status, s.ScheduledAt, s.DisplayDaysAfter, s.HostName, createdBy)
	return err
}

func (p Postgres) saveShow(ctx context.Context, s Show) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`UPDATE shows SET title = $2, youtube_video_id = $3, status = $4, scheduled_at = $5, started_at = $6,
		        ended_at = $7, display_days_after = $8, next_number = $9, host_name = $10, updated_at = now()
		 WHERE id = $1`,
		s.ID, s.Title, s.YouTubeVideoID, s.Status, s.ScheduledAt, s.StartedAt, s.EndedAt,
		s.DisplayDaysAfter, s.NextNumber, s.HostName)
	if db.ConstraintViolated(err) == "shows_one_live" {
		return apperr.Conflict("another_show_live", "another show is live; end it first")
	}
	return err
}

const itemCols = `id, show_id, number, source, product_id, pinned, capture_id, broadcast_offset_sec,
	keep_listed, hide_at, hidden_at, published_at, published_by`

func scanItem(row pgx.Row) (Item, error) {
	var it Item
	err := row.Scan(&it.ID, &it.ShowID, &it.Number, &it.Source, &it.ProductID, &it.Pinned, &it.CaptureID,
		&it.BroadcastOffsetSec, &it.KeepListed, &it.HideAt, &it.HiddenAt, &it.PublishedAt, &it.PublishedBy)
	if db.IsNoRows(err) {
		return it, apperr.NotFound("item")
	}
	return it, err
}

func (p Postgres) items(ctx context.Context, sql string, args ...any) ([]Item, error) {
	rows, err := p.DB.Q(ctx).Query(ctx, `SELECT `+itemCols+` FROM show_items `+sql, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Item, error) { return scanItem(r) })
}

func (p Postgres) item(ctx context.Context, id uuid.UUID) (Item, error) {
	return scanItem(p.DB.Q(ctx).QueryRow(ctx, `SELECT `+itemCols+` FROM show_items WHERE id = $1`, id))
}

func (p Postgres) itemByKey(ctx context.Context, showID uuid.UUID, key string) (Item, bool, error) {
	it, err := scanItem(p.DB.Q(ctx).QueryRow(ctx,
		`SELECT `+itemCols+` FROM show_items WHERE show_id = $1 AND idempotency_key = $2`, showID, key))
	if apperr.Is(err, "item_not_found") {
		return it, false, nil
	}
	return it, err == nil, err
}

func (p Postgres) insertItem(ctx context.Context, it Item, key string) error {
	var k *string
	if key != "" {
		k = &key
	}
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO show_items (id, show_id, number, source, product_id, capture_id, broadcast_offset_sec, published_by, idempotency_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		it.ID, it.ShowID, it.Number, it.Source, it.ProductID, it.CaptureID, it.BroadcastOffsetSec, it.PublishedBy, k)
	if db.ConstraintViolated(err) == "show_items_product" {
		return apperr.Conflict("already_in_show", "this product is already an item in this show")
	}
	return err
}

func (p Postgres) insertCapture(ctx context.Context, c Capture) error {
	_, err := p.DB.Q(ctx).Exec(ctx,
		`INSERT INTO captures (id, show_id, object_key, content_type, width, height, broadcast_offset_sec, source, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		c.ID, c.ShowID, c.ObjectKey, c.ContentType, c.Width, c.Height, c.BroadcastOffsetSec, c.Source, c.CreatedBy)
	return err
}

func (p Postgres) capture(ctx context.Context, id uuid.UUID) (Capture, error) {
	var c Capture
	err := p.DB.Q(ctx).QueryRow(ctx,
		`SELECT id, show_id, object_key, content_type, width, height, broadcast_offset_sec, source, created_by, created_at
		 FROM captures WHERE id = $1`, id).
		Scan(&c.ID, &c.ShowID, &c.ObjectKey, &c.ContentType, &c.Width, &c.Height, &c.BroadcastOffsetSec, &c.Source, &c.CreatedBy, &c.CreatedAt)
	if db.IsNoRows(err) {
		return c, apperr.NotFound("capture")
	}
	return c, err
}

// dueForHiding returns items whose hide time has passed.
func (p Postgres) dueForHiding(ctx context.Context, limit int) ([]Item, error) {
	return p.items(ctx, `WHERE hidden_at IS NULL AND NOT keep_listed AND hide_at <= now() ORDER BY hide_at LIMIT $1`, limit)
}

// stillFeatured reports whether a product is still on sale through another
// show item: one in a show that has not ended, kept listed, or not yet due.
func (p Postgres) stillFeatured(ctx context.Context, productID, exceptItem uuid.UUID) (bool, error) {
	var ok bool
	err := p.DB.Q(ctx).QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM show_items si JOIN shows s ON s.id = si.show_id
		   WHERE si.product_id = $1 AND si.id <> $2 AND si.hidden_at IS NULL
		     AND (s.status <> 'ended' OR si.keep_listed OR si.hide_at > now()))`,
		productID, exceptItem).Scan(&ok)
	return ok, err
}
