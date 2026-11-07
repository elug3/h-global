package catalog

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
)

// Listener is told, inside the same transaction, that a product (its fields,
// variants, stock or media) changed. The live module uses it to push the
// item's new state to viewers.
type Listener func(ctx context.Context, productID uuid.UUID) error

// Service implements catalog use cases.
type Service struct {
	DB           *db.DB
	Store        Postgres
	MediaBaseURL string // e.g. "/media"

	listeners []Listener
}

// OnChange registers a listener.
func (s *Service) OnChange(l Listener) { s.listeners = append(s.listeners, l) }

func (s *Service) changed(ctx context.Context, productID uuid.UUID) error {
	if err := s.Store.touch(ctx, productID); err != nil {
		return err
	}
	for _, l := range s.listeners {
		if err := l(ctx, productID); err != nil {
			return err
		}
	}
	return nil
}

// MediaURL turns a storage key into a URL. Only public keys get one.
func (s *Service) MediaURL(key string) string {
	if !strings.HasPrefix(key, storage.PublicPrefix) {
		return ""
	}
	return s.MediaBaseURL + "/" + strings.TrimPrefix(key, storage.PublicPrefix)
}

// Create adds a product with its options, variants (every combination of
// option values), stock and photos.
func (s *Service) Create(ctx context.Context, n NewProduct) (Product, error) {
	if err := n.normalize(); err != nil {
		return Product{}, err
	}
	var out Product
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		id, err := s.Store.insert(ctx, n)
		if err != nil {
			return err
		}
		if out, err = s.Get(ctx, id); err != nil {
			return err
		}
		return s.changed(ctx, id)
	})
	return out, err
}

// Get returns one product.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Product, error) {
	list, err := s.Store.load(ctx, []uuid.UUID{id}, s.MediaURL)
	if err != nil {
		return Product{}, err
	}
	if len(list) == 0 {
		return Product{}, apperr.NotFound("product")
	}
	return list[0], nil
}

// ProductOfVariant returns the product that owns a variant, and the variant.
func (s *Service) ProductOfVariant(ctx context.Context, variantID uuid.UUID) (Product, Variant, error) {
	pid, err := s.Store.productIDOfVariant(ctx, variantID)
	if err != nil {
		return Product{}, Variant{}, err
	}
	p, err := s.Get(ctx, pid)
	if err != nil {
		return Product{}, Variant{}, err
	}
	v, _ := p.Variant(variantID)
	return p, v, nil
}

// GetMany returns products in the order of idList; missing ids are skipped.
func (s *Service) GetMany(ctx context.Context, idList []uuid.UUID) ([]Product, error) {
	return s.Store.load(ctx, idList, s.MediaURL)
}

// List returns products, newest first.
func (s *Service) List(ctx context.Context, f Filter) ([]Product, error) {
	if f.Limit <= 0 || f.Limit > 100 {
		f.Limit = 50
	}
	idList, err := s.Store.list(ctx, f)
	if err != nil {
		return nil, err
	}
	return s.Store.load(ctx, idList, s.MediaURL)
}

// ProductPatch changes product fields; nil fields are kept.
type ProductPatch struct {
	Title       *string            `json:"title"`
	Description *string            `json:"description"`
	PriceWon    *int64             `json:"price_won"`
	Status      *string            `json:"status"`
	Specs       *map[string]string `json:"specs"`
	Tags        *[]string          `json:"tags"`
}

