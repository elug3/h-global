// Package ids creates identifiers. Every entity uses a UUIDv7: time-ordered,
// so new rows land at the end of indexes, and the same kind of value that
// PostgreSQL 18's uuidv7() produces as a column default.
package ids

import "github.com/google/uuid"

// New returns a new UUIDv7.
func New() uuid.UUID {
	return uuid.Must(uuid.NewV7())
}

// Parse parses a UUID from a path or body value.
func Parse(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}
