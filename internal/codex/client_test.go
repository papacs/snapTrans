package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The test executable acts as an App Server, exercising real pipes, process
// cancellation and interleaved JSONL notifications without an account or model.
func TestMain(m *testing.M) {
	if mode := os.Getenv("SNAPTRANS_CODEX_FAKE"); mode != "" {
		fakeServer(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeServer(mode string) {
	accountReads := 0
	modelReads, configReads, threadStarts, turnStarts, unsubscribes := 0, 0, 0, 0, 0
	lastUnsubscribed := ""
	var turnThreads, threadInstructions []string
	var threadEffort any
	input := bufio.NewScanner(os.Stdin)
	write := func(value any) { _ = json.NewEncoder(os.Stdout).Encode(value) }
	notify := func(method string, params any) { write(map[string]any{"method": method, "params": params}) }
	for input.Scan() {
		var request struct {
			ID     int
			Method string
			Params map[string]any
		}
		if json.Unmarshal(input.Bytes(), &request) != nil {
			os.Exit(2)
		}
		if request.Method == "initialized" {
			continue
		}
		respond := func(value any) { write(map[string]any{"id": request.ID, "result": value}) }
		switch request.Method {
		case "initialize":
			if mode == "prewarm-hang" && firstFakeAttempt() {
				time.Sleep(time.Minute)
				return
			}
			respond(map[string]any{})
		case "account/read":
			accountReads++
			if mode == "one-auth-only" && accountReads > 1 {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "account read repeated"}})
				continue
			}
			if mode == "routing-retry" && accountReads == 1 || mode == "routing-failed" {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "workspace routing discovery timed out"}})
				continue
			}
			if mode == "hang" {
				time.Sleep(time.Minute)
				return
			}
			account := map[string]any{"type": "chatgpt", "email": "test@example.com", "planType": "plus"}
			if mode == "apikey" {
				account = map[string]any{"type": "apiKey"}
			}
			respond(map[string]any{"account": account})
		case "model/list":
			modelReads++
			if mode == "prepared-model-fail" && modelReads == 1 {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "catalog unavailable"}})
				continue
			}
			if request.Params["cursor"] == nil {
				respond(map[string]any{"data": []any{map[string]any{"id": "alias", "model": "test-model", "displayName": "Test model", "isDefault": true}}, "nextCursor": "page2"})
			} else {
				respond(map[string]any{"data": []any{map[string]any{"id": "other-model", "hidden": false}, map[string]any{"id": "hidden-model", "hidden": true}}})
			}
		case "config/read":
			configReads++
			respond(map[string]any{"config": map[string]any{"mcp_servers": map[string]any{"external": map[string]any{"command": "never-start-this"}}}})
		case "thread/start":
			threadStarts++
			if mode == "prepared-setup-fail" && threadStarts == 1 {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "thread setup failed"}})
				continue
			}
			if mode == "slow-thread" {
				time.Sleep(250 * time.Millisecond)
			}
			cfg, _ := request.Params["config"].(map[string]any)
			threadEffort = cfg["model_reasoning_effort"]
			features, _ := cfg["features"].(map[string]any)
			servers, _ := cfg["mcp_servers"].(map[string]any)
			external, _ := servers["external"].(map[string]any)
			instructions := request.Params["baseInstructions"]
			instructionText, _ := instructions.(string)
			threadInstructions = append(threadInstructions, instructionText)
			if (!strings.HasPrefix(mode, "prepared") && (request.Params["model"] != "test-model" || instructions != "translate only")) || request.Params["modelProvider"] != subscriptionProvider || request.Params["ephemeral"] != true || request.Params["sandbox"] != "read-only" || features["shell_tool"] != false || external["enabled"] != false {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "incorrect model, instructions or isolation settings"}})
				continue
			}
			for _, feature := range []string{"hooks", "plugins", "plugin_discovery_prefetch", "skill_search", "apps", "view_image"} {
				if features[feature] != false {
					os.Exit(7)
				}
			}
			threadID := "thread-test"
			if strings.HasPrefix(mode, "prepared") {
				threadID = fmt.Sprintf("thread-%d", threadStarts)
			}
			respond(map[string]any{"thread": map[string]any{"id": threadID}})
		case "turn/start":
			turnStarts++
			threadID, _ := request.Params["threadId"].(string)
			if strings.HasPrefix(mode, "prepared-batch") {
				for _, previous := range turnThreads {
					if previous == threadID {
						os.Exit(9)
					}
				}
			}
			turnThreads = append(turnThreads, threadID)
			if mode == "prepared-batch-error" && turnStarts == 2 {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "repair was rejected"}})
				continue
			}
			if mode == "consistent-model" && request.Params["model"] != "test-model" {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "turn did not receive the resolved thread model"}})
				continue
			}
			if mode == "consistent-effort" && request.Params["effort"] != threadEffort {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "thread and turn reasoning effort differ"}})
				continue
			}
			if mode == "user-message-progress" {
				respond(map[string]any{"turn": map[string]any{"id": "turn-test"}})
				for i := 0; i < 5; i++ {
					notify("item/started", map[string]any{"threadId": threadID, "item": map[string]any{"type": "userMessage"}})
					time.Sleep(60 * time.Millisecond)
				}
				time.Sleep(time.Minute)
				return
			}
			if mode == "reasoning-progress" {
				respond(map[string]any{"turn": map[string]any{"id": "turn-test"}})
				for i := 0; i < 5; i++ {
					notify("item/reasoning/summaryTextDelta", map[string]any{"threadId": threadID, "delta": "thinking"})
					time.Sleep(60 * time.Millisecond)
				}
				notify("item/agentMessage/delta", map[string]any{"threadId": threadID, "delta": "[1] 你好"})
				notify("turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{"status": "completed"}})
				continue
			}
			if (mode == "first-output-hang" && firstFakeAttempt()) || mode == "no-output" {
				respond(map[string]any{"turn": map[string]any{"id": "turn-test"}})
				time.Sleep(time.Minute)
				return
			}
			if mode == "warm-only" {
				os.Exit(8)
			}
			if mode == "image" {
				parts, _ := request.Params["input"].([]any)
				if len(parts) != 2 {
					os.Exit(5)
				}
				text, _ := parts[0].(map[string]any)
				image, _ := parts[1].(map[string]any)
				if text["type"] != "text" || text["text"] != "source" || image["type"] != "image" || image["url"] != "data:image/png;base64,test-image" {
					os.Exit(6)
				}
			}
			if mode == "fast" && request.Params["effort"] != "low" {
				os.Exit(4)
			}
			if mode == "completion-no-ack" || mode == "completion-late-ack" {
				notify("item/agentMessage/delta", map[string]any{"threadId": threadID, "delta": "[1] 你好"})
				notify("turn/completed", map[string]any{"threadId": threadID, "turn": map[string]any{"status": "completed"}})
				if mode == "completion-no-ack" {
					time.Sleep(time.Minute)
					return
				}
				time.Sleep(20 * time.Millisecond)
				respond(map[string]any{"turn": map[string]any{"id": "turn-test"}})
				continue
			}
			notify("item/agentMessage/delta", map[string]any{"threadId": "other-thread", "delta": "ignore"})
			// Tokens can precede the turn/start response.
			notify("item/agentMessage/delta", map[string]any{"threadId": threadID, "delta": "[1] 你"})
			if mode == "early-token-before-ack" {
				deadline := time.Now().Add(time.Second)
				for {
					if _, err := os.Stat(os.Getenv("SNAPTRANS_CODEX_FAKE_TOKEN_SEEN")); err == nil {
						break
					}
					if time.Now().After(deadline) {
						write(map[string]any{"id": request.ID, "error": map[string]any{"message": "first token was buffered behind turn/start"}})
						return
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			if mode == "turn-start-error" {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "turn was rejected"}})
				continue
			}
			respond(map[string]any{"turn": map[string]any{"id": "turn-test"}})
			if mode == "stream-error" || mode == "stream-retry" || mode == "stream-retry-hang" {
				notify("error", map[string]any{"threadId": threadID, "willRetry": mode != "stream-error", "error": map[string]any{"message": "upstream connection timed out"}})
				if mode != "stream-retry" {
					time.Sleep(time.Minute)
					return
				}
			}
			if mode == "eof" {
				return
			}
			if mode == "stream-hang" {
				time.Sleep(time.Minute)
				return
			}
			notify("item/agentMessage/delta", map[string]any{"threadId": threadID, "delta": "好"})
			turn := map[string]any{"status": "completed"}
			if mode == "failed" {
				turn = map[string]any{"status": "failed", "error": map[string]any{"message": "subscription limit reached"}}
			}
			notify("turn/completed", map[string]any{"threadId": threadID, "turn": turn})
		case "thread/unsubscribe":
			unsubscribes++
			lastUnsubscribed, _ = request.Params["threadId"].(string)
			if mode == "prepared-unsubscribe-error" {
				write(map[string]any{"id": request.ID, "error": map[string]any{"message": "cleanup refused"}})
				continue
			}
			if mode == "unsubscribe-hang" || mode == "prepared-unsubscribe-hang" {
				time.Sleep(time.Minute)
				return
			}
			respond(map[string]any{})
		case "test/stats":
			respond(map[string]any{"modelReads": modelReads, "configReads": configReads, "threadStarts": threadStarts, "turnStarts": turnStarts, "unsubscribes": unsubscribes, "lastUnsubscribed": lastUnsubscribed, "turnThreads": turnThreads, "threadInstructions": threadInstructions})
		case "account/login/start":
			loginURL := "https://auth.openai.com/authorize?test=1"
			if mode == "bad-url" {
				loginURL = "https://example.com/phishing"
			}
			if mode == "login-server-request" {
				respond(map[string]any{"loginId": "login-test", "authUrl": loginURL})
				write(map[string]any{"id": 900, "method": "account/login/completed", "params": map[string]any{"loginId": "login-test", "success": true}})
				notify("account/login/completed", map[string]any{"loginId": "login-test", "success": false, "error": "login callback failed"})
				continue
			}
			// Exercise buffering of a fast callback before the RPC response.
			notify("account/login/completed", map[string]any{"loginId": "login-test", "success": true})
			respond(map[string]any{"loginId": "login-test", "authUrl": loginURL})
		default:
			fmt.Fprintln(os.Stderr, "unexpected request", request.Method)
			os.Exit(3)
		}
	}
}