// Update changes product fields.
func (s *Service) Update(ctx context.Context, id uuid.UUID, p ProductPatch) (Product, error) {
	var out Product
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		cur, err := s.Get(ctx, id)
		if err != nil {
			return err
		}
		if p.Title != nil {
			if cur.Title, err = normTitle(*p.Title); err != nil {
				return err
			}
		}
		if p.Description != nil {
			cur.Description = strings.TrimSpace(*p.Description)
		}
		if p.PriceWon != nil {
			if err := checkPrice(*p.PriceWon); err != nil {
				return err
			}
			cur.PriceWon = *p.PriceWon
		}
		if p.Status != nil {
			if err := checkStatus(*p.Status); err != nil {
				return err
			}
			cur.Status = *p.Status
		}
		if p.Specs != nil {
			if cur.Specs, err = normSpecs(*p.Specs); err != nil {
				return err
			}
		}
		if p.Tags != nil {
			if cur.Tags, err = normTags(*p.Tags); err != nil {
				return err
			}
		}
		if _, err := s.DB.Q(ctx).Exec(ctx,
			`UPDATE products SET title = $2, description = $3, price_won = $4, status = $5, specs = $6, tags = $7 WHERE id = $1`,
			id, cur.Title, cur.Description, cur.PriceWon, cur.Status, cur.Specs, cur.Tags); err != nil {
			return err
		}
		if err := s.changed(ctx, id); err != nil {
			return err
		}
		out, err = s.Get(ctx, id)
		return err
	})
	return out, err
}

// SetStatus changes only the status (used by the live module's auto-hide).
func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := s.Update(ctx, id, ProductPatch{Status: &status})
	return err
}

// VariantPatch changes a variant; nil fields are kept.
type VariantPatch struct {
	StockMode          *string `json:"stock_mode"`
	StockQty           *int    `json:"stock_qty"`
	PriceOverrideWon   *int64  `json:"price_override_won"`
	ClearPriceOverride bool    `json:"clear_price_override"`
	SoldOut            *bool   `json:"sold_out"`
	Status             *string `json:"status"`
}

// UpdateVariant changes stock, price override, sold-out flag or status.
func (s *Service) UpdateVariant(ctx context.Context, productID, variantID uuid.UUID, p VariantPatch) (Product, error) {
	var out Product
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		cur, err := s.Get(ctx, productID)
		if err != nil {
			return err
		}
		v, ok := cur.Variant(variantID)
		if !ok {
			return apperr.NotFound("variant")
		}
		if p.StockMode != nil {
			v.StockMode = *p.StockMode
		}
		if p.StockQty != nil {
			v.StockQty = *p.StockQty
		}
		if err := checkStock(v.StockMode, v.StockQty); err != nil {
			return err
		}
		if p.ClearPriceOverride {
			v.PriceOverrideWon = nil
		} else if p.PriceOverrideWon != nil {
			if err := checkPrice(*p.PriceOverrideWon); err != nil {
				return err
			}
			v.PriceOverrideWon = p.PriceOverrideWon
		}
		if p.SoldOut != nil {
			v.SoldOut = *p.SoldOut
		}
		if p.Status != nil {
			if *p.Status != "active" && *p.Status != "inactive" {
				return apperr.Invalid("invalid_status", "variant status must be active or inactive")
			}
			v.Status = *p.Status
		}
		_, err = s.DB.Q(ctx).Exec(ctx,
			`UPDATE variants SET stock_mode = $2, stock_qty = $3, price_override_won = $4, sold_out = $5, status = $6 WHERE id = $1`,
			v.ID, v.StockMode, v.StockQty, v.PriceOverrideWon, v.SoldOut, v.Status)
		if db.ConstraintViolated(err) == "variants_counted_stock" {
			return apperr.Conflict("stock_below_committed",
				"stock cannot be less than held plus sold (%d)", v.HeldQty+v.SoldQty)
		}
		if err != nil {
			return err
		}
		if err := s.changed(ctx, productID); err != nil {
			return err
		}
		out, err = s.Get(ctx, productID)
		return err
	})
	return out, err
}

// MarkSoldOut marks every variant of a product sold out.
func (s *Service) MarkSoldOut(ctx context.Context, productID uuid.UUID) (Product, error) {
	var out Product
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		tag, err := s.DB.Q(ctx).Exec(ctx, `UPDATE variants SET sold_out = true WHERE product_id = $1`, productID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return apperr.NotFound("product")
		}
		if err := s.changed(ctx, productID); err != nil {
			return err
		}
		out, err = s.Get(ctx, productID)
		return err
	})
	return out, err
}

