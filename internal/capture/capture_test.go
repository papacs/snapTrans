package capture

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLogicalDisplaysSingleDisplay(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 2560, 1440), Scale: 1.5},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 1)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 0, displays[0].Y)
	require.Equal(t, 1707, displays[0].Width)
	require.Equal(t, 960, displays[0].Height)
	require.InDelta(t, 1.5, displays[0].Scale, 0.0001)
}

func TestLogicalDisplaysMixedDPIKeepsPerDisplayScale(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 2560, 1440), Scale: 2.0},
		{Rect: image.Rect(2560, 0, 3840, 1080), Scale: 1.0},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 2)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 0, displays[0].Y)
	require.Equal(t, 1280, displays[0].Width)
	require.Equal(t, 720, displays[0].Height)
	require.InDelta(t, 2.0, displays[0].Scale, 0.0001)

	require.Equal(t, 2560, displays[1].X)
	require.Equal(t, 0, displays[1].Y)
	require.Equal(t, 1280, displays[1].Width)
	require.Equal(t, 1080, displays[1].Height)
	require.InDelta(t, 1.0, displays[1].Scale, 0.0001)
}

func TestLogicalDisplaysNegativeCoordinates(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(-1920, 0, 0, 1080), Scale: 1.0},
		{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1.0},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 2)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 1920, displays[1].X)
	require.Equal(t, 1920, displays[0].Width)
	require.Equal(t, 1920, displays[1].Width)
}

func TestLogicalDisplaysOrderedBelowPrimary(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1.25},
		{Rect: image.Rect(0, 1080, 3840, 2160), Scale: 2.0},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 2)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 0, displays[0].Y)
	require.Equal(t, 1536, displays[0].Width)
	require.Equal(t, 864, displays[0].Height)

	require.Equal(t, 0, displays[1].X)
	require.Equal(t, 540, displays[1].Y)
	require.Equal(t, 1920, displays[1].Width)
	require.Equal(t, 540, displays[1].Height)
}

func TestLogicalDisplaysEmpty(t *testing.T) {
	require.Nil(t, LogicalDisplays(nil))
}

func TestMonitorForPointSelectsDisplayContainingCursor(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(-1920, 0, 0, 1080), Scale: 1.0},
		{Rect: image.Rect(0, 0, 2560, 1440), Scale: 1.5},
	}

	monitor, ok := monitorForPoint(monitors, image.Pt(-640, 300))

	require.True(t, ok)
	require.Equal(t, monitors[0], monitor)
}

func TestMonitorForPointUsesNearestDisplayForCoordinateGap(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1.0},
		{Rect: image.Rect(2200, 0, 3480, 1024), Scale: 1.0},
	}

	monitor, ok := monitorForPoint(monitors, image.Pt(2100, 400))

	require.True(t, ok)
	require.Equal(t, monitors[1], monitor)
}

func TestMonitorForPointRejectsEmptyMonitorList(t *testing.T) {
	_, ok := monitorForPoint(nil, image.Pt(10, 10))
	require.False(t, ok)
}

func TestIsBlankCaptureDetectsOpaqueBlackFrame(t *testing.T) {
	frame := solidFrame(64, 48, color.RGBA{A: 255})
	require.True(t, isBlankCapture(frame))
}

func TestIsBlankCaptureDetectsTransparentFrame(t *testing.T) {
	frame := solidFrame(64, 48, color.RGBA{})
	require.True(t, isBlankCapture(frame))
}

func TestIsBlankCaptureKeepsLegitimateUniformColor(t *testing.T) {
	frame := solidFrame(64, 48, color.RGBA{R: 18, G: 86, B: 160, A: 255})
	require.False(t, isBlankCapture(frame))
}

func solidFrame(width int, height int, fill color.RGBA) *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			frame.SetRGBA(x, y, fill)
		}
	}
	return frame
}
