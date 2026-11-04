package desktop

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
	"snaptrans/internal/history"
	"snaptrans/internal/hotkeys"
)

func configTransactionApp(t *testing.T) *App {
	t.Helper()
	app := NewApp(nil)
	app.cfg = config.Default()
	app.cfg.PersistentOCR = false
	app.configStore = config.NewStoreAt(filepath.Join(t.TempDir(), "config.json"), "")
	require.NoError(t, app.configStore.Save(app.cfg))
	t.Cleanup(app.cancelTranslationWarmups)
	return app
}

func TestConfigCommitInvalidatesOldWorkBeforePublishingNewSettings(t *testing.T) {
	app := configTransactionApp(t)
	old, cancelOld := app.newProcessingContext(0)
	defer cancelOld()
	action, cancelAction, _ := app.newTextActionContext("old", 0, time.Minute)
	defer cancelAction()
	extraction, cancelExtraction := app.newExtractionContext(0)
	defer cancelExtraction()
	_, warmer := app.translationWarmSnapshot()
	previous, next := app.cfg, app.cfg
	next.SystemPrompt = "new instructions"
	require.NoError(t, app.saveConfigAndShortcuts(previous, next))
	require.Equal(t, next, app.cfg)
	require.ErrorIs(t, old.Err(), context.Canceled)
	require.ErrorIs(t, action.Err(), context.Canceled)
	require.ErrorIs(t, extraction.Err(), context.Canceled)
	require.ErrorIs(t, warmer.Err(), context.Canceled)
	current, cancelCurrent := app.newProcessingContext(app.currentCaptureEpoch())
	defer cancelCurrent()
	// Repeated saves must not treat work using the committed settings as old.
	require.NoError(t, app.SaveConfig(next))
	require.NoError(t, current.Err())
}

func TestConfigLoadWaitsForSaveTransactionBeforeReadingDisk(t *testing.T) {
	app := configTransactionApp(t)
	app.configMu.Lock()
	locked := true
	defer func() {
		if locked {
			app.configMu.Unlock()
		}
	}()
	started, finished := make(chan struct{}), make(chan config.Config, 1)
	go func() {
		close(started)
		cfg, err := app.LoadConfig()
		if err != nil {
			finished <- config.Config{}
			return
		}
		finished <- cfg
	}()
	<-started
	require.Never(t, func() bool { return len(finished) != 0 }, 100*time.Millisecond, 5*time.Millisecond)
	next := app.cfg
	next.Model = "committed-model"
	require.NoError(t, app.configStore.Save(next))
	app.configMu.Unlock()
	locked = false
	select {
	case loaded := <-finished:
		require.Equal(t, next, loaded)
		require.Equal(t, next, app.cfg)
	case <-time.After(5 * time.Second):
		t.Fatal("configuration load did not resume after save")
	}
}

func TestShutdownWaitsForConfigResourceUpdates(t *testing.T) {
	app := configTransactionApp(t)
	processing, cancel := app.newProcessingContext(0)
	defer cancel()
	app.configMu.Lock()
	locked := true
	defer func() {
		if locked {
			app.configMu.Unlock()
		}
	}()
	started, finished := make(chan struct{}), make(chan struct{})
	go func() { close(started); app.stopBackgroundWork(); close(finished) }()
	<-started
	require.Never(t, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 5*time.Millisecond)
	require.NoError(t, processing.Err(), "shutdown must not pass a pending save's resource updates")
	app.configMu.Unlock()
	locked = false
	select {
	case <-finished:
		require.ErrorIs(t, processing.Err(), context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not resume after save")
	}
}

func TestQueuedHistoryAppendHonorsCommittedPrivacyPreference(t *testing.T) {
	app := configTransactionApp(t)
	app.historyStore = history.NewStore(filepath.Join(t.TempDir(), "history.json"), 50)
	app.configMu.Lock()
	locked := true
	defer func() {
		if locked {
			app.configMu.Unlock()
		}
	}()
	started, finished := make(chan struct{}), make(chan struct{})
	go func() { close(started); app.saveHistory("private", "translation", "to-zh"); close(finished) }()
	<-started
	require.Never(t, func() bool {
		select {
		case <-finished:
			return true
		default:
			return false
		}
	}, 100*time.Millisecond, 5*time.Millisecond)
	app.mu.Lock()
	app.cfg.RecordHistory = false
	app.mu.Unlock()
	app.configMu.Unlock()
	locked = false
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("history append did not resume after settings save")
	}
	entries, err := app.historyStore.List()
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestSaveRetriesMissingUnchangedShortcutAndRollsBackNewHandle(t *testing.T) {
	app := shortcutTestApp(t)
	app.ctx = context.Background()
	require.NoError(t, app.shortcut.Unregister())
	app.shortcut = nil
	blocker := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(blocker, []byte("not a directory"), 0600))
	app.configStore.Path = filepath.Join(blocker, "config.json")
	require.Error(t, app.SaveConfig(app.cfg))
	require.Nil(t, app.shortcut, "failed save must restore the original missing-handle state")
	available, err := hotkeys.Register(app.cfg.ShortcutKey, func() {})
	require.NoError(t, err, "rollback must release the newly recovered binding")
	require.NoError(t, available.Unregister())
	app.configStore.Path = filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, app.SaveConfig(app.cfg))
	require.NotNil(t, app.shortcut, "saving the same key should retry an earlier failed registration")
}

func TestSettingsWaitForShutdownTransactionBeforeTouchingAutostart(t *testing.T) {
	app := configTransactionApp(t)
	app.configMu.Lock()
	locked := true
	defer func() {
		if locked {
			app.configMu.Unlock()
		}
	}()
	var calls atomic.Int32
	started, finished := make(chan struct{}), make(chan error, 1)
	go func() {
		close(started)
		finished <- app.saveSettings(app.cfg, true, func() (bool, error) {
			calls.Add(1)
			return false, nil
		}, func(bool) error { calls.Add(1); return nil })
	}()
	<-started
	require.Never(t, func() bool { return calls.Load() != 0 || len(finished) != 0 }, 100*time.Millisecond, 5*time.Millisecond)
	app.mu.Lock()
	app.shuttingDown = true
	app.mu.Unlock()
	app.configMu.Unlock()
	locked = false
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("settings save did not observe shutdown")
	}
	require.Zero(t, calls.Load(), "a canceled settings transaction must not read or change OS autostart")
}
