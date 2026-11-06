//go:build windows

package capture

import (
	"image"
	"testing"

	"github.com/kbinani/screenshot"
	"github.com/stretchr/testify/require"
)

func TestDesktopBGRAConversionKeepsTopDownPixelsOpaque(t *testing.T) {
	source := []byte{1, 2, 3, 0, 4, 5, 6, 7, 8, 9, 10, 0, 11, 12, 13, 14}
	result := make([]byte, len(source))
	copyDesktopBGRA(result, source)
	require.Equal(t, []byte{3, 2, 1, 255, 6, 5, 4, 255, 10, 9, 8, 255, 13, 12, 11, 255}, result)
	require.Equal(t, byte(0), source[3], "native source memory stays unchanged")
}

func TestScreenRegionRejectsInvalidBoundsBeforeAllocating(t *testing.T) {
	for _, rect := range []image.Rectangle{image.Rectangle{}, image.Rect(0, 0, 40000, 10), image.Rect(0, 0, 20000, 20000)} {
		_, err := ScreenRegion(rect)
		require.ErrorContains(t, err, "bounds")
	}
}

// Opt-in benchmark: only timings are reported; no desktop pixels are saved.
func BenchmarkDesktopPixelCapture(b *testing.B) {
	monitors, err := physicalMonitors()
	require.NoError(b, err)
	require.NotEmpty(b, monitors)
	rect := monitors[0].Rect
	for _, capture := range []struct {
		name string
		fn   func(image.Rectangle) (*image.RGBA, error)
	}{{"compatible-bitmap", screenshot.CaptureRect}, {"direct-dib", ScreenRegion}} {
		b.Run(capture.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				frame, err := capture.fn(rect)
				require.NoError(b, err)
				require.Equal(b, rect.Size(), frame.Bounds().Size())
			}
		})
	}
}
