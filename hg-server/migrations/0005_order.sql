-- Orders: order -> inspection -> customs check -> shipping.

CREATE SEQUENCE order_number_seq;

CREATE TABLE orders (
    id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
    number             text        NOT NULL UNIQUE,
    customer_id        uuid        NOT NULL REFERENCES customers (id),
    status             text        NOT NULL CHECK (status IN (
        'pending_payment', 'paid', 'inspecting', 'customs_check',
        'shipped', 'delivered', 'completed', 'canceled')),
    subtotal_won       bigint      NOT NULL CHECK (subtotal_won >= 0),
    shipping_fee_won   bigint      NOT NULL CHECK (shipping_fee_won >= 0),
    total_won          bigint      NOT NULL CHECK (total_won >= 0),
    refunded_won       bigint      NOT NULL DEFAULT 0 CHECK (refunded_won >= 0),
    shipping           jsonb       NOT NULL,
    payment_method     text        NOT NULL CHECK (payment_method IN ('card', 'virtual_account')),
    payment_deadline   timestamptz NOT NULL,
    paid_at            timestamptz,
    inspection_done_at timestamptz,
    customs_checked_at timestamptz,
    customs_checked_by uuid        REFERENCES staff (id),
    shipped_at         timestamptz,
    carrier            text        NOT NULL DEFAULT '',
    tracking_number    text        NOT NULL DEFAULT '',
    delivered_at       timestamptz,
    completed_at       timestamptz,
    canceled_at        timestamptz,
    cancel_reason      text        NOT NULL DEFAULT '',
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX orders_customer ON orders (customer_id, created_at DESC);
CREATE INDEX orders_status ON orders (status, created_at DESC);
CREATE INDEX orders_pending_deadline ON orders (payment_deadline) WHERE status = 'pending_payment';

-- A snapshot of what was bought; later product edits do not change it.
CREATE TABLE order_lines (
    id             uuid   PRIMARY KEY DEFAULT uuidv7(),
    order_id       uuid   NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    position       int    NOT NULL,
    product_id     uuid   NOT NULL REFERENCES products (id),
    variant_id     uuid   NOT NULL REFERENCES variants (id),
    title          text   NOT NULL,
    variant_label  text   NOT NULL,
    photo_key      text   NOT NULL DEFAULT '',
    unit_price_won bigint NOT NULL CHECK (unit_price_won >= 0),
    qty            int    NOT NULL CHECK (qty > 0),
    UNIQUE (order_id, position)
);

-- Inspection photos and videos for one order.
CREATE TABLE order_media (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    order_id     uuid        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    kind         text        NOT NULL CHECK (kind IN ('photo', 'video')),
    object_key   text        NOT NULL,
    content_type text        NOT NULL,
    created_by   uuid        REFERENCES staff (id),
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_media_order ON order_media (order_id, created_at);

CREATE TABLE order_history (
    id          bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id    uuid        NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    from_status text        NOT NULL DEFAULT '',
    to_status   text        NOT NULL,
    actor_kind  text        NOT NULL CHECK (actor_kind IN ('customer', 'staff', 'system', 'payment')),
    actor_id    uuid,
    note        text        NOT NULL DEFAULT '',
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_history_order ON order_history (order_id, id);