// Hold reserves qty units of a variant for a buyer. It fails with
// "sold_out" when the stock is not there.
func (s *Service) Hold(ctx context.Context, variantID uuid.UUID, qty int) error {
	if qty <= 0 || qty > 100 {
		return apperr.Invalid("invalid_qty", "quantity must be 1 to 100")
	}
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		pid, err := s.Store.hold(ctx, variantID, qty)
		if err != nil {
			return err
		}
		return s.changed(ctx, pid)
	})
}

// Release returns held units (checkout expired or order canceled unpaid).
func (s *Service) Release(ctx context.Context, variantID uuid.UUID, qty int) error {
	return s.move(ctx, sqlRelease, variantID, qty)
}

// CommitHold turns held units into sold units (payment succeeded).
func (s *Service) CommitHold(ctx context.Context, variantID uuid.UUID, qty int) error {
	return s.move(ctx, sqlCommitHold, variantID, qty)
}

// ReturnSold puts sold units back (canceled before shipping).
func (s *Service) ReturnSold(ctx context.Context, variantID uuid.UUID, qty int) error {
	return s.move(ctx, sqlReturnSold, variantID, qty)
}

func (s *Service) move(ctx context.Context, sql string, variantID uuid.UUID, qty int) error {
	return s.DB.InTx(ctx, func(ctx context.Context) error {
		pid, err := s.Store.moveStock(ctx, sql, variantID, qty)
		if err != nil {
			return err
		}
		return s.changed(ctx, pid)
	})
}

// AddMedia records an already stored file on a product.
func (s *Service) AddMedia(ctx context.Context, productID uuid.UUID, kind, key, contentType string) (Media, error) {
	switch kind {
	case MediaPhoto, MediaInspectionPhoto, MediaInspectionVideo:
	default:
		return Media{}, apperr.Invalid("invalid_media_kind", "kind must be photo, inspection_photo or inspection_video")
	}
	m := Media{ID: ids.New(), Kind: kind, ObjectKey: key, ContentType: contentType, URL: s.MediaURL(key)}
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		err := s.DB.Q(ctx).QueryRow(ctx,
			`INSERT INTO product_media (id, product_id, kind, object_key, content_type, position)
			 SELECT $1, p.id, $3, $4, $5, coalesce((SELECT max(position) + 1 FROM product_media WHERE product_id = p.id AND kind = $3), 0)
			 FROM products p WHERE p.id = $2
			 RETURNING position`, m.ID, productID, kind, key, contentType).Scan(&m.Position)
		if db.IsNoRows(err) {
			return apperr.NotFound("product")
		}
		if err != nil {
			return err
		}
		return s.changed(ctx, productID)
	})
	return m, err
}

// DeleteMedia removes a media record and returns its storage key, so the
// caller can delete the file after commit.
func (s *Service) DeleteMedia(ctx context.Context, productID, mediaID uuid.UUID) (string, error) {
	var key string
	err := s.DB.InTx(ctx, func(ctx context.Context) error {
		err := s.DB.Q(ctx).QueryRow(ctx,
			`DELETE FROM product_media WHERE id = $1 AND product_id = $2 RETURNING object_key`, mediaID, productID).Scan(&key)
		if db.IsNoRows(err) {
			return apperr.NotFound("media")
		}
		if err != nil {
			return err
		}
		return s.changed(ctx, productID)
	})
	return key, err
}

// Suggestions returns the option values used most in the last 90 days for an
// option name, so "Brown" keeps one spelling and one swatch color.
func (s *Service) Suggestions(ctx context.Context, option string) ([]Suggestion, error) {
	option = strings.TrimSpace(option)
	if option == "" {
		return nil, apperr.Invalid("invalid_option", "option is required")
	}
	return s.Store.suggestions(ctx, option, 20)
}
