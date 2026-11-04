package desktop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/hotkeys"
)

func TestLoadConfigAppliesExternalRuntimeChangesWithoutSavingAgain(t *testing.T) {
	app := configTransactionApp(t)
	processing, cancel := app.newProcessingContext(app.currentCaptureEpoch())
	defer cancel()
	next := app.cfg
	next.SystemPrompt = "externally updated instructions"
	next.PersistentOCR = true
	next.RapidOCRPath = filepath.Join(t.TempDir(), "missing-test-worker.exe")
	next.RapidOCRTimeoutSeconds++
	require.NoError(t, app.configStore.Save(next))
	before, err := os.ReadFile(app.configStore.Path)
	require.NoError(t, err)
	backupBefore, err := os.ReadFile(app.configStore.Path + ".bak")
	require.NoError(t, err)
	loaded, err := app.LoadConfig()
	defer app.stopBackgroundWork()
	require.NoError(t, err)
	require.Equal(t, next, loaded)
	require.Equal(t, next, app.cfg)
	require.ErrorIs(t, processing.Err(), context.Canceled)
	require.NotNil(t, app.ocrWorker)
	defer app.ocrWorker.Close()
	require.Equal(t, workerSettings(next), app.ocrWorkerConfig)
	after, err := os.ReadFile(app.configStore.Path)
	require.NoError(t, err)
	backupAfter, err := os.ReadFile(app.configStore.Path + ".bak")
	require.NoError(t, err)
	require.Equal(t, before, after, "loading external settings must not rewrite them")
	require.Equal(t, backupBefore, backupAfter, "loading must not rotate the recovery copy")
}

func TestLoadConfigRebindsExternalShortcutChanges(t *testing.T) {
	app := shortcutTestApp(t)
	oldCapture, oldScreenshot := app.shortcut, app.screenshotShortcut
	next := app.cfg
	next.ShortcutKey, next.ScreenshotShortcutKey = next.ScreenshotShortcutKey, next.ShortcutKey
	require.NoError(t, app.configStore.Save(next))
	loaded, err := app.LoadConfig()
	require.NoError(t, err)
	require.Equal(t, next, loaded)
	require.NotSame(t, oldCapture, app.shortcut)
	require.NotSame(t, oldScreenshot, app.screenshotShortcut)
}

func TestLoadConfigRejectsExternalShortcutConflictWithoutChangingRuntime(t *testing.T) {
	app := shortcutTestApp(t)
	occupied, err := hotkeys.Register("Ctrl+Alt+Shift+F16", func() {})
	if err != nil {
		t.Skipf("isolated test shortcut unavailable: %v", err)
	}
	defer occupied.Unregister()
	previous, next := app.cfg, app.cfg
	next.ShortcutKey = "Ctrl+Alt+Shift+F16"
	require.NoError(t, app.configStore.Save(next))
	_, err = app.LoadConfig()
	require.ErrorContains(t, err, "unavailable")
	require.Equal(t, previous, app.cfg)
	onDisk, err := app.configStore.Load()
	require.NoError(t, err)
	require.Equal(t, next, onDisk, "a failed runtime reload must preserve the external edit")
}
