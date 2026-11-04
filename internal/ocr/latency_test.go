package ocr

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Optional offline comparison using synthetic screenshots and stock-engine
// reference JSON. No image or OCR text is sent to a translation service.
func TestLocalOCRRuntimeLatency(t *testing.T) {
	if os.Getenv("SNAPTRANS_OCR_LATENCY_CHECK") != "1" {
		t.Skip("optional installed OCR latency and quality check")
	}
	fixtures := os.Getenv("SNAPTRANS_OCR_FIXTURES")
	require.NotEmpty(t, fixtures)
	worker := NewRapidOCRWorker(os.Getenv("SNAPTRANS_OCR_EXE"), 30*time.Second)
	defer worker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	started := time.Now()
	err := worker.Start(ctx)
	cancel()
	require.NoError(t, err)
	require.Contains(t, worker.cmd.Env, "ORT_LOAD_CONFIG_FROM_MODEL=1", "the production optimized runtime must actually be active")
	t.Logf("background_startup_ms=%d", time.Since(started).Milliseconds())
	for pass := 0; pass < 2; pass++ {
		for _, name := range []string{"light", "dark", "tiny", "long", "narrow", "rotated"} {
			png, err := os.ReadFile(filepath.Join(fixtures, name+".png"))
			require.NoError(t, err)
			width, height := imageDimensions(png)
			raw, err := os.ReadFile(filepath.Join(fixtures, "quality-result-baseline-"+name+".json"))
			require.NoError(t, err)
			expected, err := ExtractResultFromJSON(raw, width, height)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			started := time.Now()
			actual, err := worker.Run(ctx, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png))
			cancel()
			require.NoError(t, err)
			require.Equal(t, expected, actual, "recognized text and normalized boxes must match the stock engine")
			t.Logf("pass=%d fixture=%s total_ms=%d blocks=%d", pass+1, name, time.Since(started).Milliseconds(), len(actual.Blocks))
		}
	}
}

func TestLocalOCRUnicodeRuntimePath(t *testing.T) {
	if os.Getenv("SNAPTRANS_OCR_LATENCY_CHECK") != "1" || runtime.GOOS != "windows" {
		t.Skip("optional Windows OCR cache-path check")
	}
	fixtures := os.Getenv("SNAPTRANS_OCR_FIXTURES")
	require.NotEmpty(t, fixtures)
	t.Setenv("LOCALAPPDATA", filepath.Join(fixtures, "中文缓存 空格"))
	worker := NewRapidOCRWorker(os.Getenv("SNAPTRANS_OCR_EXE"), 30*time.Second)
	defer worker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	require.NoError(t, worker.Start(ctx))
	require.Contains(t, worker.cmd.Dir, "中文缓存 空格")
	require.Contains(t, worker.cmd.Args, "--models=.")
	require.Contains(t, worker.cmd.Env, "ORT_LOAD_CONFIG_FROM_MODEL=1")
	png, err := os.ReadFile(filepath.Join(fixtures, "narrow.png"))
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(fixtures, "quality-result-baseline-narrow.json"))
	require.NoError(t, err)
	width, height := imageDimensions(png)
	expected, err := ExtractResultFromJSON(raw, width, height)
	require.NoError(t, err)
	actual, err := worker.Run(ctx, base64.StdEncoding.EncodeToString(png))
	require.NoError(t, err)
	require.Equal(t, expected, actual)
}

func TestLocalOCROneShotUnicodeTempPath(t *testing.T) {
	if os.Getenv("SNAPTRANS_OCR_LATENCY_CHECK") != "1" || runtime.GOOS != "windows" {
		t.Skip("optional installed OCR one-shot and Unicode temp-path check")
	}
	fixtures := os.Getenv("SNAPTRANS_OCR_FIXTURES")
	require.NotEmpty(t, fixtures)
	tempDir := filepath.Join(t.TempDir(), "中文临时目录 空格🚀")
	require.NoError(t, os.MkdirAll(tempDir, 0o700))
	t.Setenv("TMP", tempDir)
	t.Setenv("TEMP", tempDir)
	png, err := os.ReadFile(filepath.Join(fixtures, "narrow.png"))
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(fixtures, "quality-result-baseline-narrow.json"))
	require.NoError(t, err)
	width, height := imageDimensions(png)
	expected, err := ExtractResultFromJSON(raw, width, height)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	actual, err := NewRapidOCR(os.Getenv("SNAPTRANS_OCR_EXE"), 20*time.Second).ExtractResult(ctx, base64.StdEncoding.EncodeToString(png))
	require.NoError(t, err)
	require.Equal(t, expected, actual)
	entries, err := os.ReadDir(tempDir)
	require.NoError(t, err)
	require.Empty(t, entries, "single-use OCR must not retain screen images")
}
