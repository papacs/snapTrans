//go:build windows

package main

import (
	"context"
	"unsafe"

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
)

// waitForWindowHidden waits for the compositor to apply WindowHide before
// capturing. This replaces a fixed 120 ms delay with at most one DWM frame.
func waitForWindowHidden() {
	_, _, _ = procDwmFlush.Call()
}

const (
	smCyScreen              = 1
	monitorDefaultToNearest = 2
)

type windowRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
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
