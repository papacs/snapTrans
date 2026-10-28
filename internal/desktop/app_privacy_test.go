package desktop

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
	"snaptrans/internal/history"
)

func TestDisablingHistoryStopsNewRecordsAndPreservesExistingEntries(t *testing.T) {
	app := NewApp(nil)
	app.cfg = config.Default()
	app.historyStore = history.NewStore(filepath.Join(t.TempDir(), "history.json"), 50)
	app.saveHistory("existing", "saved", "to-zh")
	require.NoError(t, json.Unmarshal([]byte(`{"recordHistory":false}`), &app.cfg))
	app.saveHistory("private", "not saved", "to-zh")
	entries, err := app.GetHistory()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "existing", entries[0].Source)
}
