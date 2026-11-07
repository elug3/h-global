package catalog

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/db"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
)

// Postgres stores the catalog.
type Postgres struct{ DB *db.DB }

// Filter narrows List.
type Filter struct {
	Statuses   []string
	Query      string
	Incomplete bool // only products still missing details
	Limit      int
	Offset     int
}

func (s Postgres) insert(ctx context.Context, n NewProduct) (uuid.UUID, error) {
	q := s.DB.Q(ctx)
	id := ids.New()
	specs, _ := json.Marshal(n.Specs)
	if _, err := q.Exec(ctx,
		`INSERT INTO products (id, title, description, price_won, status, source, specs, tags, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		id, n.Title, n.Description, n.PriceWon, n.Status, n.Source, specs, n.Tags, n.CreatedBy); err != nil {
		return uuid.Nil, err
	}
	valueIDs := make([][]uuid.UUID, len(n.Options))
	sizes := make([]int, len(n.Options))
	labels := make([][]string, len(n.Options))
	for i, o := range n.Options {
		oid := ids.New()
		if _, err := q.Exec(ctx, `INSERT INTO product_options (id, product_id, name, position) VALUES ($1, $2, $3, $4)`,
			oid, id, o.Name, i); err != nil {
			return uuid.Nil, err
		}
		sizes[i] = len(o.Values)
		for j, v := range o.Values {
			vid := ids.New()
			h1, _ := normHex(v.Hex1)
			h2, _ := normHex(v.Hex2)
			if _, err := q.Exec(ctx,
				`INSERT INTO option_values (id, option_id, label, hex1, hex2, position) VALUES ($1, $2, $3, $4, $5, $6)`,
				vid, oid, v.Label, h1, h2, j); err != nil {
				return uuid.Nil, err
			}
			valueIDs[i] = append(valueIDs[i], vid)
			labels[i] = append(labels[i], v.Label)
		}
	}
	for pos, combo := range combinations(sizes) {
		vids := make([]uuid.UUID, len(combo))
		parts := make([]string, len(combo))
		for i, k := range combo {
			vids[i] = valueIDs[i][k]
			parts[i] = labels[i][k]
		}
		if _, err := q.Exec(ctx,
			`INSERT INTO variants (id, product_id, option_value_ids, label, position, stock_mode, stock_qty)
			 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			ids.New(), id, vids, strings.Join(parts, " / "), pos, n.Stock.Mode, n.Stock.Qty); err != nil {
			return uuid.Nil, err
		}
	}
	for i, key := range n.PhotoKeys {
		if _, err := q.Exec(ctx,
			`INSERT INTO product_media (id, product_id, kind, object_key, content_type, position) VALUES ($1, $2, 'photo', $3, 'image/jpeg', $4)`,
			ids.New(), id, key, i); err != nil {
			return uuid.Nil, err
		}
	}
	return id, nil
}

