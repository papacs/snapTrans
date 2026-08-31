package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"snaptrans/internal/capture"
	"snaptrans/internal/selection"
	"snaptrans/internal/textregion"
	"snaptrans/internal/translator"
)

type textRegionsPayload struct {
	Generation int                `json:"generation"`
	Text       string             `json:"text"`
	Blocks     []textregion.Block `json:"blocks"`
	Source     string             `json:"source"`
}

func (a *App) currentCaptureEpoch() uint64             { a.mu.Lock(); defer a.mu.Unlock(); return a.captureEpoch }
func (a *App) captureRequestCurrent(epoch uint64) bool { return a.currentCaptureEpoch() == epoch }
func (a *App) invalidateCaptureRequest() {
	a.mu.Lock()
	a.captureEpoch++
	a.selectedText = nil
	a.frame = nil
	if a.captureAssets != nil {
		a.captureAssets.Clear()
	}
	a.mu.Unlock()
}

// TriggerTranslation is the global-hotkey entry. Explicit tray Capture keeps
// its original manual-box behavior even when another app retains a selection.
func (a *App) TriggerTranslation() error {
	a.cancelTextAction()
	if a.ctx == nil {
		return nil
	}
	window := selection.Foreground()
	wasVisible, epoch, started := a.beginCapture()
	if !started {
		return nil
	}
	a.cancelProcessing()
	go func() {
		defer a.finishCapture()
		if window == 0 {
			a.captureAndEmit(wasVisible, "translate", epoch, "")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), selection.ProbeTimeout)
		selected, err := selection.DefaultReader().Read(ctx, window)
		cancel()
		if !a.captureRequestCurrent(epoch) {
			return
		}
		if errors.Is(err, selection.ErrChanged) || !selection.StillForeground(window) {
			return
		}
		if err == nil && selection.StillForeground(window) {
			captureErr := a.captureSelectedText(selected, wasVisible, epoch)
			if captureErr == nil || errors.Is(captureErr, selection.ErrChanged) || errors.Is(captureErr, context.Canceled) {
				return
			}
			err = captureErr
		}
		if !a.captureRequestCurrent(epoch) {
			return
		}
		notice := ""
		if err != nil && !errors.Is(err, selection.ErrNoSelection) && window != 0 {
			notice = "Selection unavailable — drag to translate / 无法定位选中文字，请框选翻译"
		}
		a.captureAndEmit(wasVisible, "translate", epoch, notice)
	}()
	return nil
}
func (a *App) captureSelectedText(selected selection.Result, wasVisible bool, epoch uint64) error {
	if !selection.StillForeground(selected.Window) {
		return selection.ErrChanged
	}
	startedAt := time.Now()
	// Preserve the visible source without changing its selection or clipboard.
	// Only the text reaches the translation API.
	if wasVisible {
		runtime.WindowHide(a.ctx)
		waitForWindowHidden()
	}
	if !a.captureRequestCurrent(epoch) {
		return context.Canceled
	}
	frame, err := capture.SelectionDisplay(context.Background(), selected.Bounds())
	if err != nil {
		return err
	}
	if !selection.StillForeground(selected.Window) || !a.captureRequestCurrent(epoch) {
		return selection.ErrChanged
	}
	screen := image.Rect(frame.OriginX, frame.OriginY, frame.OriginX+frame.Width, frame.OriginY+frame.Height)
	region, blocks, err := selected.Normalize(screen)
	if err != nil {
		return err
	}
	frame.SelectedText = &capture.SelectedText{ID: fmt.Sprintf("selection-%d", epoch), Block: region, Blocks: blocks}
	a.publishCapture(frame, "translate", epoch, "", startedAt)
	return nil
}
func (a *App) TranslateSelection(id string, direction string, generation int) error {
	a.mu.Lock()
	selected := a.selectedText
	cfg := a.cfg.WithDefaults()
	if selected == nil || selected.ID != id {
		a.mu.Unlock()
		return errors.New("text selection expired; select text again")
	}
	blocks := append([]textregion.Block(nil), selected.Blocks...)
	if a.processing != nil {
		a.processing()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.processing = cancel
	a.mu.Unlock()
	lines := make([]string, len(blocks))
	for i, block := range blocks {
		lines[i] = block.Text
	}
	go func() {
		defer cancel()
		a.processText(ctx, cfg, strings.Join(lines, "\n"), blocks, translator.NormalizeDirection(direction), generation, time.Now(), "selection")
	}()
	return nil
}
