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

func TestHistoryRecordingDefaultsAndExplicitDisableRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStoreAt(path, "")
	require.NoError(t, os.WriteFile(path, []byte(`{"recordHistory":false}`), 0600))
	cfg, err := store.Load()
	require.NoError(t, err)
	require.NoError(t, store.Save(cfg))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"recordHistory": false`)
	require.NoError(t, os.WriteFile(path, []byte(`{}`), 0600))
	cfg, err = store.Load()
	require.NoError(t, err)
	require.NoError(t, store.Save(cfg))
	raw, err = os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"recordHistory": true`)
}

func TestLoadRecoversLastGoodConfigAfterInterruptedWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStoreAt(path, "")
	cfg := Default()
	cfg.APIKey = "recoverable-key"
	cfg.Model = "previous-model"
	require.NoError(t, store.Save(cfg))
	cfg.Model = "latest-model"
	require.NoError(t, store.Save(cfg))
	require.NoError(t, os.WriteFile(path, []byte(`{"model":`), 0600))
	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, "previous-model", loaded.Model)
	require.Equal(t, "recoverable-key", loaded.APIKey)
	recovered, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, loaded, recovered)
}

func TestCodexSettingsRoundTripAndKeepAPIConfiguration(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))
	cfg := Default()
	cfg.Provider, cfg.CodexPath, cfg.CodexModel = "codex", `C:\Program Files\Codex\codex.exe`, "subscription-model"
	cfg.CodexReasoningEffort = "medium"
	cfg.ReplyShortcutKey = "Ctrl+Alt+R"
	cfg.APIKey = "saved-api-key"
	require.NoError(t, store.Save(cfg))
	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, cfg, loaded)
	loaded.CodexModel = ""
	require.Empty(t, loaded.WithDefaults().CodexModel, "Codex selects its own default, not the API model")
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
	require.Equal(t, "openai-compatible", cfg.Provider)
	require.Equal(t, "low", cfg.CodexReasoningEffort)
	require.Equal(t, "Alt+R", cfg.ReplyShortcutKey)
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

func TestLoadDefaultsThemeToLightWhenMissing(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600))

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.Equal(t, "light", cfg.Theme)
}

func TestThemeRoundTripsAndInvalidValuesUseLight(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.Theme = "dark"
	require.NoError(t, store.Save(expected))
	actual, err := store.Load()

	require.NoError(t, err)
	require.Equal(t, "dark", actual.Theme)
	expected.Theme = "sepia"
	require.Equal(t, "light", expected.WithDefaults().Theme)
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

func TestLoadDefaultsTranslationTimeoutToSixtySeconds(t *testing.T) {
	temp := t.TempDir()
	configPath := filepath.Join(temp, "config.json")
	require.NoError(t, os.WriteFile(configPath, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600))

	cfg, err := NewStoreAt(configPath, filepath.Join(temp, ".env")).Load()

	require.NoError(t, err)
	require.Equal(t, 60, cfg.TranslationTimeoutSeconds)
}

func TestTranslationTimeoutRoundTripsAndRejectsNonPositive(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))

	expected := Default()
	expected.TranslationTimeoutSeconds = 120
	require.NoError(t, store.Save(expected))

	actual, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, 120, actual.TranslationTimeoutSeconds)

	require.Equal(t, 60, Config{TranslationTimeoutSeconds: -5}.WithDefaults().TranslationTimeoutSeconds)
}

func TestTranslationTimeoutEnvOverride(t *testing.T) {
	temp := t.TempDir()
	envPath := filepath.Join(temp, ".env")
	require.NoError(t, os.WriteFile(envPath, []byte("SNAPTRANS_TRANSLATION_TIMEOUT_SECONDS=90\n"), 0o600))

	cfg, err := NewStoreAt(filepath.Join(temp, "config.json"), envPath).Load()

	require.NoError(t, err)
	require.Equal(t, 90, cfg.TranslationTimeoutSeconds)
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

func TestMigrateOnlyOfficialLegacyDefaultModel(t *testing.T) {
	cfg := Default()
	cfg.Model = "deepseek-chat"
	require.Equal(t, "deepseek-v4-flash", cfg.WithDefaults().Model)
	cfg.BaseURL = "https://custom.example/v1"
	require.Equal(t, "deepseek-chat", cfg.WithDefaults().Model)
	cfg.BaseURL = "https://api.deepseek.com.proxy.example"
	require.Equal(t, "deepseek-chat", cfg.WithDefaults().Model)
	cfg.BaseURL, cfg.Model = "https://api.deepseek.com/v1", "custom-model"
	require.Equal(t, "custom-model", cfg.WithDefaults().Model)
}

func TestDirectImageInputIsOptInAndPersists(t *testing.T) {
	temp := t.TempDir()
	store := NewStoreAt(filepath.Join(temp, "config.json"), filepath.Join(temp, ".env"))
	require.NoError(t, os.WriteFile(store.Path, []byte(`{"shortcutKey":"Alt+Q"}`), 0o600))
	cfg, err := store.Load()
	require.NoError(t, err)
	require.False(t, cfg.DirectImageInput)
	require.True(t, cfg.FastTranslation)
	cfg.DirectImageInput = true
	cfg.FastTranslation = false
	require.NoError(t, store.Save(cfg))
	loaded, err := store.Load()
	require.NoError(t, err)
	require.True(t, loaded.DirectImageInput)
	require.False(t, loaded.FastTranslation)
}
