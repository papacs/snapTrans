package codex

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSlowWarmupCannotConsumeRealRequestTimeout(t *testing.T) {
	CloseIdle()
	path := filepath.Join(t.TempDir(), "missing-audit-runtime.exe")
	warmProcess.Lock()
	warmProcess.warming = true
	warmProcess.warmingPath = path
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
	_, err := StreamFast(ctx, path, "", "", "", "low", func(string) {})
	require.Error(t, err)
	require.NoError(t, ctx.Err(), "slow prewarming must not hold the request until its deadline")
}

func TestRealRequestCancelsHungPrewarmAndCompletes(t *testing.T) {
	path := fakeExecutable(t, "prewarm-hang")
	t.Setenv("SNAPTRANS_CODEX_FAKE_ATTEMPT", filepath.Join(t.TempDir(), "attempt"))
	warmCtx, cancelWarm := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelWarm()
	finished := make(chan error, 1)
	go func() { finished <- Warm(warmCtx, path, "test-model") }()
	require.Eventually(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		_, err := os.Stat(os.Getenv("SNAPTRANS_CODEX_FAKE_ATTEMPT"))
		return warmProcess.warming && err == nil
	}, 5*time.Second, 10*time.Millisecond)
	// Process startup is test setup; it must not consume the foreground budget.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
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

func TestUserMessageEventsCannotKeepSilentRequestAlive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := streamFast(ctx, fakeExecutable(t, "user-message-progress"), "test-model", "translate only", "source", "low", 150*time.Millisecond, func(string) {})
	require.ErrorIs(t, err, errFirstOutputTimeout)
	require.NoError(t, ctx.Err(), "echoing the input must not count as model activity")
}

func TestFirstOutputDeadlineStopsAfterTextArrives(t *testing.T) {
	path := fakeExecutable(t, "stream-hang")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var tokens string
	output, err := streamFast(ctx, path, "test-model", "translate only", "source", "low", 150*time.Millisecond, func(token string) {
		tokens += token
		// Start the assertion's wait after the token, excluding Windows process
		// startup. The first-output watchdog must stay stopped throughout it.
		time.AfterFunc(300*time.Millisecond, cancel)
	})
	require.ErrorIs(t, err, context.Canceled)
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

func TestPrewarmDoesNotStartCompetingProcessDuringTranslation(t *testing.T) {
	path := fakeExecutable(t, "fast")
	var prewarmErr error
	var competing *connection
	output, err := StreamFast(context.Background(), path, "test-model", "translate only", "source", "low", func(string) {
		prewarmErr = Warm(context.Background(), path)
		warmProcess.Lock()
		competing = warmProcess.idle
		warmProcess.Unlock()
	})
	require.NoError(t, err)
	require.NoError(t, prewarmErr)
	require.Equal(t, "[1] 你好", output)
	require.Nil(t, competing, "capture prewarming must not start another runtime while translation owns one")
}

func TestWarmSessionPreparesAndReplenishesUnusedConversation(t *testing.T) {
	path := fakeExecutable(t, "fast")
	key := sessionKey{model: "test-model", instructions: "translate only", effort: "low"}
	require.NoError(t, WarmSession(context.Background(), path, key.model, key.instructions, key.effort))
	warmProcess.Lock()
	first := warmProcess.idle
	prepared := first != nil && first.hasPreparedSession(key)
	warmProcess.Unlock()
	require.True(t, prepared)
	output, err := StreamFast(context.Background(), path, key.model, key.instructions, "source", key.effort, func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	require.Eventually(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		return warmProcess.idle == first && first.hasPreparedSession(key) && !warmProcess.warming
	}, 5*time.Second, 10*time.Millisecond, "successful translation should prepare the next independent conversation")
}

func TestStaleSessionRefillCannotRunAfterCloseIdle(t *testing.T) {
	path := fakeExecutable(t, "fast")
	key := sessionKey{model: "test-model", instructions: "translate only", effort: "low"}
	require.NoError(t, WarmSession(context.Background(), path, key.model, key.instructions, key.effort))
	warmProcess.Lock()
	epoch := warmProcess.epoch
	warmProcess.Unlock()
	CloseIdle()
	require.NoError(t, warm(context.Background(), path, &key, &epoch))
	warmProcess.Lock()
	defer warmProcess.Unlock()
	require.Nil(t, warmProcess.idle)
	require.False(t, warmProcess.warming)
}

func TestFailedSessionPreparationDoesNotEnterPool(t *testing.T) {
	path := fakeExecutable(t, "apikey")
	err := WarmSession(context.Background(), path, "test-model", "translate only", "low")
	require.ErrorContains(t, err, "sign in to Codex with ChatGPT")
	warmProcess.Lock()
	defer warmProcess.Unlock()
	require.Nil(t, warmProcess.idle)
	require.False(t, warmProcess.warming)
}

func TestStaleWarmCheckoutCannotBorrowNewGeneration(t *testing.T) {
	path := fakeExecutable(t, "fast")
	require.NoError(t, Warm(context.Background(), path))
	warmProcess.Lock()
	oldEpoch := warmProcess.epoch
	warmProcess.Unlock()
	CloseIdle()
	require.NoError(t, Warm(context.Background(), path))
	warmProcess.Lock()
	current := warmProcess.idle
	warmProcess.Unlock()
	c, _, err := takeWarm(context.Background(), path, oldEpoch)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, c)
	warmProcess.Lock()
	defer warmProcess.Unlock()
	require.Same(t, current, warmProcess.idle, "obsolete preparation must not steal a newer connection")
}

