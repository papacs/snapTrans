package capture

import (
	"context"
	"github.com/stretchr/testify/require"
	"image"
	"testing"
)

func TestSelectedTextChoosesContainingMonitor(t *testing.T) {
	monitors := []physicalMonitor{{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1}, {Rect: image.Rect(-2560, -300, 0, 1140), Scale: 1.5}}
	selected, ok := containingMonitor(monitors, image.Rect(-2400, -100, -1800, 300))
	require.True(t, ok)
	require.Equal(t, monitors[1], selected)
	for _, rect := range []image.Rectangle{image.Rect(-100, 20, 100, 200), image.Rect(-2700, -100, -2200, 200), {}, image.Rect(10, 1200, 300, 1300)} {
		_, ok = containingMonitor(monitors, rect)
		require.False(t, ok)
	}
}
func TestSelectedDisplayHonorsCancellationBeforeNativeCapture(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := SelectionDisplay(ctx, image.Rect(0, 0, 100, 100))
	require.ErrorIs(t, err, context.Canceled)
}
