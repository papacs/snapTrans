//go:build windows

package desktop

import (
	"context"
	"errors"
	"image"
	"os"
	"unsafe"

	"github.com/lxn/win"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"golang.org/x/sys/windows"
	"snaptrans/internal/capture"
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
	procSetWindowRgn     = user32.NewProc("SetWindowRgn")
)

// waitForWindowHidden waits for the compositor to apply WindowHide before
// capturing. This replaces a fixed 120 ms delay with at most one DWM frame.
func waitForWindowHidden() {
	_, _, _ = procDwmFlush.Call()
}

// WindowShow posts a native UI callback. ScreenGetAll posts behind it and
// waits for that queue, so later hide/focus operations cannot be overtaken.
func showWindowSynchronously(ctx context.Context, lifetime context.Context) bool {
	if lifetime.Err() != nil {
		return false
	}
	runtime.WindowShow(ctx)
	ready := make(chan struct{})
	go func() {
		_, _ = runtime.ScreenGetAll(ctx)
		close(ready)
	}()
	// Wails invokes OnShutdown after its native message loop has exited. Do
	// not hold the window transition lock waiting for an already stopped UI.
	select {
	case <-ready:
		return lifetime.Err() == nil
	case <-lifetime.Done():
		return false
	}
}

const (
	smCyScreen              = 1
	monitorDefaultToNearest = 2
	getAncestorRoot         = 2
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
	// The entire captured rectangle must expose the live desktop. An inset
	// leaves the overlay's border inside every frame and blocks edge scrollbars.
	return selection.Sub(windowBounds.Min), nil
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

func focusManualScrollTarget(target manualScrollTarget) {
	if target.window != 0 {
		// Wheel input must also work when Windows' inactive-window scrolling is
		// disabled. The overlay stays topmost, with a hole over the target.
		win.SetForegroundWindow(win.HWND(target.window))
	}
}

// captureManualScrollRegion reads the live pixels exposed by the native hole.
// PrintWindow requires the target application to render synchronously and can
// hang or return stale/blank GPU content; desktop capture avoids that dependency.
func captureManualScrollRegion(window uintptr, rect image.Rectangle) (image.Image, error) {
	var bounds win.RECT
	if window == 0 || !win.GetWindowRect(win.HWND(window), &bounds) {
		return nil, errors.New("scrolling capture window is unavailable")
	}
	if rect.Empty() {
		return nil, errors.New("scrolling capture region is empty")
	}
	return capture.ScreenRegion(rect)
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