func firstFakeAttempt() bool {
	file, err := os.OpenFile(os.Getenv("SNAPTRANS_CODEX_FAKE_ATTEMPT"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

func fakeExecutable(t *testing.T, mode string) string {
	t.Helper()
	CloseIdle()
	t.Cleanup(CloseIdle)
	t.Setenv("SNAPTRANS_CODEX_FAKE", mode)
	path, err := os.Executable()
	require.NoError(t, err)
	return path
}

func TestInspectListsSubscriptionModelsAcrossPages(t *testing.T) {
	status, err := Inspect(context.Background(), fakeExecutable(t, "success"))
	require.NoError(t, err)
	require.True(t, status.Authenticated)
	require.Equal(t, "plus", status.Plan)
	require.Equal(t, []Model{{ID: "test-model", Name: "Test model", IsDefault: true}, {ID: "other-model"}}, status.Models)
}

func TestAPIKeyLoginDoesNotCountAsSubscription(t *testing.T) {
	path := fakeExecutable(t, "apikey")
	status, err := Inspect(context.Background(), path)
	require.NoError(t, err)
	require.False(t, status.Authenticated)
	require.Empty(t, status.Models)
	_, err = Stream(context.Background(), path, "test-model", "translate only", "source", func(string) {})
	require.ErrorContains(t, err, "sign in to Codex with ChatGPT")
}

func TestStreamPreservesEarlyUnicodeDeltasAndDefaultModel(t *testing.T) {
	var tokens []string
	result, err := Stream(context.Background(), fakeExecutable(t, "success"), "", "translate only", "source", func(token string) { tokens = append(tokens, token) })
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", result)
	require.Equal(t, []string{"[1] 你", "好"}, tokens)
}

func TestStreamDeliversTokensBeforeTurnStartAcknowledgement(t *testing.T) {
	path := fakeExecutable(t, "early-token-before-ack")
	signal := t.TempDir() + string(os.PathSeparator) + "token-seen"
	t.Setenv("SNAPTRANS_CODEX_FAKE_TOKEN_SEEN", signal)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var tokens string
	output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(token string) {
		tokens += token
		require.NoError(t, os.WriteFile(signal, nil, 0600))
	})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	require.Equal(t, output, tokens)
}

func TestStreamProgressExcludesUserMessageItems(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "user-message-progress"))
	require.NoError(t, err)
	defer c.close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.ctx = ctx
	stop := context.AfterFunc(ctx, c.close)
	defer stop()
	var progress []string
	_, err = c.streamWithProgress("test-model", "translate only", "source", "low", func(string) {}, func(stage string) {
		progress = append(progress, stage)
		if stage == "turn/start" {
			time.AfterFunc(400*time.Millisecond, cancel)
		}
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, []string{"turn/start"}, progress)
}

func TestThreadAndTurnUseSameExplicitReasoningEffort(t *testing.T) {
	for _, effort := range []string{"low", "default", "", " low ", " default "} {
		t.Run(effort, func(t *testing.T) {
			output, err := StreamFast(context.Background(), fakeExecutable(t, "consistent-effort"), "test-model", "translate only", "source", effort, func(string) {})
			require.NoError(t, err)
			require.Equal(t, "[1] 你好", output)
		})
	}
}

func TestThreadAndTurnUseSameResolvedModel(t *testing.T) {
	for _, model := range []string{"test-model", ""} {
		t.Run(model, func(t *testing.T) {
			output, err := StreamFast(context.Background(), fakeExecutable(t, "consistent-model"), model, "translate only", "source", "low", func(string) {})
			require.NoError(t, err)
			require.Equal(t, "[1] 你好", output)
		})
	}
}

func preparedSessionStats(t *testing.T, c *connection) map[string]any {
	t.Helper()
	var stats map[string]any
	require.NoError(t, c.call("test/stats", nil, &stats))
	return stats
}

func TestPreparedSessionStartsNoTurnAndSkipsSetupExactlyOnce(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "prepared"))
	require.NoError(t, err)
	defer c.close()
	key := sessionKey{model: "test-model", instructions: "translate only", effort: "low"}
	require.NoError(t, c.prepareSession(key))
	require.True(t, c.hasPreparedSession(key))
	require.True(t, c.prepareEnabled)
	require.NoError(t, c.prepareSession(key), "repeated prewarming must preserve the unused conversation")
	first, err := c.startThread(key.model, key.instructions, key.effort)
	require.NoError(t, err)
	require.Equal(t, "thread-1", first)
	require.Nil(t, c.prepared)
	stats := preparedSessionStats(t, c)
	require.Equal(t, float64(1), stats["configReads"])
	require.Equal(t, float64(1), stats["threadStarts"])
	require.Equal(t, float64(0), stats["turnStarts"], "preparing must not submit content or invoke a model")
	second, err := c.startThread(key.model, key.instructions, key.effort)
	require.NoError(t, err)
	require.Equal(t, "thread-2", second, "a consumed conversation must never be reused")
	stats = preparedSessionStats(t, c)
	require.Equal(t, float64(2), stats["threadStarts"])
}

