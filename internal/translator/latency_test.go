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

func TestLocalConfiguredAPITranslationLatency(t *testing.T) {
	if os.Getenv("SNAPTRANS_CONFIGURED_API_BENCHMARK") != "1" {
		t.Skip("optional comparison using the locally configured API; never saves settings")
	}
	store, err := config.NewStore("snapTrans")
	require.NoError(t, err)
	cfg, err := store.Load()
	require.NoError(t, err)
	if cfg.APIKey == "" {
		t.Skip("no configured API key")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := NewOpenAICompatible(Options{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model})
	started := time.Now()
	var first time.Duration
	var output string
	err = client.Translate(ctx, "Please send the updated schedule this afternoon.", DirectionToChinese, func(token string) {
		if first == 0 {
			first = time.Since(started)
		}
		output += token
	})
	require.NoError(t, err)
	require.NotEmpty(t, output)
	t.Logf("configured API model=%s first_token_ms=%d total_ms=%d", cfg.Model, first.Milliseconds(), time.Since(started).Milliseconds())
}

func TestLocalTranslationProfiles(t *testing.T) {
	if os.Getenv("SNAPTRANS_TRANSLATION_PROFILES") != "1" {
		t.Skip("optional translation-only latency comparison")
	}
	store, err := config.NewStore("snapTrans")
	require.NoError(t, err)
	cfg, err := store.Load()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	defer codex.CloseIdle()
	require.NoError(t, codex.Warm(ctx, cfg.CodexPath, cfg.CodexModel))
	source := "Please send the updated schedule this afternoon.\nToday's tasks are already fully booked.\nPlease confirm which task should take priority."
	for _, profile := range []string{"subscription-current", "subscription-fast", "subscription-fast", "subscription-current"} {
		options := Options{Provider: "codex", CodexPath: cfg.CodexPath, Model: cfg.CodexModel, CodexReasoningEffort: cfg.CodexReasoningEffort}
		options.FastTranslation = profile == "subscription-fast"
		client := NewOpenAICompatible(options).forTranslation()
		request := buildTranslationRequest(options.Model, source, DirectionToChinese, "", "")
		request = client.prepareTranslationRequest(request, DirectionToChinese, numberedOCRLines(source))
		started := time.Now()
		var first time.Duration
		output, err := client.stream(ctx, request, func(string) {
			if first == 0 {
				first = time.Since(started)
			}
		})
		require.NoError(t, err)
		require.Empty(t, missingNumberedOCRLines(source, output))
		t.Logf("profile=%s first_token_ms=%d total_ms=%d", profile, first.Milliseconds(), time.Since(started).Milliseconds())
	}
}

func TestLocalFastTranslationReuse(t *testing.T) {
	if os.Getenv("SNAPTRANS_FAST_TRANSLATION_REUSE") != "1" {
		t.Skip("optional live subscription translation and memo check")
	}
	store, err := config.NewStore("snapTrans")
	require.NoError(t, err)
	cfg, err := store.Load()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	defer codex.CloseIdle()
	require.NoError(t, codex.Warm(ctx, cfg.CodexPath, cfg.CodexModel))
	options := Options{Provider: "codex", Model: cfg.CodexModel, CodexPath: cfg.CodexPath, CodexReasoningEffort: cfg.CodexReasoningEffort, FastTranslation: true}
	for _, sample := range []struct {
		text      string
		direction Direction
	}{
		{"Please send the updated schedule this afternoon.\nToday's tasks are already fully booked.\nPlease confirm which task should take priority.", DirectionToChinese},
		{"请在下午发送更新后的排期。\n今天的任务已经排满了。\n请确认哪一项任务应该优先处理。", DirectionToEnglish},
	} {
		var previous string
		for attempt := 0; attempt < 2; attempt++ {
			started := time.Now()
			var first time.Duration
			var output string
			err = NewOpenAICompatible(options).Translate(ctx, sample.text, sample.direction, func(token string) {
				if first == 0 {
					first = time.Since(started)
				}
				output += token
			})
			require.NoError(t, err)
			require.Empty(t, missingNumberedOCRLines(sample.text, output))
			if attempt == 1 {
				require.Equal(t, previous, output)
			}
			previous = output
			t.Logf("direction=%s cached=%t first_token_ms=%d total_ms=%d output=%s", sample.direction, attempt == 1, first.Milliseconds(), time.Since(started).Milliseconds(), output)
		}
	}
}
