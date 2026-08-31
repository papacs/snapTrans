package history

import (
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestSavedItemsSurviveRecentEvictionAndClear(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "history.json"), 2)
	require.NoError(t, s.Add("original", "译文", "to-zh"))
	entries, err := s.List()
	require.NoError(t, err)
	id := entries[0].ID
	require.NoError(t, s.SetFavorite(id, true))
	require.NoError(t, s.AddLearning("hello", "你好", "hello there"))
	for _, source := range []string{"one", "two", "three"} {
		require.NoError(t, s.Add(source, "译文", "to-zh"))
	}
	entries, err = s.List()
	require.NoError(t, err)
	require.Len(t, entries, 4)
	require.NoError(t, s.ClearRecent())
	entries, err = s.List()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.NoError(t, s.AddLearning("hello", "你好", "hello there"))
	entries, _ = s.List()
	require.Len(t, entries, 2)
	reloaded := NewStore(s.path, 2)
	entries, err = reloaded.List()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.NoError(t, s.DeleteSaved(id))
	entries, _ = s.List()
	require.Len(t, entries, 1)
	require.Error(t, s.SetFavorite("missing", true))
	require.Error(t, s.AddLearning("", "meaning", ""))
}
func TestUnfavoritingReturnsEntryToRecentLimit(t *testing.T) {
	s := NewStore(filepath.Join(t.TempDir(), "history.json"), 1)
	require.NoError(t, s.Add("old", "译文", "to-zh"))
	entries, _ := s.List()
	id := entries[0].ID
	require.NoError(t, s.SetFavorite(id, true))
	require.NoError(t, s.Add("new", "译文", "to-zh"))
	require.NoError(t, s.SetFavorite(id, false))
	entries, _ = s.List()
	require.Len(t, entries, 1)
	require.Equal(t, "new", entries[0].Source)
}
