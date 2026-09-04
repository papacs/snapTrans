package capture

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVerticalStitcherUsesMeasuredOverlap(t *testing.T) {
	content := patternedScrollContent(48, 300)
	frames := []image.Image{
		content.SubImage(image.Rect(0, 0, 48, 100)),
		content.SubImage(image.Rect(0, 60, 48, 160)),
		content.SubImage(image.Rect(0, 120, 48, 220)),
		content.SubImage(image.Rect(0, 200, 48, 300)),
	}

	stitcher, err := newVerticalStitcher(frames[0], 1_000_000)
	require.NoError(t, err)
	for _, frame := range frames[1:] {
		accepted := stitcher.addManual(frame)
		require.True(t, accepted)
	}

	stitched := stitcher.image()
	require.Equal(t, image.Rect(0, 0, 48, 300), stitched.Bounds())
	assertSamePixels(t, content, stitched)
}

func TestVerticalStitcherIgnoresScreenWhenItNoLongerMoves(t *testing.T) {
	content := patternedScrollContent(40, 180)
	first := content.SubImage(image.Rect(0, 0, 40, 100))
	last := content.SubImage(image.Rect(0, 80, 40, 180))
	stitcher, err := newVerticalStitcher(first, 1_000_000)
	require.NoError(t, err)

	accepted := stitcher.addManual(last)
	require.True(t, accepted)
	accepted = stitcher.addManual(last)
	require.False(t, accepted)
	require.Equal(t, 180, stitcher.image().Bounds().Dy())
}

func TestVerticalStitcherRejectsStationaryRepeatedRowsWithDynamicRegion(t *testing.T) {
	previous, next := stationaryRepeatedRowsWithDynamicRegion(80, 120, 54)
	stitcher, err := newVerticalStitcher(previous, 1_000_000)
	require.NoError(t, err)

	require.False(t, stitcher.addManual(next))
	require.Equal(t, 120, stitcher.image().Bounds().Dy())
}

func TestVerticalStitcherRejectsUpwardScrollAndResumeWithinCapturedContent(t *testing.T) {
	content := directionalRepeatedListContent(100, 260)
	first := content.SubImage(image.Rect(0, 80, 100, 180))
	down := content.SubImage(image.Rect(0, 100, 100, 200))
	up := content.SubImage(image.Rect(0, 80, 100, 180))
	beyond := content.SubImage(image.Rect(0, 120, 100, 220))
	stitcher, err := newVerticalStitcher(first, 1_000_000)
	require.NoError(t, err)

	require.True(t, stitcher.addManual(down))
	require.False(t, stitcher.addManual(up))
	require.False(t, stitcher.addManual(down))
	require.True(t, stitcher.addManual(beyond))
	require.Equal(t, 140, stitcher.image().Bounds().Dy())
}

func TestVerticalStitcherTracksRejectedUpwardFramesBeforeReturningDownward(t *testing.T) {
	content := patternedScrollContent(48, 260)
	first := content.SubImage(image.Rect(0, 0, 48, 100))
	stitcher, err := newVerticalStitcher(first, 1_000_000)
	require.NoError(t, err)

	require.True(t, stitcher.addManual(content.SubImage(image.Rect(0, 60, 48, 160))))
	require.True(t, stitcher.addManual(content.SubImage(image.Rect(0, 120, 48, 220))))
	require.Equal(t, 120, stitcher.observedTop)

	require.False(t, stitcher.addManual(content.SubImage(image.Rect(0, 90, 48, 190))))
	require.Equal(t, 90, stitcher.observedTop)
	require.False(t, stitcher.addManual(content.SubImage(image.Rect(0, 105, 48, 205))))
	require.Equal(t, 105, stitcher.observedTop)
	require.False(t, stitcher.addManual(content.SubImage(image.Rect(0, 120, 48, 220))))
	require.Equal(t, 120, stitcher.observedTop)

	require.True(t, stitcher.addManual(content.SubImage(image.Rect(0, 135, 48, 235))))
	require.Equal(t, 235, stitcher.image().Bounds().Dy())
	assertSamePixels(t, content.SubImage(image.Rect(0, 0, 48, 235)), stitcher.image())
}

