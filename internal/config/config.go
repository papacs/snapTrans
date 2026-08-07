package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ShortcutKey            string `json:"shortcutKey"`
	APIKey                 string `json:"apiKey"`
	BaseURL                string `json:"baseURL"`
	Model                  string `json:"model"`
	RapidOCRPath           string `json:"rapidOCRPath"`
	RapidOCRTimeoutSeconds int    `json:"rapidOCRTimeoutSeconds"`
	AutoDirection          bool   `json:"autoDirection"`
	SystemPrompt           string `json:"systemPrompt"`
	Glossary               string `json:"glossary"`
	PersistentOCR          bool   `json:"persistentOCR"`
	AutoCopy               bool   `json:"autoCopy"`
}

// persistedConfig accepts both the current generic LLM fields and the
// DeepSeek-specific fields written by versions before LiteLLM support.
type persistedConfig struct {
	ShortcutKey            string `json:"shortcutKey"`
	APIKey                 string `json:"apiKey"`
	BaseURL                string `json:"baseURL"`
	Model                  string `json:"model"`
	DeepSeekAPIKey         string `json:"deepSeekAPIKey"`
	DeepSeekBaseURL        string `json:"deepSeekBaseURL"`
	DeepSeekModel          string `json:"deepSeekModel"`
	RapidOCRPath           string    `json:"rapidOCRPath"`
	RapidOCRTimeoutSeconds int       `json:"rapidOCRTimeoutSeconds"`
	AutoDirection          *bool     `json:"autoDirection"`
	SystemPrompt           string    `json:"systemPrompt"`
	Glossary               string    `json:"glossary"`
	PersistentOCR          *bool     `json:"persistentOCR"`
	AutoCopy               *bool     `json:"autoCopy"`
}

func (p persistedConfig) Config() Config {
	return Config{
		ShortcutKey:            p.ShortcutKey,
		APIKey:                 firstNonEmpty(p.APIKey, p.DeepSeekAPIKey),
		BaseURL:                firstNonEmpty(p.BaseURL, p.DeepSeekBaseURL),
		Model:                  firstNonEmpty(p.Model, p.DeepSeekModel),
		RapidOCRPath:           p.RapidOCRPath,
		RapidOCRTimeoutSeconds: p.RapidOCRTimeoutSeconds,
		AutoDirection:          p.AutoDirection == nil || *p.AutoDirection,
		SystemPrompt:           p.SystemPrompt,
		Glossary:               p.Glossary,
		PersistentOCR:          p.PersistentOCR == nil || *p.PersistentOCR,
		AutoCopy:               p.AutoCopy != nil && *p.AutoCopy,
	}
}

type Store struct {
	Path    string
	EnvPath string
}

func Default() Config {
	return Config{
		ShortcutKey:            "Alt+Q",
		BaseURL:                "https://api.deepseek.com",
		Model:                  "deepseek-chat",
		RapidOCRPath:           "./RapidOCR-json_v0.2.0",
		RapidOCRTimeoutSeconds: 15,
		AutoDirection:          true,
		PersistentOCR:          true,
	}
}

func NewStore(appName string) (*Store, error) {
	if strings.TrimSpace(appName) == "" {
		return nil, errors.New("app name is required")
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}

	return &Store{
		Path:    filepath.Join(base, appName, "config.json"),
		EnvPath: ".env",
	}, nil
}

func NewStoreAt(path string, envPath string) *Store {
	return &Store{Path: path, EnvPath: envPath}
}

func (s *Store) Load() (Config, error) {
	cfg := Default()
	env, envErr := readEnvFile(s.EnvPath)
	applyEnvOverrides(&cfg, env)

	if s.Path == "" {
		return cfg, envErr
	}

	raw, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	var saved persistedConfig
	if err := json.Unmarshal(raw, &saved); err != nil {
		return cfg, err
	}

	loaded := saved.Config()
	applyEnvFallback(&loaded, env)
	return loaded, nil
}

func (s *Store) Save(cfg Config) error {
	if s.Path == "" {
		return errors.New("config path is required")
	}

	cfg = cfg.WithDefaults()
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.Path, raw, 0o600)
}

