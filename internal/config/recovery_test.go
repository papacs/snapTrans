package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigWithWrongJSONTypeRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStoreAt(path, "")
	cfg := Default()
	cfg.Model = "previous-working-model"
	require.NoError(t, store.Save(cfg))
	cfg.Model = "latest-model"
	require.NoError(t, store.Save(cfg))
	require.NoError(t, os.WriteFile(path, []byte(`{"model": []}`), 0600))
	loaded, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, "previous-working-model", loaded.Model)
}

func TestLegacyPlaintextKeyIsNotCopiedIntoRecoveryBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	store := NewStoreAt(path, "")
	require.NoError(t, os.WriteFile(path, []byte(`{"apiKey":"legacy-test-secret","model":"legacy"}`), 0600))
	cfg, err := store.Load()
	require.NoError(t, err)
	require.NoError(t, store.Save(cfg))
	backup, err := os.ReadFile(path+".bak")
	require.NoError(t, err)
	require.NotContains(t, string(backup), "legacy-test-secret", "the recovery file must not reintroduce plaintext credentials during migration")
}
