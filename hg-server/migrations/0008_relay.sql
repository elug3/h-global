-- Broadcast relay: the per-show ingest key is derived from a server secret and
-- this version (rotating the key bumps it), and the time the relay first
-- received the stream anchors replay offsets.
ALTER TABLE shows ADD COLUMN ingest_key_version int NOT NULL DEFAULT 1;
ALTER TABLE shows ADD COLUMN ingest_started_at timestamptz;
