-- Catalog: products created on the spot. No brand, style, color or size
-- masters: options and their values belong to each product.

CREATE TABLE products (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    title       text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    price_won   bigint      NOT NULL CHECK (price_won >= 0),
    status      text        NOT NULL CHECK (status IN ('draft', 'live', 'hidden', 'archived')),
    source      text        NOT NULL CHECK (source IN ('live_capture', 'manual')),
    specs       jsonb       NOT NULL DEFAULT '{}'::jsonb,
    tags        text[]      NOT NULL DEFAULT '{}',
    created_by  uuid        REFERENCES staff (id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_status_created ON products (status, created_at DESC);

CREATE TABLE product_options (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    product_id uuid NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    name       text NOT NULL,
    position   int  NOT NULL,
    UNIQUE (product_id, position)
);

CREATE TABLE option_values (
    id         uuid        PRIMARY KEY DEFAULT uuidv7(),
    option_id  uuid        NOT NULL REFERENCES product_options (id) ON DELETE CASCADE,
    label      text        NOT NULL,
    hex1       text        CHECK (hex1 ~ '^#[0-9a-f]{6}$'),
    hex2       text        CHECK (hex2 ~ '^#[0-9a-f]{6}$'),
    position   int         NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (option_id, position)
);
CREATE INDEX option_values_created ON option_values (created_at DESC);

CREATE TABLE variants (
    id                 uuid    PRIMARY KEY DEFAULT uuidv7(),
    product_id         uuid    NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    option_value_ids   uuid[]  NOT NULL DEFAULT '{}',
    label              text    NOT NULL DEFAULT '',
    position           int     NOT NULL,
    price_override_won bigint  CHECK (price_override_won >= 0),
    stock_mode         text    NOT NULL CHECK (stock_mode IN ('counted', 'until_sold_out')),
    stock_qty          int     NOT NULL DEFAULT 0 CHECK (stock_qty >= 0),
    held_qty           int     NOT NULL DEFAULT 0 CHECK (held_qty >= 0),
    sold_qty           int     NOT NULL DEFAULT 0 CHECK (sold_qty >= 0),
    sold_out           boolean NOT NULL DEFAULT false,
    status             text    NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    -- PostgreSQL 18 virtual generated column: computed when read, never stored.
    -- NULL means unlimited (stock mode until_sold_out).
    available_qty      int GENERATED ALWAYS AS (
        CASE WHEN stock_mode = 'counted' THEN stock_qty - held_qty - sold_qty END
    ) VIRTUAL,
    UNIQUE (product_id, position),
    CONSTRAINT variants_counted_stock CHECK (stock_mode <> 'counted' OR held_qty + sold_qty <= stock_qty)
);
CREATE INDEX variants_product ON variants (product_id);

CREATE TABLE product_media (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    product_id   uuid        NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('photo', 'inspection_photo', 'inspection_video')),
    object_key   text        NOT NULL,
    content_type text        NOT NULL,
    position     int         NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX product_media_product ON product_media (product_id, kind, position);