// load reads full products for ids, in the order given.
func (s Postgres) load(ctx context.Context, idList []uuid.UUID, mediaURL func(string) string) ([]Product, error) {
	if len(idList) == 0 {
		return []Product{}, nil
	}
	q := s.DB.Q(ctx)
	byID := map[uuid.UUID]*Product{}
	rows, err := q.Query(ctx,
		`SELECT id, title, description, price_won, status, source, specs, tags, created_by, created_at, updated_at
		 FROM products WHERE id = ANY($1)`, idList)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p Product
		var specs []byte
		if err := rows.Scan(&p.ID, &p.Title, &p.Description, &p.PriceWon, &p.Status, &p.Source, &specs,
			&p.Tags, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal(specs, &p.Specs)
		byID[p.ID] = &p
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	optByID := map[uuid.UUID]*Option{}
	optOwner := map[uuid.UUID]uuid.UUID{}
	var optOrder []uuid.UUID
	rows, err = q.Query(ctx,
		`SELECT id, product_id, name, position FROM product_options WHERE product_id = ANY($1) ORDER BY product_id, position`, idList)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var o Option
		var pid uuid.UUID
		if err := rows.Scan(&o.ID, &pid, &o.Name, &o.Position); err != nil {
			rows.Close()
			return nil, err
		}
		o.Values = []OptionValue{}
		optByID[o.ID] = &o
		optOwner[o.ID] = pid
		optOrder = append(optOrder, o.ID)
	}
	rows.Close()
	if len(optOrder) > 0 {
		rows, err = q.Query(ctx,
			`SELECT id, option_id, label, hex1, hex2, position FROM option_values WHERE option_id = ANY($1) ORDER BY option_id, position`, optOrder)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var v OptionValue
			var oid uuid.UUID
			if err := rows.Scan(&v.ID, &oid, &v.Label, &v.Hex1, &v.Hex2, &v.Position); err != nil {
				rows.Close()
				return nil, err
			}
			optByID[oid].Values = append(optByID[oid].Values, v)
		}
		rows.Close()
	}
	for _, oid := range optOrder {
		if p := byID[optOwner[oid]]; p != nil {
			p.Options = append(p.Options, *optByID[oid])
		}
	}

	rows, err = q.Query(ctx,
		`SELECT id, product_id, option_value_ids, label, position, price_override_won, stock_mode,
		        stock_qty, held_qty, sold_qty, available_qty, sold_out, status
		 FROM variants WHERE product_id = ANY($1) ORDER BY product_id, position`, idList)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		v, err := scanVariant(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if p := byID[v.ProductID]; p != nil {
			p.Variants = append(p.Variants, v)
		}
	}
	rows.Close()

	rows, err = q.Query(ctx,
		`SELECT id, product_id, kind, object_key, content_type, position FROM product_media
		 WHERE product_id = ANY($1) ORDER BY product_id, kind, position`, idList)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var m Media
		var pid uuid.UUID
		if err := rows.Scan(&m.ID, &pid, &m.Kind, &m.ObjectKey, &m.ContentType, &m.Position); err != nil {
			rows.Close()
			return nil, err
		}
		m.URL = mediaURL(m.ObjectKey)
		if p := byID[pid]; p != nil {
			p.Media = append(p.Media, m)
		}
	}
	rows.Close()

	out := make([]Product, 0, len(idList))
	for _, id := range idList {
		if p := byID[id]; p != nil {
			p.finish()
			out = append(out, *p)
		}
	}
	return out, nil
}

func scanVariant(row pgx.Row) (Variant, error) {
	var v Variant
	err := row.Scan(&v.ID, &v.ProductID, &v.OptionValueIDs, &v.Label, &v.Position, &v.PriceOverrideWon,
		&v.StockMode, &v.StockQty, &v.HeldQty, &v.SoldQty, &v.AvailableQty, &v.SoldOut, &v.Status)
	if v.OptionValueIDs == nil {
		v.OptionValueIDs = []uuid.UUID{}
	}
	return v, err
}

func (s Postgres) list(ctx context.Context, f Filter) ([]uuid.UUID, error) {
	where := []string{"true"}
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.ReplaceAll(cond, "?", "$"+strconv.Itoa(len(args))))
	}
	if len(f.Statuses) > 0 {
		add("status = ANY(?)", f.Statuses)
	}
	if f.Query != "" {
		add("title ILIKE ?", "%"+escapeLike(f.Query)+"%")
	}
	if f.Incomplete {
		where = append(where, "(description = '' OR title LIKE 'Live #%')")
	}
	args = append(args, f.Limit, f.Offset)
	rows, err := s.DB.Q(ctx).Query(ctx,
		`SELECT id FROM products WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC LIMIT $`+strconv.Itoa(len(args)-1)+` OFFSET $`+strconv.Itoa(len(args)), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s Postgres) productIDOfVariant(ctx context.Context, variantID uuid.UUID) (uuid.UUID, error) {
	var pid uuid.UUID
	err := s.DB.Q(ctx).QueryRow(ctx, `SELECT product_id FROM variants WHERE id = $1`, variantID).Scan(&pid)
	if db.IsNoRows(err) {
		return uuid.Nil, apperr.NotFound("variant")
	}
	return pid, err
}

