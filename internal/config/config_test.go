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
	err := os.WriteFile(envPath, []byte("LLM_API_KEY=env-key\nLLM_BASE_URL=https://litellm.example/v1\nLLM_MODEL=gemini/gemini-3.5-flash-lite\nSNAPTRANS_SHORTCUT=Ctrl+Shift+S\nRAPIDOCR_EXE_PATH=C:/tools/rapidocr_json.exe\nRAPIDOCR_TIMEOUT_SECONDS=20\n"), 0o600)
	require.NoError(t, err)

	store := NewStoreAt(filepath.Join(temp, "config.json"), envPath)
	cfg, err := store.Load()

	require.NoError(t, err)
	require.Equal(t, "env-key", cfg.APIKey)
	require.Equal(t, "https://litellm.example/v1", cfg.BaseURL)
	require.Equal(t, "gemini/gemini-3.5-flash-lite", cfg.Model)
	require.Equal(t, "Ctrl+Shift+S", cfg.ShortcutKey)
	require.Equal(t, "C:/tools/rapidocr_json.exe", cfg.RapidOCRPath)
	require.Equal(t, 20, cfg.RapidOCRTimeoutSeconds)
	require.Equal(t, "gemini/gemini-3.5-flash-lite", cfg.Model)
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.APIKey = "saved-key"
	expected.ShortcutKey = "Alt+Q"

	require.NoError(t, store.Save(expected))
	actual, err := store.Load()

	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestScreenshotShortcutDefaultsAndRoundTrips(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	legacyPath := filepath.Join(temp, "legacy.json")
	require.NoError(t, os.WriteFile(legacyPath, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600))
	legacy, err := NewStoreAt(legacyPath, filepath.Join(temp, ".env")).Load()
	require.NoError(t, err)
	require.Equal(t, "Alt+W", legacy.ScreenshotShortcutKey)

	expected := Default()
	expected.ScreenshotShortcutKey = "Ctrl+Shift+W"
	require.NoError(t, store.Save(expected))
	actual, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, "Ctrl+Shift+W", actual.ScreenshotShortcutKey)
}

func TestLoadMigratesLegacyDeepSeekConfig(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	err := os.WriteFile(configPath, []byte(`{"deepSeekAPIKey":"legacy-key","deepSeekBaseURL":"https://legacy.example/v1","deepSeekModel":"legacy-model"}`), 0o600)
	require.NoError(t, err)

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.Equal(t, "legacy-key", cfg.APIKey)
	require.Equal(t, "https://legacy.example/v1", cfg.BaseURL)
	require.Equal(t, "legacy-model", cfg.Model)
}

func TestLoadDefaultsAutoDirectionWhenMissing(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	err := os.WriteFile(configPath, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600)
	require.NoError(t, err)

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.True(t, cfg.AutoDirection)
}

func TestLoadDefaultsUILanguageToChineseWhenMissing(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600))

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.Equal(t, "zh-CN", cfg.UILanguage)
}

func TestWithDefaultsNormalizesUnsupportedUILanguage(t *testing.T) {
	cfg := Default()
	cfg.UILanguage = "fr"

	require.Equal(t, "zh-CN", cfg.WithDefaults().UILanguage)
}

func TestLoadHonorsSavedAutoDirectionFalse(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	err := os.WriteFile(configPath, []byte(`{"autoDirection":false}`), 0o600)
	require.NoError(t, err)

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.False(t, cfg.AutoDirection)
}

func TestSaveAndLoadRoundTripKeepsAutoDirection(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.AutoDirection = false

	require.NoError(t, store.Save(expected))
	actual, err := store.Load()

	require.NoError(t, err)
	require.False(t, actual.AutoDirection)
}

func TestLoadDefaultsPersistentOCRWhenMissing(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	err := os.WriteFile(configPath, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600)
	require.NoError(t, err)

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.True(t, cfg.PersistentOCR)
}

func TestLoadHonorsSavedPersistentOCROff(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	err := os.WriteFile(configPath, []byte(`{"persistentOCR":false}`), 0o600)
	require.NoError(t, err)

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.False(t, cfg.PersistentOCR)
}
