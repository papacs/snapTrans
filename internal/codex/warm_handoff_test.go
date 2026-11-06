package codex

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestForegroundReusesAuthenticatedProcessWhileSessionFinishes(t *testing.T) {
	for _, instructions := range []string{"translate only", "different translation direction"} {
		t.Run(instructions, func(t *testing.T) {
			path := fakeExecutable(t, "prepared-handoff")
			marker := filepath.Join(t.TempDir(), "warm-ready")
			t.Setenv("SNAPTRANS_CODEX_FAKE_WARM_READY", marker)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			warmed := make(chan error, 1)
			go func() { warmed <- WarmSession(ctx, path, "test-model", "translate only", "low") }()
			require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 5*time.Second, 10*time.Millisecond)
			raw, err := os.ReadFile(marker)
			require.NoError(t, err)
			pid, err := strconv.Atoi(string(raw))
			require.NoError(t, err)
			request, finish := DeferSessionRefill(ctx)
			defer finish()
			finished := make(chan error, 1)
			go func() {
				_, err := StreamFast(request, path, "test-model", instructions, "source", "low", func(string) {})
				finished <- err
			}()
			require.Eventually(t, func() bool {
				warmProcess.Lock()
				defer warmProcess.Unlock()
				return warmProcess.active > 1
			}, time.Second, 5*time.Millisecond)
			// Authentication is complete. A short remaining thread setup must
			// not throw away that process after the cold-start grace expires.
			time.Sleep(250 * time.Millisecond)
			require.NoError(t, os.WriteFile(marker+".release", nil, 0600))
			require.NoError(t, <-finished)
			require.NoError(t, <-warmed)
			borrowed, epoch, err := takeWarm(ctx, path)
			require.NoError(t, err)
			defer returnWarm(borrowed, borrowed.cmd.Path, epoch)
			require.Equal(t, pid, borrowed.cmd.Process.Pid, "keep the initialized, authenticated process even if the prepared prompt differs")
			stats := preparedSessionStats(t, borrowed)
			require.Equal(t, float64(1), stats["turnStarts"], "only the foreground request may submit content")
		})
	}
}

func TestAuthenticatedSessionPreparationWaitIsBounded(t *testing.T) {
	path := fakeExecutable(t, "prewarm-session-hang")
	marker := filepath.Join(t.TempDir(), "attempt")
	t.Setenv("SNAPTRANS_CODEX_FAKE_ATTEMPT", marker)
	warmCtx, cancelWarm := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelWarm()
	warmed := make(chan error, 1)
	go func() { warmed <- WarmSession(warmCtx, path, "test-model", "translate only", "low") }()
	require.Eventually(t, func() bool { _, err := os.Stat(marker); return err == nil }, 5*time.Second, 10*time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	require.ErrorIs(t, <-warmed, context.Canceled)
	require.NoError(t, ctx.Err(), "hung session preparation must leave time for foreground recovery")
}

func TestClosedIdleRuntimeDropsItsRetentionTimer(t *testing.T) {
	path := fakeExecutable(t, "fast")
	require.NoError(t, Warm(context.Background(), path))
	CloseIdle()
	warmProcess.Lock()
	defer warmProcess.Unlock()
	require.Nil(t, warmProcess.idle)
	require.Nil(t, warmProcess.timer, "a stopped timer closure must not retain the closed runtime and its buffers")
}

func TestExpiredOldIdleTimerCannotCloseReturnedRuntime(t *testing.T) {
	path := fakeExecutable(t, "fast")
	require.NoError(t, Warm(context.Background(), path))
	warmProcess.Lock()
	first, oldTimer := warmProcess.idle, warmProcess.timer
	warmProcess.Unlock()
	// Reproduce an old AfterFunc callback that has already been scheduled,
	// then paused before acquiring the pool lock. Timer.Stop cannot stop it.
	queued, release, expired := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		close(queued)
		<-release
		expireWarm(first, oldTimer)
		close(expired)
	}()
	<-queued
	borrowed, epoch, err := takeWarm(context.Background(), path)
	require.NoError(t, err)
	require.Same(t, first, borrowed)
	returnWarm(borrowed, borrowed.cmd.Path, epoch)
	warmProcess.Lock()
	currentTimer := warmProcess.timer
	warmProcess.Unlock()
	defer currentTimer.Stop()
	close(release)
	<-expired
	warmProcess.Lock()
	current, timer := warmProcess.idle, warmProcess.timer
	warmProcess.Unlock()
	require.Same(t, first, current, "an old idle lease must not close the reused process")
	require.Same(t, currentTimer, timer)
	require.Nil(t, first.cmd.ProcessState)
	expireWarm(first, currentTimer)
	warmProcess.Lock()
	current, timer = warmProcess.idle, warmProcess.timer
	warmProcess.Unlock()
	require.Nil(t, current)
	require.Nil(t, timer)
	require.NotNil(t, first.cmd.ProcessState, "the current timer must still release its own process")
}

func TestColdSuccessfulRequestPreparesItsActualNextSession(t *testing.T) {
	path := fakeExecutable(t, "prepared")
	key := sessionKey{model: "test-model", instructions: "translate to English", effort: "low"}
	ctx, finish := DeferSessionRefill(context.Background())
	defer finish()
	_, err := StreamFast(ctx, path, key.model, key.instructions, "source", key.effort, func(string) {})
	require.NoError(t, err)
	warmProcess.Lock()
	first := warmProcess.idle
	warmProcess.Unlock()
	require.NotNil(t, first)
	finish()
	require.Eventually(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		return !warmProcess.warming && warmProcess.idle == first && first.hasPreparedSession(key)
	}, 5*time.Second, 10*time.Millisecond, "a cold successful translation must recover preparation using its actual direction and prompt")
	borrowed, epoch, err := takeWarm(context.Background(), path)
	require.NoError(t, err)
	defer returnWarm(borrowed, borrowed.cmd.Path, epoch)
	stats := preparedSessionStats(t, borrowed)
	require.Equal(t, float64(2), stats["threadStarts"])
	require.Equal(t, float64(1), stats["turnStarts"], "refill prepares an unused thread without resending content")
}

func TestResponseIdleDeadlineExcludesLocalInputPreparation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	output, err := streamFast(ctx, fakeExecutable(t, "slow-input"), "test-model", "translate only", "source", "low", 150*time.Millisecond, func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
}

func TestLocalInputPreparationStillHonorsOverallDeadline(t *testing.T) {
	path := fakeExecutable(t, "input-prep-hang")
	require.NoError(t, WarmSession(context.Background(), path, "test-model", "translate only", "low"))
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	_, err := streamFast(ctx, path, "test-model", "translate only", "source", "low", 50*time.Millisecond, func(string) {})
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