func TestSpeculativeWarmPreservesLastUnusedConversation(t *testing.T) {
	path := fakeExecutable(t, "prepared")
	key := sessionKey{model: "test-model", instructions: "reply in English", effort: "low"}
	require.NoError(t, WarmSession(context.Background(), path, key.model, key.instructions, key.effort))
	warmProcess.Lock()
	c := warmProcess.idle
	prepared, seq := c.prepared, c.seq
	warmProcess.Unlock()
	require.NoError(t, WarmSession(context.Background(), path, key.model, "translate to Chinese", key.effort))
	warmProcess.Lock()
	require.Same(t, c, warmProcess.idle)
	require.Same(t, prepared, c.prepared)
	require.Equal(t, seq, c.seq, "a speculative direction must not replace the last-used session")
	warmProcess.Unlock()
	borrowed, epoch, err := takeWarm(context.Background(), path)
	require.NoError(t, err)
	id, err := borrowed.startThread(key.model, key.instructions, key.effort)
	require.NoError(t, err)
	require.Equal(t, prepared.id, id)
	returnWarm(borrowed, borrowed.cmd.Path, epoch)
}

func TestRequestedWarmReplacesOnlyMismatchedUnusedSession(t *testing.T) {
	path := fakeExecutable(t, "prepared")
	require.NoError(t, WarmSession(context.Background(), path, "test-model", "translate to Chinese", "low"))
	warmProcess.Lock()
	first := warmProcess.idle
	old := first.prepared
	warmProcess.Unlock()
	key := sessionKey{model: "test-model", instructions: "reply in English", effort: "high"}
	require.NoError(t, WarmSessionRequested(context.Background(), path, key.model, key.instructions, key.effort))
	require.NoError(t, WarmSessionRequested(context.Background(), path, key.model, key.instructions, key.effort))
	warmProcess.Lock()
	current, prepared := warmProcess.idle, first.prepared
	matched := first.hasPreparedSession(key)
	warmProcess.Unlock()
	require.Same(t, first, current)
	require.True(t, matched, "a known reply must prepare its actual prompt and saved effort")
	require.NotNil(t, prepared)
	require.NotEqual(t, old.id, prepared.id)
	borrowed, epoch, err := takeWarm(context.Background(), path)
	require.NoError(t, err)
	var stats struct{ ThreadStarts, Unsubscribes, TurnStarts int }
	require.NoError(t, borrowed.call("test/stats", nil, &stats))
	require.Equal(t, 2, stats.ThreadStarts)
	require.Equal(t, 1, stats.Unsubscribes)
	require.Zero(t, stats.TurnStarts, "preparation must not submit model input")
	returnWarm(borrowed, borrowed.cmd.Path, epoch)
}

