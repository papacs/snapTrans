package config

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestFeatureMigrationAndExplicitDisable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"uiLanguage":"en"}`), 0600))
	store := NewStoreAt(path, "")
	cfg, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, DefaultFeatures(), cfg.Features)
	require.False(t, cfg.Features.MemeExplanation)
	require.False(t, cfg.Features.LearningCards)
	require.False(t, cfg.Features.ShareCards)
	require.False(t, cfg.Features.ImageCompare)
	cfg.Features = Features{ShareCards: true}
	require.NoError(t, store.Save(cfg))
	next, err := store.Load()
	require.NoError(t, err)
	require.Equal(t, cfg.Features, next.Features)
}
