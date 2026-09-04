package desktop

import (
	"fmt"
	"snaptrans/internal/config"
)

// SaveSettings keeps the OS toggle and configuration in one save operation.
// If configuration/hotkey validation fails, restore the previous OS state.
func (a *App) SaveSettings(next config.Config, autoStart bool) error {
	a.settingsMu.Lock()
	defer a.settingsMu.Unlock()
	previous, err := a.IsAutoStartEnabled()
	if err != nil {
		return fmt.Errorf("read autostart: %w", err)
	}
	return saveSettingsChanges(previous, autoStart, a.SetAutoStart, func() error { return a.SaveConfig(next) })
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
