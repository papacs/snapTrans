//go:build windows

package selection

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/lxn/win"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Opt-in test uses our own non-activating native EDIT window, never user text.
// Run with SNAPTRANS_NATIVE_SELECTION_TEST=1 on an interactive Windows desktop.
func TestNativeSelectionIntegration(t *testing.T) {
	if os.Getenv("SNAPTRANS_NATIVE_SELECTION_TEST") != "1" {
		t.Skip("opt-in native UI integration")
	}
	type fixture struct {
		edit   win.HWND
		thread uint32
	}
	ready := make(chan fixture, 1)
	stop := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		class, _ := windows.UTF16PtrFromString("STATIC")
		editClass, _ := windows.UTF16PtrFromString("EDIT")
		text, _ := windows.UTF16PtrFromString("First selected line.\r\nSecond selected line.\r\n中文第三行")
		parent := win.CreateWindowEx(0x08000000, class, nil, win.WS_POPUP|win.WS_VISIBLE, 80, 80, 650, 160, 0, 0, 0, nil)
		edit := win.CreateWindowEx(0, editClass, text, win.WS_CHILD|win.WS_VISIBLE|win.ES_MULTILINE|win.ES_READONLY, 10, 10, 620, 140, parent, 0, 0, nil)
		win.SendMessage(edit, win.EM_SETSEL, 6, uintptr(^uint32(0)))
		ready <- fixture{edit: edit, thread: windows.GetCurrentThreadId()}
		var message win.MSG
		for win.GetMessage(&message, 0, 0, 0) > 0 {
			win.TranslateMessage(&message)
			win.DispatchMessage(&message)
		}
		win.DestroyWindow(parent)
		close(stop)
	}()
	target := <-ready
	require.NotZero(t, target.edit)
	defer func() {
		windows.NewLazySystemDLL("user32.dll").NewProc("PostThreadMessageW").Call(uintptr(target.thread), win.WM_QUIT, 0, 0)
		select {
		case <-stop:
		case <-time.After(time.Second):
			t.Error("fixture did not stop")
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	automation, err := newAutomation()
	require.NoError(t, err)
	defer ole.CoUninitialize()
	defer automation.release()
	element, err := automation.object(6, uintptr(target.edit))
	require.NoError(t, err)
	defer element.release()
	pattern, err := element.object(16, 10014)
	require.NoError(t, err)
	defer pattern.release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := readPattern(ctx, pattern)
	require.NoError(t, err)
	require.Len(t, result.Lines, 3)
	require.Equal(t, "selected line.", result.Lines[0].Text)
	require.Equal(t, "中文第三行", result.Lines[2].Text)
	require.Greater(t, result.Lines[1].Y, result.Lines[0].Y)
	require.Greater(t, result.Lines[0].X, result.Lines[1].X)
	win.SendMessage(target.edit, win.EM_SETSEL, 0, 0)
	_, err = readPattern(ctx, pattern)
	require.ErrorIs(t, err, ErrNoSelection)
}
