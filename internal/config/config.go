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
	DeepSeekAPIKey         string `json:"deepSeekAPIKey"`
	DeepSeekBaseURL        string `json:"deepSeekBaseURL"`
	DeepSeekModel          string `json:"deepSeekModel"`
	RapidOCRPath           string `json:"rapidOCRPath"`
	RapidOCRTimeoutSeconds int    `json:"rapidOCRTimeoutSeconds"`
}

type Store struct {
	Path    string
	EnvPath string
}

func Default() Config {
	return Config{
		ShortcutKey:            "Alt+Q",
		DeepSeekBaseURL:        "https://api.deepseek.com",
		DeepSeekModel:          "deepseek-chat",
		RapidOCRPath:           "./RapidOCR-json_v0.2.0",
		RapidOCRTimeoutSeconds: 15,
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

	var saved Config
	if err := json.Unmarshal(raw, &saved); err != nil {
		return cfg, err
	}

	saved = saved.WithDefaults()
	applyEnvFallback(&saved, env)
	return saved, nil
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
	if strings.TrimSpace(c.DeepSeekBaseURL) == "" {
		c.DeepSeekBaseURL = defaults.DeepSeekBaseURL
	}
	if strings.TrimSpace(c.DeepSeekModel) == "" {
		c.DeepSeekModel = defaults.DeepSeekModel
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
	if cfg.DeepSeekAPIKey == "" {
		cfg.DeepSeekAPIKey = env["DEEPSEEK_API_KEY"]
	}
	if cfg.DeepSeekBaseURL == "" {
		cfg.DeepSeekBaseURL = env["DEEPSEEK_BASE_URL"]
	}
	if cfg.DeepSeekModel == "" {
		cfg.DeepSeekModel = env["DEEPSEEK_MODEL"]
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
	*cfg = cfg.WithDefaults()
}

func applyEnvOverrides(cfg *Config, env map[string]string) {
	if env["DEEPSEEK_API_KEY"] != "" {
		cfg.DeepSeekAPIKey = env["DEEPSEEK_API_KEY"]
	}
	if env["DEEPSEEK_BASE_URL"] != "" {
		cfg.DeepSeekBaseURL = env["DEEPSEEK_BASE_URL"]
	}
	if env["DEEPSEEK_MODEL"] != "" {
		cfg.DeepSeekModel = env["DEEPSEEK_MODEL"]
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
