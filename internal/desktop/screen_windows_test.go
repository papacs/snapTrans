//go:build windows

package desktop

import (
	"context"
	"image"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNativeWindowShowDoesNotQueueWorkAfterShutdown(t *testing.T) {
	lifetime, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, showWindowSynchronously(context.Background(), lifetime))
}

func TestManualScrollHoleRectMapsScreenSelectionIntoOverlayWindow(t *testing.T) {
	hole, err := manualScrollHoleRect(
		image.Rect(100, 200, 900, 800),
		image.Rect(180, 270, 620, 560),
	)

	require.NoError(t, err)
	require.Equal(t, image.Rect(80, 70, 520, 360), hole)
}

func TestManualScrollHoleIncludesEveryCapturedPixelAcrossNegativeMonitorCoordinates(t *testing.T) {
	window := image.Rect(-1920, -200, 1920, 1080)
	for _, selection := range []image.Rectangle{
		image.Rect(-1500, -100, -700, 600),
		window,
		image.Rect(-20, 10, 30, 70),
	} {
		hole, err := manualScrollHoleRect(window, selection)
		require.NoError(t, err)
		require.Equal(t, selection, hole.Add(window.Min), "the overlay must not enter the captured rectangle")
	}
}

func TestManualScrollHoleRejectsInvalidSelection(t *testing.T) {
	window := image.Rect(100, 200, 900, 800)
	for _, selection := range []image.Rectangle{image.Rectangle{}, image.Rect(90, 200, 300, 400)} {
		_, err := manualScrollHoleRect(window, selection)
		require.Error(t, err)
	}
}
