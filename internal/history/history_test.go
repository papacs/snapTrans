package history

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddAndListReturnsMostRecentFirst(t *testing.T) {
	temp := t.TempDir()
	store := NewStore(filepath.Join(temp, "history.json"), 5)

	require.NoError(t, store.Add("Hello", "\u4f60\u597d", "to-zh"))
	require.NoError(t, store.Add("World", "\u4e16\u754c", "to-zh"))

	entries, err := store.List()

	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "World", entries[0].Source)
	require.Equal(t, "\u4e16\u754c", entries[0].Translation)
	require.Equal(t, "Hello", entries[1].Source)
}

func TestAddTrimsToMaximum(t *testing.T) {
	temp := t.TempDir()
	store := NewStore(filepath.Join(temp, "history.json"), 3)

	for i := 0; i < 6; i++ {
		require.NoError(t, store.Add("entry", "value", "to-zh"))
	}

	entries, err := store.List()

	require.NoError(t, err)
	require.Len(t, entries, 3)
}

func TestClearRemovesEntries(t *testing.T) {
	temp := t.TempDir()
	store := NewStore(filepath.Join(temp, "history.json"), 5)

	require.NoError(t, store.Add("Hello", "\u4f60\u597d", "to-zh"))
	require.NoError(t, store.Clear())

	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 0)
}

func TestListSurvivesMissingFile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), "nope.json"), 5)

	entries, err := store.List()

	require.NoError(t, err)
	require.Nil(t, entries)
}

func TestNilStoreIsNoOp(t *testing.T) {
	require.NoError(t, (*Store)(nil).Add("a", "b", "to-zh"))
	entries, err := (*Store)(nil).List()
	require.NoError(t, err)
	require.Nil(t, entries)
	require.NoError(t, (*Store)(nil).Clear())
}
