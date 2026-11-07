// Package upload reads multipart file uploads and stores them.
package upload

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"path"
	"time"

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
	"github.com/elug3/h-global/hg-server/internal/platform/ids"
	"github.com/elug3/h-global/hg-server/internal/platform/imageutil"
	"github.com/elug3/h-global/hg-server/internal/platform/storage"
)

// Limits.
const (
	MaxImageBytes = 15 << 20
	MaxVideoBytes = 200 << 20
)

// File is a stored upload.
type File struct {
	Key         string
	ContentType string
	Width       int // images only
	Height      int
}

// Read returns the bytes of form field "file" and the other form values.
func Read(r *http.Request, maxBytes int64) ([]byte, map[string]string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxBytes+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		return nil, nil, apperr.Invalid("invalid_upload", "expected multipart/form-data")
	}
	var data []byte
	fields := map[string]string{}
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, apperr.Invalid("invalid_upload", "could not read upload")
		}
		if part.FormName() == "file" {
			data, err = io.ReadAll(io.LimitReader(part, maxBytes+1))
			if err != nil {
				return nil, nil, apperr.Invalid("invalid_upload", "could not read file")
			}
			if int64(len(data)) > maxBytes {
				return nil, nil, apperr.Invalid("file_too_large", "file is larger than %d MB", maxBytes>>20)
			}
			continue
		}
		v, _ := io.ReadAll(io.LimitReader(part, 4096))
		fields[part.FormName()] = string(v)
	}
	if len(data) == 0 {
		return nil, fields, apperr.Invalid("invalid_upload", "form field \"file\" is missing or empty")
	}
	return data, fields, nil
}

// Key builds a storage key: <prefix><yyyy>/<mm>/<id><ext>.
func Key(prefix, ext string) string {
	now := time.Now().UTC()
	return path.Join(prefix+now.Format("2006"), now.Format("01"), ids.New().String()+ext)
}

// StoreImage re-encodes an image as JPEG (crop optional) and stores it.
func StoreImage(ctx context.Context, st storage.Storage, prefix string, data []byte, crop *imageutil.Rect) (File, error) {
	out, w, h, err := imageutil.CropJPEG(data, crop)
	if err != nil {
		return File{}, err
	}
	key := Key(prefix, ".jpg")
	if err := st.Put(ctx, key, bytes.NewReader(out)); err != nil {
		return File{}, err
	}
	return File{Key: key, ContentType: "image/jpeg", Width: w, Height: h}, nil
}

// StoreOriginalImage stores an image unchanged after checking it is one
// (used for captured frames, which must stay full quality).
func StoreOriginalImage(ctx context.Context, st storage.Storage, prefix string, data []byte) (File, error) {
	format, w, h, err := imageutil.Info(data)
	if err != nil {
		return File{}, err
	}
	ext, ct := ".jpg", "image/jpeg"
	if format == "png" {
		ext, ct = ".png", "image/png"
	}
	key := Key(prefix, ext)
	if err := st.Put(ctx, key, bytes.NewReader(data)); err != nil {
		return File{}, err
	}
	return File{Key: key, ContentType: ct, Width: w, Height: h}, nil
}

// VideoType recognizes MP4/QuickTime and WebM by their signatures.
func VideoType(data []byte) (contentType, ext string, ok bool) {
	switch {
	case len(data) >= 12 && string(data[4:8]) == "ftyp":
		if string(data[8:10]) == "qt" {
			return "video/quicktime", ".mov", true
		}
		return "video/mp4", ".mp4", true
	case len(data) >= 4 && bytes.Equal(data[:4], []byte{0x1A, 0x45, 0xDF, 0xA3}):
		return "video/webm", ".webm", true
	}
	return "", "", false
}

// StoreVideo stores an MP4, QuickTime or WebM video.
func StoreVideo(ctx context.Context, st storage.Storage, prefix string, data []byte) (File, error) {
	ct, ext, ok := VideoType(data)
	if !ok {
		return File{}, apperr.Invalid("invalid_video", "video must be MP4, QuickTime or WebM")
	}
	key := Key(prefix, ext)
	if err := st.Put(ctx, key, bytes.NewReader(data)); err != nil {
		return File{}, err
	}
	return File{Key: key, ContentType: ct}, nil
}
