package translator

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/codex"
	"snaptrans/internal/config"
)

func TestLocalWrappedNewsletterTranslation(t *testing.T) {
	if os.Getenv("SNAPTRANS_NEWSLETTER_TRANSLATION") != "1" {
		t.Skip("optional live translation of public sample text; never changes settings")
	}
	store, err := config.NewStore("snapTrans")
	require.NoError(t, err)
	cfg, err := store.Load()
	require.NoError(t, err)
	client := NewOpenAICompatible(Options{Provider: "codex", CodexPath: cfg.CodexPath, Model: cfg.CodexModel,
		CodexReasoningEffort: "low", FastTranslation: true}).forTranslation()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	defer codex.CloseIdle()
	require.NoError(t, client.WarmTranslation(ctx, DirectionToChinese))
	source := "Hi Alex,\nThe rankings order every model by tokens sent through the OpenRouter APl,\nsplit by task, so you can see what developers run for coding, for agents, or for\nextraction. Pick one near the top for the work you have in mind. Any slug on\nthe page drops into the model field of the same request, on the same key.\nOr let the router choose: typesafe/jev-router picks a model and a reasoning effort\nfor each request, billed at the price of the model that answers.\nNEXT"
	started := time.Now()
	var first time.Duration
	output, err := client.stream(ctx, client.translationRequest(source, DirectionToChinese), func(token string) {
		if first == 0 && token != "" {
			first = time.Since(started)
		}
	})
	require.NoError(t, err)
	t.Logf("profile=%s first_token_ms=%d total_ms=%d missing_regions=%d output=%s", os.Getenv("SNAPTRANS_NEWSLETTER_PROFILE"), first.Milliseconds(), time.Since(started).Milliseconds(), len(missingNumberedOCRLines(source, output)), output)
	require.Empty(t, missingNumberedOCRLines(source, output))
	require.Contains(t, output, "Alex")
	require.Contains(t, output, "OpenRouter")
	require.Contains(t, output, "typesafe/jev-router")
}
