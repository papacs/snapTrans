package main

import (
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
