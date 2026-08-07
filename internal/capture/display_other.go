//go:build !windows

package capture

import (
	"errors"
	"image"

	"github.com/kbinani/screenshot"
)

func cursorPosition() (image.Point, error) {
	return image.Point{}, errors.New("cursor position is unavailable on this platform")
}

// physicalMonitors is the non-Windows fallback: it uses the screenshot
// package bounds with a scale factor of 1.0.
func physicalMonitors() ([]physicalMonitor, error) {
	count := screenshot.NumActiveDisplays()
	if count <= 0 {
		return nil, errors.New("no active displays found")
	}

	monitors := make([]physicalMonitor, 0, count)
	for i := 0; i < count; i++ {
		monitors = append(monitors, physicalMonitor{
			Rect:  screenshot.GetDisplayBounds(i),
			Scale: 1,
		})
	}
	return monitors, nil
}
