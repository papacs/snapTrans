package hotkeys

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.design/x/hotkey"
)

func TestParseShortcutValidCombinations(t *testing.T) {
	cases := map[string]struct {
		mods []hotkey.Modifier
		key  hotkey.Key
	}{
		"Alt+Q":       {[]hotkey.Modifier{hotkey.ModAlt}, hotkey.KeyQ},
		"Ctrl+Shift+S": {[]hotkey.Modifier{hotkey.ModCtrl, hotkey.ModShift}, hotkey.KeyS},
		"Alt+Space":    {[]hotkey.Modifier{hotkey.ModAlt}, hotkey.KeySpace},
		"Win+F12":      {[]hotkey.Modifier{hotkey.ModWin}, hotkey.KeyF12},
		"ctrl+1":       {[]hotkey.Modifier{hotkey.ModCtrl}, hotkey.Key1},
	}

	for input, expected := range cases {
		mods, key, err := ParseShortcut(input)
		require.NoError(t, err, "unexpected error for %q", input)
		require.Equal(t, expected.mods, mods, "modifiers mismatch for %q", input)
		require.Equal(t, expected.key, key, "key mismatch for %q", input)
	}
}

func TestParseShortcutRejectsInvalidInput(t *testing.T) {
	invalid := []string{
		"",
		"Q",
		"Alt+",
		"Alt+Unknown",
		"+Q",
		"Unknown+Q",
	}

	for _, input := range invalid {
		_, _, err := ParseShortcut(input)
		require.Error(t, err, "expected error for %q", input)
	}
}
