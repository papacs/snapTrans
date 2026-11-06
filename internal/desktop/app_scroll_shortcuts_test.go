package desktop

import (
	"testing"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/capture"
)

func TestScrollCommandsOnlyApplyToActiveSession(t *testing.T) {
	app := NewApp(nil)
	app.captureEpoch = 3
	require.False(t, app.scrollCommandCurrentLocked(3))
	app.scrollCapture = &capture.ManualScrollCapture{}
	require.True(t, app.scrollCommandCurrentLocked(3))
	require.False(t, app.scrollCommandCurrentLocked(2))
	app.invalidateCaptureRequest()
	require.False(t, app.scrollCommandCurrentLocked(3))
	app.captureEpoch = 3
	app.shuttingDown = true
	require.False(t, app.scrollCommandCurrentLocked(3))
}
