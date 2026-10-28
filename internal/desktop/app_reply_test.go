package desktop

import (
	"context"
	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
	"testing"
)

func TestReplyShortcutsRejectEquivalentBindings(t *testing.T) {
	cfg := config.Default()
	require.NoError(t, validateShortcuts(cfg))
	cfg.ShortcutKey, cfg.ReplyShortcutKey = "Ctrl+Shift+Q", "Shift+Ctrl+Q"
	require.ErrorContains(t, validateShortcuts(cfg), "must be different")
}

func TestInvalidReplyDoesNotCancelCurrentAction(t *testing.T) {
	app := NewApp(nil)
	app.cfg = config.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.extensionProcessing = cancel
	app.extensionRequestID = "existing"
	for _, request := range []TextActionRequest{
		{ID: "new", Action: "reply", Text: "message", Scene: "unknown"},
		{ID: "new", Action: "reply", Region: &TranslateRegionRequest{Width: 0, Height: 10}},
		{ID: "new", Action: "reply", Region: &TranslateRegionRequest{Width: 20, Height: 20}},
	} {
		require.Error(t, app.StartTextAction(request))
		require.NoError(t, ctx.Err())
	}
	source, err := app.replySource(context.Background(), app.cfg, TextActionRequest{Text: "  current message  "}, nil)
	require.NoError(t, err)
	require.Equal(t, "current message", source)
}

func TestReplyHotkeyCancelsPreviousGeneration(t *testing.T) {
	app := NewApp(nil)
	app.cfg = config.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.extensionProcessing = cancel
	require.NoError(t, app.TriggerReply())
	require.ErrorIs(t, ctx.Err(), context.Canceled)
}
