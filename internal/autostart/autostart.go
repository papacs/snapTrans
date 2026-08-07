//go:build windows

// Package autostart registers or removes snapTrans in the current user's
// Windows "Run" key so the tool starts with Windows.
package autostart

import (
	"errors"
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`

// IsEnabled reports whether an autostart entry exists for the given value
// name.
func IsEnabled(valueName string) (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("autostart: open run key: %w", err)
	}
	defer key.Close()

	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("autostart: read value: %w", err)
	}
	return strings.TrimSpace(value) != "", nil
}

// Set enables or disables the autostart entry. When enabled, the entry
// points at executablePath.
func Set(valueName string, executablePath string, enabled bool) error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			key, _, err = registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
			if err != nil {
				return fmt.Errorf("autostart: create run key: %w", err)
			}
		} else {
			return fmt.Errorf("autostart: open run key: %w", err)
		}
	}
	defer key.Close()

	if !enabled {
		if err := key.DeleteValue(valueName); err != nil && !errors.Is(err, registry.ErrNotExist) {
			return fmt.Errorf("autostart: delete value: %w", err)
		}
		return nil
	}

	command := `"` + executablePath + `"`
	if err := key.SetStringValue(valueName, command); err != nil {
		return fmt.Errorf("autostart: set value: %w", err)
	}
	return nil
}
