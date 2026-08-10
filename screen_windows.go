//go:build windows

package main

import (
	"context"
	"errors"
	"image"
	"os"
	"unsafe"

	"github.com/lxn/win"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	dwmapi = windows.NewLazySystemDLL("dwmapi.dll")

	procGetSystemMetrics = user32.NewProc("GetSystemMetrics")
	procMonitorFromRect  = user32.NewProc("MonitorFromRect")
	procGetMonitorInfoW  = user32.NewProc("GetMonitorInfoW")
	procDwmFlush         = dwmapi.NewProc("DwmFlush")
	procWindowFromPoint  = user32.NewProc("WindowFromPoint")
	procGetAncestor      = user32.NewProc("GetAncestor")
	procPrintWindow      = user32.NewProc("PrintWindow")
	procGetWindowDC      = user32.NewProc("GetWindowDC")
	procSetWindowRgn     = user32.NewProc("SetWindowRgn")
)

// waitForWindowHidden waits for the compositor to apply WindowHide before
// capturing. This replaces a fixed 120 ms delay with at most one DWM frame.
func waitForWindowHidden() {
	_, _, _ = procDwmFlush.Call()
}

const (
	smCyScreen              = 1
	monitorDefaultToNearest = 2
	getAncestorRoot         = 2
	pwRenderFullContent     = 2
)

type cursorPoint struct {
	X int32
	Y int32
}

type windowRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type manualScrollTarget struct {
	window uintptr
}

func manualScrollOverlayWindow() (uintptr, error) {
	overlay := win.GetForegroundWindow()
	if overlay == 0 {
		return 0, errors.New("could not find the scrolling capture overlay window")
	}
	var processID uint32
	win.GetWindowThreadProcessId(overlay, &processID)
	if processID != uint32(os.Getpid()) {
		return 0, errors.New("the scrolling capture overlay is not the foreground window")
	}
	return uintptr(overlay), nil
}

func manualScrollHoleRect(windowBounds image.Rectangle, selection image.Rectangle) (image.Rectangle, error) {
	if windowBounds.Empty() || selection.Empty() || !selection.In(windowBounds) {
		return image.Rectangle{}, errors.New("scrolling capture region must stay within the overlay window")
	}
	relative := selection.Sub(windowBounds.Min)
	hole := image.Rect(relative.Min.X+2, relative.Min.Y+2, relative.Max.X-2, relative.Max.Y-2)
	if hole.Empty() {
		return image.Rectangle{}, errors.New("scrolling capture region is too small for an input hole")
	}
	return hole, nil
}

func applyManualScrollHole(overlay uintptr, selection image.Rectangle) error {
	if overlay == 0 {
		return errors.New("scrolling capture overlay is unavailable")
	}
	var nativeBounds win.RECT
	if !win.GetWindowRect(win.HWND(overlay), &nativeBounds) {
		return errors.New("could not read the scrolling capture overlay bounds")
	}
	windowBounds := image.Rect(
		int(nativeBounds.Left),
		int(nativeBounds.Top),
		int(nativeBounds.Right),
		int(nativeBounds.Bottom),
	)
	hole, err := manualScrollHoleRect(windowBounds, selection)
	if err != nil {
		return err
	}

	fullRegion := win.CreateRectRgn(0, 0, int32(windowBounds.Dx()), int32(windowBounds.Dy()))
	if fullRegion == 0 {
		return errors.New("could not create the scrolling capture overlay region")
	}
	holeRegion := win.CreateRectRgn(int32(hole.Min.X), int32(hole.Min.Y), int32(hole.Max.X), int32(hole.Max.Y))
	if holeRegion == 0 {
		win.DeleteObject(win.HGDIOBJ(fullRegion))
		return errors.New("could not create the scrolling capture input region")
	}
	defer win.DeleteObject(win.HGDIOBJ(holeRegion))

	if win.CombineRgn(fullRegion, fullRegion, holeRegion, win.RGN_DIFF) == 0 {
		win.DeleteObject(win.HGDIOBJ(fullRegion))
		return errors.New("could not cut the scrolling capture input region")
	}
	if result, _, callErr := procSetWindowRgn.Call(overlay, uintptr(fullRegion), 1); result == 0 {
		win.DeleteObject(win.HGDIOBJ(fullRegion))
		return errors.New("could not apply the scrolling capture input region: " + callErr.Error())
	}
	return nil
}

func restoreManualScrollOverlay(overlay uintptr) {
	if overlay != 0 {
		_, _, _ = procSetWindowRgn.Call(overlay, 0, 1)
	}
}

