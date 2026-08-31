//go:build windows

package selection

import (
	"context"
	"encoding/json"
	ole "github.com/go-ole/go-ole"
	"github.com/lxn/win"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Only read the controlled, public fixture, never a user's browser document.
// Open internal/selection/testdata/browser-selection.html in a test browser,
// select #sample, then opt in with SNAPTRANS_BROWSER_SELECTION_TEST=1.
func TestBrowserSelectionIntegration(t *testing.T) {
	if os.Getenv("SNAPTRANS_BROWSER_SELECTION_TEST") != "1" {
		t.Skip("opt-in controlled browser integration")
	}
	var window uintptr
	windows.NewLazySystemDLL("user32.dll").NewProc("EnumWindows").Call(syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var title [256]uint16
		windows.NewLazySystemDLL("user32.dll").NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&title[0])), uintptr(len(title)))
		if strings.Contains(windows.UTF16ToString(title[:]), "snapTrans native selection fixture") {
			window = hwnd
			return 0
		}
		return 1
	}), 0)
	require.NotZero(t, window, "open the controlled fixture before testing")
	foregroundBefore := win.GetForegroundWindow()
	sequence := windows.NewLazySystemDLL("user32.dll").NewProc("GetClipboardSequenceNumber")
	before, _, _ := sequence.Call()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	automation, err := newAutomation()
	require.NoError(t, err)
	defer ole.CoUninitialize()
	defer automation.release()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	started := time.Now()
	root, err := automation.object(6, window)
	require.NoError(t, err)
	defer root.release()
	walker, err := automation.object(16)
	require.NoError(t, err)
	defer walker.release()
	var result Result
	visited := 0
	var readTree func(*comPtr, int) bool
	readTree = func(element *comPtr, depth int) bool {
		visited++
		if depth > 16 || visited > 600 || ctx.Err() != nil {
			return false
		}
		pattern, patternErr := element.object(16, 10014)
		if patternErr == nil {
			selected, readErr := readPattern(ctx, pattern)
			t.Logf("TextPattern at depth %d: %d lines, error %v", depth, len(selected.Lines), readErr)
			pattern.release()
			if readErr == nil {
				currentPID, _ := element.integer(20)
				var windowPID uint32
				win.GetWindowThreadProcessId(win.HWND(window), &windowPID)
				require.Equal(t, windowPID, uint32(currentPID), "selected document must belong to foreground browser process")
				result = selected
				return true
			}
		}
		child, childErr := walker.object(4, uintptr(unsafe.Pointer(element)))
		for childErr == nil {
			if readTree(child, depth+1) {
				child.release()
				return true
			}
			next, nextErr := walker.object(6, uintptr(unsafe.Pointer(child)))
			child.release()
			child, childErr = next, nextErr
		}
		return false
	}
	require.True(t, readTree(root, 0), "no selected text in controlled fixture (%d nodes)", visited)
	require.NoError(t, err)
	require.NotEmpty(t, result.Lines)
	if os.Getenv("SNAPTRANS_TEST_ACTIVATE_FIXTURE") == "1" {
		// Activate only our opt-in fixture briefly, restoring the preceding window.
		previous := win.GetForegroundWindow()
		require.True(t, win.SetForegroundWindow(win.HWND(window)), "could not activate fixture; no focus restrictions will be bypassed")
		defer func() {
			if win.GetForegroundWindow() == win.HWND(window) {
				win.SetForegroundWindow(previous)
			}
		}()
		require.Eventually(t, func() bool { return StillForeground(window) }, 500*time.Millisecond, 5*time.Millisecond)
		direct, directErr := readForeground(automation, ctx, window)
		require.NoError(t, directErr)
		require.Equal(t, result.Lines, direct.Lines)
		if win.GetForegroundWindow() == win.HWND(window) {
			win.SetForegroundWindow(previous)
			require.Eventually(t, func() bool { return win.GetForegroundWindow() == previous }, 500*time.Millisecond, 5*time.Millisecond)
		}
	}
	result.Window = window
	var text strings.Builder
	for _, line := range result.Lines {
		text.WriteString(line.Text)
		text.WriteByte('\n')
	}
	require.Contains(t, text.String(), "Long threads now load over 90% faster.")
	require.Contains(t, text.String(), "Decreased the memory footprint of long threads by over 90%.")
	after, _, _ := sequence.Call()
	require.Equal(t, before, after, "probing must not modify the clipboard")
	require.Equal(t, foregroundBefore, win.GetForegroundWindow(), "probing must not activate another window")
	t.Logf("read %d selected lines in %s", len(result.Lines), time.Since(started))
	var pid uint32
	win.GetWindowThreadProcessId(win.HWND(window), &pid)
	require.NotEqual(t, uint32(os.Getpid()), pid)
	// Optional evidence is only written beneath our fixture directory.
	data, err := json.MarshalIndent(result, "", "  ")
	require.NoError(t, err)
	evidenceDir := filepath.Join("..", "..", "output", "playwright")
	require.NoError(t, os.MkdirAll(evidenceDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(evidenceDir, "native-selection-result.json"), data, 0600))
}
