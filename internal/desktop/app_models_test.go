package desktop

import (
	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
	"testing"
)

func TestTranslationOptionsUseSelectedProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Model, cfg.APIKey, cfg.CodexModel, cfg.CodexPath = "api-model", "api-key", "subscription-model", "codex.exe"
	require.Equal(t, "api-model", translationOptions(cfg).Model)
	cfg.Provider = "codex"
	options := translationOptions(cfg)
	require.Equal(t, "codex", options.Provider)
	require.Equal(t, "subscription-model", options.Model)
	require.Equal(t, "codex.exe", options.CodexPath)
	cfg.CodexModel = ""
	require.Empty(t, translationOptions(cfg).Model)
	cfg.FastTranslation = true
	require.True(t, translationTaskOptions(cfg).FastTranslation)
	require.False(t, translationOptions(cfg).FastTranslation)
}

func TestUnknownModelProviderIsRejected(t *testing.T) {
	app := NewApp(nil)
	cfg := config.Default()
	cfg.Provider = "typo"
	require.ErrorContains(t, app.TestConnection(cfg), "unsupported model provider")
	require.ErrorContains(t, app.SaveConfig(cfg), "unsupported model provider")
}