func TestPreparedSessionMismatchDiscardsUnusedThread(t *testing.T) {
	for _, next := range []sessionKey{
		{model: "other-model", instructions: "translate only", effort: "low"},
		{model: "test-model", instructions: "other instructions", effort: "low"},
		{model: "test-model", instructions: "translate only", effort: "high"},
	} {
		t.Run(next.model+next.instructions+next.effort, func(t *testing.T) {
			c, err := start(context.Background(), fakeExecutable(t, "prepared"))
			require.NoError(t, err)
			defer c.close()
			require.NoError(t, c.prepareSession(sessionKey{model: "test-model", instructions: "translate only", effort: "low"}))
			require.False(t, c.hasPreparedSession(next))
			id, err := c.startThread(next.model, next.instructions, next.effort)
			require.NoError(t, err)
			require.Equal(t, "thread-2", id)
			require.Nil(t, c.prepared)
			stats := preparedSessionStats(t, c)
			require.Equal(t, float64(2), stats["configReads"])
			require.Equal(t, float64(2), stats["threadStarts"])
			require.Equal(t, float64(1), stats["unsubscribes"])
			require.Equal(t, "thread-1", stats["lastUnsubscribed"])
		})
	}
}

func TestPreparedSessionNormalizesOptionsAndCachesDefaultModel(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "prepared"))
	require.NoError(t, err)
	defer c.close()
	require.NoError(t, c.prepareSession(sessionKey{instructions: "translate only", effort: " default "}))
	require.True(t, c.hasPreparedSession(sessionKey{model: " ", instructions: "translate only"}))
	require.True(t, c.hasPreparedSession(sessionKey{model: " test-model ", instructions: "translate only", effort: "default"}))
	for _, expected := range []string{"thread-1", "thread-2"} {
		id, err := c.startThread("", "translate only", "default")
		require.NoError(t, err)
		require.Equal(t, expected, id)
	}
	stats := preparedSessionStats(t, c)
	require.Equal(t, float64(2), stats["modelReads"], "the two-page default catalog must be resolved once per process")
}

