package desktop

import (
	"context"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/capture"
)

func TestBeginCaptureCoalescesConcurrentRequests(t *testing.T) {
	app := NewApp(nil)

	wasVisible, epoch, started := app.beginCapture()
	require.True(t, started)
	require.False(t, wasVisible)

	_, _, started = app.beginCapture()
	require.False(t, started)

	app.finishCapture(epoch)
	_, _, started = app.beginCapture()
	require.True(t, started)
}

func TestCanceledCaptureAllowsNewCaptureBeforeOldCleanup(t *testing.T) {
	for _, dismiss := range []struct {
		name string
		run  func(*App) error
	}{{"hide", (*App).HideWindow}, {"settings", (*App).ShowSettings}} {
		t.Run(dismiss.name, func(t *testing.T) {
			app := NewApp(nil)
			_, oldEpoch, started := app.beginCapture()
			require.True(t, started)
			require.NoError(t, dismiss.run(app))
			_, newEpoch, started := app.beginCapture()
			require.True(t, started, "canceled native capture/probe must not hold the next capture slot")
			app.finishCapture(oldEpoch)
			require.True(t, app.captureInFlight, "old completion must not release the new capture slot")
			app.windowVisible = true
			require.False(t, app.hideCaptureWindow(oldEpoch, false))
			require.True(t, app.windowVisible, "old cleanup must not hide the new window")
			app.finishCapture(newEpoch)
			require.False(t, app.captureInFlight)
		})
	}
}

func TestBeginCaptureOnlyRequestsHideDelayForVisibleWindow(t *testing.T) {
	app := NewApp(nil)
	app.windowVisible = true

	wasVisible, _, started := app.beginCapture()

	require.True(t, started)
	require.True(t, wasVisible)
	require.False(t, app.windowVisible)
}

func TestNewCaptureCannotReusePreviousFrameBeforeItsImageIsReady(t *testing.T) {
	app := NewApp(nil)
	app.frame = solidTestFrame(32, 32)
	_, _, started := app.beginCapture()
	require.True(t, started)
	require.Nil(t, app.frame)
	require.ErrorContains(t, app.TranslateRegion(TranslateRegionRequest{Width: 16, Height: 16}, "to-zh", 1), "no captured frame")
}

func TestNewCaptureReleasesPreviousEncodedImages(t *testing.T) {
	app := NewApp(nil)
	previousURL := app.captureAssets.Store(make([]byte, 4<<20))
	_, epoch, started := app.beginCapture()
	require.True(t, started)
	require.Empty(t, app.captureAssets.images, "old captures must not accumulate between shortcuts")
	require.Zero(t, app.captureAssets.bytes)
	require.NotEqual(t, previousURL, app.captureAssets.Store([]byte("current")))
	app.finishCapture(epoch)
}

func TestCaptureWindowRejectsDecodedImageFromCanceledOrOlderCapture(t *testing.T) {
	app := NewApp(nil)
	_, oldEpoch, _ := app.beginCapture()
	app.frame = solidTestFrame(32, 32)
	require.True(t, app.captureWindowCurrentLocked(oldEpoch))
	app.finishCapture(oldEpoch)
	_, currentEpoch, _ := app.beginCapture()
	require.False(t, app.captureWindowCurrentLocked(oldEpoch))
	require.False(t, app.captureWindowCurrentLocked(currentEpoch), "capture is not decoded yet")
	app.frame = solidTestFrame(32, 32)
	require.True(t, app.captureWindowCurrentLocked(currentEpoch))
	require.NoError(t, app.HideWindow())
	require.False(t, app.captureWindowCurrentLocked(currentEpoch))
}

func TestLateSettingsRenderCannotShowOverNewCapture(t *testing.T) {
	app := NewApp(nil)
	require.True(t, app.settingsWindowCurrentLocked())
	_, epoch, _ := app.beginCapture()
	require.False(t, app.settingsWindowCurrentLocked())
	app.frame = solidTestFrame(32, 32)
	app.finishCapture(epoch)
	require.False(t, app.settingsWindowCurrentLocked())
	require.NoError(t, app.ShowSettings())
	require.True(t, app.settingsWindowCurrentLocked())
	app.shuttingDown = true
	require.False(t, app.settingsWindowCurrentLocked())
}

