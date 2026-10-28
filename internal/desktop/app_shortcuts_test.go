package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
	"snaptrans/internal/hotkeys"
)

func shortcutTestApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(nil)
	app.cfg = config.Default()
	app.cfg.PersistentOCR = false
	app.cfg.ShortcutKey, app.cfg.ScreenshotShortcutKey, app.cfg.ReplyShortcutKey = "Ctrl+Alt+Shift+F17", "Ctrl+Alt+Shift+F18", "Ctrl+Alt+Shift+F19"
	app.configStore = config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"), "")
	for i, key := range []string{app.cfg.ShortcutKey, app.cfg.ScreenshotShortcutKey, app.cfg.ReplyShortcutKey} {
		registration, err := hotkeys.Register(key, func() {})
		if err != nil {
			t.Skipf("isolated test shortcut unavailable: %v", err)
		}
		switch i {
		case 0:
			app.shortcut = registration
		case 1:
			app.screenshotShortcut = registration
		case 2:
			app.replyShortcut = registration
		}
		t.Cleanup(func() { _ = registration.Unregister() })
	}
	t.Cleanup(func() {
		_ = app.shortcut.Unregister()
		_ = app.screenshotShortcut.Unregister()
		_ = app.replyShortcut.Unregister()
	})
	require.NoError(t, app.configStore.Save(app.cfg))
	return app
}

func TestSaveConfigCanSwapRegisteredShortcuts(t *testing.T) {
	app := shortcutTestApp(t)
	next := app.cfg
	next.ShortcutKey, next.ScreenshotShortcutKey = next.ScreenshotShortcutKey, next.ShortcutKey
	require.NoError(t, app.SaveConfig(next))
	require.Equal(t, next, app.cfg)
}

func TestSaveConfigRestoresShortcutsWhenDiskWriteFails(t *testing.T) {
	app := shortcutTestApp(t)
	previous := app.cfg
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0600))
	app.configStore.Path = filepath.Join(blocker, "config.json")
	next := previous
	next.ShortcutKey, next.ScreenshotShortcutKey = next.ScreenshotShortcutKey, next.ShortcutKey
	require.Error(t, app.SaveConfig(next))
	require.Equal(t, previous, app.cfg)
	registration, err := hotkeys.Register(previous.ShortcutKey, func() {})
	if registration != nil {
		_ = registration.Unregister()
	}
	require.Error(t, err, "original shortcut must still be owned after rollback")
	app.configStore.Path = filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, app.SaveConfig(next), "rollback must leave shortcuts usable for the next save")
}

func TestSaveConfigRestoresShortcutsWhenAnotherAppOwnsNewBinding(t *testing.T) {
	app := shortcutTestApp(t)
	occupied, err := hotkeys.Register("Ctrl+Alt+Shift+F16", func() {})
	if err != nil {
		t.Skipf("test conflict key unavailable: %v", err)
	}
	defer occupied.Unregister()
	previous := app.cfg
	next := previous
	next.ShortcutKey = "Ctrl+Alt+Shift+F16"
	require.ErrorContains(t, app.SaveConfig(next), "unavailable")
	require.Equal(t, previous, app.cfg)
	loaded, err := app.configStore.Load()
	require.NoError(t, err)
	require.Equal(t, previous, loaded)
	next = previous
	next.ShortcutKey, next.ScreenshotShortcutKey, next.ReplyShortcutKey = next.ReplyShortcutKey, next.ShortcutKey, next.ScreenshotShortcutKey
	require.NoError(t, app.SaveConfig(next), "three-way rotation must work after a registration failure")
}
