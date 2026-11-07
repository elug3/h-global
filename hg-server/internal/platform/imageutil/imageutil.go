// Package imageutil validates, crops and re-encodes uploaded images.
// Re-encoding strips metadata (camera location, etc.) from photos that become
// public.
package imageutil

import (
	"bytes"
	"image"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register PNG decoding

	"github.com/elug3/h-global/hg-server/internal/platform/apperr"
)

// MaxPixels guards against decompression bombs.
const MaxPixels = 40_000_000

// Rect is a crop rectangle in source pixels.
type Rect struct {
	X int `json:"x"`
	Y int `json:"y"`
	W int `json:"w"`
	H int `json:"h"`
}

// Info returns an image's format and size without decoding pixels.
func Info(data []byte) (format string, w, h int, err error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, apperr.Invalid("invalid_image", "not a JPEG or PNG image")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return "", 0, 0, apperr.Invalid("invalid_image", "image is too large")
	}
	return format, cfg.Width, cfg.Height, nil
}

// CropJPEG crops data to r (nil = whole image) and encodes it as JPEG.
func CropJPEG(data []byte, r *Rect) (out []byte, w, h int, err error) {
	if _, _, _, err := Info(data); err != nil {
		return nil, 0, 0, err
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, apperr.Invalid("invalid_image", "could not decode image")
	}
	b := src.Bounds()
	area := b
	if r != nil {
		area = image.Rect(b.Min.X+r.X, b.Min.Y+r.Y, b.Min.X+r.X+r.W, b.Min.Y+r.Y+r.H)
		if r.W < 16 || r.H < 16 || !area.In(b) {
			return nil, 0, 0, apperr.Invalid("invalid_crop", "crop must lie inside the %dx%d image and be at least 16x16", b.Dx(), b.Dy())
		}
	}
	dst := image.NewRGBA(image.Rect(0, 0, area.Dx(), area.Dy()))
	draw.Draw(dst, dst.Bounds(), src, area.Min, draw.Src)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 88}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), area.Dx(), area.Dy(), nil
}
