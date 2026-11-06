package desktop

import (
	"fmt"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"snaptrans/internal/hotkeys"
)

func (a *App) registerScrollShortcuts(epoch uint64) ([]*hotkeys.Registration, error) {
	var registrations []*hotkeys.Registration
	for _, binding := range []struct{ key, action string }{{"enter", "finish"}, {"escape", "cancel"}} {
		action := binding.action
		registration, err := hotkeys.RegisterTemporaryKey(binding.key, func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.scrollCommandCurrentLocked(epoch) && a.ctx != nil {
				runtime.EventsEmit(a.ctx, "scroll-capture-command", struct {
					Action    string `json:"action"`
					CaptureID uint64 `json:"captureId"`
				}{action, epoch})
			}
		})
		if err != nil {
			unregisterScrollShortcuts(registrations)
			return nil, fmt.Errorf("reserve scrolling capture %s key: %w", binding.key, err)
		}
		registrations = append(registrations, registration)
	}
	return registrations, nil
}

func (a *App) scrollCommandCurrentLocked(epoch uint64) bool {
	return !a.shuttingDown && a.captureEpoch == epoch && a.scrollCapture != nil
}

func unregisterScrollShortcuts(registrations []*hotkeys.Registration) {
	for _, registration := range registrations {
		_ = registration.Unregister()
	}
}
