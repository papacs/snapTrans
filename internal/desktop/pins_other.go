//go:build !windows

package desktop

import (
	"errors"
	"image"
)

func showNativePin(image.Image, int, int) error {
	return errors.New("desktop pins are only available on Windows")
}
func closeAllPins()   {}
func restoreAllPins() {}
