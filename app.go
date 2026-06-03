package main

import (
	"context"
	_ "embed"
	"errors"
	"sync"
	"time"

	"snaptrans/internal/capture"
	"snaptrans/internal/config"
	"snaptrans/internal/hotkeys"
	"snaptrans/internal/ocr"
	"snaptrans/internal/translator"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

type workflowError struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type ocrResultPayload struct {
	Text   string      `json:"text"`
	Blocks []ocr.Block `json:"blocks"`
}

type App struct {
	ctx context.Context

	configStore *config.Store

	mu         sync.Mutex
	cfg        config.Config
	shortcut   *hotkeys.Registration
	processing context.CancelFunc
	trayOnce   sync.Once
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

	a.startTray()
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
	systray.Quit()
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

func (a *App) ShowCaptureWindow() error {
	if a.ctx == nil {
		return nil
	}

	runtime.WindowShow(a.ctx)
	runtime.WindowFullscreen(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
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
		runtime.WindowUnfullscreen(a.ctx)
		runtime.WindowHide(a.ctx)
	}
	return nil
}

func (a *App) QuitApp() error {
	a.cancelProcessing()
	systray.Quit()
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	return nil
}

func (a *App) ShowSettings() error {
	if a.ctx == nil {
		return nil
	}

	runtime.WindowUnfullscreen(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowSetSize(a.ctx, 520, 420)
	runtime.WindowCenter(a.ctx)
	runtime.WindowShow(a.ctx)
	runtime.EventsEmit(a.ctx, "settings-open", map[string]string{})
	return nil
}

func (a *App) captureAndEmit() {
	if a.ctx == nil {
		return
	}

	runtime.WindowHide(a.ctx)
	time.Sleep(120 * time.Millisecond)

	result, err := capture.AllDisplays(context.Background())
	if err != nil {
		a.emitError("capture", err)
		return
	}

	runtime.EventsEmit(a.ctx, "capture-start", result)
}

func (a *App) startTray() {
	a.trayOnce.Do(func() {
		go systray.Run(a.onTrayReady, func() {})
	})
}

func (a *App) onTrayReady() {
	if len(trayIcon) > 0 {
		systray.SetIcon(trayIcon)
	}
	systray.SetTitle("snapTrans")
	systray.SetTooltip("snapTrans screenshot translator")

	captureItem := systray.AddMenuItem("Capture  Alt+Q", "Start screenshot translation")
	settingsItem := systray.AddMenuItem("Settings", "Open settings")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Quit", "Exit snapTrans")

	go func() {
		for {
			select {
			case <-captureItem.ClickedCh:
				_ = a.TriggerCapture()
			case <-settingsItem.ClickedCh:
				_ = a.ShowSettings()
			case <-quitItem.ClickedCh:
				_ = a.QuitApp()
				return
			}
		}
	}()
}

func (a *App) processImage(ctx context.Context, cfg config.Config, base64Crop string) {
	runtime.EventsEmit(a.ctx, "ocr-start", map[string]string{})

	ocrClient := ocr.NewRapidOCR(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
	result, err := ocrClient.ExtractResult(ctx, base64Crop)
	if err != nil {
		a.emitError("ocr", err)
		return
	}
	if result.Text == "" {
		a.emitError("ocr", errors.New("OCR returned no text"))
		return
	}

	runtime.EventsEmit(a.ctx, "ocr-result", ocrResultPayload{
		Text:   result.Text,
		Blocks: result.Blocks,
	})
	runtime.EventsEmit(a.ctx, "translation-start", map[string]string{})
	if translated, ok := translator.TryFastTranslation(result.Text); ok {
		runtime.EventsEmit(a.ctx, "translation-token", translated)
		runtime.EventsEmit(a.ctx, "translation-done", map[string]string{})
		return
	}

	client := translator.NewDeepSeek(translator.Options{
		APIKey:  cfg.DeepSeekAPIKey,
		BaseURL: cfg.DeepSeekBaseURL,
		Model:   cfg.DeepSeekModel,
	})
	err = client.Translate(ctx, result.Text, func(token string) {
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
