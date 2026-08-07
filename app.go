package main

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"snaptrans/internal/capture"
	"snaptrans/internal/config"
	"snaptrans/internal/history"
	"snaptrans/internal/hotkeys"
	"snaptrans/internal/logfile"
	"snaptrans/internal/ocr"
	"snaptrans/internal/translator"

	"snaptrans/internal/autostart"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var trayIcon []byte

type workflowError struct {
	Generation int    `json:"generation"`
	Stage      string `json:"stage"`
	Message    string `json:"message"`
}

type ocrResultPayload struct {
	Generation int         `json:"generation"`
	Text       string      `json:"text"`
	Blocks     []ocr.Block `json:"blocks"`
}

type translationTokenPayload struct {
	Generation int    `json:"generation"`
	Token      string `json:"token"`
}

type translationDirectionPayload struct {
	Generation int    `json:"generation"`
	Direction  string `json:"direction"`
}

type sentinelPayload struct {
	Generation int `json:"generation"`
}

type App struct {
	ctx context.Context

	configStore  *config.Store
	historyStore *history.Store
	log          *logfile.Logger

	trayMu      sync.Mutex
	captureItem *systray.MenuItem

	mu         sync.Mutex
	cfg        config.Config
	shortcut   *hotkeys.Registration
	processing context.CancelFunc
	ocrWorker  *ocr.Worker
	trayOnce   sync.Once

	captureOriginX  int
	captureOriginY  int
	captureInFlight bool
	windowVisible   bool
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	store, err := config.NewStore("snapTrans")
	if err != nil {
		a.emitError("config", err, 0)
		return
	}
	a.configStore = store
	a.historyStore = history.NewStore(filepath.Join(filepath.Dir(store.Path), "history.json"), 50)
	a.log = logfile.NewLogger(filepath.Join(filepath.Dir(store.Path), "logs", "snaptrans.log"), 0)
	a.log.Infof("snapTrans started")

	cfg, err := store.Load()
	if err != nil {
		a.emitError("config", err, 0)
		cfg = config.Default()
	}

	a.mu.Lock()
	a.cfg = cfg
	a.mu.Unlock()

	if err := a.registerShortcut(cfg.ShortcutKey); err != nil {
		a.emitError("config", err, 0)
	}

	a.syncOCRWorker(cfg)
	a.startTray()
}

// syncOCRWorker starts, stops, or restarts the persistent OCR worker to
// match the current configuration.
func (a *App) syncOCRWorker(cfg config.Config) {
	a.mu.Lock()
	worker := a.ocrWorker
	currentPath := a.cfg.RapidOCRPath
	a.mu.Unlock()

	if !cfg.PersistentOCR {
		if worker != nil {
			worker.Close()
			a.mu.Lock()
			a.ocrWorker = nil
			a.mu.Unlock()
		}
		return
	}

	if worker == nil {
		a.mu.Lock()
		worker = ocr.NewRapidOCRWorker(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
		a.ocrWorker = worker
		a.mu.Unlock()
		go a.warmOCRWorker(worker)
		return
	}

	if cfg.RapidOCRPath != currentPath {
		worker.SetExecutable(cfg.RapidOCRPath)
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := worker.Restart(ctx); err != nil {
				a.emitError("ocr", fmt.Errorf("OCR worker restart failed: %w", err), 0)
			}
		}()
	}
}