// findManualScrollTarget runs while the overlay is hidden and remembers the
// underlying control at the centre of the selected region.
func findManualScrollTarget(rect image.Rectangle) (manualScrollTarget, error) {
	point := cursorPoint{
		X: int32(rect.Min.X + rect.Dx()/2),
		Y: int32(rect.Min.Y + rect.Dy()/2),
	}
	pointValue := *(*uintptr)(unsafe.Pointer(&point))
	target, _, _ := procWindowFromPoint.Call(pointValue)
	if target == 0 {
		return manualScrollTarget{}, errors.New("could not find the window beneath the scrolling capture region")
	}
	root, _, _ := procGetAncestor.Call(target, getAncestorRoot)
	if root == 0 {
		root = target
	}
	return manualScrollTarget{window: root}, nil
}

// captureManualScrollRegion renders the underlying top-level window into an
// off-screen bitmap. It does not read the protected Wails overlay from the
// composed desktop, so the live frame remains available without hide/show.
func captureManualScrollRegion(window uintptr, rect image.Rectangle) (image.Image, error) {
	if window == 0 {
		return nil, errors.New("scrolling capture window is unavailable")
	}

	var nativeBounds win.RECT
	if !win.GetWindowRect(win.HWND(window), &nativeBounds) {
		return nil, errors.New("could not read the scrolling capture window bounds")
	}
	windowBounds := image.Rect(
		int(nativeBounds.Left),
		int(nativeBounds.Top),
		int(nativeBounds.Right),
		int(nativeBounds.Bottom),
	)
	if windowBounds.Empty() || !rect.In(windowBounds) {
		return nil, errors.New("scrolling capture region must stay within one window")
	}

	windowDCValue, _, _ := procGetWindowDC.Call(window)
	if windowDCValue == 0 {
		return nil, errors.New("could not open the scrolling capture window")
	}
	windowDC := win.HDC(windowDCValue)
	defer win.ReleaseDC(win.HWND(window), windowDC)

	memoryDC := win.CreateCompatibleDC(windowDC)
	if memoryDC == 0 {
		return nil, errors.New("could not create the scrolling capture device context")
	}
	defer win.DeleteDC(memoryDC)

	width := windowBounds.Dx()
	height := windowBounds.Dy()
	bitmap := win.CreateCompatibleBitmap(windowDC, int32(width), int32(height))
	if bitmap == 0 {
		return nil, errors.New("could not create the scrolling capture bitmap")
	}
	defer win.DeleteObject(win.HGDIOBJ(bitmap))

	previous := win.SelectObject(memoryDC, win.HGDIOBJ(bitmap))
	if previous == 0 {
		return nil, errors.New("could not select the scrolling capture bitmap")
	}
	defer win.SelectObject(memoryDC, previous)

	if rendered, _, _ := procPrintWindow.Call(window, uintptr(memoryDC), pwRenderFullContent); rendered != 0 {
		data, err := readWindowBGRA(windowDC, bitmap, width, height)
		if err == nil {
			frame, cropErr := cropWindowBGRA(data, width, height, windowBounds, rect)
			if cropErr == nil && !manualFrameIsBlank(frame) {
				return frame, nil
			}
		}
	}

	// Some GPU-backed windows do not implement PrintWindow. Reading their
	// window DC is the fallback and still avoids the protected desktop bitmap.
	if !win.BitBlt(memoryDC, 0, 0, int32(width), int32(height), windowDC, 0, 0, win.SRCCOPY) {
		return nil, errors.New("could not render the scrolling capture window")
	}
	data, err := readWindowBGRA(windowDC, bitmap, width, height)
	if err != nil {
		return nil, err
	}
	frame, err := cropWindowBGRA(data, width, height, windowBounds, rect)
	if err != nil {
		return nil, err
	}
	if manualFrameIsBlank(frame) {
		return nil, errors.New("the selected window returned a blank scrolling frame")
	}
	return frame, nil
}

