// Package catalog holds products created on the spot: a product needs only a
// price (and, from a live stream, a photo). Options such as Color and Size are
// free text per product; there are no brand, style, color or size masters.
package catalog

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
)

// Product statuses.
const (
	StatusDraft    = "draft"
	StatusLive     = "live" // buyable and visible
	StatusHidden   = "hidden"
	StatusArchived = "archived"
)

// Product sources.
const (
	SourceLiveCapture = "live_capture"
	SourceManual      = "manual"
)

// Stock modes.
const (
	StockCounted      = "counted"
	StockUntilSoldOut = "until_sold_out" // sellable until staff mark it sold out
)

// Media kinds.
const (
	MediaPhoto           = "photo"
	MediaInspectionPhoto = "inspection_photo"
	MediaInspectionVideo = "inspection_video"
)

// Limits.
const (
	MaxPriceWon      = 10_000_000_000
	MaxOptions       = 3
	MaxValues        = 30
	MaxVariants      = 100
	MaxStockQty      = 100_000
	PlaceholderTitle = "Live #" // titles starting with this still need a real name
)

// Product is the staff view of a product.
type Product struct {
	ID              uuid.UUID         `json:"id"`
	Title           string            `json:"title"`
	Description     string            `json:"description"`
	PriceWon        int64             `json:"price_won"`
	Status          string            `json:"status"`
	Source          string            `json:"source"`
	Specs           map[string]string `json:"specs"`
	Tags            []string          `json:"tags"`
	DetailsComplete bool              `json:"details_complete"`
	CreatedBy       *uuid.UUID        `json:"created_by"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	Options         []Option          `json:"options"`
	Variants        []Variant         `json:"variants"`
	Media           []Media           `json:"media"`
}

// Option is one axis of choice, e.g. "Color".
type Option struct {
	ID       uuid.UUID     `json:"id"`
	Name     string        `json:"name"`
	Position int           `json:"position"`
	Values   []OptionValue `json:"values"`
}

// OptionValue is one choice, e.g. "Brown". Hex1 draws the swatch; Hex2 makes
// it two-tone.
type OptionValue struct {
	ID       uuid.UUID `json:"id"`
	Label    string    `json:"label"`
	Hex1     *string   `json:"hex1"`
	Hex2     *string   `json:"hex2"`
	Position int       `json:"position"`
}

// Variant is one sellable combination of option values.
type Variant struct {
	ID               uuid.UUID   `json:"id"`
	ProductID        uuid.UUID   `json:"product_id"`
	OptionValueIDs   []uuid.UUID `json:"option_value_ids"`
	Label            string      `json:"label"`
	Position         int         `json:"position"`
	PriceOverrideWon *int64      `json:"price_override_won"`
	PriceWon         int64       `json:"price_won"` // effective price
	StockMode        string      `json:"stock_mode"`
	StockQty         int         `json:"stock_qty"`
	HeldQty          int         `json:"held_qty"`
	SoldQty          int         `json:"sold_qty"`
	AvailableQty     *int        `json:"available_qty"` // nil = unlimited
	SoldOut          bool        `json:"sold_out"`
	Status           string      `json:"status"`
	Buyable          bool        `json:"buyable"`
}

// Media is a photo or inspection file.
type Media struct {
	ID          uuid.UUID `json:"id"`
	Kind        string    `json:"kind"`
	URL         string    `json:"url"`
	ContentType string    `json:"content_type"`
	Position    int       `json:"position"`
	ObjectKey   string    `json:"-"`
}

// FirstPhotoKey returns the first photo's storage key, or "".
func (p Product) FirstPhotoKey() string {
	for _, m := range p.Media {
		if m.Kind == MediaPhoto {
			return m.ObjectKey
		}
	}
	return ""
}

// Variant returns the variant with id.
func (p Product) Variant(id uuid.UUID) (Variant, bool) {
	for _, v := range p.Variants {
		if v.ID == id {
			return v, true
		}
	}
	return Variant{}, false
}

// SoldOut reports whether no variant can be bought.
func (p Product) SoldOut() bool {
	for _, v := range p.Variants {
		if v.Buyable {
			return false
		}
	}
	return true
}

func (p *Product) finish() {
	p.DetailsComplete = p.Title != "" && !strings.HasPrefix(p.Title, PlaceholderTitle) && p.Description != ""
	for i := range p.Variants {
		v := &p.Variants[i]
		v.PriceWon = p.PriceWon
		if v.PriceOverrideWon != nil {
			v.PriceWon = *v.PriceOverrideWon
		}
		v.Buyable = p.Status == StatusLive && v.Status == "active" && !v.SoldOut &&
			(v.AvailableQty == nil || *v.AvailableQty > 0)
	}
	if p.Specs == nil {
		p.Specs = map[string]string{}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Options == nil {
		p.Options = []Option{}
	}
	if p.Media == nil {
		p.Media = []Media{}
	}
}

// PublicProduct is what storefront sees: no stock bookkeeping.
type PublicProduct struct {
	ID          uuid.UUID         `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	PriceWon    int64             `json:"price_won"`
	Status      string            `json:"status"`
	Buyable     bool              `json:"buyable"`
	SoldOut     bool              `json:"sold_out"`
	Specs       map[string]string `json:"specs"`
	Tags        []string          `json:"tags"`
	Options     []Option          `json:"options"`
	Variants    []PublicVariant   `json:"variants"`
	Photos      []string          `json:"photos"`
	Inspection  []PublicMedia     `json:"inspection"`
}

