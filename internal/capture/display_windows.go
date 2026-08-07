//go:build windows

package capture

import (
	"image"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32 = windows.NewLazySystemDLL("user32.dll")
	shcore = windows.NewLazySystemDLL("Shcore.dll")

	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
)

const mdtEffectiveDpi = 0

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type monitorEntry struct {
	handle syscall.Handle
	rect   winRect
}

type winPoint struct {
	X int32
	Y int32
}

func cursorPosition() (image.Point, error) {
	var point winPoint
	ret, _, callErr := procGetCursorPos.Call(uintptr(unsafe.Pointer(&point)))
	if ret == 0 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return image.Point{}, callErr
		}
		return image.Point{}, syscall.Errno(windows.ERROR_INVALID_FUNCTION)
	}
	return image.Pt(int(point.X), int(point.Y)), nil
}

// physicalMonitors enumerates active displays with their physical pixel
// bounds (per the virtual desktop coordinate space) and their effective DPI
// scale factors. The enumeration order matches the order used to compose
// the union canvas, so per-display scales line up with the captured images.
func physicalMonitors() ([]physicalMonitor, error) {
	monitors, err := enumerateMonitors()
	if err != nil {
		return nil, err
	}

	result := make([]physicalMonitor, 0, len(monitors))
	for _, entry := range monitors {
		result = append(result, physicalMonitor{
			Rect: image.Rect(
				int(entry.rect.Left), int(entry.rect.Top),
				int(entry.rect.Right), int(entry.rect.Bottom),
			),
			Scale: dpiScaleForMonitor(entry.handle),
		})
	}
	return result, nil
}

func enumerateMonitors() ([]monitorEntry, error) {
	var monitors []monitorEntry
	callback := syscall.NewCallback(func(hmonitor syscall.Handle, _ syscall.Handle, lprc unsafe.Pointer, _ uintptr) uintptr {
		rect := *(*winRect)(lprc)
		monitors = append(monitors, monitorEntry{handle: hmonitor, rect: rect})
		return 1
	})

	ret, _, _ := procEnumDisplayMonitors.Call(0, 0, callback, 0)
	if ret == 0 {
		return nil, syscall.Errno(windows.ERROR_ACCESS_DENIED)
	}
	if len(monitors) == 0 {
		return nil, syscall.Errno(windows.ERROR_DEVICE_NOT_AVAILABLE)
	}
	return monitors, nil
}

// dpiScaleForMonitor returns the effective DPI scale (physical pixels per
// logical 96-DPI pixel) of the given monitor, falling back to 1.0.
func dpiScaleForMonitor(hmonitor syscall.Handle) float64 {
	var dpiX uint32
	var dpiY uint32
	ret, _, _ := procGetDpiForMonitor.Call(uintptr(hmonitor), mdtEffectiveDpi, uintptr(unsafe.Pointer(&dpiX)), uintptr(unsafe.Pointer(&dpiY)))
	if ret != 0 || dpiX == 0 {
		return 1
	}
	return float64(dpiX) / 96.0
}
