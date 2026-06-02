package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadUsesEnvFallbackWhenConfigIsMissing(t *testing.T) {
	temp := t.TempDir()
	envPath := filepath.Join(temp, ".env")
	err := os.WriteFile(envPath, []byte("DEEPSEEK_API_KEY=env-key\nSNAPTRANS_SHORTCUT=Ctrl+Shift+S\nRAPIDOCR_EXE_PATH=C:/tools/rapidocr_json.exe\nRAPIDOCR_TIMEOUT_SECONDS=20\n"), 0o600)
	require.NoError(t, err)

	store := NewStoreAt(filepath.Join(temp, "config.json"), envPath)
	cfg, err := store.Load()

	require.NoError(t, err)
	require.Equal(t, "env-key", cfg.DeepSeekAPIKey)
	require.Equal(t, "Ctrl+Shift+S", cfg.ShortcutKey)
	require.Equal(t, "C:/tools/rapidocr_json.exe", cfg.RapidOCRPath)
	require.Equal(t, 20, cfg.RapidOCRTimeoutSeconds)
	require.Equal(t, "deepseek-chat", cfg.DeepSeekModel)
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.DeepSeekAPIKey = "saved-key"
	expected.ShortcutKey = "Alt+Q"

	require.NoError(t, store.Save(expected))
	actual, err := store.Load()

	require.NoError(t, err)
	require.Equal(t, expected, actual)
}