func holdProcessWarm(t *testing.T, path string) func() {
	t.Helper()
	warmProcess.Lock()
	ready := make(chan struct{})
	warmProcess.warming, warmProcess.warmingPath, warmProcess.ready = true, path, ready
	warmProcess.Unlock()
	var once sync.Once
	release := func() {
		once.Do(func() {
			warmProcess.Lock()
			if warmProcess.ready == ready {
				warmProcess.warming, warmProcess.ready = false, nil
			}
			close(ready)
			warmProcess.Unlock()
		})
	}
	t.Cleanup(release)
	return release
}

func TestRequestedWarmWaitsForProcessPreparation(t *testing.T) {
	path := fakeExecutable(t, "prepared")
	require.NoError(t, Warm(context.Background(), path))
	release := holdProcessWarm(t, path)
	finished := make(chan error, 1)
	key := sessionKey{model: "test-model", instructions: "reply in English", effort: "high"}
	go func() {
		finished <- WarmSessionRequested(context.Background(), path, key.model, key.instructions, key.effort)
	}()
	select {
	case err := <-finished:
		t.Fatalf("known task was dropped during process preparation: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	release()
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("requested preparation did not finish")
	}
	warmProcess.Lock()
	defer warmProcess.Unlock()
	require.NotNil(t, warmProcess.idle)
	require.True(t, warmProcess.idle.hasPreparedSession(key))
}

func TestRequestedWarmWaitingCannotReviveClosedGeneration(t *testing.T) {
	path := fakeExecutable(t, "prepared")
	require.NoError(t, Warm(context.Background(), path))
	release := holdProcessWarm(t, path)
	finished := make(chan error, 1)
	go func() { finished <- WarmSessionRequested(context.Background(), path, "test-model", "reply", "high") }()
	select {
	case err := <-finished:
		t.Fatalf("requested preparation did not wait: %v", err)
	case <-time.After(30 * time.Millisecond):
	}
	CloseIdle()
	release()
	select {
	case err := <-finished:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("stale requested preparation did not finish")
	}
	warmProcess.Lock()
	defer warmProcess.Unlock()
	require.Nil(t, warmProcess.idle)
	require.False(t, warmProcess.warming)
}

func TestRequestedWarmWaitRespectsCancellationAndActiveRequest(t *testing.T) {
	path := fakeExecutable(t, "prepared")
	require.NoError(t, Warm(context.Background(), path))
	release := holdProcessWarm(t, path)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	require.ErrorIs(t, WarmSessionRequested(ctx, path, "test-model", "reply", "high"), context.DeadlineExceeded)
	release()
	output, err := StreamFast(context.Background(), path, "test-model", "translate only", "source", "low", func(string) {
		require.NoError(t, WarmSessionRequested(context.Background(), path, "test-model", "reply", "high"))
		warmProcess.Lock()
		competing := warmProcess.idle
		warmProcess.Unlock()
		require.Nil(t, competing, "a known task must not start competing preparation during inference")
	})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
}

func TestSessionRefillScopeReusesProcessForFreshRepairThenReplenishes(t *testing.T) {
	path := fakeExecutable(t, "prepared-batch")
	key := sessionKey{model: "test-model", instructions: "translate only", effort: "low"}
	require.NoError(t, WarmSession(context.Background(), path, key.model, key.instructions, key.effort))
	warmProcess.Lock()
	first := warmProcess.idle
	warmProcess.Unlock()
	ctx, finish := DeferSessionRefill(context.Background())
	defer finish()
	for _, instructions := range []string{key.instructions, "repair only"} {
		output, err := StreamFast(ctx, path, key.model, instructions, "source", key.effort, func(string) {})
		require.NoError(t, err)
		require.Equal(t, "[1] 你好", output)
		// A coverage check can take time between passes. Neither automatic
		// refill nor opportunistic capture warmup may steal the idle process.
		require.NoError(t, WarmSession(context.Background(), path, key.model, key.instructions, key.effort))
		require.Never(t, func() bool {
			warmProcess.Lock()
			defer warmProcess.Unlock()
			return warmProcess.warming || warmProcess.idle != first || first.prepared != nil
		}, 100*time.Millisecond, 5*time.Millisecond)
	}
	borrowed, epoch, err := takeWarm(ctx, path)
	require.NoError(t, err)
	require.Same(t, first, borrowed, "repair must reuse the authenticated runtime")
	stats := preparedSessionStats(t, borrowed)
	require.Equal(t, float64(2), stats["threadStarts"])
	require.Equal(t, float64(2), stats["turnStarts"])
	require.Equal(t, float64(2), stats["unsubscribes"])
	require.Equal(t, []any{"thread-1", "thread-2"}, stats["turnThreads"], "each physical call must use a fresh ephemeral conversation")
	returnWarm(borrowed, borrowed.cmd.Path, epoch)
	finish()
	finish()
	require.Eventually(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		return !warmProcess.warming && warmProcess.idle == first && first.hasPreparedSession(key)
	}, 5*time.Second, 10*time.Millisecond)
	borrowed, epoch, err = takeWarm(context.Background(), path)
	require.NoError(t, err)
	require.Same(t, first, borrowed)
	stats = preparedSessionStats(t, borrowed)
	require.Equal(t, float64(3), stats["threadStarts"], "finish must prepare exactly one next conversation")
	require.Equal(t, float64(2), stats["turnStarts"], "preparation must not send old input")
	require.Equal(t, []any{key.instructions, "repair only", key.instructions}, stats["threadInstructions"], "the next capture must use normal translation instructions")
	returnWarm(borrowed, borrowed.cmd.Path, epoch)
}

