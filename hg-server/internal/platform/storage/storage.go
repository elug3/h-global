// Package storage stores files (photos, captured frames, inspection videos).
// The Storage interface matches what an S3-compatible bucket offers; Local is
// the filesystem implementation used in development and single-machine
// deployments. Keys are slash-separated, e.g. "media/2026/11/<id>.jpg".
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrNotFound is returned for missing keys.
var ErrNotFound = errors.New("object not found")

// Storage stores objects by key.
type Storage interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

// Prefixes. Only PublicPrefix is served without sign-in.
const (
	PublicPrefix  = "media/"    // product photos and product inspection media
	CapturePrefix = "captures/" // raw frames, staff only
	OrderPrefix   = "orders/"   // per-order inspection media
)

// ContentType guesses a content type from the key's extension.
func ContentType(key string) string {
	if t := mime.TypeByExtension(path.Ext(key)); t != "" {
		return t
	}
	return "application/octet-stream"
}

// Local stores objects under Dir.
type Local struct {
	Dir string
}

func (l Local) path(key string) (string, error) {
	clean := path.Clean("/" + key)[1:]
	if clean == "" || clean != key || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid key %q", key)
	}
	return filepath.Join(l.Dir, filepath.FromSlash(clean)), nil
}

// Put writes the object atomically (temp file + rename).
func (l Local) Put(_ context.Context, key string, r io.Reader) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".upload-*")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, r); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), p)
}

// Open opens the object for reading.
func (l Local) Open(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := l.path(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	return f, err
}

// Delete removes the object; a missing object is not an error.
func (l Local) Delete(_ context.Context, key string) error {
	p, err := l.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