func TestPreparedSessionSetupFailureIsNeverCached(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "prepared-setup-fail"))
	require.NoError(t, err)
	defer c.close()
	key := sessionKey{model: "test-model", instructions: "translate only", effort: "low"}
	require.ErrorContains(t, c.prepareSession(key), "thread setup failed")
	require.False(t, c.hasPreparedSession(key))
	require.False(t, c.prepareEnabled)
	require.NoError(t, c.prepareSession(key))
	require.True(t, c.hasPreparedSession(key))
	stats := preparedSessionStats(t, c)
	require.Equal(t, float64(2), stats["threadStarts"])
	require.Equal(t, float64(0), stats["turnStarts"])
}

func TestDefaultModelResolutionFailureIsNeverCached(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "prepared-model-fail"))
	require.NoError(t, err)
	defer c.close()
	_, err = c.resolveModel("")
	require.ErrorContains(t, err, "catalog unavailable")
	require.Empty(t, c.defaultModel)
	model, err := c.resolveModel("")
	require.NoError(t, err)
	require.Equal(t, "test-model", model)
}

func TestPreparedSessionMismatchCleanupIsBounded(t *testing.T) {
	for _, mode := range []string{"prepared-unsubscribe-hang", "prepared-unsubscribe-error"} {
		t.Run(mode, func(t *testing.T) {
			c, err := start(context.Background(), fakeExecutable(t, mode))
			require.NoError(t, err)
			defer c.close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c.ctx = ctx
			stop := context.AfterFunc(ctx, c.close)
			defer stop()
			require.NoError(t, c.prepareSession(sessionKey{model: "test-model", instructions: "translate only", effort: "low"}))
			_, err = c.startThread("test-model", "other instructions", "low")
			require.ErrorIs(t, err, errConnectionClosed)
			require.Nil(t, c.prepared)
			require.NoError(t, ctx.Err(), "unused conversation cleanup must not consume the foreground deadline")
		})
	}
}