// PublicVariant is a variant without stock bookkeeping.
type PublicVariant struct {
	ID             uuid.UUID   `json:"id"`
	Label          string      `json:"label"`
	OptionValueIDs []uuid.UUID `json:"option_value_ids"`
	PriceWon       int64       `json:"price_won"`
	AvailableQty   *int        `json:"available_qty"`
	SoldOut        bool        `json:"sold_out"`
	Buyable        bool        `json:"buyable"`
}

// PublicMedia is an inspection photo or video.
type PublicMedia struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// Public converts to the storefront view.
func (p Product) Public() PublicProduct {
	out := PublicProduct{
		ID: p.ID, Title: p.Title, Description: p.Description, PriceWon: p.PriceWon,
		Status: p.Status, Specs: p.Specs, Tags: p.Tags, Options: p.Options,
		Variants: []PublicVariant{}, Photos: []string{}, Inspection: []PublicMedia{},
	}
	for _, v := range p.Variants {
		if v.Status != "active" {
			continue
		}
		out.Variants = append(out.Variants, PublicVariant{
			ID: v.ID, Label: v.Label, OptionValueIDs: v.OptionValueIDs, PriceWon: v.PriceWon,
			AvailableQty: v.AvailableQty, SoldOut: v.SoldOut || !v.Buyable && p.Status == StatusLive, Buyable: v.Buyable,
		})
		if v.Buyable {
			out.Buyable = true
		}
	}
	out.SoldOut = p.Status == StatusLive && !out.Buyable
	for _, m := range p.Media {
		if m.Kind == MediaPhoto {
			out.Photos = append(out.Photos, m.URL)
		} else {
			out.Inspection = append(out.Inspection, PublicMedia{Kind: m.Kind, URL: m.URL})
		}
	}
	return out
}

// NewProduct is the input for creating a product.
type NewProduct struct {
	Title       string            `json:"title"`
	Description string            `json:"description"`
	PriceWon    int64             `json:"price_won"`
	Status      string            `json:"status"`
	Specs       map[string]string `json:"specs"`
	Tags        []string          `json:"tags"`
	Options     []NewOption       `json:"options"`
	Stock       NewStock          `json:"stock"`

	Source    string     `json:"-"`
	CreatedBy *uuid.UUID `json:"-"`
	PhotoKeys []string   `json:"-"` // already stored photos, in order
}

// NewOption is one option with its values.
type NewOption struct {
	Name   string           `json:"name"`
	Values []NewOptionValue `json:"values"`
}

// NewOptionValue is one value of a new option.
type NewOptionValue struct {
	Label string `json:"label"`
	Hex1  string `json:"hex1"`
	Hex2  string `json:"hex2"`
}

// NewStock applies to every generated variant.
type NewStock struct {
	Mode string `json:"mode"`
	Qty  int    `json:"qty"`
}

var hexRe = regexp.MustCompile(`^#[0-9a-f]{6}$`)

func normHex(s string) (*string, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return nil, nil
	}
	if !hexRe.MatchString(s) {
		return nil, apperr.Invalid("invalid_color", "%q is not a #rrggbb color", s)
	}
	return &s, nil
}

func checkPrice(p int64) error {
	if p < 0 || p > MaxPriceWon {
		return apperr.Invalid("invalid_price", "price must be between 0 and %d won", int64(MaxPriceWon))
	}
	return nil
}

func normTitle(t string) (string, error) {
	t = strings.TrimSpace(t)
	if t == "" || len([]rune(t)) > 200 {
		return "", apperr.Invalid("invalid_title", "title must be 1 to 200 characters")
	}
	return t, nil
}

