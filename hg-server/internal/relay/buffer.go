// Package relay is the broadcast relay (plan 5.5). While a show is live, an
// ffmpeg process receives the broadcaster's SRT stream, copies it unchanged to
// YouTube and decodes two frames a second into a ring buffer here, so frames
// reach H-Global Studio (and later hg-agent) before YouTube shows them.
package relay

import (
	"bufio"
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

// IngestKey derives a show's SRT passphrase from the server secret, the show
// and its key version. Nothing secret is stored; rotating bumps the version.
func IngestKey(secret string, showID uuid.UUID, version int) string {
	m := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(m, "ingest:%s:%d", showID, version)
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(m.Sum(nil))[:32]
}

// Frame is one decoded frame.
type Frame struct {
	Seq  int64
	At   time.Time // when hg-server received it
	JPEG []byte
}

// Buffer keeps the frames of the last Window, oldest first.
type Buffer struct {
	Window time.Duration

	mu      sync.Mutex
	frames  []Frame
	nextSeq int64
	subs    map[chan Frame]struct{}
}

// NewBuffer returns an empty buffer.
func NewBuffer(window time.Duration) *Buffer {
	return &Buffer{Window: window, nextSeq: 1, subs: map[chan Frame]struct{}{}}
}

// Add stores a frame and drops frames older than the window.
func (b *Buffer) Add(at time.Time, jpeg []byte) Frame {
	b.mu.Lock()
	defer b.mu.Unlock()
	f := Frame{Seq: b.nextSeq, At: at, JPEG: jpeg}
	b.nextSeq++
	b.frames = append(b.frames, f)
	cut := 0
	for cut < len(b.frames) && at.Sub(b.frames[cut].At) > b.Window {
		cut++
	}
	if cut > 0 {
		b.frames = append([]Frame(nil), b.frames[cut:]...)
	}
	for ch := range b.subs {
		select {
		case ch <- f:
		default: // a slow preview skips frames
		}
	}
	return f
}

// Get returns the frame with seq.
func (b *Buffer) Get(seq int64) (Frame, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, f := range b.frames {
		if f.Seq == seq {
			return f, true
		}
	}
	return Frame{}, false
}

// After returns up to limit frames with Seq > after; after <= 0 returns the
// newest limit frames.
func (b *Buffer) After(after int64, limit int) []Frame {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Frame
	if after <= 0 {
		start := len(b.frames) - limit
		if start < 0 {
			start = 0
		}
		return append(out, b.frames[start:]...)
	}
	for _, f := range b.frames {
		if f.Seq > after {
			out = append(out, f)
			if len(out) == limit {
				break
			}
		}
	}
	return out
}

// Len returns the number of buffered frames.
func (b *Buffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.frames)
}

// Latest returns the newest frame.
func (b *Buffer) Latest() (Frame, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.frames) == 0 {
		return Frame{}, false
	}
	return b.frames[len(b.frames)-1], true
}

// Subscribe delivers new frames until cancel is called.
func (b *Buffer) Subscribe() (<-chan Frame, func()) {
	ch := make(chan Frame, 4)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
}

// maxFrameBytes guards against a runaway stream.
const maxFrameBytes = 8 << 20

var errFrameTooLarge = errors.New("frame larger than 8 MB")

// JPEGReader splits a stream of concatenated JPEG images (ffmpeg's mjpeg
// image2pipe output) at the start and end markers. Baseline JPEG data never
// contains an unescaped end marker, so the split is exact.
type JPEGReader struct {
	r *bufio.Reader
}

// NewJPEGReader wraps r.
func NewJPEGReader(r io.Reader) *JPEGReader {
	return &JPEGReader{r: bufio.NewReaderSize(r, 256<<10)}
}

// Next returns the next whole image.
func (j *JPEGReader) Next() ([]byte, error) {
	// Find the start of image: FF D8.
	for {
		b, err := j.r.ReadByte()
		if err != nil {
			return nil, err
		}
		if b != 0xFF {
			continue
		}
		n, err := j.r.Peek(1)
		if err != nil {
			return nil, err
		}
		if n[0] == 0xD8 {
			_, _ = j.r.ReadByte()
			break
		}
	}
	var buf bytes.Buffer
	buf.Write([]byte{0xFF, 0xD8})
	prev := byte(0)
	for {
		b, err := j.r.ReadByte()
		if err != nil {
			return nil, err
		}
		buf.WriteByte(b)
		if prev == 0xFF && b == 0xD9 {
			return buf.Bytes(), nil
		}
		if buf.Len() > maxFrameBytes {
			return nil, errFrameTooLarge
		}
		prev = b
	}
}
