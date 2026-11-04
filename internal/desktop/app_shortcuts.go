package desktop

import (
	"context"
	"errors"
	"fmt"

	"snaptrans/internal/codex"
	"snaptrans/internal/config"
	"snaptrans/internal/hotkeys"
)

// SaveConfig serializes these transactions. Hold mu until the disk write and
// registrations agree, so capture and shutdown see one complete configuration.
func (a *App) saveConfigAndShortcuts(current, next config.Config) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.shuttingDown {
		return context.Canceled
	}
	oldKeys := [3]string{current.ShortcutKey, current.ScreenshotShortcutKey, current.ReplyShortcutKey}
	newKeys := [3]string{next.ShortcutKey, next.ScreenshotShortcutKey, next.ReplyShortcutKey}
	registrations := [3]*hotkeys.Registration{a.shortcut, a.screenshotShortcut, a.replyShortcut}
	previous := registrations
	actions := [3]func() error{a.TriggerTranslation, a.TriggerScreenshot, a.TriggerReply}
	register := func(index int, key string) (*hotkeys.Registration, error) {
		return hotkeys.Register(key, func() { _ = actions[index]() })
	}
	publish := func() {
		a.shortcut, a.screenshotShortcut, a.replyShortcut = registrations[0], registrations[1], registrations[2]
	}
	rollback := func(cause error) error {
		for i := range registrations {
			if registrations[i] != nil && registrations[i] != previous[i] {
				cause = errors.Join(cause, registrations[i].Unregister())
				registrations[i] = nil
			}
		}
		for i := range registrations {
			if newKeys[i] == oldKeys[i] || previous[i] == nil {
				continue
			}
			var err error
			registrations[i], err = register(i, oldKeys[i])
			if err != nil {
				cause = errors.Join(cause, fmt.Errorf("restore shortcut %q: %w", oldKeys[i], err))
			}
		}
		publish()
		return cause
	}
	// Release all moving bindings first, allowing swaps and three-way rotations.
	var releaseErr error
	for i := range registrations {
		if newKeys[i] != oldKeys[i] && registrations[i] != nil {
			releaseErr = errors.Join(releaseErr, registrations[i].Unregister())
			registrations[i] = nil
		}
	}
	if releaseErr != nil {
		return rollback(releaseErr)
	}
	for i := range registrations {
		// Startup owns initial bindings; a running app retries missing handles.
		if newKeys[i] == oldKeys[i] && (registrations[i] != nil || a.ctx == nil) {
			continue
		}
		var err error
		registrations[i], err = register(i, newKeys[i])
		if err != nil {
			return rollback(fmt.Errorf("shortcut %q is unavailable: %w", newKeys[i], err))
		}
	}
	if err := a.configStore.Save(next); err != nil {
		return rollback(err)
	}
	// Invalidate old work before publishing the new configuration. A request
	// admitted after this lock is released must never be canceled as old work.
	a.cancelTranslationWarmupsLocked()
	translationChanged := translationTaskOptions(current) != translationTaskOptions(next) || current.DirectImageInput != next.DirectImageInput
	if translationChanged || workerSettings(current) != workerSettings(next) || current.PersistentOCR != next.PersistentOCR {
		a.invalidateCaptureRequestLocked()
		a.cancelProcessingLocked()
		a.cancelTextActionLocked()
	}
	if translationChanged && (current.Provider == "codex" || next.Provider == "codex") {
		codex.CloseIdle()
	}
	a.cfg = next
	publish()
	return nil
}
