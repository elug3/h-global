-- Platform: event log (SSE replay) and sessions.

CREATE TABLE events (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    topic      text        NOT NULL,
    type       text        NOT NULL,
    payload    jsonb       NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX events_topic_id ON events (topic, id);
CREATE INDEX events_created_at ON events (created_at);

-- Only a SHA-256 of the session token is stored.
CREATE TABLE sessions (
    token_hash text        PRIMARY KEY,
    kind       text        NOT NULL CHECK (kind IN ('staff', 'customer')),
    subject_id uuid        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_subject ON sessions (kind, subject_id);
CREATE INDEX sessions_expires ON sessions (expires_at);
