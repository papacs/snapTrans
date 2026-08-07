package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoggerWritesTimedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "snaptrans.log")
	logger := NewLogger(path, 0)

	logger.Infof("started")
	logger.Errorf("stage=%s error=%v", "ocr", "boom")

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	content := string(raw)

	require.Contains(t, content, "[INFO] started")
	require.Contains(t, content, "[ERROR] stage=ocr error=boom")
	require.Contains(t, content, "T")
}

func TestLoggerTruncatesOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.txt")
	require.NoError(t, os.WriteFile(path, []byte(strings.Repeat("x", 1024)), 0o600))

	logger := NewLogger(path, 512)
	logger.Infof("after truncation")

	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Less(t, info.Size(), int64(1024))
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(content), "after truncation")
}

func TestNilLoggerIsNoOp(t *testing.T) {
	require.NotPanics(t, func() {
		(*Logger)(nil).Infof("nope")
		(*Logger)(nil).Errorf("nope")
	})
}
