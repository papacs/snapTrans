package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"snaptrans/internal/config"
	"snaptrans/internal/ocr"
	"testing"
)

func TestConnectionUsesUnsavedDraft(t *testing.T) {
	var receivedKey, receivedModel string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("Authorization")
		var body struct{ Model string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		receivedModel = body.Model
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"choices\":[{\"message\":{\"role\":\"assistant\",\"content\":\"ok\"}}]}"))
	}))
	defer server.Close()
	app := NewApp(nil)
	app.cfg = config.Default()
	previous := app.cfg
	draft := config.Default()
	draft.APIKey, draft.BaseURL, draft.Model = "draft-test-key", server.URL, "draft-model"
	require.NoError(t, app.TestConnection(draft))
	require.Equal(t, "Bearer draft-test-key", receivedKey)
	require.Equal(t, "draft-model", receivedModel)
	require.Equal(t, previous, app.cfg)
}
func TestSettingsRollsBackAutostartWhenSaveFails(t *testing.T) {
	var states []bool
	failure := errors.New("hotkey unavailable")
	err := saveSettingsChanges(false, true, func(v bool) error { states = append(states, v); return nil }, func() error { return failure })
	require.ErrorIs(t, err, failure)
	require.Equal(t, []bool{true, false}, states)
}
func TestSettingsDoesNotSaveWhenAutostartFails(t *testing.T) {
	saved := false
	err := saveSettingsChanges(false, true, func(bool) error { return errors.New("access denied") }, func() error { saved = true; return nil })
	require.Error(t, err)
	require.False(t, saved)
}
func TestSettingsReportsRollbackFailure(t *testing.T) {
	err := saveSettingsChanges(false, true, func(v bool) error {
		if !v {
			return errors.New("rollback denied")
		}
		return nil
	}, func() error { return errors.New("save failed") })
	require.ErrorContains(t, err, "restore autostart failed")
}
func TestOCRCacheReusesOnlyMatchingSuccessfulRecognition(t *testing.T) {
	var cache ocrResultCache
	cfg := config.Default()
	calls := 0
	extract := func() (ocr.Result, error) { calls++; return ocr.Result{Text: "Hello"}, nil }
	for i := 0; i < 2; i++ {
		result, err := cache.Run(context.Background(), cfg, "same-image", extract)
		require.NoError(t, err)
		require.Equal(t, "Hello", result.Text)
	}
	require.Equal(t, 1, calls)
	cfg.RapidOCRPath = "other-ocr"
	_, _ = cache.Run(context.Background(), cfg, "same-image", extract)
	require.Equal(t, 2, calls)
	_, _ = cache.Run(context.Background(), cfg, "new-image", extract)
	require.Equal(t, 3, calls)
	cache.Clear()
	_, _ = cache.Run(context.Background(), cfg, "new-image", extract)
	require.Equal(t, 4, calls)
}
func TestOCRCacheDoesNotRetainErrorsOrLateResults(t *testing.T) {
	var cache ocrResultCache
	cfg := config.Default()
	_, err := cache.Run(context.Background(), cfg, "a", func() (ocr.Result, error) { return ocr.Result{}, errors.New("failure") })
	require.Error(t, err)
	require.False(t, cache.valid)
	_, err = cache.Run(context.Background(), cfg, "a", func() (ocr.Result, error) { cache.Clear(); return ocr.Result{Text: "late"}, nil })
	require.NoError(t, err)
	require.False(t, cache.valid)
	ctx, cancel := context.WithCancel(context.Background())
	_, err = cache.Run(ctx, cfg, "a", func() (ocr.Result, error) { cancel(); return ocr.Result{Text: "cancelled"}, nil })
	require.ErrorIs(t, err, context.Canceled)
	require.False(t, cache.valid)
}
func TestOCRWorkerTracksActualPathAndTimeout(t *testing.T) {
	app := NewApp(nil)
	cfg := config.Default()
	cfg.RapidOCRPath = "missing-test-ocr"
	app.cfg = cfg
	app.syncOCRWorker(cfg)
	first := app.ocrWorker
	app.syncOCRWorker(cfg)
	require.Same(t, first, app.ocrWorker)
	cfg.RapidOCRPath = "changed-test-ocr"
	app.cfg = cfg // Reproduce SaveConfig assigning before worker synchronization.
	app.syncOCRWorker(cfg)
	require.NotSame(t, first, app.ocrWorker)
	second := app.ocrWorker
	cfg.RapidOCRTimeoutSeconds++
	app.cfg = cfg
	app.syncOCRWorker(cfg)
	require.NotSame(t, second, app.ocrWorker)
	cfg.PersistentOCR = false
	app.syncOCRWorker(cfg)
	require.Nil(t, app.ocrWorker)
}