func TestSessionRefillScopeCannotReviveOldGeneration(t *testing.T) {
	path := fakeExecutable(t, "prepared-batch")
	key := sessionKey{model: "test-model", instructions: "translate only", effort: "low"}
	require.NoError(t, WarmSession(context.Background(), path, key.model, key.instructions, key.effort))
	ctx, finish := DeferSessionRefill(context.Background())
	defer finish()
	_, err := StreamFast(ctx, path, key.model, key.instructions, "source", key.effort, func(string) {})
	require.NoError(t, err)
	CloseIdle()
	current, epoch, err := takeWarm(context.Background(), path)
	require.NoError(t, err)
	returnWarm(current, current.cmd.Path, epoch)
	finish()
	require.Never(t, func() bool {
		warmProcess.Lock()
		defer warmProcess.Unlock()
		return warmProcess.warming || warmProcess.idle != current
	}, 100*time.Millisecond, 5*time.Millisecond)
	borrowed, epoch, err := takeWarm(context.Background(), path)
	require.NoError(t, err)
	require.Same(t, current, borrowed)
	require.Equal(t, float64(0), preparedSessionStats(t, borrowed)["threadStarts"], "an old batch must not prepare on a new generation")
	returnWarm(borrowed, borrowed.cmd.Path, epoch)
}

func TestSessionRefillScopeSkipsCanceledOrFailedRepair(t *testing.T) {
	for _, outcome := range []string{"canceled", "failed"} {
		t.Run(outcome, func(t *testing.T) {
			path := fakeExecutable(t, "prepared-batch-error")
			require.NoError(t, WarmSession(context.Background(), path, "test-model", "translate only", "low"))
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx, finish := DeferSessionRefill(parent)
			defer finish()
			_, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) {})
			require.NoError(t, err)
			if outcome == "canceled" {
				cancel()
			} else {
				_, err = StreamFast(ctx, path, "test-model", "repair only", "source", "low", func(string) {})
				require.ErrorContains(t, err, "repair was rejected")
			}
			finish()
			require.Never(t, func() bool {
				warmProcess.Lock()
				defer warmProcess.Unlock()
				return warmProcess.warming || (warmProcess.idle != nil && warmProcess.idle.prepared != nil)
			}, 100*time.Millisecond, 5*time.Millisecond)
			warmProcess.Lock()
			active := warmProcess.active
			warmProcess.Unlock()
			require.Zero(t, active, "finish must release the foreground guard even on cancellation or error")
		})
	}
}