func (c Config) WithDefaults() Config {
	defaults := Default()
	if strings.TrimSpace(c.ShortcutKey) == "" {
		c.ShortcutKey = defaults.ShortcutKey
	}
	if strings.TrimSpace(c.BaseURL) == "" {
		c.BaseURL = defaults.BaseURL
	}
	if strings.TrimSpace(c.Model) == "" {
		c.Model = defaults.Model
	}
	if strings.TrimSpace(c.RapidOCRPath) == "" {
		c.RapidOCRPath = defaults.RapidOCRPath
	}
	if c.RapidOCRTimeoutSeconds <= 0 {
		c.RapidOCRTimeoutSeconds = defaults.RapidOCRTimeoutSeconds
	}
	return c
}

func readEnvFile(path string) (map[string]string, error) {
	values := map[string]string{}
	if strings.TrimSpace(path) == "" {
		return values, nil
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return values, nil
		}
		return values, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		values[key] = value
	}

	return values, scanner.Err()
}

func applyEnvFallback(cfg *Config, env map[string]string) {
	if cfg.APIKey == "" {
		cfg.APIKey = firstNonEmpty(env["LLM_API_KEY"], env["LITELLM_API_KEY"], env["DEEPSEEK_API_KEY"])
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = firstNonEmpty(env["LLM_BASE_URL"], env["LITELLM_BASE_URL"], env["DEEPSEEK_BASE_URL"])
	}
	if cfg.Model == "" {
		cfg.Model = firstNonEmpty(env["LLM_MODEL"], env["LITELLM_MODEL"], env["DEEPSEEK_MODEL"])
	}
	if cfg.RapidOCRPath == "" {
		cfg.RapidOCRPath = env["RAPIDOCR_EXE_PATH"]
	}
	if cfg.ShortcutKey == "" {
		cfg.ShortcutKey = env["SNAPTRANS_SHORTCUT"]
	}
	if cfg.RapidOCRTimeoutSeconds <= 0 && env["RAPIDOCR_TIMEOUT_SECONDS"] != "" {
		if parsed, err := strconv.Atoi(env["RAPIDOCR_TIMEOUT_SECONDS"]); err == nil {
			cfg.RapidOCRTimeoutSeconds = parsed
		}
	}
	if env["SNAPTRANS_AUTO_DIRECTION"] != "" {
		if parsed, err := strconv.ParseBool(env["SNAPTRANS_AUTO_DIRECTION"]); err == nil {
			cfg.AutoDirection = parsed
		}
	}
	if env["SNAPTRANS_PERSISTENT_OCR"] != "" {
		if parsed, err := strconv.ParseBool(env["SNAPTRANS_PERSISTENT_OCR"]); err == nil {
			cfg.PersistentOCR = parsed
		}
	}
	*cfg = cfg.WithDefaults()
}

func applyEnvOverrides(cfg *Config, env map[string]string) {
	if value := firstNonEmpty(env["LLM_API_KEY"], env["LITELLM_API_KEY"], env["DEEPSEEK_API_KEY"]); value != "" {
		cfg.APIKey = value
	}
	if value := firstNonEmpty(env["LLM_BASE_URL"], env["LITELLM_BASE_URL"], env["DEEPSEEK_BASE_URL"]); value != "" {
		cfg.BaseURL = value
	}
	if value := firstNonEmpty(env["LLM_MODEL"], env["LITELLM_MODEL"], env["DEEPSEEK_MODEL"]); value != "" {
		cfg.Model = value
	}
	if env["RAPIDOCR_EXE_PATH"] != "" {
		cfg.RapidOCRPath = env["RAPIDOCR_EXE_PATH"]
	}
	if env["SNAPTRANS_SHORTCUT"] != "" {
		cfg.ShortcutKey = env["SNAPTRANS_SHORTCUT"]
	}
	if env["RAPIDOCR_TIMEOUT_SECONDS"] != "" {
		if parsed, err := strconv.Atoi(env["RAPIDOCR_TIMEOUT_SECONDS"]); err == nil {
			cfg.RapidOCRTimeoutSeconds = parsed
		}
	}
	*cfg = cfg.WithDefaults()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
