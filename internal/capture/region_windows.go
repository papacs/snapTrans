//go:build windows

package capture

import (
	"errors"
	"image"
	"runtime"
	"unsafe"

	"github.com/lxn/win"
)

// ScreenRegion captures physical desktop pixels directly into a top-down DIB.
// Unlike GetDIBits on a selected compatible bitmap, this has no intermediate
// device-dependent bitmap/readback and works with negative monitor origins.
func ScreenRegion(rect image.Rectangle) (*image.RGBA, error) {
	width, height := rect.Dx(), rect.Dy()
	if rect.Empty() || width > 32767 || height > 32767 || int64(width)*int64(height) > 100_000_000 {
		return nil, errors.New("invalid desktop capture bounds")
	}
	// GetDC/ReleaseDC and GdiFlush operate on the calling native thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	desktop := win.GetDC(0)
	if desktop == 0 {
		return nil, errors.New("could not open desktop device context")
	}
	defer win.ReleaseDC(0, desktop)
	memory := win.CreateCompatibleDC(desktop)
	if memory == 0 {
		return nil, errors.New("could not create desktop capture device context")
	}
	defer win.DeleteDC(memory)
	header := win.BITMAPINFOHEADER{
		BiSize:  uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
		BiWidth: int32(width), BiHeight: -int32(height),
		BiPlanes: 1, BiBitCount: 32, BiCompression: win.BI_RGB,
	}
	var pixels unsafe.Pointer
	bitmap := win.CreateDIBSection(desktop, &header, win.DIB_RGB_COLORS, &pixels, 0, 0)
	if bitmap == 0 || pixels == nil {
		return nil, errors.New("could not create desktop capture bitmap")
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))
	previous := win.SelectObject(memory, win.HGDIOBJ(bitmap))
	if previous == 0 {
		return nil, errors.New("could not select desktop capture bitmap")
	}
	defer win.SelectObject(memory, previous)
	if !win.BitBlt(memory, 0, 0, int32(width), int32(height), desktop, int32(rect.Min.X), int32(rect.Min.Y), win.SRCCOPY) {
		return nil, errors.New("could not copy desktop pixels")
	}
	if !win.GdiFlush() {
		return nil, errors.New("could not finish desktop pixel copy")
	}
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	copyDesktopBGRA(frame.Pix, unsafe.Slice((*byte)(pixels), len(frame.Pix)))
	return frame, nil
}

func copyDesktopBGRA(destination, source []byte) {
	for i := 0; i+3 < len(destination); i += 4 {
		destination[i], destination[i+1], destination[i+2], destination[i+3] = source[i+2], source[i+1], source[i], 255
	}
}
