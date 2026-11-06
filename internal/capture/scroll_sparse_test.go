package capture

import (
	"context"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManualScrollDefaultAllowsManySmallWheelMovements(t *testing.T) {
	content := patternedScrollContent(48, 240)
	offset := 0
	session, err := startManualScrollCapture(context.Background(), image.Rect(0, 0, 48, 100), ManualScrollOptions{}, func(image.Rectangle) (image.Image, error) {
		frame := content.SubImage(image.Rect(0, offset, 48, offset+100))
		offset += 4
		return frame, nil
	})
	require.NoError(t, err)
	for index := 0; index < 30; index++ {
		step, err := session.CaptureNext()
		require.NoError(t, err)
		require.True(t, step.Appended)
		require.False(t, step.LimitReached, "small wheel movements must not exhaust a long screenshot after 16 frames")
	}
	require.Equal(t, 220, session.stitcher.totalHeight)
	storedPixels := 0
	for _, frame := range session.stitcher.frames {
		storedPixels += frame.Bounds().Dx() * frame.Bounds().Dy()
	}
	require.Equal(t, 48*220, storedPixels, "retain each output pixel once, rather than a full viewport per wheel movement")
	assertSamePixels(t, content.SubImage(image.Rect(0, 0, 48, 220)), session.stitcher.image())
}

func TestVerticalStitcherTracksSparseTextOnWhitePage(t *testing.T) {
	content := sparseScrollText(480, 640)
	first := content.SubImage(image.Rect(0, 0, 480, 300))
	next := content.SubImage(image.Rect(0, 73, 480, 373))
	stitcher, err := newVerticalStitcher(first, 1_000_000)
	require.NoError(t, err)

	require.True(t, stitcher.addManual(next), "blank page rows must not hide moving text")
	require.Equal(t, 373, stitcher.totalHeight)
	assertSamePixels(t, content.SubImage(image.Rect(0, 0, 480, 373)), stitcher.image())
}

func TestVerticalAdvanceRequiresVisibleOverlapFeatures(t *testing.T) {
	blank := image.NewRGBA(image.Rect(0, 0, 48, 100))
	draw.Draw(blank, blank.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	_, _, found := verticalAdvance(blank, blank)
	require.False(t, found, "blank backgrounds cannot establish a scroll distance")
}

func TestVerticalAdvancePreservesHorizontalColorBands(t *testing.T) {
	content := image.NewRGBA(image.Rect(0, 0, 48, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 48; x++ {
			content.SetRGBA(x, y, color.RGBA{R: uint8(y * 3), G: uint8(y * 7), B: uint8(y * 11), A: 255})
		}
	}
	advance, _, found := verticalAdvance(content.SubImage(image.Rect(0, 0, 48, 100)), content.SubImage(image.Rect(0, 73, 48, 173)))
	require.True(t, found)
	require.Equal(t, 73, advance)
}

func sparseScrollText(width, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(result, result.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	for line, top := 0, 16; top+8 < height; line, top = line+1, top+23+line%5 {
		for glyph := 0; glyph < 12+line%23; glyph++ {
			for y := 0; y < 7; y++ {
				for x := 0; x < 4; x++ {
					if (x+y+glyph+line)%3 != 0 {
						result.SetRGBA(24+glyph*7+x, top+y, color.RGBA{R: 35, G: 35, B: 35, A: 255})
					}
				}
			}
		}
	}
	return result
}
