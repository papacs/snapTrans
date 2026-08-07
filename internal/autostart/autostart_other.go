//go:build !windows

// Package autostart is a no-op stub on non-Windows platforms.
package autostart

import "errors"

var ErrNotSupported = errors.New("autostart is not supported on this platform")

func IsEnabled(valueName string) (bool, error) {
	return false, ErrNotSupported
}

func Set(valueName string, executablePath string, enabled bool) error {
	return ErrNotSupported
}
