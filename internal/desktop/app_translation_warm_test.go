package desktop

import (
	"context"
	"image"
	"snaptrans/internal/config"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTranslationWarmStateThrottlesSuccessfulWarmups(t *testing.T) {
	var state translationWarmState
	startedAt := time.Unix(1000, 0)

	require.True(t, state.begin("https://gateway.example/v1", startedAt))
	require.False(t, state.begin("https://gateway.example/v1", startedAt.Add(time.Second)))

	state.finish("https://gateway.example/v1", startedAt.Add(2*time.Second), true)
	require.False(t, state.begin("https://gateway.example/v1", startedAt.Add(59*time.Second)))
	require.True(t, state.begin("https://gateway.example/v1", startedAt.Add(63*time.Second)))
}

func TestTranslationWarmStateRetriesFailuresAndEndpointChanges(t *testing.T) {
	var state translationWarmState
	startedAt := time.Unix(1000, 0)

	require.True(t, state.begin("https://first.example/v1", startedAt))
	state.finish("https://first.example/v1", startedAt.Add(time.Second), false)
	require.True(t, state.begin("https://first.example/v1", startedAt.Add(2*time.Second)))
	require.True(t, state.begin("https://second.example/v1", startedAt.Add(3*time.Second)))
}

func TestReplyWarmRequiresCurrentReplyCapture(t *testing.T) {
	app := &App{cfg: config.Config{Provider: "openai-compatible"}}
	require.ErrorContains(t, app.WarmReply("unknown", "ocr"), "unknown reply scene")
	require.ErrorContains(t, app.WarmReply("general", "unknown"), "unknown reply input mode")
	require.ErrorIs(t, app.WarmReply("general", "ocr"), context.Canceled)
	app.captureCtx, app.captureCancel = context.WithCancel(context.Background())
	defer app.captureCancel()
	app.frame = image.NewRGBA(image.Rect(0, 0, 10, 10))
	app.captureMode = "translate"
	require.ErrorIs(t, app.WarmReply("general", "ocr"), context.Canceled)
	app.captureMode = "reply"
	require.NoError(t, app.WarmReply("work", "image"), "known API reply shape needs no model thread or inference")
	app.captureCancel()
	require.ErrorIs(t, app.WarmReply("general", "ocr"), context.Canceled)
}

func TestSettingsWarmCancellationAlsoCancelsReplyPreparation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	app := &App{replyWarmStop: cancel}
	app.cancelTranslationWarmups()
	require.ErrorIs(t, ctx.Err(), context.Canceled)
	require.Nil(t, app.replyWarmStop)
}
