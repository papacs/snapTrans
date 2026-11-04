package codex

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Optional real subscription comparison. The fixed synthetic input contains no
// screenshot data; the three-second gap represents capture/selection/OCR time.
func TestLocalCodexPreparedSessionLatency(t *testing.T) {
	if os.Getenv("SNAPTRANS_CODEX_SESSION_LATENCY_CHECK") != "1" {
		t.Skip("optional live prepared-session comparison")
	}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	defer slog.SetDefault(previous)
	defer CloseIdle()
	const model = "gpt-6-luna"
	const instructions = "Translate the numbered text into Chinese. Preserve [1]. Output the translation only."
	profiles := []string{"process", "prepared"}
	if profile := os.Getenv("SNAPTRANS_CODEX_SESSION_LATENCY_PROFILE"); profile != "" {
		require.Contains(t, profiles, profile)
		profiles = []string{profile}
	}
	runs := 3
	if value := os.Getenv("SNAPTRANS_CODEX_SESSION_LATENCY_RUNS"); value != "" {
		parsed, err := strconv.Atoi(value)
		require.NoError(t, err)
		require.True(t, parsed >= 1 && parsed <= 20, "sample count must be from 1 to 20")
		runs = parsed
	}
	for _, profile := range profiles {
		CloseIdle()
		warmCtx, cancelWarm := context.WithTimeout(context.Background(), 25*time.Second)
		started := time.Now()
		var err error
		if profile == "prepared" {
			err = WarmSession(warmCtx, os.Getenv("SNAPTRANS_CODEX_PATH"), model, instructions, "low")
		} else {
			err = Warm(warmCtx, os.Getenv("SNAPTRANS_CODEX_PATH"))
		}
		cancelWarm()
		require.NoError(t, err)
		t.Logf("profile=%s background_prepare_ms=%d", profile, time.Since(started).Milliseconds())
		for i := 0; i < runs; i++ {
			time.Sleep(3 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			started = time.Now()
			var first time.Duration
			output, err := StreamFast(ctx, os.Getenv("SNAPTRANS_CODEX_PATH"), model, instructions, "[1] Please send the updated schedule this afternoon.", "low", func(token string) {
				if first == 0 && token != "" {
					first = time.Since(started)
				}
			})
			cancel()
			t.Logf("profile=%s request=%d first_token_ms=%d total_ms=%d success=%t", profile, i+1, first.Milliseconds(), time.Since(started).Milliseconds(), err == nil)
			require.NoError(t, err)
			require.Contains(t, output, "[1]")
		}
	}
}