func TestStreamReportsRejectedTurnAfterEarlyToken(t *testing.T) {
	path := fakeExecutable(t, "turn-start-error")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var tokens string
	output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(token string) { tokens += token })
	require.ErrorContains(t, err, "turn was rejected")
	require.Equal(t, "[1] 你", output)
	require.Equal(t, output, tokens)
	warmProcess.Lock()
	idle := warmProcess.idle
	warmProcess.Unlock()
	require.Nil(t, idle)
}

func TestCompletedStreamDoesNotWaitForHungUnsubscribe(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := StreamFast(ctx, fakeExecutable(t, "unsubscribe-hang"), "test-model", "translate only", "source", "low", func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	require.NoError(t, ctx.Err(), "completed output must not wait for the request deadline during cleanup")
	warmProcess.Lock()
	idle := warmProcess.idle
	warmProcess.Unlock()
	require.Nil(t, idle, "an unresponsive process must never return to the warm pool")
}

func TestWarmRequestsReuseRecentSubscriptionCheck(t *testing.T) {
	path := fakeExecutable(t, "one-auth-only")
	var first *connection
	for i := 0; i < 2; i++ {
		output, err := StreamFast(context.Background(), path, "test-model", "translate only", "source", "low", func(string) {})
		require.NoError(t, err)
		require.Equal(t, "[1] 你好", output)
		warmProcess.Lock()
		idle := warmProcess.idle
		warmProcess.Unlock()
		if first == nil {
			first = idle
		} else {
			require.Same(t, first, idle, "recent auth checks must not force a new process")
		}
	}
}

func TestStreamRecoversAfterIdleProcessExits(t *testing.T) {
	path := fakeExecutable(t, "fast")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	warmProcess.Lock()
	previous := warmProcess.idle
	warmProcess.Unlock()
	require.NotNil(t, previous)
	require.NoError(t, previous.cmd.Process.Kill())
	var tokens string
	output, err = StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(token string) { tokens += token })
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
	require.Equal(t, output, tokens)
	warmProcess.Lock()
	idle := warmProcess.idle
	warmProcess.Unlock()
	require.NotNil(t, idle)
	require.NotSame(t, previous, idle)
}