func TestVerticalStitcherAcceptsDownwardScrollWhenRepeatedRowsAreDirectionallyAmbiguous(t *testing.T) {
	content := alternatingRepeatedRowsContent(100, 260)
	first := content.SubImage(image.Rect(0, 80, 100, 180))
	down := content.SubImage(image.Rect(0, 100, 100, 200))
	stitcher, err := newVerticalStitcher(first, 1_000_000)
	require.NoError(t, err)

	require.True(t, stitcher.addManual(down))
	require.Equal(t, 120, stitcher.image().Bounds().Dy())
}

func TestVerticalAdvanceIgnoresStickyHeader(t *testing.T) {
	content := patternedScrollContent(52, 220)
	first := stickyHeaderFrame(content, 0, 100, 16)
	next := stickyHeaderFrame(content, 55, 100, 16)

	advance, score, ok := verticalAdvance(first, next)

	require.True(t, ok)
	require.Equal(t, 55, advance)

	stitcher, err := newVerticalStitcher(first, 1_000_000)
	require.NoError(t, err)
	require.True(t, stitcher.addManual(next))
	require.Equal(t, 155, stitcher.image().Bounds().Dy())
	require.Less(t, score, 10.0)
}

func TestManualScrollingSessionEncodesUserScrolledFrames(t *testing.T) {
	content := patternedScrollContent(44, 190)
	frames := []image.Image{
		content.SubImage(image.Rect(0, 0, 44, 100)),
		content.SubImage(image.Rect(0, 50, 44, 150)),
		content.SubImage(image.Rect(0, 90, 44, 190)),
		content.SubImage(image.Rect(0, 90, 44, 190)),
	}
	index := 0
	session, err := startManualScrollCapture(
		context.Background(),
		image.Rect(10, 20, 54, 120),
		ManualScrollOptions{MaxFrames: 8, MaxPixels: 1_000_000},
		func(_ image.Rectangle) (image.Image, error) {
			frame := frames[index]
			if index < len(frames)-1 {
				index++
			}
			return frame, nil
		},
	)
	require.NoError(t, err)
	firstStep, err := session.CaptureNext()
	require.NoError(t, err)
	require.True(t, firstStep.Appended)
	require.Equal(t, 2, firstStep.Frames)
	// The selected area is a native input/rendering hole that already exposes
	// the live target window. Returning another full-size PNG for that same
	// area only stalls the next sample; steps should carry the small stitched
	// preview instead.
	require.Empty(t, firstStep.CurrentImageBytes)
	require.NotEmpty(t, firstStep.PreviewImageBytes)
	secondStep, err := session.CaptureNext()
	require.NoError(t, err)
	require.True(t, secondStep.Appended)
	require.Equal(t, 3, secondStep.Frames)
	preview, err := png.Decode(bytes.NewReader(secondStep.PreviewImageBytes))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 44, 190), preview.Bounds())
	assertSamePixels(t, content, preview)
	unchanged, err := session.CaptureNext()
	require.NoError(t, err)
	require.False(t, unchanged.Appended)
	require.Equal(t, 3, unchanged.Frames)
	result, err := session.Finish()
	require.Empty(t, unchanged.CurrentImageBytes)
	require.Empty(t, unchanged.PreviewImageBytes)
	require.NoError(t, err)
	require.Equal(t, 3, result.ScrollFrames)
	require.Equal(t, 44, result.Width)
	require.Equal(t, 190, result.Height)
	decoded, err := png.Decode(bytes.NewReader(result.ImageBytes))
	require.NoError(t, err)
	assertSamePixels(t, content, decoded)
}

func patternedScrollContent(width int, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			result.SetRGBA(x, y, color.RGBA{
				R: uint8((x*17 + y*3) % 251),
				G: uint8((x*7 + y*11) % 253),
				B: uint8((x*5 + y*19) % 255),
				A: 255,
			})
		}
	}
	return result
}

