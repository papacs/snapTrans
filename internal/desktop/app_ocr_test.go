package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"snaptrans/internal/config"
	"snaptrans/internal/logfile"
	"snaptrans/internal/ocr"

	"github.com/stretchr/testify/require"
)

func TestOCRErrorClassDoesNotExposeEngineOutput(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{nil, "none"}, {context.Canceled, "canceled"}, {context.DeadlineExceeded, "deadline"},
		{errors.New("RapidOCR timed out after 15s"), "deadline"},
		{errors.New("ocr worker: write command: PRIVATE_ENGINE_OUTPUT"), "transport"},
		{errors.New("OCR executable not found at PRIVATE_LOCAL_PATH"), "init"},
		{errors.New("PRIVATE_CAPTURE_TEXT"), "error"},
	} {
		require.Equal(t, test.want, ocrErrorClass(test.err))
	}
}

func TestOCRDiagnosticsDistinguishCacheAndFallbackWithoutScreenContent(t *testing.T) {
	for _, mode := range []string{"cache", "persistent-fallback", "oneshot"} {
		t.Run(mode, func(t *testing.T) {
			app := NewApp(nil)
			path := filepath.Join(t.TempDir(), "ocr.log")
			app.log = logfile.NewLogger(path, 0)
			cfg := config.Default()
			cfg.RapidOCRPath = filepath.Join(t.TempDir(), "PRIVATE_LOCAL_PATH.exe")
			const input = "PRIVATE_BASE64_IMAGE"
			if mode == "cache" {
				_, err := app.ocrCache.Run(context.Background(), cfg, input, func() (ocr.Result, error) {
					return ocr.Result{Text: "PRIVATE_CAPTURE_TEXT"}, nil
				})
				require.NoError(t, err)
			} else if mode == "persistent-fallback" {
				app.ocrWorker = ocr.NewRapidOCRWorker(cfg.RapidOCRPath, time.Second)
				app.ocrWorkerConfig = workerSettings(cfg)
				t.Cleanup(app.ocrWorker.Close)
			} else {
				cfg.PersistentOCR = false
			}
			_, err := app.runOCR(context.Background(), cfg, input)
			if mode == "cache" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			log := string(raw)
			require.Contains(t, log, "total_ms=")
			require.NotContains(t, log, "PRIVATE_")
			if mode == "cache" {
				require.Contains(t, log, "mode=cache cache_hit=true")
				require.Contains(t, log, "fallback=false")
			} else {
				require.Contains(t, log, "mode=oneshot cache_hit=false")
				require.Contains(t, log, "error_class=init")
				if mode == "persistent-fallback" {
					require.Contains(t, log, "fallback=true")
					require.Contains(t, log, "fallback_reason=init")
				} else {
					require.Contains(t, log, "fallback=false")
				}
			}
		})
	}
}

func TestOCRWarmDiagnosticsReportReadinessWithoutLocalPath(t *testing.T) {
	app := NewApp(nil)
	path := filepath.Join(t.TempDir(), "ocr.log")
	app.log = logfile.NewLogger(path, 0)
	worker := ocr.NewRapidOCRWorker(filepath.Join(t.TempDir(), "PRIVATE_LOCAL_PATH.exe"), time.Second)
	app.ocrWorker = worker
	t.Cleanup(worker.Close)
	app.warmOCRWorker(worker)
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(raw), "OCR warm-up completed startup_ms=")
	require.Contains(t, string(raw), "ready=false error_class=init")
	require.NotContains(t, string(raw), "PRIVATE_")
}
