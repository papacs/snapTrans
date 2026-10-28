//go:build windows

package codex

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFindsLatestDesktopRuntimeAndHonorsExplicitPath(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "OpenAI", "Codex", "bin", "old", "codex.exe")
	latest := filepath.Join(root, "OpenAI", "Codex", "bin", "new", "codex.exe")
	for _, path := range []string{old, latest} {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
		require.NoError(t, os.WriteFile(path, []byte("test"), 0600))
	}
	yesterday := time.Now().Add(-24 * time.Hour)
	require.NoError(t, os.Chtimes(old, yesterday, yesterday))
	t.Setenv("LOCALAPPDATA", root)
	path, err := resolveExecutable("")
	require.NoError(t, err)
	require.Equal(t, latest, path)
	path, err = resolveExecutable(old)
	require.NoError(t, err)
	require.Equal(t, old, path)
	cmd := exec.Command(path)
	configureCommand(cmd)
	require.True(t, cmd.SysProcAttr.HideWindow)
}
