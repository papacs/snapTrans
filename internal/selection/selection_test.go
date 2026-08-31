package selection

import (
	"context"
	"github.com/stretchr/testify/require"
	"image"
	"math"
	"testing"
	"time"
)

func TestNormalizeSelectionPhysicalCoordinates(t *testing.T) {
	for _, scale := range []float64{1, 1.25, 1.5, 2} {
		screen := image.Rect(-1920, 0, -1920+int(1200*scale), int(800*scale))
		input := Result{Lines: []Line{{Text: "Hello", X: -1920 + 100*scale, Y: 80 * scale, Width: 300 * scale, Height: 20 * scale}, {Text: "World", X: -1920 + 100*scale, Y: 110 * scale, Width: 200 * scale, Height: 20 * scale}}}
		region, blocks, err := input.Normalize(screen)
		require.NoError(t, err)
		require.InDelta(t, 100.0/1200, region.X, 0.001)
		require.InDelta(t, .1, region.Y, 0.001)
		require.Len(t, blocks, 2)
		require.InDelta(t, 2.0/3, blocks[1].Width, .02)
	}
}
func TestNormalizeRejectsInvisibleAndMalformedSelections(t *testing.T) {
	screen := image.Rect(0, 0, 800, 600)
	for _, line := range []Line{{Text: "hidden", X: -20, Y: 20, Width: 100, Height: 20}, {Text: "nan", X: math.NaN(), Width: 30, Height: 20}, {Text: "empty", Width: 0, Height: 20}, {Text: " ", Width: 10, Height: 20}} {
		_, _, err := (Result{Lines: []Line{line}}).Normalize(screen)
		require.Error(t, err)
	}
}
func TestMergeInlineRectsRejectsColumns(t *testing.T) {
	result, err := mergeLineRects([]Line{{X: 50, Y: 10, Width: 30, Height: 20}, {X: 10, Y: 10, Width: 40, Height: 20}})
	require.NoError(t, err)
	require.Equal(t, 70.0, result.Width)
	_, err = mergeLineRects([]Line{{X: 10, Y: 10, Width: 40, Height: 20}, {X: 400, Y: 10, Width: 40, Height: 20}})
	require.Error(t, err)
	_, err = mergeLineRects([]Line{{X: 10, Y: 10, Width: 40, Height: 20}, {X: 50, Y: 40, Width: 40, Height: 20}})
	require.Error(t, err)
}
func TestBusyReaderReturnsImmediatelyWithoutQueue(t *testing.T) {
	reader := &Reader{requests: make(chan request), done: make(chan struct{})}
	start := time.Now()
	_, err := reader.Read(context.Background(), 1)
	require.ErrorIs(t, err, ErrUnsupported)
	require.Less(t, time.Since(start), time.Second)
}
func TestReaderHonorsCanceledContext(t *testing.T) {
	reader := &Reader{requests: make(chan request), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := reader.Read(ctx, 1)
	require.ErrorIs(t, err, context.Canceled)
}

func TestTimedOutReplyCannotReplaceNextSelection(t *testing.T) {
	reader := &Reader{requests: make(chan request, 1), done: make(chan struct{})}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err := reader.Read(ctx, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	expired := <-reader.requests
	expired.result <- response{result: Result{Lines: []Line{{Text: "expired"}}}}
	completed := make(chan response, 1)
	nextCtx, nextCancel := context.WithTimeout(context.Background(), time.Second)
	defer nextCancel()
	go func() { result, err := reader.Read(nextCtx, 2); completed <- response{result: result, err: err} }()
	select {
	case next := <-reader.requests:
		next.result <- response{result: Result{Lines: []Line{{Text: "current"}}}}
	case <-nextCtx.Done():
		t.Fatal("second read was not admitted")
	}
	got := <-completed
	require.NoError(t, got.err)
	require.Equal(t, "current", got.result.Lines[0].Text)
}
func TestNormalizePreservesNativeColors(t *testing.T) {
	_, blocks, err := (Result{Lines: []Line{{Text: "Hello", X: 10, Y: 10, Width: 100, Height: 20, Background: "#ffffff", Foreground: "#123456"}}}).Normalize(image.Rect(0, 0, 800, 600))
	require.NoError(t, err)
	require.Equal(t, "#ffffff", blocks[0].Background)
	require.Equal(t, "#123456", blocks[0].Foreground)
}
