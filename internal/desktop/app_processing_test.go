package desktop

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClosedCaptureCannotRestartProcessingAfterCropFinishes(t *testing.T) {
	app := NewApp(nil)
	_, epoch, _ := app.beginCapture()
	require.NoError(t, app.HideWindow())

	ctx, cancel := app.newProcessingContext(epoch)
	if cancel != nil {
		defer cancel()
	}
	require.Nil(t, ctx)
	require.Nil(t, cancel)
	require.Nil(t, app.processing)
}

func TestStaleCaptureCannotCancelNewCaptureProcessing(t *testing.T) {
	app := NewApp(nil)
	_, oldEpoch, _ := app.beginCapture()
	app.finishCapture(oldEpoch)
	_, newEpoch, _ := app.beginCapture()
	current, cancel := app.newProcessingContext(newEpoch)
	defer cancel()

	stale, staleCancel := app.newProcessingContext(oldEpoch)
	if staleCancel != nil {
		defer staleCancel()
	}
	require.Nil(t, stale)
	require.Nil(t, staleCancel)
	require.NoError(t, current.Err())
	app.cancelProcessing()
	require.ErrorIs(t, current.Err(), context.Canceled)
}

func TestCurrentCaptureReplacesAndCancelsPreviousProcessing(t *testing.T) {
	app := NewApp(nil)
	_, epoch, _ := app.beginCapture()
	first, firstCancel := app.newProcessingContext(epoch)
	defer firstCancel()
	second, secondCancel := app.newProcessingContext(epoch)
	defer secondCancel()

	require.ErrorIs(t, first.Err(), context.Canceled)
	require.NoError(t, second.Err())
	app.cancelProcessing()
	require.ErrorIs(t, second.Err(), context.Canceled)
}
