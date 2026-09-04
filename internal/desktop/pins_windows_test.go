//go:build windows

package desktop

import (
	"github.com/lxn/win"
	"github.com/stretchr/testify/require"
	"image"
	"image/color"
	"os"
	"testing"
	"time"
)

// Opt-in because this briefly creates a real topmost window on the desktop.
func TestNativePinSmoke(t *testing.T) {
	if os.Getenv("SNAPTRANS_TEST_NATIVE_PINS") != "1" {
		t.Skip("set SNAPTRANS_TEST_NATIVE_PINS=1 to test a real native pin")
	}
	frame := image.NewRGBA(image.Rect(0, 0, 320, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 320; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: 220, G: 40, B: 60, A: 255})
		}
	}
	require.NoError(t, showNativePin(frame, 80, 80))
	t.Cleanup(closeAllPins)
	var hwnd win.HWND
	pinWindows.Range(func(key, value any) bool { hwnd = key.(win.HWND); return false })
	require.NotZero(t, hwnd)
	require.True(t, win.IsWindowVisible(hwnd))
	win.UpdateWindow(hwnd)
	dc := win.GetDC(hwnd)
	defer win.ReleaseDC(hwnd, dc)
	require.Eventually(t, func() bool { return uint32(win.GetPixel(dc, 20, 20)) == uint32(220|40<<8|60<<16) }, time.Second, 10*time.Millisecond)
	var before win.RECT
	require.True(t, win.GetWindowRect(hwnd, &before))
	win.SendMessage(hwnd, win.WM_MOUSEWHEEL, uintptr(120<<16), 0)
	var after win.RECT
	require.True(t, win.GetWindowRect(hwnd, &after))
	require.Greater(t, after.Right-after.Left, before.Right-before.Left)
	win.SetWindowLong(hwnd, win.GWL_EXSTYLE, win.GetWindowLong(hwnd, win.GWL_EXSTYLE)|win.WS_EX_TRANSPARENT)
	restoreAllPins()
	require.Eventually(t, func() bool { return win.GetWindowLong(hwnd, win.GWL_EXSTYLE)&win.WS_EX_TRANSPARENT == 0 }, time.Second, 10*time.Millisecond)
	closeAllPins()
	require.Eventually(t, func() bool { result, _, _ := user32.NewProc("IsWindow").Call(uintptr(hwnd)); return result == 0 }, time.Second, 10*time.Millisecond)
}