func TestSubscriptionCheckExpiresAndDoesNotCacheFailures(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "one-auth-only"))
	require.NoError(t, err)
	defer c.close()
	require.NoError(t, c.requireSubscription())
	require.NoError(t, c.requireSubscription())
	c.subscriptionCheckedAt = time.Now().Add(-time.Minute)
	require.ErrorContains(t, c.requireSubscription(), "account read repeated")
	// A failed check must not refresh an expired success.
	require.ErrorContains(t, c.requireSubscription(), "account read repeated")
}

func TestStartDisablesInheritedAgentCapabilities(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "warm-only"))
	require.NoError(t, err)
	defer c.close()
	for _, feature := range []string{"hooks", "plugins", "plugin_discovery_prefetch", "skill_search", "apps"} {
		require.Contains(t, c.cmd.Args, "features."+feature+"=false")
	}
}

func TestStreamFastSendsInlineImageWithoutFileAccess(t *testing.T) {
	output, err := StreamFast(context.Background(), fakeExecutable(t, "image"), "test-model", "translate only", "source", "none", func(string) {}, "data:image/png;base64,test-image")
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", output)
}

func TestStreamDoesNotAcceptPartialOutputAsSuccess(t *testing.T) {
	for _, mode := range []string{"failed", "eof"} {
		t.Run(mode, func(t *testing.T) {
			result, err := Stream(context.Background(), fakeExecutable(t, mode), "test-model", "translate only", "source", func(string) {})
			require.Error(t, err)
			require.Contains(t, result, "[1] 你")
			if mode == "failed" {
				require.ErrorContains(t, err, "subscription limit reached")
			}
		})
	}
}

