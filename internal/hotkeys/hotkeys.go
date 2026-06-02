package hotkeys

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	"golang.design/x/hotkey"
)

type Registration struct {
	hotkey *hotkey.Hotkey
	done   chan struct{}
	once   sync.Once
}

func Register(shortcut string, callback func()) (*Registration, error) {
	mods, key, err := ParseShortcut(shortcut)
	if err != nil {
		return nil, err
	}

	hk := hotkey.New(mods, key)
	if err := hk.Register(); err != nil {
		return nil, err
	}

	registration := &Registration{
		hotkey: hk,
		done:   make(chan struct{}),
	}

	go func() {
		for {
			select {
			case <-hk.Keydown():
				callback()
			case <-registration.done:
				return
			}
		}
	}()

	return registration, nil
}

func (r *Registration) Unregister() error {
	var err error
	r.once.Do(func() {
		close(r.done)
		err = r.hotkey.Unregister()
	})
	return err
}

func ParseShortcut(shortcut string) ([]hotkey.Modifier, hotkey.Key, error) {
	parts := strings.Split(shortcut, "+")
	if len(parts) < 2 {
		return nil, 0, fmt.Errorf("shortcut must include a modifier and key: %q", shortcut)
	}

	mods := make([]hotkey.Modifier, 0, len(parts)-1)
	var key hotkey.Key

	for index, part := range parts {
		token := strings.ToLower(strings.TrimSpace(part))
		if token == "" {
			continue
		}

		if index == len(parts)-1 {
			parsed, err := parseKey(token)
			if err != nil {
				return nil, 0, err
			}
			key = parsed
			continue
		}

		mod, err := parseModifier(token)
		if err != nil {
			return nil, 0, err
		}
		mods = append(mods, mod)
	}

	if len(mods) == 0 || key == 0 {
		return nil, 0, fmt.Errorf("invalid shortcut: %q", shortcut)
	}

	return mods, key, nil
}

func parseModifier(token string) (hotkey.Modifier, error) {
	switch token {
	case "ctrl", "control":
		return hotkey.ModCtrl, nil
	case "shift":
		return hotkey.ModShift, nil
	case "alt", "option":
		return hotkey.ModAlt, nil
	case "win", "meta", "super", "cmd", "command":
		return hotkey.ModWin, nil
	default:
		return 0, fmt.Errorf("unsupported shortcut modifier: %s", token)
	}
}

func parseKey(token string) (hotkey.Key, error) {
	if len(token) == 1 {
		char := token[0]
		if char >= 'a' && char <= 'z' {
			return hotkey.KeyA + hotkey.Key(char-'a'), nil
		}
		if char >= '0' && char <= '9' {
			if char == '0' {
				return hotkey.Key0, nil
			}
			return hotkey.Key1 + hotkey.Key(char-'1'), nil
		}
	}

	switch token {
	case "space":
		return hotkey.KeySpace, nil
	case "enter", "return":
		return hotkey.KeyReturn, nil
	case "esc", "escape":
		return hotkey.KeyEscape, nil
	case "tab":
		return hotkey.KeyTab, nil
	}

	if strings.HasPrefix(token, "f") {
		number, err := strconv.Atoi(strings.TrimPrefix(token, "f"))
		if err == nil && number >= 1 && number <= 20 {
			return hotkey.KeyF1 + hotkey.Key(number-1), nil
		}
	}

	return 0, fmt.Errorf("unsupported shortcut key: %s", token)
}
