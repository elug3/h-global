-- Live: streams (shows), captured frames and numbered show items.

CREATE TABLE shows (
    id                 uuid        PRIMARY KEY DEFAULT uuidv7(),
    title              text        NOT NULL,
    youtube_video_id   text        NOT NULL DEFAULT '',
    status             text        NOT NULL CHECK (status IN ('draft', 'scheduled', 'live', 'ended')),
    scheduled_at       timestamptz,
    started_at         timestamptz,
    ended_at           timestamptz,
    display_days_after int         NOT NULL DEFAULT 3 CHECK (display_days_after BETWEEN 0 AND 365),
    next_number        int         NOT NULL DEFAULT 1 CHECK (next_number > 0),
    host_name          text        NOT NULL DEFAULT '',
    created_by         uuid        REFERENCES staff (id),
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now()
);
-- One live show at a time.
CREATE UNIQUE INDEX shows_one_live ON shows ((true)) WHERE status = 'live';
CREATE INDEX shows_status_scheduled ON shows (status, scheduled_at);

CREATE TABLE captures (
    id                   uuid        PRIMARY KEY DEFAULT uuidv7(),
    show_id              uuid        NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    object_key           text        NOT NULL,
    content_type         text        NOT NULL,
    width                int         NOT NULL,
    height               int         NOT NULL,
    broadcast_offset_sec int,
    source               text        NOT NULL CHECK (source IN ('upload', 'relay')),
    created_by           uuid        REFERENCES staff (id),
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX captures_show ON captures (show_id, created_at DESC);

CREATE TABLE show_items (
    id                   uuid        PRIMARY KEY DEFAULT uuidv7(),
    show_id              uuid        NOT NULL REFERENCES shows (id) ON DELETE CASCADE,
    number               int         NOT NULL CHECK (number > 0),
    source               text        NOT NULL CHECK (source IN ('preselected', 'captured', 'matched')),
    product_id           uuid        NOT NULL REFERENCES products (id),
    pinned               boolean     NOT NULL DEFAULT false,
    capture_id           uuid        REFERENCES captures (id),
    broadcast_offset_sec int,
    keep_listed          boolean     NOT NULL DEFAULT false,
    hide_at              timestamptz,
    hidden_at            timestamptz,
    published_at         timestamptz NOT NULL DEFAULT now(),
    published_by         uuid        REFERENCES staff (id),
    idempotency_key      text,
    -- Deferrable so preselected items can be renumbered inside one transaction.
    CONSTRAINT show_items_number UNIQUE (show_id, number) DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT show_items_product UNIQUE (show_id, product_id),
    CONSTRAINT show_items_idempotency UNIQUE (show_id, idempotency_key)
);
CREATE INDEX show_items_product_id ON show_items (product_id);
CREATE INDEX show_items_hide ON show_items (hide_at) WHERE hidden_at IS NULL;
