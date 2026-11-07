-- Payments through one provider adapter; webhook receipts make webhooks idempotent.

CREATE TABLE payments (
    id              uuid        PRIMARY KEY DEFAULT uuidv7(),
    order_id        uuid        NOT NULL REFERENCES orders (id),
    provider        text        NOT NULL,
    method          text        NOT NULL CHECK (method IN ('card', 'virtual_account')),
    status          text        NOT NULL CHECK (status IN ('pending', 'succeeded', 'failed', 'canceled')),
    amount_won      bigint      NOT NULL CHECK (amount_won >= 0),
    refunded_won    bigint      NOT NULL DEFAULT 0 CHECK (refunded_won >= 0),
    provider_ref    text        NOT NULL DEFAULT '',
    redirect_url    text        NOT NULL DEFAULT '',
    virtual_account jsonb,
    failure_reason  text        NOT NULL DEFAULT '',
    succeeded_at    timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),
    CHECK (refunded_won <= amount_won)
);
CREATE UNIQUE INDEX payments_provider_ref ON payments (provider, provider_ref) WHERE provider_ref <> '';
CREATE INDEX payments_order ON payments (order_id, created_at DESC);

CREATE TABLE refunds (
    id           uuid        PRIMARY KEY DEFAULT uuidv7(),
    payment_id   uuid        NOT NULL REFERENCES payments (id),
    amount_won   bigint      NOT NULL CHECK (amount_won > 0),
    reason       text        NOT NULL DEFAULT '',
    provider_ref text        NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE webhook_receipts (
    provider    text        NOT NULL,
    event_id    text        NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (provider, event_id)
);
