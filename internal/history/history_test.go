package history

import (
	"os"
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
	require.NotNil(t, entries)
	require.Empty(t, entries)
}

func TestCorruptHistoryRecoversAndAcceptsNewEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path, 5)
	require.NoError(t, store.Add("first", "one", "to-zh"))
	require.NoError(t, store.Add("second", "two", "to-zh"))
	require.NoError(t, os.WriteFile(path, []byte(`[{"source":`), 0600))
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "first", entries[0].Source)
	require.NoError(t, store.Add("third", "three", "to-zh"))
	require.NoError(t, store.Clear())
	entries, err = store.List()
	require.NoError(t, err)
	require.Empty(t, entries, "clearing must not resurrect backup history")
}

func TestCorruptHistoryWithoutBackupIsPreservedAndReset(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	broken := []byte(`[{"source":`)
	require.NoError(t, os.WriteFile(path, broken, 0600))
	store := NewStore(path, 5)
	require.NoError(t, store.Add("new", "value", "to-zh"))
	preserved, err := filepath.Glob(path + ".corrupt-*")
	require.NoError(t, err)
	require.Len(t, preserved, 1)
	data, err := os.ReadFile(preserved[0])
	require.NoError(t, err)
	require.Equal(t, broken, data)
}

func TestCorruptHistoryWithUnreadableBackupDoesNotEraseOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	broken := []byte(`[{"source":`)
	require.NoError(t, os.WriteFile(path, broken, 0600))
	require.NoError(t, os.Mkdir(path+".bak", 0700))
	store := NewStore(path, 5)
	require.Error(t, store.Add("new", "value", "to-zh"))
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, broken, raw)
}

func TestMissingHistoryWithCorruptBackupCanRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	require.NoError(t, os.WriteFile(path+".bak", []byte(`[{"source":`), 0600))
	store := NewStore(path, 5)
	require.NoError(t, store.Add("new", "value", "to-zh"))
	require.NoError(t, os.Remove(path))
	entries, err := store.List()
	require.NoError(t, err)
	require.Empty(t, entries)
	preserved, err := filepath.Glob(path + ".bak.corrupt-*")
	require.NoError(t, err)
	require.Len(t, preserved, 1)
}

func TestHistoryWithWrongJSONTypeRecoversBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path, 5)
	require.NoError(t, store.Add("first", "one", "to-zh"))
	require.NoError(t, store.Add("second", "two", "to-zh"))
	require.NoError(t, os.WriteFile(path, []byte(`{"entries":"wrong format"}`), 0600))
	entries, err := store.List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "first", entries[0].Source)
}

func TestClearAlsoRemovesQuarantinedHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	store := NewStore(path, 5)
	require.NoError(t, os.WriteFile(path, []byte(`[{"source":`), 0600))
	require.NoError(t, store.Add("new", "value", "to-zh"))
	require.NoError(t, store.Clear())
	preserved, err := filepath.Glob(path + "*")
	require.NoError(t, err)
	require.Empty(t, preserved)
}

func TestNilStoreIsNoOp(t *testing.T) {
	require.NoError(t, (*Store)(nil).Add("a", "b", "to-zh"))
	entries, err := (*Store)(nil).List()
	require.NoError(t, err)
	require.NotNil(t, entries)
	require.Empty(t, entries)
	require.NoError(t, (*Store)(nil).Clear())
}

func TestListNormalizesJSONNullToEmptyList(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	require.NoError(t, os.WriteFile(path, []byte("null"), 0o600))
	store := NewStore(path, 5)

	entries, err := store.List()

	require.NoError(t, err)
	require.NotNil(t, entries)
	require.Empty(t, entries)
}
