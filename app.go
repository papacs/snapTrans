package main

import (
	"context"
	"errors"
	"sync"
	"time"

	"snaptrans/internal/capture"
	"snaptrans/internal/config"
	"snaptrans/internal/hotkeys"
	"snaptrans/internal/ocr"
	"snaptrans/internal/translator"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type workflowError struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type App struct {
	ctx context.Context

	configStore *config.Store

	mu         sync.Mutex
	cfg        config.Config
	shortcut   *hotkeys.Registration
	processing context.CancelFunc
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	store, err := config.NewStore("snapTrans")
	if err != nil {
		a.emitError("config", err)
		return
	}
	a.configStore = store

	cfg, err := store.Load()
	if err != nil {
		a.emitError("config", err)
		cfg = config.Default()
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()

	if err := a.registerShortcut(cfg.ShortcutKey); err != nil {
		a.emitError("config", err)
	}
}

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.shortcut != nil {
		_ = a.shortcut.Unregister()
		a.shortcut = nil
	}
	if a.processing != nil {
		a.processing()
		a.processing = nil
	}
}

func (a *App) LoadConfig() (config.Config, error) {
	if a.configStore == nil {
		return config.Default(), nil
	}

	cfg, err := a.configStore.Load()
	if err != nil {
		return config.Default(), err
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()
	return cfg, nil
}

func (a *App) SaveConfig(next config.Config) error {
	next = next.WithDefaults()
	if a.configStore == nil {
		return errors.New("config store is not ready")
	}

	if err := a.configStore.Save(next); err != nil {
		return err
	}

	a.mu.Lock()
	a.cfg = next
	a.mu.Unlock()

	return a.registerShortcut(next.ShortcutKey)
}

func (a *App) TriggerCapture() error {
	go a.captureAndEmit()
	return nil
}

func (a *App) ProcessImage(base64Crop string) error {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	if a.processing != nil {
		a.processing()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.processing = cancel
	a.mu.Unlock()

	go a.processImage(ctx, cfg, base64Crop)
	return nil
}

func (a *App) HideWindow() error {
	a.cancelProcessing()
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
	return nil
}

func (a *App) captureAndEmit() {
	if a.ctx == nil {
		return
	}

	runtime.WindowShow(a.ctx)
	runtime.WindowFullscreen(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)

	result, err := capture.AllDisplays(context.Background())
	if err != nil {
		a.emitError("capture", err)
		return
	}

	runtime.EventsEmit(a.ctx, "capture-start", result)
}

func (a *App) processImage(ctx context.Context, cfg config.Config, base64Crop string) {
	runtime.EventsEmit(a.ctx, "ocr-start", map[string]string{})

	ocrClient := ocr.NewRapidOCR(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
	text, err := ocrClient.ExtractText(ctx, base64Crop)
	if err != nil {
		a.emitError("ocr", err)
		return
	}
	if text == "" {
		a.emitError("ocr", errors.New("OCR returned no text"))
		return
	}

	runtime.EventsEmit(a.ctx, "translation-start", map[string]string{})

	client := translator.NewDeepSeek(translator.Options{
		APIKey:  cfg.DeepSeekAPIKey,
		BaseURL: cfg.DeepSeekBaseURL,
		Model:   cfg.DeepSeekModel,
	})
	err = client.Translate(ctx, text, func(token string) {
		runtime.EventsEmit(a.ctx, "translation-token", token)
	})
	if err != nil {
		a.emitError("translation", err)
		return
	}

	runtime.EventsEmit(a.ctx, "translation-done", map[string]string{})
}

func (a *App) registerShortcut(shortcut string) error {
	if shortcut == "" {
		shortcut = config.Default().ShortcutKey
	}

	registration, err := hotkeys.Register(shortcut, func() {
		_ = a.TriggerCapture()
	})
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shortcut != nil {
		_ = a.shortcut.Unregister()
	}
	a.shortcut = registration
	return nil
}

func (a *App) cancelProcessing() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.processing != nil {
		a.processing()
		a.processing = nil
	}
}

func (a *App) emitError(stage string, err error) {
	if a.ctx == nil || err == nil {
		return
	}

	runtime.EventsEmit(a.ctx, "workflow-error", workflowError{
		Stage:   stage,
		Message: err.Error(),
	})
}