func TestCancellationStopsHungServer(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "hang"))
	require.NoError(t, err)
	defer c.close()
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	c.ctx = ctx
	stop := context.AfterFunc(ctx, c.close)
	defer stop()
	_, err = c.account()
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.ErrorContains(t, err, "account/read")
}

func TestAccountRetriesTransientWorkspaceRoutingFailure(t *testing.T) {
	path := fakeExecutable(t, "routing-retry")
	status, err := Inspect(context.Background(), path)
	require.NoError(t, err)
	require.True(t, status.Authenticated)
}

func TestAccountRoutingRetryIsBounded(t *testing.T) {
	path := fakeExecutable(t, "routing-failed")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := Inspect(ctx, path)
	require.ErrorContains(t, err, "workspace routing discovery timed out")
	require.NoError(t, ctx.Err())
}

func TestStreamReportsTerminalErrorWithoutWaitingForDeadline(t *testing.T) {
	path := fakeExecutable(t, "stream-error")
	require.NoError(t, Warm(context.Background(), path))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) {})
	require.ErrorContains(t, err, "upstream connection timed out")
	require.NoError(t, ctx.Err(), "terminal server errors must be reported immediately")
	require.Equal(t, "[1] 你", output)
}

func TestStreamAllowsServerRetryAndPreservesFailureOnTimeout(t *testing.T) {
	for _, mode := range []string{"stream-retry", "stream-retry-hang"} {
		t.Run(mode, func(t *testing.T) {
			path := fakeExecutable(t, mode)
			require.NoError(t, Warm(context.Background(), path))
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			output, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) {})
			if mode == "stream-retry" {
				require.NoError(t, err)
				require.Equal(t, "[1] 你好", output)
			} else {
				require.ErrorIs(t, err, context.DeadlineExceeded)
				require.ErrorContains(t, err, "upstream connection timed out")
			}
		})
	}
}

func TestLoginBuffersCallbackAndValidatesURL(t *testing.T) {
	for _, mode := range []string{"success", "bad-url"} {
		t.Run(mode, func(t *testing.T) {
			opened := ""
			err := Login(context.Background(), fakeExecutable(t, mode), func(url string) { opened = url })
			if mode == "bad-url" {
				require.Error(t, err)
				require.Empty(t, opened)
			} else {
				require.NoError(t, err)
				require.Equal(t, "https://auth.openai.com/authorize?test=1", opened)
			}
		})
	}
}

func TestLoginIgnoresRejectedServerRequest(t *testing.T) {
	err := Login(context.Background(), fakeExecutable(t, "login-server-request"), func(string) {})
	require.ErrorContains(t, err, "login callback failed")
}

