package desktop

import (
	"context"
	"errors"
	"time"

	"snaptrans/internal/codex"
	"snaptrans/internal/config"
	"snaptrans/internal/translator"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func translationOptions(cfg config.Config) translator.Options {
	model := cfg.Model
	if cfg.Provider == "codex" {
		model = cfg.CodexModel
	}
	return translator.Options{Provider: cfg.Provider, CodexPath: cfg.CodexPath,
		CodexReasoningEffort: cfg.CodexReasoningEffort,
		APIKey:               cfg.APIKey, BaseURL: cfg.BaseURL, Model: model,
		SystemPrompt: cfg.SystemPrompt, Glossary: cfg.Glossary}
}

func translationTaskOptions(cfg config.Config) translator.Options {
	options := translationOptions(cfg)
	options.FastTranslation = cfg.FastTranslation
	return options
}

func (a *App) GetCodexStatus(path string) (codex.Status, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return codex.Inspect(ctx, path)
}

// Login owns its own process, so canceling it never signs out the user's CLI.
func (a *App) LoginCodex(path string) error {
	a.codexLoginMu.Lock()
	if a.codexLoginCancel != nil {
		a.codexLoginMu.Unlock()
		return errors.New("a Codex login is already in progress")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	a.codexLoginCancel = cancel
	a.codexLoginMu.Unlock()
	defer func() {
		cancel()
		a.codexLoginMu.Lock()
		a.codexLoginCancel = nil
		a.codexLoginMu.Unlock()
	}()
	err := codex.Login(ctx, path, func(url string) {
		if a.ctx != nil {
			runtime.BrowserOpenURL(a.ctx, url)
		}
	})
	if err == nil {
		codex.CloseIdle()
	}
	return err
}

func (a *App) CancelCodexLogin() {
	a.codexLoginMu.Lock()
	defer a.codexLoginMu.Unlock()
	if a.codexLoginCancel != nil {
		a.codexLoginCancel()
	}
}