// warmOCRWorker starts a persistent RapidOCR worker in the background so
// the first screenshot avoids the model-loading delay.
func (a *App) warmOCRWorker(worker *ocr.Worker) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := worker.Start(ctx); err != nil {
		a.emitError("ocr", fmt.Errorf("OCR worker warm-up failed: %w", err), 0)
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
	if a.ocrWorker != nil {
		a.ocrWorker.Close()
		a.ocrWorker = nil
	}
	if a.log != nil {
		a.log.Infof("snapTrans stopped")
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

	a.mu.Lock()
	current := a.cfg
	a.mu.Unlock()

	if _, _, err := hotkeys.ParseShortcut(next.ShortcutKey); err != nil {
		if a.log != nil {
			a.log.Errorf("invalid shortcut %q: %v", next.ShortcutKey, err)
		}
		return fmt.Errorf("invalid shortcut %q: %w", next.ShortcutKey, err)
	}

	var registration *hotkeys.Registration
	if next.ShortcutKey != current.ShortcutKey {
		var err error
		registration, err = hotkeys.Register(next.ShortcutKey, func() {
			_ = a.TriggerCapture()
		})
		if err != nil {
			if a.log != nil {
				a.log.Errorf("shortcut registration failed for %q: %v", next.ShortcutKey, err)
			}
			return fmt.Errorf("shortcut %q is unavailable: %w", next.ShortcutKey, err)
		}
	}

	if err := a.configStore.Save(next); err != nil {
		if registration != nil {
			_ = registration.Unregister()
		}
		return err
	}

	a.mu.Lock()
	a.cfg = next
	if registration != nil {
		if a.shortcut != nil {
			_ = a.shortcut.Unregister()
		}
		a.shortcut = registration
	}
	a.mu.Unlock()

	a.updateTrayShortcut(next.ShortcutKey)
	a.syncOCRWorker(next)
	return nil
}

func (a *App) updateTrayShortcut(shortcut string) {
	a.trayMu.Lock()
	item := a.captureItem
	a.trayMu.Unlock()
	if item != nil {
		item.SetTitle("Capture  " + shortcut)
	}
}

func (a *App) TriggerCapture() error {
	wasVisible, started := a.beginCapture()
	if !started {
		return nil
	}

	go func() {
		defer a.finishCapture()
		a.captureAndEmit(wasVisible)
	}()
	return nil
}

func (a *App) beginCapture() (wasVisible bool, started bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.captureInFlight {
		return false, false
	}
	a.captureInFlight = true
	wasVisible = a.windowVisible
	a.windowVisible = false
	return wasVisible, true
}

func (a *App) finishCapture() {
	a.mu.Lock()
	a.captureInFlight = false
	a.mu.Unlock()
}

func (a *App) ShowCaptureWindow() error {
	if a.ctx == nil {
		return nil
	}

	a.mu.Lock()
	originX := a.captureOriginX
	originY := a.captureOriginY
	a.mu.Unlock()

	runtime.WindowUnfullscreen(a.ctx)
	moveWindowToDisplay(a.ctx, originX, originY)
	runtime.WindowFullscreen(a.ctx)
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowShow(a.ctx)
	a.mu.Lock()
	a.windowVisible = true
	a.mu.Unlock()
	return nil
}

func (a *App) ProcessImage(base64Crop string, direction string, generation int) error {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	if a.processing != nil {
		a.processing()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.processing = cancel
	a.mu.Unlock()

	go a.processImage(ctx, cfg, base64Crop, translator.NormalizeDirection(direction), generation)
	return nil
}

func (a *App) HideWindow() error {
	a.cancelProcessing()
	a.mu.Lock()
	a.windowVisible = false
	a.mu.Unlock()
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
	width, height := 680, 780
	if screenHeight := availableScreenHeight(); screenHeight > 0 && height > screenHeight-48 {
		height = screenHeight - 48
	}
	if height < 480 {
		height = 480
	}
	runtime.WindowSetSize(a.ctx, width, height)
	runtime.WindowCenter(a.ctx)
	runtime.WindowShow(a.ctx)
	a.mu.Lock()
	a.windowVisible = true
	a.mu.Unlock()
	runtime.EventsEmit(a.ctx, "settings-open", map[string]string{})
	return nil
}

// GetWindowPosition returns the frameless window's current position.
func (a *App) GetWindowPosition() (int, int, error) {
	if a.ctx == nil {
		return 0, 0, nil
	}
	x, y := runtime.WindowGetPosition(a.ctx)
	return x, y, nil
}

// SetWindowPosition moves the frameless window to the given position.
func (a *App) SetWindowPosition(x int, y int) error {
	if a.ctx == nil {
		return nil
	}
	runtime.WindowSetPosition(a.ctx, x, y)
	return nil
}

func (a *App) captureAndEmit(wasVisible bool) {
	if a.ctx == nil {
		return
	}

	startedAt := time.Now()
	a.cancelProcessing()
	runtime.WindowHide(a.ctx)
	if wasVisible {
		time.Sleep(120 * time.Millisecond)
	}

	result, err := capture.ActiveDisplay(context.Background())
	if err != nil {
		a.emitError("capture", err, 0)
		if _, dialogErr := runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "snapTrans capture failed",
			Message: err.Error(),
			Buttons: []string{"OK"},
		}); dialogErr != nil && a.log != nil {
			a.log.Errorf("capture error dialog failed: %v", dialogErr)
		}
		return
	}

	a.mu.Lock()
	a.captureOriginX = result.OriginX
	a.captureOriginY = result.OriginY
	a.mu.Unlock()
	if a.log != nil {
		scale := 1.0
		if len(result.Displays) > 0 {
			scale = result.Displays[0].Scale
		}
		a.log.Infof(
			"capture completed duration_ms=%d image=%dx%d origin=(%d,%d) scale=%.2f encoded_bytes=%d",
			time.Since(startedAt).Milliseconds(),
			result.Width,
			result.Height,
			result.OriginX,
			result.OriginY,
			scale,
			len(result.Image),
		)
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

	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	captureItem.SetTitle("Capture  " + cfg.ShortcutKey)
	a.trayMu.Lock()
	a.captureItem = captureItem
	a.trayMu.Unlock()

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

func (a *App) processImage(ctx context.Context, cfg config.Config, base64Crop string, direction translator.Direction, generation int) {
	startedAt := time.Now()
	runtime.EventsEmit(a.ctx, "ocr-start", sentinelPayload{Generation: generation})

	result, err := a.runOCR(ctx, cfg, base64Crop)
	if err != nil {
		a.emitError("ocr", err, generation)
		return
	}
	ocrElapsed := time.Since(startedAt)
	if a.log != nil {
		a.log.Infof("generation=%d ocr_ms=%d text_chars=%d blocks=%d", generation, ocrElapsed.Milliseconds(), len(result.Text), len(result.Blocks))
	}
	if result.Text == "" {
		a.emitError("ocr", errors.New("OCR returned no text"), generation)
		return
	}

	if direction == translator.DirectionAuto {
		direction = translator.DetectDirection(result.Text)
	}
	runtime.EventsEmit(a.ctx, "translation-direction", translationDirectionPayload{
		Generation: generation,
		Direction:  string(direction),
	})

	runtime.EventsEmit(a.ctx, "ocr-result", ocrResultPayload{
		Generation: generation,
		Text:       result.Text,
		Blocks:     result.Blocks,
	})
	runtime.EventsEmit(a.ctx, "translation-start", sentinelPayload{Generation: generation})
	if translated, ok := translator.TryFastTranslation(result.Text, direction); ok {
		runtime.EventsEmit(a.ctx, "translation-token", translationTokenPayload{
			Generation: generation,
			Token:      translated,
		})
		runtime.EventsEmit(a.ctx, "translation-done", sentinelPayload{Generation: generation})
		a.saveHistory(result.Text, translated, string(direction))
		return
	}

	var translated strings.Builder
	client := translator.NewOpenAICompatible(translator.Options{
		APIKey:       cfg.APIKey,
		BaseURL:      cfg.BaseURL,
		Model:        cfg.Model,
		SystemPrompt: cfg.SystemPrompt,
		Glossary:     cfg.Glossary,
	})
	err = client.Translate(ctx, result.Text, direction, func(token string) {
		translated.WriteString(token)
		runtime.EventsEmit(a.ctx, "translation-token", translationTokenPayload{
			Generation: generation,
			Token:      token,
		})
	})
	if err != nil {
		a.emitError("translation", err, generation)
		return
	}

	runtime.EventsEmit(a.ctx, "translation-done", sentinelPayload{Generation: generation})
	if a.log != nil {
		a.log.Infof("generation=%d direction=%s total_ms=%d", generation, direction, time.Since(startedAt).Milliseconds())
	}
	a.saveHistory(result.Text, translated.String(), string(direction))
}

func (a *App) saveHistory(source string, translated string, direction string) {
	if a.historyStore == nil {
		return
	}
	if err := a.historyStore.Add(source, translated, direction); err != nil {
		a.emitError("config", fmt.Errorf("save history: %w", err), 0)
	}
}

func (a *App) GetHistory() ([]history.Entry, error) {
	if a.historyStore == nil {
		return nil, nil
	}
	return a.historyStore.List()
}

func (a *App) ClearHistory() error {
	if a.historyStore == nil {
		return nil
	}
	return a.historyStore.Clear()
}

// TestConnection verifies the configured LLM endpoint with the current
// settings without saving them.
func (a *App) TestConnection() error {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	a.mu.Unlock()

	client := translator.NewOpenAICompatible(translator.Options{
		APIKey:  cfg.APIKey,
		BaseURL: cfg.BaseURL,
		Model:   cfg.Model,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.Ping(ctx)
}

type EnvironmentStatus struct {
	OCRReady         bool   `json:"ocrReady"`
	OCRDetail        string `json:"ocrDetail"`
	APIKeyConfigured bool   `json:"apiKeyConfigured"`
	Shortcut         string `json:"shortcut"`
}

const autostartValueName = "snapTrans"

// appVersion is shown in the settings window and log output.
const appVersion = "0.2.0"

func (a *App) GetVersion() string {
	return appVersion
}

// OpenLogFolder reveals the local log directory in Explorer.
func (a *App) OpenLogFolder() error {
	if a.log == nil {
		return errors.New("log folder is not available")
	}
	dir := a.log.Dir()
	if dir == "" {
		return errors.New("log folder is not available")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return exec.Command("explorer", dir).Start()
}

func (a *App) SetAutoStart(enabled bool) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("autostart: resolve executable: %w", err)
	}
	return autostart.Set(autostartValueName, executable, enabled)
}

func (a *App) IsAutoStartEnabled() (bool, error) {
	return autostart.IsEnabled(autostartValueName)
}

// GetEnvironmentStatus reports whether the OCR executable and the LLM API
// key are configured, so the settings window can surface issues up front.
func (a *App) GetEnvironmentStatus() EnvironmentStatus {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	a.mu.Unlock()

	status := EnvironmentStatus{
		APIKeyConfigured: strings.TrimSpace(cfg.APIKey) != "",
		Shortcut:         cfg.ShortcutKey,
	}

	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	resolved, err := ocr.ResolveExecutablePath(cfg.RapidOCRPath, cwd, executable)
	if err != nil {
		status.OCRReady = false
		status.OCRDetail = err.Error()
		return status
	}
	status.OCRReady = true
	status.OCRDetail = resolved
	return status
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

// runOCR prefers the persistent worker; if the worker is unavailable or
// fails, it falls back to a one-shot RapidOCR invocation.
func (a *App) runOCR(ctx context.Context, cfg config.Config, base64Crop string) (ocr.Result, error) {
	a.mu.Lock()
	worker := a.ocrWorker
	a.mu.Unlock()

	if worker != nil {
		result, err := worker.Run(ctx, base64Crop)
		if err == nil {
			return result, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ocr.Result{}, err
		}
		fallback := ocr.NewRapidOCR(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
		if result, fallbackErr := fallback.ExtractResult(ctx, base64Crop); fallbackErr == nil {
			return result, nil
		} else {
			return ocr.Result{}, fallbackErr
		}
	}

	fallback := ocr.NewRapidOCR(cfg.RapidOCRPath, time.Duration(cfg.RapidOCRTimeoutSeconds)*time.Second)
	return fallback.ExtractResult(ctx, base64Crop)
}

func (a *App) emitError(stage string, err error, generation int) {
	if a.ctx == nil || err == nil {
		return
	}
	if a.log != nil {
		a.log.Errorf("stage=%s generation=%d error=%v", stage, generation, err)
	}

	runtime.EventsEmit(a.ctx, "workflow-error", workflowError{
		Generation: generation,
		Stage:      stage,
		Message:    err.Error(),
	})
}