func normSpecs(m map[string]string) (map[string]string, error) {
	if len(m) > 30 {
		return nil, apperr.Invalid("invalid_specs", "at most 30 specs")
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if k == "" || len([]rune(k)) > 50 || len([]rune(v)) > 500 {
			return nil, apperr.Invalid("invalid_specs", "spec names are 1 to 50 characters, values up to 500")
		}
		out[k] = v
	}
	return out, nil
}

func normTags(tags []string) ([]string, error) {
	out := []string{}
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" {
			continue
		}
		if len([]rune(t)) > 30 {
			return nil, apperr.Invalid("invalid_tags", "tags are up to 30 characters")
		}
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	if len(out) > 20 {
		return nil, apperr.Invalid("invalid_tags", "at most 20 tags")
	}
	return out, nil
}

func checkStock(mode string, qty int) error {
	switch mode {
	case StockCounted, StockUntilSoldOut:
	default:
		return apperr.Invalid("invalid_stock_mode", "stock mode must be counted or until_sold_out")
	}
	if qty < 0 || qty > MaxStockQty {
		return apperr.Invalid("invalid_stock", "stock must be between 0 and %d", MaxStockQty)
	}
	return nil
}

func checkStatus(s string) error {
	switch s {
	case StatusDraft, StatusLive, StatusHidden, StatusArchived:
		return nil
	}
	return apperr.Invalid("invalid_status", "status must be draft, live, hidden or archived")
}

// normalize validates n and fills defaults.
func (n *NewProduct) normalize() error {
	var err error
	if n.Title, err = normTitle(n.Title); err != nil {
		return err
	}
	n.Description = strings.TrimSpace(n.Description)
	if len([]rune(n.Description)) > 10_000 {
		return apperr.Invalid("invalid_description", "description is up to 10,000 characters")
	}
	if err := checkPrice(n.PriceWon); err != nil {
		return err
	}
	if n.Status == "" {
		n.Status = StatusDraft
	}
	if err := checkStatus(n.Status); err != nil {
		return err
	}
	if n.Source == "" {
		n.Source = SourceManual
	}
	if n.Specs, err = normSpecs(n.Specs); err != nil {
		return err
	}
	if n.Tags, err = normTags(n.Tags); err != nil {
		return err
	}
	if n.Stock.Mode == "" {
		n.Stock.Mode = StockCounted
	}
	if err := checkStock(n.Stock.Mode, n.Stock.Qty); err != nil {
		return err
	}
	if len(n.Options) > MaxOptions {
		return apperr.Invalid("invalid_options", "at most %d options", MaxOptions)
	}
	var names []string
	count := 1
	for i := range n.Options {
		o := &n.Options[i]
		o.Name = strings.TrimSpace(o.Name)
		if o.Name == "" || len([]rune(o.Name)) > 30 {
			return apperr.Invalid("invalid_options", "option names are 1 to 30 characters")
		}
		if slices.Contains(names, strings.ToLower(o.Name)) {
			return apperr.Invalid("invalid_options", "option %q appears twice", o.Name)
		}
		names = append(names, strings.ToLower(o.Name))
		if len(o.Values) == 0 || len(o.Values) > MaxValues {
			return apperr.Invalid("invalid_options", "option %q needs 1 to %d values", o.Name, MaxValues)
		}
		var labels []string
		for j := range o.Values {
			v := &o.Values[j]
			v.Label = strings.TrimSpace(v.Label)
			if v.Label == "" || len([]rune(v.Label)) > 40 {
				return apperr.Invalid("invalid_options", "values are 1 to 40 characters")
			}
			if slices.Contains(labels, strings.ToLower(v.Label)) {
				return apperr.Invalid("invalid_options", "value %q appears twice in %q", v.Label, o.Name)
			}
			labels = append(labels, strings.ToLower(v.Label))
			h1, err := normHex(v.Hex1)
			if err != nil {
				return err
			}
			h2, err := normHex(v.Hex2)
			if err != nil {
				return err
			}
			if h2 != nil && h1 == nil {
				return apperr.Invalid("invalid_color", "hex2 needs hex1")
			}
		}
		count *= len(o.Values)
	}
	if count > MaxVariants {
		return apperr.Invalid("too_many_variants", "options make %d variants; the limit is %d", count, MaxVariants)
	}
	return nil
}

// combinations returns every combination of value indexes, first option
// slowest: [[0 0] [0 1] [1 0] [1 1]].
func combinations(sizes []int) [][]int {
	out := [][]int{{}}
	for _, n := range sizes {
		var next [][]int
		for _, prefix := range out {
			for i := 0; i < n; i++ {
				next = append(next, append(append([]int{}, prefix...), i))
			}
		}
		out = next
	}
	return out
}
