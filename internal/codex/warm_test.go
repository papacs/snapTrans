package codex

import (
	"context"
	"errors"
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
	path := fakeExecutable(t, "warm-hang")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finished := make(chan error, 1)
	go func() { finished <- Warm(ctx, path, "test-model") }()
	require.Eventually(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		return warmProcess.warming
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
