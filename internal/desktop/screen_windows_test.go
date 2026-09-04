//go:build windows

package desktop

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestManualScrollHoleRectMapsScreenSelectionIntoOverlayWindow(t *testing.T) {
	hole, err := manualScrollHoleRect(
		image.Rect(100, 200, 900, 800),
		image.Rect(180, 270, 620, 560),
	)

	require.NoError(t, err)
	require.Equal(t, image.Rect(82, 72, 518, 358), hole)
}

func TestCropWindowBGRAUsesScreenCoordinatesAndConvertsChannels(t *testing.T) {
	windowBounds := image.Rect(100, 200, 103, 202)
	bgra := []byte{
		1, 2, 3, 0, 4, 5, 6, 0, 7, 8, 9, 0,
		10, 11, 12, 0, 13, 14, 15, 0, 16, 17, 18, 0,
	}

	frame, err := cropWindowBGRA(bgra, 3, 2, windowBounds, image.Rect(101, 200, 103, 202))

	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 2, 2), frame.Bounds())
	require.Equal(t, color.RGBA{R: 6, G: 5, B: 4, A: 255}, frame.RGBAAt(0, 0))
	require.Equal(t, color.RGBA{R: 18, G: 17, B: 16, A: 255}, frame.RGBAAt(1, 1))
}
