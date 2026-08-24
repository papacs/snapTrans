package main

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBeginCaptureCoalescesConcurrentRequests(t *testing.T) {
	app := NewApp()

	wasVisible, started := app.beginCapture()
	require.True(t, started)
	require.False(t, wasVisible)

	_, started = app.beginCapture()
	require.False(t, started)

	app.finishCapture()
	_, started = app.beginCapture()
	require.True(t, started)
}

func TestBeginCaptureOnlyRequestsHideDelayForVisibleWindow(t *testing.T) {
	app := NewApp()
	app.windowVisible = true

	wasVisible, started := app.beginCapture()

	require.True(t, started)
	require.True(t, wasVisible)
	require.False(t, app.windowVisible)
}

func TestCaptureWindowPreparationIsReusedOnSameDisplay(t *testing.T) {
	app := NewApp()

	require.True(t, app.shouldPrepareCaptureWindow(0, 0))
	require.False(t, app.shouldPrepareCaptureWindow(0, 0))
	require.True(t, app.shouldPrepareCaptureWindow(1920, 0))
	require.False(t, app.shouldPrepareCaptureWindow(1920, 0))
}

func TestCaptureWindowPreparationIsInvalidatedForSettings(t *testing.T) {
	app := NewApp()

	require.True(t, app.shouldPrepareCaptureWindow(0, 0))
	app.invalidateCaptureWindowPreparation()
	require.True(t, app.shouldPrepareCaptureWindow(0, 0))
}

func TestSettingsRequestWaitsForFrontendReady(t *testing.T) {
	app := NewApp()

	require.False(t, app.requestSettingsOpen())
	require.True(t, app.settingsPending)
	require.True(t, app.markFrontendReady())
	require.False(t, app.settingsPending)
	require.True(t, app.requestSettingsOpen())
	require.False(t, app.markFrontendReady())
}

func TestTranslateRegionRejectsInvalidRegion(t *testing.T) {
	app := NewApp()

	require.Error(t, app.TranslateRegion(TranslateRegionRequest{Width: 4, Height: 10}, "to-zh", 1))
	require.Error(t, app.TranslateRegion(TranslateRegionRequest{X: 0, Y: 0, Width: 20000, Height: 100}, "to-zh", 1))
	require.Error(t, app.TranslateRegion(TranslateRegionRequest{X: 2_000_000, Y: 0, Width: 100, Height: 100}, "to-zh", 1))
}

func TestTranslateRegionRequiresCapturedFrame(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()

	err := app.TranslateRegion(TranslateRegionRequest{X: 0, Y: 0, Width: 100, Height: 100}, "to-zh", 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no captured frame")
}

func TestTranslateRegionClipsToFrameBounds(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()
	app.frame = solidTestFrame(400, 300)

	// Region partially outside the frame is clipped to the valid area.
	err := app.TranslateRegion(TranslateRegionRequest{X: 350, Y: 0, Width: 200, Height: 300}, "to-zh", 1)
	require.NoError(t, err)
	require.NotNil(t, app.processing)

	// Fully outside the frame is rejected.
	err = app.TranslateRegion(TranslateRegionRequest{X: 500, Y: 500, Width: 100, Height: 100}, "to-zh", 2)
	require.Error(t, err)
	require.Contains(t, err.Error(), "outside the captured frame")

	app.cancelProcessing()
}

func solidTestFrame(width int, height int) *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: 20, G: 30, B: 40, A: 255})
		}
	}
	return frame
}
