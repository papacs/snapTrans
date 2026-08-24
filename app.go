package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/png"
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

type TextExtractionResult struct {
	Text   string      `json:"text"`
	Blocks []ocr.Block `json:"blocks"`
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

	trayMu         sync.Mutex
	captureItem    *systray.MenuItem
	screenshotItem *systray.MenuItem

	mu                 sync.Mutex
	cfg                config.Config
	shortcut           *hotkeys.Registration
	screenshotShortcut *hotkeys.Registration
	processing         context.CancelFunc
	ocrWorker          *ocr.Worker
	trayOnce           sync.Once

	captureOriginX   int
	captureOriginY   int
	captureInFlight  bool
	windowVisible    bool
	frontendReady    bool
	settingsPending  bool
	capturePrepared  bool
	preparedOriginX  int
	preparedOriginY  int
	captureStartedAt time.Time
	captureEmittedAt time.Time
	captureAssets    *captureAssets
	frame            image.Image
	scrollCapture    *capture.ManualScrollCapture
	scrollOverlay    uintptr
}

func NewApp() *App {
	return &App{captureAssets: newCaptureAssets()}
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
	if err := a.registerScreenshotShortcut(cfg.ScreenshotShortcutKey); err != nil {
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
	if a.screenshotShortcut != nil {
		_ = a.screenshotShortcut.Unregister()
		a.screenshotShortcut = nil
	}
	if a.processing != nil {
		a.processing()
		a.processing = nil
	}
	if a.scrollCapture != nil {
		a.scrollCapture.Cancel()
		a.scrollCapture = nil
		restoreManualScrollOverlay(a.scrollOverlay)
		a.scrollOverlay = 0
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

// FrontendReady completes the startup handshake after Vue has registered all
// backend event listeners. Any early settings request is delivered while the
// native window is still hidden.
func (a *App) FrontendReady() error {
	openSettings := a.markFrontendReady()
	if a.log != nil {
		a.log.Infof("frontend ready pending_settings=%t", openSettings)
	}
	if openSettings {
		a.emitSettingsOpen()
	}
	return nil
}

func (a *App) markFrontendReady() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.frontendReady = true
	openSettings := a.settingsPending
	a.settingsPending = false
	return openSettings
}

func (a *App) requestSettingsOpen() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.frontendReady {
		a.settingsPending = true
		return false
	}
	return true
}

func (a *App) emitSettingsOpen() {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "settings-open", map[string]string{})
	}
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
	if _, _, err := hotkeys.ParseShortcut(next.ScreenshotShortcutKey); err != nil {
		if a.log != nil {
			a.log.Errorf("invalid screenshot shortcut %q: %v", next.ScreenshotShortcutKey, err)
		}
		return fmt.Errorf("invalid screenshot shortcut %q: %w", next.ScreenshotShortcutKey, err)
	}
	if strings.EqualFold(next.ShortcutKey, next.ScreenshotShortcutKey) {
		return errors.New("translation and screenshot shortcuts must be different")
	}

	var registration *hotkeys.Registration
	var screenshotRegistration *hotkeys.Registration
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
	if next.ScreenshotShortcutKey != current.ScreenshotShortcutKey {
		var err error
		screenshotRegistration, err = hotkeys.Register(next.ScreenshotShortcutKey, func() {
			_ = a.TriggerScreenshot()
		})
		if err != nil {
			if registration != nil {
				_ = registration.Unregister()
			}
			if a.log != nil {
				a.log.Errorf("screenshot shortcut registration failed for %q: %v", next.ScreenshotShortcutKey, err)
			}
			return fmt.Errorf("screenshot shortcut %q is unavailable: %w", next.ScreenshotShortcutKey, err)
		}
	}

	if err := a.configStore.Save(next); err != nil {
		if registration != nil {
			_ = registration.Unregister()
		}
		if screenshotRegistration != nil {
			_ = screenshotRegistration.Unregister()
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
	if screenshotRegistration != nil {
		if a.screenshotShortcut != nil {
			_ = a.screenshotShortcut.Unregister()
		}
		a.screenshotShortcut = screenshotRegistration
	}
	a.mu.Unlock()

	a.updateTrayShortcuts(next.ShortcutKey, next.ScreenshotShortcutKey)
	a.syncOCRWorker(next)
	return nil
}

func (a *App) updateTrayShortcuts(shortcut string, screenshotShortcut string) {
	a.trayMu.Lock()
	item := a.captureItem
	screenshotItem := a.screenshotItem
	a.trayMu.Unlock()
	if item != nil {
		item.SetTitle("Capture  " + shortcut)
	}
	if screenshotItem != nil {
		screenshotItem.SetTitle("Screenshot  " + screenshotShortcut)
	}
}

func (a *App) TriggerCapture() error {
	return a.triggerCaptureMode("translate")
}

func (a *App) TriggerScreenshot() error {
	return a.triggerCaptureMode("screenshot")
}

type ScrollCaptureRequest struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type ScrollCaptureStepResult struct {
	CurrentImage string `json:"currentImage"`
	PreviewImage string `json:"previewImage"`
	Frames       int    `json:"frames"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Appended     bool   `json:"appended"`
}

// BeginScrollingScreenshot captures the initial frame while the overlay is
// hidden, then cuts an input hole over the selection. The real window receives
// user input while the remaining overlay shows the stitched preview.
func (a *App) BeginScrollingScreenshot(request ScrollCaptureRequest) (capture.ManualScrollStatus, error) {
	if a.ctx == nil {
		return capture.ManualScrollStatus{}, errors.New("scrolling capture requires the desktop app")
	}
	overlay, err := manualScrollOverlayWindow()
	if err != nil {
		return capture.ManualScrollStatus{}, err
	}

	if request.Width < 32 || request.Height < 32 || request.Width > 16384 || request.Height > 16384 ||
		request.X < -1_000_000 || request.X > 1_000_000 || request.Y < -1_000_000 || request.Y > 1_000_000 {
		return capture.ManualScrollStatus{}, errors.New("invalid scrolling capture region")
	}

	a.mu.Lock()
	if a.captureInFlight {
		a.mu.Unlock()
		return capture.ManualScrollStatus{}, errors.New("another capture is already running")
	}
	a.captureInFlight = true
	wasVisible := a.windowVisible
	a.windowVisible = false
	a.mu.Unlock()

	runtime.WindowHide(a.ctx)
	if wasVisible {
		waitForWindowHidden()
	}

	rect := image.Rect(request.X, request.Y, request.X+request.Width, request.Y+request.Height)
	target, err := findManualScrollTarget(rect)
	if err != nil {
		a.finishCapture()
		return capture.ManualScrollStatus{}, err
	}
	session, err := capture.StartManualScrollCaptureWithSource(
		context.Background(),
		rect,
		capture.ManualScrollOptions{},
		func(frameRect image.Rectangle) (image.Image, error) {
			return captureManualScrollRegion(target.window, frameRect)
		},
	)
	if err != nil {
		a.finishCapture()
		return capture.ManualScrollStatus{}, err
	}

	if err := applyManualScrollHole(overlay, rect); err != nil {
		session.Cancel()
		a.finishCapture()
		return capture.ManualScrollStatus{}, err
	}

	a.mu.Lock()
	a.scrollCapture = session
	a.scrollOverlay = overlay
	a.windowVisible = true
	a.mu.Unlock()
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowShow(a.ctx)
	return session.Status(), nil
}

// StepScrollingScreenshot observes the real window exposed through the overlay
// hole. User wheel and scrollbar input goes directly to that window.
func (a *App) StepScrollingScreenshot() (ScrollCaptureStepResult, error) {
	a.mu.Lock()
	session := a.scrollCapture
	a.mu.Unlock()
	if session == nil {
		return ScrollCaptureStepResult{}, errors.New("no scrolling capture is active")
	}

	snapshot, err := session.CaptureNext()
	if err != nil {
		return ScrollCaptureStepResult{}, err
	}
	result := ScrollCaptureStepResult{
		Frames:   snapshot.Frames,
		Width:    snapshot.Width,
		Height:   snapshot.Height,
		Appended: snapshot.Appended,
	}
	if len(snapshot.CurrentImageBytes) > 0 {
		result.CurrentImage = a.captureAssets.Store(snapshot.CurrentImageBytes)
	}
	if len(snapshot.PreviewImageBytes) > 0 {
		result.PreviewImage = a.captureAssets.Store(snapshot.PreviewImageBytes)
	}
	return result, nil
}

func (a *App) FinishScrollingScreenshot() (capture.Result, error) {
	session, overlay := a.takeScrollCapture()
	if session == nil {
		return capture.Result{}, errors.New("no scrolling capture is active")
	}
	if _, err := session.CaptureNext(); err != nil {
		a.finishScrollingCaptureWindow(overlay)
		return capture.Result{}, err
	}

	result, err := session.Finish()
	a.finishScrollingCaptureWindow(overlay)
	if err != nil {
		return capture.Result{}, err
	}
	result.Image = a.captureAssets.Store(result.ImageBytes)
	result.ImageBytes = nil
	if a.log != nil {
		a.log.Infof(
			"manual scrolling capture completed frames=%d image=%dx%d png_bytes=%d",
			result.ScrollFrames,
			result.Width,
			result.Height,
			result.EncodedBytes,
		)
	}
	return result, nil
}

func (a *App) CancelScrollingScreenshot() error {
	session, overlay := a.takeScrollCapture()
	if session == nil {
		return nil
	}
	session.Cancel()
	a.finishScrollingCaptureWindow(overlay)
	return nil
}

func (a *App) takeScrollCapture() (*capture.ManualScrollCapture, uintptr) {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := a.scrollCapture
	overlay := a.scrollOverlay
	a.scrollCapture = nil
	a.scrollOverlay = 0
	return session, overlay
}

func (a *App) finishScrollingCaptureWindow(overlay uintptr) {
	restoreManualScrollOverlay(overlay)
	a.finishCapture()
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
	a.mu.Lock()
	a.windowVisible = false
	a.mu.Unlock()
}

func (a *App) triggerCaptureMode(mode string) error {
	wasVisible, started := a.beginCapture()
	if !started {
		return nil
	}

	go func() {
		defer a.finishCapture()
		a.captureAndEmit(wasVisible, mode)
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
	a.captureStartedAt = time.Now()
	a.captureEmittedAt = time.Time{}
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
	startedAt := a.captureStartedAt
	emittedAt := a.captureEmittedAt
	a.mu.Unlock()

	if a.shouldPrepareCaptureWindow(originX, originY) {
		runtime.WindowUnfullscreen(a.ctx)
		moveWindowToDisplay(a.ctx, originX, originY)
		runtime.WindowFullscreen(a.ctx)
	}
	runtime.WindowSetAlwaysOnTop(a.ctx, true)
	runtime.WindowShow(a.ctx)
	a.mu.Lock()
	a.windowVisible = true
	a.mu.Unlock()
	if a.log != nil && !startedAt.IsZero() && !emittedAt.IsZero() {
		a.log.Infof(
			"capture ui ready total_ms=%d bridge_decode_ms=%d",
			time.Since(startedAt).Milliseconds(),
			time.Since(emittedAt).Milliseconds(),
		)
	}
	return nil
}

func (a *App) shouldPrepareCaptureWindow(originX int, originY int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.capturePrepared && a.preparedOriginX == originX && a.preparedOriginY == originY {
		return false
	}
	a.capturePrepared = true
	a.preparedOriginX = originX
	a.preparedOriginY = originY
	return true
}

func (a *App) invalidateCaptureWindowPreparation() {
	a.mu.Lock()
	a.capturePrepared = false
	a.mu.Unlock()
}

func (a *App) ProcessImage(base64Crop string, direction string, generation int) error {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	a.mu.Unlock()

	a.startProcessing(cfg, base64Crop, translator.NormalizeDirection(direction), generation)
	return nil
}

// TranslateRegionRequest is a selection in captured-frame pixel coordinates.
type TranslateRegionRequest struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// TranslateRegion runs the OCR and translation workflow directly on the last
// captured frame, avoiding the frontend crop and its base64 round trip over
// the Wails JSON bridge. The frontend still performs the same DPI-aware
// coordinate mapping, so the cropped input matches the previous pipeline.
func (a *App) TranslateRegion(region TranslateRegionRequest, direction string, generation int) error {
	// The frontend treats selections smaller than 8 px as unusable; keep the
	// same minimum here so tiny-but-valid selections keep working.
	if region.Width < 8 || region.Height < 8 || region.Width > 16384 || region.Height > 16384 ||
		region.X < -1_000_000 || region.X > 1_000_000 || region.Y < -1_000_000 || region.Y > 1_000_000 {
		return errors.New("invalid translation region")
	}

	a.mu.Lock()
	frame := a.frame
	cfg := a.cfg.WithDefaults()
	a.mu.Unlock()
	if frame == nil {
		return errors.New("no captured frame is available; start a new capture")
	}

	rect := image.Rect(region.X, region.Y, region.X+region.Width, region.Y+region.Height).Intersect(frame.Bounds())
	if rect.Empty() {
		return errors.New("selection is outside the captured frame")
	}

	startedAt := time.Now()
	cropped := capture.CropForOCR(frame, rect)
	encoded, err := capture.EncodePNGBytes(cropped)
	if err != nil {
		return fmt.Errorf("encode selection: %w", err)
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded)
	if a.log != nil {
		a.log.Infof(
			"generation=%d backend_crop region=%dx%d+%d+%d scale=%.1f crop_png_bytes=%d crop_ms=%d",
			generation,
			rect.Dx(),
			rect.Dy(),
			rect.Min.X,
			rect.Min.Y,
			capture.OCRScaleForRect(rect),
			len(encoded),
			time.Since(startedAt).Milliseconds(),
		)
	}

	a.startProcessing(cfg, dataURL, translator.NormalizeDirection(direction), generation)
	return nil
}

// startProcessing cancels any in-flight workflow and starts a new one with a
// bounded translation timeout so a stalled API call cannot hang the overlay.
func (a *App) startProcessing(cfg config.Config, base64Crop string, direction translator.Direction, generation int) {
	a.mu.Lock()
	if a.processing != nil {
		a.processing()
	}
	timeout := time.Duration(cfg.TranslationTimeoutSeconds) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	a.processing = cancel
	a.mu.Unlock()

	go a.processImage(ctx, cfg, base64Crop, direction, generation)
}

// ExtractText runs the existing local OCR pipeline without starting a translation.
func (a *App) ExtractText(base64Image string) (TextExtractionResult, error) {
	if strings.TrimSpace(base64Image) == "" {
		return TextExtractionResult{}, errors.New("image data is required")
	}

	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	a.mu.Unlock()

	startedAt := time.Now()
	result, err := a.runOCR(context.Background(), cfg, base64Image)
	if err != nil {
		if a.log != nil {
			a.log.Errorf("mode=extract-text stage=ocr error=%v", err)
		}
		return TextExtractionResult{}, err
	}

	result.Text = strings.TrimSpace(result.Text)
	if result.Text == "" {
		return TextExtractionResult{}, errors.New("OCR returned no text")
	}
	if a.log != nil {
		a.log.Infof("mode=extract-text ocr_ms=%d text_chars=%d blocks=%d", time.Since(startedAt).Milliseconds(), len(result.Text), len(result.Blocks))
	}

	return TextExtractionResult{
		Text:   result.Text,
		Blocks: result.Blocks,
	}, nil
}

func (a *App) HideWindow() error {
	session, overlay := a.takeScrollCapture()
	if session != nil {
		session.Cancel()
		a.finishCapture()
	}
	restoreManualScrollOverlay(overlay)
	a.cancelProcessing()
	a.mu.Lock()
	a.windowVisible = false
	a.frame = nil
	a.mu.Unlock()
	if a.ctx != nil {
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
	a.invalidateCaptureWindowPreparation()
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
	if !a.requestSettingsOpen() {
		if a.log != nil {
			a.log.Infof("settings request deferred until frontend is ready")
		}
		return nil
	}
	a.emitSettingsOpen()
	return nil
}

// ShowSettingsWindow is called by Vue only after the settings shell has been
// committed to the DOM, so an unrendered transparent WebView is never exposed.
func (a *App) ShowSettingsWindow() error {
	if a.ctx == nil {
		return nil
	}

	runtime.WindowShow(a.ctx)
	a.mu.Lock()
	a.windowVisible = true
	a.mu.Unlock()
	if a.log != nil {
		a.log.Infof("settings window shown after frontend render")
	}
	return nil
}

func (a *App) captureAndEmit(wasVisible bool, mode string) {
	if a.ctx == nil {
		return
	}

	startedAt := time.Now()
	a.cancelProcessing()
	runtime.WindowHide(a.ctx)
	if wasVisible {
		waitForWindowHidden()
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
	result.Image = a.captureAssets.Store(result.ImageBytes)
	if frame, decodeErr := png.Decode(bytes.NewReader(result.ImageBytes)); decodeErr == nil {
		a.mu.Lock()
		a.frame = frame
		a.mu.Unlock()
	} else if a.log != nil {
		a.log.Errorf("capture frame decode failed: %v", decodeErr)
	}
	result.ImageBytes = nil
	result.Mode = mode
	if a.log != nil {
		scale := 1.0
		if len(result.Displays) > 0 {
			scale = result.Displays[0].Scale
		}
		a.log.Infof(
			"capture completed total_ms=%d capture_ms=%d encode_ms=%d image=%dx%d origin=(%d,%d) scale=%.2f png_mode=%s png_bytes=%d payload_bytes=%d",
			time.Since(startedAt).Milliseconds(),
			result.CaptureDuration.Milliseconds(),
			result.EncodeDuration.Milliseconds(),
			result.Width,
			result.Height,
			result.OriginX,
			result.OriginY,
			scale,
			result.CompressionMode,
			result.EncodedBytes,
			len(result.Image),
		)
	}

	a.mu.Lock()
	a.captureEmittedAt = time.Now()
	a.mu.Unlock()
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
	screenshotItem := systray.AddMenuItem("Screenshot  Alt+W", "Capture and annotate an image")
	settingsItem := systray.AddMenuItem("Settings", "Open settings")
	systray.AddSeparator()
	quitItem := systray.AddMenuItem("Quit", "Exit snapTrans")

	a.mu.Lock()
	cfg := a.cfg
	a.mu.Unlock()
	captureItem.SetTitle("Capture  " + cfg.ShortcutKey)
	screenshotItem.SetTitle("Screenshot  " + cfg.ScreenshotShortcutKey)
	a.trayMu.Lock()
	a.captureItem = captureItem
	a.screenshotItem = screenshotItem
	a.trayMu.Unlock()

	go func() {
		for {
			select {
			case <-captureItem.ClickedCh:
				_ = a.TriggerCapture()
			case <-screenshotItem.ClickedCh:
				_ = a.TriggerScreenshot()
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
	translationStartedAt := time.Now()
	var firstTokenOnce sync.Once
	emitTranslationToken := func(token string) {
		firstTokenOnce.Do(func() {
			if a.log != nil {
				a.log.Infof(
					"generation=%d first_token_ms=%d translation_wait_ms=%d",
					generation,
					time.Since(startedAt).Milliseconds(),
					time.Since(translationStartedAt).Milliseconds(),
				)
			}
		})
		runtime.EventsEmit(a.ctx, "translation-token", translationTokenPayload{
			Generation: generation,
			Token:      token,
		})
	}
	if translated, ok := translator.TryFastTranslation(result.Text, direction); ok {
		emitTranslationToken(translated)
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
		emitTranslationToken(token)
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("translation timed out after %d seconds", cfg.TranslationTimeoutSeconds)
		}
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
		return []history.Entry{}, nil
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
	OCRReady           bool   `json:"ocrReady"`
	OCRDetail          string `json:"ocrDetail"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	Shortcut           string `json:"shortcut"`
	ScreenshotShortcut string `json:"screenshotShortcut"`
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
		APIKeyConfigured:   strings.TrimSpace(cfg.APIKey) != "",
		Shortcut:           cfg.ShortcutKey,
		ScreenshotShortcut: cfg.ScreenshotShortcutKey,
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

func (a *App) registerScreenshotShortcut(shortcut string) error {
	if shortcut == "" {
		shortcut = config.Default().ScreenshotShortcutKey
	}

	registration, err := hotkeys.Register(shortcut, func() {
		_ = a.TriggerScreenshot()
	})
	if err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.screenshotShortcut != nil {
		_ = a.screenshotShortcut.Unregister()
	}
	a.screenshotShortcut = registration
	return nil
}

// SaveScreenshot opens the native Windows save dialog and writes a PNG.
func (a *App) SaveScreenshot(dataURL string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("application window is not ready")
	}
	encoded, ok := strings.CutPrefix(dataURL, "data:image/png;base64,")
	if !ok {
		return "", errors.New("screenshot must be a PNG data URL")
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode screenshot: %w", err)
	}
	if len(data) == 0 || len(data) > 64*1024*1024 {
		return "", errors.New("screenshot data is empty or too large")
	}

	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Save screenshot",
		DefaultFilename: "snapTrans-" + time.Now().Format("20060102-150405") + ".png",
		Filters:         []runtime.FileFilter{{DisplayName: "PNG image (*.png)", Pattern: "*.png"}},
	})
	if err != nil || path == "" {
		return path, err
	}
	if filepath.Ext(path) == "" {
		path += ".png"
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
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
