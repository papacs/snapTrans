package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSlowWarmupCannotConsumeRealRequestTimeout(t *testing.T) {
	CloseIdle()
	warmProcess.Lock()
	warmProcess.warming = true
	warmProcess.warmingPath = "missing-audit-runtime.exe"
	warmProcess.ready = make(chan struct{})
	warmProcess.Unlock()
	defer func() {
		warmProcess.Lock()
		warmProcess.warming = false
		close(warmProcess.ready)
		warmProcess.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := StreamFast(ctx, "missing-audit-runtime.exe", "", "", "", "low", func(string) {})
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "slow prewarming must not hold the request until its deadline")
}

func TestRealRequestCancelsHungPrewarmAndCompletes(t *testing.T) {
	path := fakeExecutable(t, "prewarm-hang")
	t.Setenv("SNAPTRANS_CODEX_FAKE_ATTEMPT", filepath.Join(t.TempDir(), "attempt"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- Warm(ctx, path, "test-model") }()
	require.Eventually(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		_, err := os.Stat(os.Getenv("SNAPTRANS_CODEX_FAKE_ATTEMPT"))
		return warmProcess.warming && err == nil
	}, time.Second, time.Millisecond)
	output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	select {
	case err := <-finished:
		require.True(t, errors.Is(err, context.Canceled), "slow prewarming must be canceled: %v", err)
	case <-ctx.Done():
		t.Fatal("prewarming process did not exit")
	}
}

func TestSilentRequestReconnectsBeforeOverallDeadline(t *testing.T) {
	path := fakeExecutable(t, "first-output-hang")
	t.Setenv("SNAPTRANS_CODEX_FAKE_ATTEMPT", filepath.Join(t.TempDir(), "attempt"))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var tokens string
	output, err := streamFast(ctx, path, "test-model", "translate only", "source", "low", 150*time.Millisecond, func(token string) { tokens += token })
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	require.Equal(t, output, tokens)
}

func TestSilentRequestRetryIsBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := streamFast(ctx, fakeExecutable(t, "no-output"), "test-model", "translate only", "source", "low", 150*time.Millisecond, func(string) {})
	require.ErrorIs(t, err, errFirstOutputTimeout)
	require.NoError(t, ctx.Err())
}

func TestFirstOutputDeadlineStopsAfterTextArrives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var tokens string
	output, err := streamFast(ctx, fakeExecutable(t, "stream-hang"), "test-model", "translate only", "source", "low", 150*time.Millisecond, func(token string) { tokens += token })
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, "[1] 你", output)
	require.Equal(t, output, tokens)
}

func TestResponseIdleDeadlineExcludesSetupAndActiveReasoning(t *testing.T) {
	for _, mode := range []string{"slow-thread", "reasoning-progress"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			var tokens string
			output, err := streamFast(ctx, fakeExecutable(t, mode), "test-model", "translate only", "source", "low", 150*time.Millisecond, func(token string) { tokens += token })
			require.NoError(t, err)
			require.Equal(t, "[1] 你好", output)
			require.Equal(t, output, tokens)
		})
	}
}
