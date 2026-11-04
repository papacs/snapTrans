package desktop

import (
	"context"
	"fmt"
	"snaptrans/internal/config"
)

// SaveSettings keeps the OS toggle and configuration in one save operation.
// If configuration/hotkey validation fails, restore the previous OS state.
func (a *App) SaveSettings(next config.Config, autoStart bool) error {
	return a.saveSettings(next, autoStart, a.IsAutoStartEnabled, a.SetAutoStart)
}

func (a *App) saveSettings(next config.Config, autoStart bool, readAutoStart func() (bool, error), setAutoStart func(bool) error) error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.mu.Lock()
	closed := a.shuttingDown
	a.mu.Unlock()
	if closed {
		return context.Canceled
	}
	previous, err := readAutoStart()
	if err != nil {
		return fmt.Errorf("read autostart: %w", err)
	}
	return saveSettingsChanges(previous, autoStart, setAutoStart, func() error { return a.saveConfig(next) })
}

func saveSettingsChanges(previous, next bool, setAutoStart func(bool) error, save func() error) error {
	changed := previous != next
	if changed {
		if err := setAutoStart(next); err != nil {
			return fmt.Errorf("update autostart: %w", err)
		}
	}
	if err := save(); err != nil {
		if changed {
			if rollbackErr := setAutoStart(previous); rollbackErr != nil {
				return fmt.Errorf("save settings: %w; restore autostart failed: %v", err, rollbackErr)
			}
		}
		return err
	}
	return nil
}