func TestSettingsHidesCaptureBeforePreparingCompactWindow(t *testing.T) {
	app := NewApp(nil)
	app.windowVisible = true
	require.NoError(t, app.ShowSettings())
	require.False(t, app.windowVisible, "frontend must finish drawing settings before the native window reappears")
}

func TestCaptureWindowPreparationIsReusedOnSameDisplay(t *testing.T) {
	app := NewApp(nil)

	require.True(t, app.shouldPrepareCaptureWindow(0, 0))
	require.False(t, app.shouldPrepareCaptureWindow(0, 0))
	require.True(t, app.shouldPrepareCaptureWindow(1920, 0))
	require.False(t, app.shouldPrepareCaptureWindow(1920, 0))
}

func TestCaptureWindowPreparationIsInvalidatedForSettings(t *testing.T) {
	app := NewApp(nil)

	require.True(t, app.shouldPrepareCaptureWindow(0, 0))
	app.invalidateCaptureWindowPreparation()
	require.True(t, app.shouldPrepareCaptureWindow(0, 0))
}

func TestSettingsRequestWaitsForFrontendReady(t *testing.T) {
	app := NewApp(nil)

	require.False(t, app.requestSettingsOpen())
	require.True(t, app.settingsPending)
	require.True(t, app.markFrontendReady())
	require.False(t, app.settingsPending)
	require.True(t, app.requestSettingsOpen())
	require.False(t, app.markFrontendReady())
}

func TestSettingsClosesScrollingCaptureBeforeLateEditorCancellation(t *testing.T) {
	app := NewApp(nil)
	app.captureInFlight = true
	app.scrollCapture = &capture.ManualScrollCapture{}
	app.windowVisible = true
	processing, cancelProcessing := context.WithCancel(context.Background())
	defer cancelProcessing()
	app.processing = cancelProcessing
	reply, cancelReply := context.WithCancel(context.Background())
	defer cancelReply()
	app.extensionProcessing = cancelReply
	app.extensionRequestID = "reply"

	require.NoError(t, app.ShowSettings())
	require.Nil(t, app.scrollCapture)
	require.Zero(t, app.scrollOverlay)
	require.False(t, app.captureInFlight)
	require.ErrorIs(t, processing.Err(), context.Canceled)
	require.ErrorIs(t, reply.Err(), context.Canceled)

	// Simulate the settings shell being shown before the old editor's
	// asynchronous unmount cleanup reaches the backend.
	app.windowVisible = true
	require.NoError(t, app.CancelScrollingScreenshot())
	require.True(t, app.windowVisible)
	wasVisible, _, started := app.beginCapture()
	require.True(t, started)
	require.True(t, wasVisible)
}

func TestTranslateRegionRejectsInvalidRegion(t *testing.T) {
	app := NewApp(nil)

	require.Error(t, app.TranslateRegion(TranslateRegionRequest{Width: 4, Height: 10}, "to-zh", 1))
	require.Error(t, app.TranslateRegion(TranslateRegionRequest{X: 0, Y: 0, Width: 20000, Height: 100}, "to-zh", 1))
	require.Error(t, app.TranslateRegion(TranslateRegionRequest{X: 2_000_000, Y: 0, Width: 100, Height: 100}, "to-zh", 1))
}

func TestTranslateRegionRequiresCapturedFrame(t *testing.T) {
	app := NewApp(nil)
	app.ctx = context.Background()

	err := app.TranslateRegion(TranslateRegionRequest{X: 0, Y: 0, Width: 100, Height: 100}, "to-zh", 1)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no captured frame")
}

func TestTranslateRegionClipsToFrameBounds(t *testing.T) {
	frame := solidTestFrame(400, 300)
	cropped, rect, err := cropTranslationRegion(frame, TranslateRegionRequest{X: 350, Y: 0, Width: 200, Height: 300})
	require.NoError(t, err)
	require.Equal(t, image.Rect(350, 0, 400, 300), rect)
	require.Equal(t, image.Rect(0, 0, 100, 600), cropped.Bounds())
	_, _, err = cropTranslationRegion(frame, TranslateRegionRequest{X: 500, Y: 500, Width: 100, Height: 100})
	require.ErrorContains(t, err, "outside the captured frame")
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
