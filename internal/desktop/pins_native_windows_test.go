//go:build windows

package desktop

import (
	"runtime"
	"testing"
	"unsafe"

	"github.com/lxn/win"
	"github.com/stretchr/testify/require"
)

func TestNativeDPIRectPreservesNegativeMonitorCoordinates(t *testing.T) {
	suggested := win.RECT{Left: -1920, Top: -100, Right: -960, Bottom: 620}
	rect, ok := readPinSuggestedRect(uintptr(unsafe.Pointer(&suggested)))
	runtime.KeepAlive(&suggested)
	require.True(t, ok)
	require.Equal(t, suggested, rect)
	_, ok = readPinSuggestedRect(0)
	require.False(t, ok)
	_, ok = readPinSuggestedRect(1)
	require.False(t, ok)
}
