package main

import (
	"github.com/stretchr/testify/require"
	"image"
	"snaptrans/internal/capture"
	"snaptrans/internal/textregion"
	"testing"
	"time"
)

func TestNewCaptureInvalidatesOldSelection(t *testing.T) {
	app := NewApp()
	app.selectedText = &capture.SelectedText{ID: "old"}
	_, first, started := app.beginCapture()
	require.True(t, started)
	require.Nil(t, app.selectedText)
	_, coalesced, started := app.beginCapture()
	require.False(t, started)
	require.Equal(t, first, coalesced)
	app.finishCapture()
	_, next, started := app.beginCapture()
	require.True(t, started)
	require.Greater(t, next, first)
	require.ErrorContains(t, app.TranslateSelection("old", "to-zh", 1), "expired")
}
func TestClosedCaptureCannotRepublishSource(t *testing.T) {
	app := NewApp()
	_, epoch, _ := app.beginCapture()
	result := capture.Result{
		Width: 400, Height: 300, Frame: image.NewRGBA(image.Rect(0, 0, 400, 300)), ImageBytes: []byte("fixture"),
		SelectedText: &capture.SelectedText{ID: "current", Blocks: []textregion.Block{{Text: "test", Width: 1, Height: 1}}},
	}
	app.publishCapture(result, "translate", epoch, "", time.Now())
	require.NotNil(t, app.selectedText)
	require.NotNil(t, app.frame)
	require.Len(t, app.captureAssets.images, 1)
	require.NoError(t, app.HideWindow())
	app.publishCapture(result, "translate", epoch, "", time.Now())
	require.Nil(t, app.selectedText)
	require.Nil(t, app.frame)
	require.Empty(t, app.captureAssets.images)
	require.ErrorContains(t, app.TranslateSelection("current", "to-zh", 1), "expired")
}
func TestInvalidSelectionIDDoesNotCancelActiveTranslation(t *testing.T) {
	app := NewApp()
	app.selectedText = &capture.SelectedText{ID: "current"}
	canceled := false
	app.processing = func() { canceled = true }
	require.ErrorContains(t, app.TranslateSelection("older", "to-zh", 4), "expired")
	require.False(t, canceled)
}
