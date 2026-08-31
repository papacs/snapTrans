// Package selection reads explicit native text selections without clipboard access.
package selection

import (
	"context"
	"errors"
	"image"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"snaptrans/internal/textregion"
)

const MaxCharacters = 20000
const MaxLines = 128
const ProbeTimeout = 350 * time.Millisecond

var (
	ErrNoSelection = errors.New("no selected text")
	ErrUnsupported = errors.New("selection cannot be located reliably")
	ErrChanged     = errors.New("foreground window changed")
	ErrBlocked     = errors.New("protected or own window")
)

type Line struct {
	Background, Foreground string
	Text                   string
	X, Y, Width, Height    float64
}
type Result struct {
	Window uintptr
	Lines  []Line
}

func (r Result) Bounds() image.Rectangle {
	var bounds image.Rectangle
	for _, line := range r.Lines {
		rect := image.Rect(int(math.Floor(line.X)), int(math.Floor(line.Y)), int(math.Ceil(line.X+line.Width)), int(math.Ceil(line.Y+line.Height)))
		bounds = bounds.Union(rect)
	}
	return bounds
}

// Normalize rejects off-screen or oversized selections instead of silently
// translating hidden text into a visible rectangle. Coordinates are physical.
func (r Result) Normalize(screen image.Rectangle) (textregion.Block, []textregion.Block, error) {
	if len(r.Lines) == 0 || len(r.Lines) > MaxLines || screen.Empty() {
		return textregion.Block{}, nil, ErrUnsupported
	}
	count := 0
	for _, line := range r.Lines {
		count += utf8.RuneCountInString(line.Text)
		for _, v := range []float64{line.X, line.Y, line.Width, line.Height} {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 1e7 {
				return textregion.Block{}, nil, ErrUnsupported
			}
		}
		if strings.TrimSpace(line.Text) == "" || line.Width <= 0 || line.Height <= 0 {
			return textregion.Block{}, nil, ErrUnsupported
		}
	}
	bounds := r.Bounds()
	if count > MaxCharacters || bounds.Empty() || !bounds.In(screen) {
		return textregion.Block{}, nil, ErrUnsupported
	}
	region := textregion.Block{X: float64(bounds.Min.X-screen.Min.X) / float64(screen.Dx()), Y: float64(bounds.Min.Y-screen.Min.Y) / float64(screen.Dy()), Width: float64(bounds.Dx()) / float64(screen.Dx()), Height: float64(bounds.Dy()) / float64(screen.Dy())}
	blocks := make([]textregion.Block, 0, len(r.Lines))
	for _, line := range r.Lines {
		blocks = append(blocks, textregion.Block{Text: line.Text, Background: line.Background, Foreground: line.Foreground, X: (line.X - float64(bounds.Min.X)) / float64(bounds.Dx()), Y: (line.Y - float64(bounds.Min.Y)) / float64(bounds.Dy()), Width: line.Width / float64(bounds.Dx()), Height: line.Height / float64(bounds.Dy())})
	}
	return region, blocks, nil
}

type request struct {
	ctx    context.Context
	window uintptr
	result chan response
}
type response struct {
	result Result
	err    error
}

// Reader admits a single probe. An unresponsive provider cannot accumulate
// goroutines or delay later screenshot fallbacks behind a queue.
type Reader struct {
	requests chan request
	done     chan struct{}
}

func (r *Reader) Read(ctx context.Context, window uintptr) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	req := request{ctx: ctx, window: window, result: make(chan response, 1)}
	select {
	case r.requests <- req:
	case <-r.done:
		return Result{}, ErrUnsupported
	default:
		return Result{}, ErrUnsupported
	}
	select {
	case got := <-req.result:
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		return got.result, got.err
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-r.done:
		return Result{}, ErrUnsupported
	}
}
