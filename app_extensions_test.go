package main

import (
	"context"
	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
	"testing"
)

func TestDisabledActionsCannotStartOrCancelExistingWork(t *testing.T) {
	app := NewApp()
	app.cfg = config.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.extensionProcessing = cancel
	app.extensionRequestID = "running"
	require.Error(t, app.StartTextAction(TextActionRequest{ID: "new", Text: "source", Action: "meme"}))
	require.NoError(t, ctx.Err())
	require.Error(t, app.StartTextAction(TextActionRequest{ID: "", Text: "source", Action: "explain"}))
	require.NoError(t, app.CancelTextAction("old"))
	require.NoError(t, ctx.Err())
	require.NoError(t, app.CancelTextAction("running"))
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Empty(t, app.extensionRequestID)
}
func TestNewCaptureCancelsTextAction(t *testing.T) {
	app := NewApp()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.extensionProcessing = cancel
	require.NoError(t, app.TriggerCapture())
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
