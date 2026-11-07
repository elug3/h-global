-- Checkout: Buy now holds stock until the order is placed or the hold expires.

CREATE TABLE checkouts (
    id          uuid        PRIMARY KEY DEFAULT uuidv7(),
    customer_id uuid        NOT NULL REFERENCES customers (id),
    status      text        NOT NULL CHECK (status IN ('open', 'ordered', 'expired', 'canceled')),
    expires_at  timestamptz NOT NULL,
    shipping    jsonb,
    order_id    uuid        REFERENCES orders (id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX checkouts_customer ON checkouts (customer_id, created_at DESC);
CREATE INDEX checkouts_open_expiry ON checkouts (expires_at) WHERE status = 'open';

CREATE TABLE checkout_lines (
    checkout_id    uuid   NOT NULL REFERENCES checkouts (id) ON DELETE CASCADE,
    position       int    NOT NULL,
    product_id     uuid   NOT NULL REFERENCES products (id),
    variant_id     uuid   NOT NULL REFERENCES variants (id),
    title          text   NOT NULL,
    variant_label  text   NOT NULL,
    photo_key      text   NOT NULL DEFAULT '',
    unit_price_won bigint NOT NULL CHECK (unit_price_won >= 0),
    qty            int    NOT NULL CHECK (qty > 0),
    PRIMARY KEY (checkout_id, position)
);
