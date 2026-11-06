//go:build !windows

package desktop

import (
	"context"
	"errors"
	"image"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func availableScreenHeight() int {
	return 0
}

type manualScrollTarget struct {
	window uintptr
}

func findManualScrollTarget(_ image.Rectangle) (manualScrollTarget, error) {
	return manualScrollTarget{}, errors.New("manual scrolling capture is only available on Windows")
}

func focusManualScrollTarget(_ manualScrollTarget) {}

func captureManualScrollRegion(_ uintptr, _ image.Rectangle) (image.Image, error) {
	return nil, errors.New("manual scrolling capture is only available on Windows")
}

func manualScrollOverlayWindow() (uintptr, error) {
	return 0, errors.New("manual scrolling capture is only available on Windows")
}

func applyManualScrollHole(_ uintptr, _ image.Rectangle) error {
	return errors.New("manual scrolling capture is only available on Windows")
}

func restoreManualScrollOverlay(_ uintptr) {}

func moveWindowToDisplay(_ context.Context, _ int, _ int) {}

func waitForWindowHidden() {
	time.Sleep(16 * time.Millisecond)
}

func showWindowSynchronously(ctx context.Context, lifetime context.Context) bool {
	if lifetime.Err() != nil {
		return false
	}
	runtime.WindowShow(ctx)
	return true
}
