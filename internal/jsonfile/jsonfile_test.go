package jsonfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFailedBackupWriteLeavesLiveFileAndNoTemporaryFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	previous := []byte(`{"model":"working"}`)
	require.NoError(t, Write(path, previous))
	require.NoError(t, os.Mkdir(path+".bak", 0700))
	require.NoError(t, os.WriteFile(filepath.Join(path+".bak", "keep"), []byte("keep"), 0600))
	require.Error(t, Write(path, []byte(`{"model":"new"}`)))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, previous, raw)
	temps, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".snaptrans-*.tmp"))
	require.NoError(t, err)
	require.Empty(t, temps)
}

func TestUnreadableBackupDoesNotBecomeARecoverableCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"interrupted":`), 0600))
	require.NoError(t, os.Mkdir(path+".bak", 0700))
	_, err := Read(path)
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrCorrupt, "an I/O failure must not cause callers to erase data")
}
