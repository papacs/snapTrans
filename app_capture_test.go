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