func TestLocalCodexStatus(t *testing.T) {
	if os.Getenv("SNAPTRANS_CODEX_STATUS_CHECK") != "1" {
		t.Skip("optional installed-CLI status check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	status, err := Inspect(ctx, os.Getenv("SNAPTRANS_CODEX_PATH"))
	require.NoError(t, err)
	// Do not log email addresses or any credentials.
	t.Logf("local CLI available, subscription=%t, models=%d", status.Authenticated, len(status.Models))
}

func TestLocalCodexTranslation(t *testing.T) {
	if os.Getenv("SNAPTRANS_CODEX_TRANSLATION_CHECK") != "1" {
		t.Skip("optional live subscription translation check")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var streamed strings.Builder
	result, err := Stream(ctx, "", "", "Translate the numbered English text into simplified Chinese. Keep the [1] prefix. Return only the translated line. Do not use tools.", "[1] Hello, world!", func(token string) { streamed.WriteString(token) })
	require.NoError(t, err)
	require.Contains(t, result, "[1]")
	require.NotContains(t, result, "Hello")
	require.Equal(t, result, streamed.String())
	t.Logf("live subscription translation received %d bytes of streamed text", len(result))
}

func TestLocalCodexLatency(t *testing.T) {
	if os.Getenv("SNAPTRANS_CODEX_LATENCY_CHECK") != "1" {
		t.Skip("optional live latency check")
	}
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(previousLogger)
	defer CloseIdle()
	runs := 2
	if value := os.Getenv("SNAPTRANS_CODEX_LATENCY_RUNS"); value != "" {
		parsed, err := strconv.Atoi(value)
		require.NoError(t, err, "SNAPTRANS_CODEX_LATENCY_RUNS must be an integer from 1 to 20")
		require.True(t, parsed >= 1 && parsed <= 20, "SNAPTRANS_CODEX_LATENCY_RUNS must be from 1 to 20")
		runs = parsed
	}
	if os.Getenv("SNAPTRANS_CODEX_LATENCY_PREWARM") == "1" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		require.NoError(t, Warm(ctx, os.Getenv("SNAPTRANS_CODEX_PATH"), "gpt-6-luna"))
		cancel()
	}
	for i := 0; i < runs; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		started := time.Now()
		var first time.Duration
		stream := Stream
		if os.Getenv("SNAPTRANS_CODEX_LATENCY_FAST") == "1" {
			stream = func(ctx context.Context, path, model, instructions, input string, token func(string)) (string, error) {
				effort := os.Getenv("SNAPTRANS_CODEX_LATENCY_EFFORT")
				if effort == "" {
					effort = "low"
				}
				return StreamFast(ctx, path, model, instructions, input, effort, token)
			}
		}
		_, err := stream(ctx, os.Getenv("SNAPTRANS_CODEX_PATH"), "gpt-6-luna", "Translate the numbered text into Chinese. Preserve [1]. Output the translation only.", "[1] Please send the updated schedule this afternoon.", func(string) {
			if first == 0 {
				first = time.Since(started)
			}
		})
		cancel()
		require.NoError(t, err)
		t.Logf("request=%d first_token_ms=%d total_ms=%d", i+1, first.Milliseconds(), time.Since(started).Milliseconds())
	}
}

func TestWarmProcessIsReusedAndConversationsAreReleased(t *testing.T) {
	path := fakeExecutable(t, "fast")
	require.NoError(t, Warm(context.Background(), path))
	warmProcess.Lock()
	first := warmProcess.idle
	warmProcess.Unlock()
	require.NotNil(t, first)
	for i := 0; i < 2; i++ {
		result, err := StreamFast(context.Background(), path, "test-model", "translate only", "source", "low", func(string) {})
		require.NoError(t, err)
		require.Equal(t, "[1] 你好", result)
		warmProcess.Lock()
		reused := warmProcess.idle
		warmProcess.Unlock()
		require.Same(t, first, reused)
	}
	CloseIdle()
	require.NotNil(t, first.cmd.ProcessState)
}

func TestWarmOnlyInitializesProcessWithoutNetworkSetup(t *testing.T) {
	path := fakeExecutable(t, "warm-only")
	require.NoError(t, Warm(context.Background(), path, "test-model"))
	warmProcess.Lock()
	c := warmProcess.idle
	warmProcess.Unlock()
	require.NotNil(t, c)
	// Authentication and model setup belong to the foreground request. A
	// prewarm canceled during those network calls would discard their progress.
	require.Equal(t, 1, c.seq)
}

func TestCanceledFastStreamIsNeverReused(t *testing.T) {
	path := fakeExecutable(t, "stream-hang")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := StreamFast(ctx, path, "test-model", "translate only", "source", "low", func(string) { cancel() })
	require.ErrorIs(t, err, context.Canceled)
	warmProcess.Lock()
	idle := warmProcess.idle
	warmProcess.Unlock()
	require.Nil(t, idle)
	t.Setenv("SNAPTRANS_CODEX_FAKE", "fast")
	result, err := StreamFast(context.Background(), path, "test-model", "translate only", "source", "low", func(string) {})
	require.NoError(t, err)
	require.Equal(t, "[1] 你好", result)
}
