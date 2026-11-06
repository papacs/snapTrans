//go:build !windows

package capture

import (
	"image"

	"github.com/kbinani/screenshot"
)

func ScreenRegion(rect image.Rectangle) (*image.RGBA, error) {
	return screenshot.CaptureRect(rect)
}
