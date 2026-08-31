//go:build windows

package main

import (
	"errors"
	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"image"
	"runtime"
	"sync"
	"unsafe"
)

var pinWindows sync.Map
var pinClassOnce sync.Once
var pinClassError error
var pinClassName = mustPinUTF16("snapTrans.PinnedImage")
var pinClassCallback = windows.NewCallback(pinWindowProc)
var pinSlots = make(chan struct{}, 12)
var pinSetAlpha = user32.NewProc("SetLayeredWindowAttributes")
var pinAppendMenu = user32.NewProc("AppendMenuW")
var pinStretchDIBits = windows.NewLazySystemDLL("gdi32.dll").NewProc("StretchDIBits")

const pinUnlockMessage = win.WM_APP + 80

type nativePin struct {
	pixels        []byte
	width, height int
	alpha         int
}

func mustPinUTF16(text string) *uint16 { p, _ := windows.UTF16PtrFromString(text); return p }

func showNativePin(img image.Image, x, y int) error {
	select {
	case pinSlots <- struct{}{}:
	default:
		return errors.New("close a pin first (maximum 12)")
	}
	ready := make(chan error, 1)
	go func() {
		defer func() { <-pinSlots }()
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		// Physical virtual-desktop coordinates must not be DPI-virtualized.
		dpi := user32.NewProc("SetThreadDpiAwarenessContext")
		if dpi.Find() == nil {
			previous, _, _ := dpi.Call(^uintptr(3))
			defer dpi.Call(previous)
		}
		pinClassOnce.Do(func() {
			wc := win.WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(win.WNDCLASSEX{})), LpfnWndProc: pinClassCallback, LpszClassName: pinClassName}
			if win.RegisterClassEx(&wc) == 0 {
				pinClassError = errors.New("could not register pin window")
			}
		})
		if pinClassError != nil {
			ready <- pinClassError
			return
		}
		b := img.Bounds()
		p := &nativePin{width: b.Dx(), height: b.Dy(), alpha: 255, pixels: make([]byte, b.Dx()*b.Dy()*4)}
		for yy := 0; yy < p.height; yy++ {
			for xx := 0; xx < p.width; xx++ {
				r, g, b, a := img.At(xx+img.Bounds().Min.X, yy+img.Bounds().Min.Y).RGBA()
				i := (yy*p.width + xx) * 4
				// Composite transparent pixels onto white before native opaque rendering.
				p.pixels[i] = byte((b + 65535 - a) >> 8)
				p.pixels[i+1] = byte((g + 65535 - a) >> 8)
				p.pixels[i+2] = byte((r + 65535 - a) >> 8)
				p.pixels[i+3] = 255
			}
		}
		hwnd := win.CreateWindowEx(win.WS_EX_TOPMOST|win.WS_EX_TOOLWINDOW|win.WS_EX_LAYERED, pinClassName, mustPinUTF16("snapTrans · 贴钉 / Pin"), win.WS_POPUP|win.WS_BORDER, int32(x), int32(y), int32(min(p.width, 900)), int32(min(p.height, 700)), 0, 0, 0, nil)
		if hwnd == 0 {
			ready <- errors.New("could not create pin window")
			return
		}
		pinWindows.Store(hwnd, p)
		pinSetAlpha.Call(uintptr(hwnd), 0, 255, 2)
		resizePin(hwnd, x, y, p.width, p.height)
		win.ShowWindow(hwnd, win.SW_SHOWNOACTIVATE)
		ready <- nil
		var msg win.MSG
		for win.GetMessage(&msg, 0, 0, 0) > 0 {
			win.TranslateMessage(&msg)
			win.DispatchMessage(&msg)
		}
	}()
	return <-ready
}
func resizePin(hwnd win.HWND, x, y, w, h int) {
	monitor := win.MonitorFromWindow(hwnd, win.MONITOR_DEFAULTTONEAREST)
	info := win.MONITORINFO{CbSize: uint32(unsafe.Sizeof(win.MONITORINFO{}))}
	if win.GetMonitorInfo(monitor, &info) {
		rect := fitPinRect(x, y, w, h, image.Rect(int(info.RcWork.Left), int(info.RcWork.Top), int(info.RcWork.Right), int(info.RcWork.Bottom)))
		x, y, w, h = rect.Min.X, rect.Min.Y, rect.Dx(), rect.Dy()
	}
	win.SetWindowPos(hwnd, win.HWND_TOPMOST, int32(x), int32(y), int32(max(1, w)), int32(max(1, h)), win.SWP_NOACTIVATE)
	win.InvalidateRect(hwnd, nil, false)
}
func pinWindowProc(hwnd win.HWND, msg uint32, wParam, lParam uintptr) uintptr {
	value, ok := pinWindows.Load(hwnd)
	if !ok {
		return win.DefWindowProc(hwnd, msg, wParam, lParam)
	}
	p := value.(*nativePin)
	switch msg {
	case win.WM_PAINT:
		var ps win.PAINTSTRUCT
		dc := win.BeginPaint(hwnd, &ps)
		defer win.EndPaint(hwnd, &ps)
		var rect win.RECT
		win.GetClientRect(hwnd, &rect)
		info := win.BITMAPINFO{BmiHeader: win.BITMAPINFOHEADER{BiSize: uint32(unsafe.Sizeof(win.BITMAPINFOHEADER{})), BiWidth: int32(p.width), BiHeight: -int32(p.height), BiPlanes: 1, BiBitCount: 32, BiCompression: win.BI_RGB}}
		pinStretchDIBits.Call(uintptr(dc), 0, 0, uintptr(rect.Right), uintptr(rect.Bottom), 0, 0, uintptr(p.width), uintptr(p.height), uintptr(unsafe.Pointer(&p.pixels[0])), uintptr(unsafe.Pointer(&info)), 0, win.SRCCOPY)
		runtime.KeepAlive(p)
		return 0
	case win.WM_LBUTTONDOWN:
		win.ReleaseCapture()
		return win.SendMessage(hwnd, win.WM_NCLBUTTONDOWN, win.HTCAPTION, 0)
	case win.WM_MOUSEWHEEL:
		delta := int(int16(wParam >> 16))
		if delta == 0 {
			return 0
		}
		if wParam&win.MK_CONTROL != 0 {
			change := 16
			if delta < 0 {
				change = -16
			}
			p.alpha = max(64, min(255, p.alpha+change))
			pinSetAlpha.Call(uintptr(hwnd), 0, uintptr(p.alpha), 2)
		} else {
			var r win.RECT
			win.GetWindowRect(hwnd, &r)
			factor := 1.1
			if delta < 0 {
				factor = 1 / 1.1
			}
			w := max(40, int(float64(r.Right-r.Left)*factor))
			h := max(24, int(float64(w)*float64(p.height)/float64(p.width)))
			resizePin(hwnd, int(r.Left), int(r.Top), w, h)
		}
		return 0
	case win.WM_RBUTTONUP:
		menu := win.CreatePopupMenu()
		defer win.DestroyMenu(menu)
		labels := []string{"鼠标穿透 / Click-through (托盘恢复)", "恢复原尺寸 / Original size", "关闭贴钉 / Close", "拖动移动 · 滚轮缩放 · Ctrl+滚轮透明度"}
		for i, label := range labels {
			flags := uintptr(0)
			if i == 3 {
				flags = win.MF_GRAYED
			}
			pinAppendMenu.Call(uintptr(menu), flags, uintptr(i+1), uintptr(unsafe.Pointer(mustPinUTF16(label))))
		}
		var point win.POINT
		win.GetCursorPos(&point)
		win.SetForegroundWindow(hwnd)
		choice := win.TrackPopupMenu(menu, win.TPM_RETURNCMD|win.TPM_RIGHTBUTTON, point.X, point.Y, 0, hwnd, nil)
		switch choice {
		case 1:
			win.SetWindowLong(hwnd, win.GWL_EXSTYLE, win.GetWindowLong(hwnd, win.GWL_EXSTYLE)|win.WS_EX_TRANSPARENT)
		case 2:
			var r win.RECT
			win.GetWindowRect(hwnd, &r)
			resizePin(hwnd, int(r.Left), int(r.Top), p.width, p.height)
		case 3:
			win.DestroyWindow(hwnd)
		}
		return 0
	case pinUnlockMessage:
		win.SetWindowLong(hwnd, win.GWL_EXSTYLE, win.GetWindowLong(hwnd, win.GWL_EXSTYLE)&^win.WS_EX_TRANSPARENT)
		p.alpha = 255
		pinSetAlpha.Call(uintptr(hwnd), 0, 255, 2)
		var r win.RECT
		win.GetWindowRect(hwnd, &r)
		resizePin(hwnd, int(r.Left), int(r.Top), int(r.Right-r.Left), int(r.Bottom-r.Top))
		return 0
	case 0x02E0: // WM_DPICHANGED: use the monitor's suggested physical bounds.
		r := (*win.RECT)(unsafe.Pointer(lParam))
		resizePin(hwnd, int(r.Left), int(r.Top), int(r.Right-r.Left), int(r.Bottom-r.Top))
		return 0
	case win.WM_KEYDOWN:
		if wParam == win.VK_ESCAPE {
			win.DestroyWindow(hwnd)
			return 0
		}
	case win.WM_CLOSE:
		win.DestroyWindow(hwnd)
		return 0
	case win.WM_DESTROY:
		pinWindows.Delete(hwnd)
		win.PostQuitMessage(0)
		return 0
	}
	return win.DefWindowProc(hwnd, msg, wParam, lParam)
}
func closeAllPins() {
	pinWindows.Range(func(key, value any) bool { win.PostMessage(key.(win.HWND), win.WM_CLOSE, 0, 0); return true })
}
func restoreAllPins() {
	pinWindows.Range(func(key, value any) bool { win.PostMessage(key.(win.HWND), pinUnlockMessage, 0, 0); return true })
}
