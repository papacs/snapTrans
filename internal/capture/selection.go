package capture

import (
	"context"
	"errors"
	"image"
	"snaptrans/internal/textregion"
)

type SelectedText struct {
	ID string `json:"id"`
	textregion.Block
	Blocks []textregion.Block `json:"blocks"`
}

// SelectionDisplay chooses the monitor containing the selection, not the
// cursor. The current overlay is single-monitor, so cross-screen ranges fail.
func SelectionDisplay(ctx context.Context, rect image.Rectangle) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	monitors, err := physicalMonitors()
	if err != nil {
		return Result{}, err
	}
	monitor, ok := containingMonitor(monitors, rect)
	if !ok {
		return Result{}, errors.New("selected text spans displays or is off-screen")
	}
	return captureMonitors(ctx, []physicalMonitor{monitor})
}
func containingMonitor(monitors []physicalMonitor, rect image.Rectangle) (physicalMonitor, bool) {
	if rect.Empty() {
		return physicalMonitor{}, false
	}
	for _, monitor := range monitors {
		if rect.In(monitor.Rect) {
			return monitor, true
		}
	}
	return physicalMonitor{}, false
}