func readWindowBGRA(hdc win.HDC, bitmap win.HBITMAP, width int, height int) ([]byte, error) {
	dataSize := int64(width) * int64(height) * 4
	if width <= 0 || height <= 0 || dataSize <= 0 || dataSize > int64(^uint(0)>>1) {
		return nil, errors.New("invalid scrolling capture bitmap size")
	}

	memory := win.GlobalAlloc(win.GMEM_MOVEABLE, uintptr(dataSize))
	if memory == 0 {
		return nil, errors.New("could not allocate scrolling capture pixels")
	}
	defer win.GlobalFree(memory)
	memoryPointer := win.GlobalLock(memory)
	if memoryPointer == nil {
		return nil, errors.New("could not lock scrolling capture pixels")
	}
	defer win.GlobalUnlock(memory)

	header := win.BITMAPINFOHEADER{
		BiSize:        uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})),
		BiWidth:       int32(width),
		BiHeight:      int32(-height),
		BiPlanes:      1,
		BiBitCount:    32,
		BiCompression: win.BI_RGB,
	}
	if win.GetDIBits(
		hdc,
		bitmap,
		0,
		uint32(height),
		(*byte)(memoryPointer),
		(*win.BITMAPINFO)(unsafe.Pointer(&header)),
		win.DIB_RGB_COLORS,
	) == 0 {
		return nil, errors.New("could not read scrolling capture pixels")
	}

	source := unsafe.Slice((*byte)(memoryPointer), int(dataSize))
	data := make([]byte, len(source))
	copy(data, source)
	return data, nil
}

func cropWindowBGRA(
	data []byte,
	windowWidth int,
	windowHeight int,
	windowBounds image.Rectangle,
	rect image.Rectangle,
) (*image.RGBA, error) {
	expectedSize := int64(windowWidth) * int64(windowHeight) * 4
	if windowWidth <= 0 || windowHeight <= 0 || expectedSize != int64(len(data)) ||
		windowBounds.Dx() != windowWidth || windowBounds.Dy() != windowHeight || !rect.In(windowBounds) {
		return nil, errors.New("invalid scrolling capture window pixels")
	}

	frame := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	sourceX := rect.Min.X - windowBounds.Min.X
	sourceY := rect.Min.Y - windowBounds.Min.Y
	for y := 0; y < rect.Dy(); y++ {
		sourceOffset := ((sourceY+y)*windowWidth + sourceX) * 4
		targetOffset := y * frame.Stride
		for x := 0; x < rect.Dx(); x++ {
			frame.Pix[targetOffset] = data[sourceOffset+2]
			frame.Pix[targetOffset+1] = data[sourceOffset+1]
			frame.Pix[targetOffset+2] = data[sourceOffset]
			frame.Pix[targetOffset+3] = 255
			sourceOffset += 4
			targetOffset += 4
		}
	}
	return frame, nil
}

func manualFrameIsBlank(frame *image.RGBA) bool {
	if frame == nil || frame.Bounds().Empty() {
		return true
	}
	stepX := frame.Bounds().Dx() / 32
	stepY := frame.Bounds().Dy() / 24
	if stepX < 1 {
		stepX = 1
	}
	if stepY < 1 {
		stepY = 1
	}
	for y := 0; y < frame.Bounds().Dy(); y += stepY {
		for x := 0; x < frame.Bounds().Dx(); x += stepX {
			pixel := frame.RGBAAt(x, y)
			if pixel.R > 2 || pixel.G > 2 || pixel.B > 2 {
				return false
			}
		}
	}
	return true
}

type windowMonitorInfo struct {
	Size    uint32
	Monitor windowRect
	Work    windowRect
	Flags   uint32
}

// availableScreenHeight returns the primary screen's height in logical
// pixels, or 0 when it cannot be queried.
func availableScreenHeight() int {
	value, _, _ := procGetSystemMetrics.Call(smCyScreen)
	if value == 0 {
		return 0
	}
	return int(value)
}

// moveWindowToDisplay compensates for Wails v2 WindowSetPosition being
// relative to the work area of the monitor the window currently occupies.
// targetX and targetY are physical virtual-desktop coordinates from the
// screenshot monitor bounds.
func moveWindowToDisplay(ctx context.Context, targetX int, targetY int) {
	currentX, currentY := runtime.WindowGetPosition(ctx)
	currentRect := windowRect{
		Left:   int32(currentX),
		Top:    int32(currentY),
		Right:  int32(currentX + 1),
		Bottom: int32(currentY + 1),
	}
	hMonitor, _, _ := procMonitorFromRect.Call(
		uintptr(unsafe.Pointer(&currentRect)),
		monitorDefaultToNearest,
	)
	if hMonitor == 0 {
		runtime.WindowSetPosition(ctx, targetX, targetY)
		return
	}

	info := windowMonitorInfo{Size: uint32(unsafe.Sizeof(windowMonitorInfo{}))}
	ret, _, _ := procGetMonitorInfoW.Call(hMonitor, uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		runtime.WindowSetPosition(ctx, targetX, targetY)
		return
	}

	runtime.WindowSetPosition(
		ctx,
		targetX-int(info.Work.Left),
		targetY-int(info.Work.Top),
	)
}
