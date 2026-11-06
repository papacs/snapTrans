package capture

import (
	"context"
	"image"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRGBAScrollScoresExactlyMatchGenericReference(t *testing.T) {
	pattern := patternedScrollContent(96, 260)
	sparse := sparseScrollText(480, 220)
	alpha := image.NewRGBA(image.Rect(-8, -9, 96, 200))
	for offset := 0; offset < len(alpha.Pix); offset += 4 {
		alpha.Pix[offset], alpha.Pix[offset+1], alpha.Pix[offset+2], alpha.Pix[offset+3] = byte(offset%64), byte((offset/7)%64), byte((offset/17)%64), byte(128+offset%127)
	}
	for _, frames := range [][2]image.Image{
		{pattern.SubImage(image.Rect(7, 20, 87, 120)), pattern.SubImage(image.Rect(7, 33, 87, 133))},
		{sparse.SubImage(image.Rect(0, 0, 480, 100)), sparse.SubImage(image.Rect(0, 13, 480, 113))},
		{alpha.SubImage(image.Rect(-5, 5, 75, 105)), alpha.SubImage(image.Rect(-5, 18, 75, 118))},
	} {
		for _, advance := range []int{-1, 0, 1, 13, 45, 55, 80, 99, 100} {
			for _, kept := range []int{7, 10} {
				expected := sampledDifferenceKeepingRowsGeneric(frames[0], frames[1], advance, kept)
				actual := sampledDifferenceKeepingRows(frames[0], frames[1], advance, kept)
				require.Equal(t, expected, actual, "score changed at advance=%d kept=%d", advance, kept)
			}
		}
	}
}

func TestCanceledScrollSearchDoesNotAppendOrMoveObservedFrame(t *testing.T) {
	content := patternedScrollContent(80, 200)
	stitcher, err := newVerticalStitcher(content.SubImage(image.Rect(0, 0, 80, 100)), 1_000_000)
	require.NoError(t, err)
	observed := stitcher.observedFrame
	ctx := &cancelScrollAfterChecks{Context: context.Background(), threshold: 7}
	next := content.SubImage(image.Rect(0, 50, 80, 150))
	appended, err := stitcher.addManualContext(ctx, next)
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, appended)
	require.Equal(t, 1, stitcher.frameCount())
	require.Equal(t, 100, stitcher.totalHeight)
	require.Zero(t, stitcher.observedTop)
	require.Same(t, observed, stitcher.observedFrame)
	require.False(t, stitcher.limitReached)
	appended, err = stitcher.addManualContext(context.Background(), next)
	require.NoError(t, err)
	require.True(t, appended)
	require.Equal(t, 150, stitcher.totalHeight)
}

func TestManualScrollCancelReachesProcessingContext(t *testing.T) {
	frame := patternedScrollContent(80, 100)
	session, err := startManualScrollCapture(context.Background(), frame.Bounds(), ManualScrollOptions{}, func(image.Rectangle) (image.Image, error) {
		return frame, nil
	})
	require.NoError(t, err)
	session.Cancel()
	require.ErrorIs(t, session.ctx.Err(), context.Canceled, "matching and encoding must observe the same session cancellation")
}

func TestManualScrollFinishObservesCancellationInsidePNGEncoding(t *testing.T) {
	frame := patternedScrollContent(128, 128)
	session, err := startManualScrollCapture(context.Background(), frame.Bounds(), ManualScrollOptions{}, func(image.Rectangle) (image.Image, error) {
		return frame, nil
	})
	require.NoError(t, err)
	defer session.Cancel()
	session.ctx = &cancelScrollAfterChecks{Context: context.Background(), threshold: 4}
	result, err := session.Finish()
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, result.ImageBytes, "do not finish and retain an obsolete long PNG")
}

type cancelScrollAfterChecks struct {
	context.Context
	checks    int
	threshold int
}

func (ctx *cancelScrollAfterChecks) Err() error {
	ctx.checks++
	if ctx.checks >= ctx.threshold {
		return context.Canceled
	}
	return nil
}