// hold reserves qty of a live, buyable variant in one conditional update, so
// two buyers can never both take the last unit.
func (s Postgres) hold(ctx context.Context, variantID uuid.UUID, qty int) (uuid.UUID, error) {
	var pid uuid.UUID
	err := s.DB.Q(ctx).QueryRow(ctx,
		`UPDATE variants v SET held_qty = v.held_qty + $2
		 FROM products p
		 WHERE v.id = $1 AND p.id = v.product_id AND p.status = 'live'
		   AND v.status = 'active' AND NOT v.sold_out
		   AND (v.stock_mode = 'until_sold_out' OR v.stock_qty - v.held_qty - v.sold_qty >= $2)
		 RETURNING v.product_id`, variantID, qty).Scan(&pid)
	if db.IsNoRows(err) {
		if _, err := s.productIDOfVariant(ctx, variantID); err != nil {
			return uuid.Nil, err
		}
		return uuid.Nil, apperr.Conflict("sold_out", "not enough stock").WithDetails(map[string]any{"variant_id": variantID})
	}
	return pid, err
}

// moveStock runs one bookkeeping update that must affect exactly one row.
func (s Postgres) moveStock(ctx context.Context, sql string, variantID uuid.UUID, qty int) (uuid.UUID, error) {
	var pid uuid.UUID
	err := s.DB.Q(ctx).QueryRow(ctx, sql, variantID, qty).Scan(&pid)
	if db.IsNoRows(err) {
		return uuid.Nil, apperr.Conflict("stock_mismatch", "stock bookkeeping does not match for variant %s", variantID)
	}
	return pid, err
}

const (
	sqlRelease    = `UPDATE variants SET held_qty = held_qty - $2 WHERE id = $1 AND held_qty >= $2 RETURNING product_id`
	sqlCommitHold = `UPDATE variants SET held_qty = held_qty - $2, sold_qty = sold_qty + $2 WHERE id = $1 AND held_qty >= $2 RETURNING product_id`
	sqlReturnSold = `UPDATE variants SET sold_qty = sold_qty - $2 WHERE id = $1 AND sold_qty >= $2 RETURNING product_id`
)

func (s Postgres) touch(ctx context.Context, productID uuid.UUID) error {
	_, err := s.DB.Q(ctx).Exec(ctx, `UPDATE products SET updated_at = now() WHERE id = $1`, productID)
	return err
}

// Suggestion is a recently used option value.
type Suggestion struct {
	Label string  `json:"label"`
	Hex1  *string `json:"hex1"`
	Hex2  *string `json:"hex2"`
	Uses  int     `json:"uses"`
}

func (s Postgres) suggestions(ctx context.Context, option string, limit int) ([]Suggestion, error) {
	rows, err := s.DB.Q(ctx).Query(ctx,
		`SELECT label, hex1, hex2, uses FROM (
		   SELECT DISTINCT ON (lower(ov.label)) ov.label, ov.hex1, ov.hex2,
		          count(*) OVER (PARTITION BY lower(ov.label)) AS uses, ov.created_at
		   FROM option_values ov JOIN product_options po ON po.id = ov.option_id
		   WHERE lower(po.name) = lower($1) AND ov.created_at > now() - interval '90 days'
		   ORDER BY lower(ov.label), ov.created_at DESC
		 ) s ORDER BY uses DESC, created_at DESC LIMIT $2`, option, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Suggestion, error) {
		var sg Suggestion
		err := r.Scan(&sg.Label, &sg.Hex1, &sg.Hex2, &sg.Uses)
		return sg, err
	})
}