func stickyHeaderFrame(content *image.RGBA, offset int, height int, headerHeight int) *image.RGBA {
	frame := rgbaImage(content.SubImage(image.Rect(0, offset, content.Bounds().Dx(), offset+height)))
	header := color.RGBA{R: 12, G: 34, B: 56, A: 255}
	for y := 0; y < headerHeight; y++ {
		for x := 0; x < frame.Bounds().Dx(); x++ {
			frame.SetRGBA(x, y, header)
		}
	}
	return frame
}

func stationaryRepeatedRowsWithDynamicRegion(width int, height int, dynamicHeight int) (*image.RGBA, *image.RGBA) {
	previous := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		rowPattern := y + 37
		if y >= dynamicHeight+20 {
			rowPattern = (y - dynamicHeight - 20) % 20
		}
		for x := 0; x < width; x++ {
			previous.SetRGBA(x, y, color.RGBA{
				R: uint8((x*11 + rowPattern*17) % 251),
				G: uint8((x*7 + rowPattern*13) % 253),
				B: uint8((x*5 + rowPattern*19) % 255),
				A: 255,
			})
		}
	}

	next := rgbaImage(previous)
	for y := 0; y < dynamicHeight; y++ {
		for x := 0; x < width; x++ {
			next.SetRGBA(x, y, previous.RGBAAt(x, y+20))
		}
	}
	return previous, next
}

func directionalRepeatedListContent(width int, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			rowPattern := y % 20
			if x >= width*3/4 {
				rowPattern = y
			}
			result.SetRGBA(x, y, color.RGBA{
				R: uint8((x*11 + rowPattern*17) % 251),
				G: uint8((x*7 + rowPattern*13) % 253),
				B: uint8((x*5 + rowPattern*19) % 255),
				A: 255,
			})
		}
	}
	return result
}

func alternatingRepeatedRowsContent(width int, height int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		rowPattern := y % 40
		for x := 0; x < width; x++ {
			result.SetRGBA(x, y, color.RGBA{
				R: uint8((x*11 + rowPattern*17) % 251),
				G: uint8((x*7 + rowPattern*13) % 253),
				B: uint8((x*5 + rowPattern*19) % 255),
				A: 255,
			})
		}
	}
	return result
}

func assertSamePixels(t *testing.T, expected image.Image, actual image.Image) {
	t.Helper()
	require.Equal(t, expected.Bounds().Dx(), actual.Bounds().Dx())
	require.Equal(t, expected.Bounds().Dy(), actual.Bounds().Dy())
	for y := 0; y < expected.Bounds().Dy(); y++ {
		for x := 0; x < expected.Bounds().Dx(); x++ {
			require.Equal(t, color.RGBAModel.Convert(expected.At(expected.Bounds().Min.X+x, expected.Bounds().Min.Y+y)), color.RGBAModel.Convert(actual.At(actual.Bounds().Min.X+x, actual.Bounds().Min.Y+y)))
		}
	}
}

func TestManualScrollReportsLimitsAndStopsCapturing(t *testing.T) {
	for _, pixelLimit := range []bool{false, true} {
		t.Run(map[bool]string{false: "frames", true: "pixels"}[pixelLimit], func(t *testing.T) {
			content := patternedScrollContent(48, 200)
			calls := 0
			options := ManualScrollOptions{MaxFrames: 2, MaxPixels: 1_000_000}
			if pixelLimit {
				options.MaxFrames = 10
				options.MaxPixels = 48 * 110
			}
			session, err := startManualScrollCapture(context.Background(), image.Rect(0, 0, 48, 100), options, func(image.Rectangle) (image.Image, error) {
				top := 0
				if calls > 0 {
					top = 60
				}
				calls++
				return content.SubImage(image.Rect(0, top, 48, top+100)), nil
			})
			require.NoError(t, err)
			snapshot, err := session.CaptureNext()
			require.NoError(t, err)
			require.True(t, snapshot.LimitReached)
			require.Equal(t, !pixelLimit, snapshot.Appended)
			snapshot, err = session.CaptureNext()
			require.NoError(t, err)
			require.True(t, snapshot.LimitReached)
			require.Equal(t, 2, calls)
			result, err := session.Finish()
			require.NoError(t, err)
			require.NotEmpty(t, result.ImageBytes)
		})
	}
}
