// Package logfile writes timestamped, telemetry-free diagnostic lines to a
// local log file, truncating it when it grows too large.
package logfile

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger appends lines to a single local file. It is safe for concurrent
// use.
type Logger struct {
	path     string
	maxBytes int64
	mu       sync.Mutex
}

func NewLogger(path string, maxBytes int64) *Logger {
	if maxBytes <= 0 {
		maxBytes = 2 << 20
	}
	return &Logger{path: path, maxBytes: maxBytes}
}

// Dir returns the directory that contains the log file.
func (l *Logger) Dir() string {
	if l == nil || l.path == "" {
		return ""
	}
	return filepath.Dir(l.path)
}

// Infof writes a timestamped informational line.
func (l *Logger) Infof(format string, args ...any) {
	l.write("INFO", format, args...)
}

// Errorf writes a timestamped error line.
func (l *Logger) Errorf(format string, args ...any) {
	l.write("ERROR", format, args...)
}

func (l *Logger) write(level string, format string, args ...any) {
	if l == nil || l.path == "" {
		return
	}
	line := fmt.Sprintf("%s [%s] %s\n", time.Now().Format(time.RFC3339), level, fmt.Sprintf(format, args...))
	l.mu.Lock()
	defer l.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return
	}

	info, err := os.Stat(l.path)
	if err == nil && info.Size() > l.maxBytes {
		_ = os.Remove(l.path)
	}

	file, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}
