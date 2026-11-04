package codex

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type protocolWriter struct{ io.Writer }

func (protocolWriter) Close() error { return nil }

func protocolConnection(t *testing.T, messages ...any) (*connection, *bytes.Buffer) {
	t.Helper()
	var incoming, outgoing bytes.Buffer
	for _, msg := range messages {
		require.NoError(t, json.NewEncoder(&incoming).Encode(msg))
	}
	scanner := bufio.NewScanner(&incoming)
	scanner.Buffer(make([]byte, 4096), 4<<20)
	c := &connection{ctx: context.Background(), in: protocolWriter{&outgoing}, out: scanner}
	// Transcript tests have no child process. Real-process deadline and cleanup
	// behavior is exercised by the fake App Server integration tests below.
	c.closeOnce.Do(func() {})
	return c, &outgoing
}

func protocolNotification(method string, params any) any {
	return map[string]any{"method": method, "params": params}
}

func protocolResult(id int, result any) any {
	return map[string]any{"id": id, "result": result}
}

func TestStreamIgnoresUnrelatedNotificationShapes(t *testing.T) {
	c, _ := protocolConnection(t,
		protocolNotification("future/array", []string{"metadata"}),
		protocolNotification("future/string", "metadata"),
		map[string]any{"method": "future/empty"},
		map[string]any{"id": 77, "method": "item/agentMessage/delta", "params": map[string]any{"threadId": "ready", "delta": "ignore this request"}},
		protocolResult(1, map[string]any{}),
		protocolNotification("item/agentMessage/delta", map[string]any{"threadId": "ready", "delta": "你好"}),
		protocolNotification("turn/completed", map[string]any{"threadId": "ready", "turn": map[string]any{"status": "completed"}}),
		protocolResult(2, map[string]any{}),
	)
	c.prepared = &preparedThread{key: sessionKey{model: "model", instructions: "translate"}, id: "ready"}
	var tokens strings.Builder
	text, err := c.stream("model", "translate", "hello", "", func(token string) { tokens.WriteString(token) })
	require.NoError(t, err)
	require.Equal(t, "你好", text)
	require.Equal(t, text, tokens.String())
}

func TestCompletedStreamStillRequiresTurnAcceptance(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "accepted", true: "rejected"}[rejected], func(t *testing.T) {
			ack := protocolResult(1, map[string]any{})
			if rejected {
				ack = map[string]any{"id": 1, "error": map[string]any{"message": "turn was rejected"}}
			}
			c, _ := protocolConnection(t,
				protocolNotification("item/agentMessage/delta", map[string]any{"threadId": "ready", "delta": "partial"}),
				protocolNotification("turn/completed", map[string]any{"threadId": "ready", "turn": map[string]any{"status": "completed"}}),
				protocolNotification("item/agentMessage/delta", map[string]any{"threadId": "ready", "delta": "ignore after completion"}),
				ack,
				protocolResult(2, map[string]any{}),
			)
			c.prepared = &preparedThread{key: sessionKey{model: "model", instructions: "translate"}, id: "ready"}
			var tokens strings.Builder
			text, err := c.stream("model", "translate", "hello", "", func(token string) { tokens.WriteString(token) })
			if rejected {
				require.ErrorContains(t, err, "turn was rejected")
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, "partial", text)
			require.Equal(t, text, tokens.String())
		})
	}
}

func TestCompletedStreamAcknowledgementWaitIsBounded(t *testing.T) {
	for _, mode := range []string{"completion-no-ack", "completion-late-ack"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			c, err := start(ctx, fakeExecutable(t, mode))
			require.NoError(t, err)
			defer c.close()
			started := time.Now()
			var tokens strings.Builder
			text, err := c.stream("test-model", "translate only", "source", "", func(token string) { tokens.WriteString(token) })
			if mode == "completion-no-ack" {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.NoError(t, ctx.Err(), "missing acknowledgement must not consume the overall deadline")
			require.Less(t, time.Since(started), 2*time.Second)
			require.Equal(t, "[1] 你好", text)
			require.Equal(t, text, tokens.String())
		})
	}
}

func TestCallDropsUnconsumedNotifications(t *testing.T) {
	c, _ := protocolConnection(t,
		protocolNotification("thread/tokenUsage/updated", map[string]any{"metadata": "unused"}),
		protocolNotification("account/login/completed", map[string]any{"loginId": "login", "success": true}),
		protocolResult(1, map[string]any{}),
	)
	require.NoError(t, c.call("account/login/start", nil, nil))
	require.Len(t, c.pending, 1)
	require.Equal(t, "account/login/completed", c.pending[0].Method)
}

func TestDeferredNotificationsAreBounded(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		count         int
	}{
		{name: "message count", count: 257, payload: "x"},
		{name: "payload bytes", count: 3, payload: strings.Repeat("x", 2<<20)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			messages := make([]any, 0, tc.count+1)
			for i := 0; i < tc.count; i++ {
				messages = append(messages, protocolNotification("item/agentMessage/delta", map[string]any{"delta": tc.payload}))
			}
			messages = append(messages, protocolResult(1, map[string]any{}))
			c, _ := protocolConnection(t, messages...)
			err := c.call("thread/start", nil, nil)
			require.ErrorContains(t, err, "notification buffer limit")
			require.LessOrEqual(t, len(c.pending), 256)
		})
	}
}

func TestCanceledConnectionDoesNotDeliverBufferedMessages(t *testing.T) {
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "scanner", true: "pending"}[queued], func(t *testing.T) {
			c, _ := protocolConnection(t, protocolNotification("item/agentMessage/delta", map[string]any{"delta": "late"}))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			c.ctx = ctx
			if queued {
				c.pending = []message{{Method: "item/agentMessage/delta", Params: json.RawMessage(`{"delta":"late"}`)}}
			}
			_, err := c.next()
			require.ErrorIs(t, err, context.Canceled)
		})
	}
}

func TestConsumedNotificationReleasesPayload(t *testing.T) {
	c, _ := protocolConnection(t,
		protocolNotification("account/login/completed", map[string]any{"loginId": "one"}),
		protocolResult(1, map[string]any{}),
	)
	require.NoError(t, c.call("account/login/start", nil, nil))
	backing := c.pending
	msg, err := c.next()
	require.NoError(t, err)
	require.Equal(t, "account/login/completed", msg.Method)
	require.Equal(t, message{}, backing[0])
	require.Nil(t, c.pending)
}

func TestModelListRejectsNonAdjacentCursorCycle(t *testing.T) {
	c, _ := protocolConnection(t,
		protocolResult(1, map[string]any{"nextCursor": "A"}),
		protocolResult(2, map[string]any{"nextCursor": "B"}),
		protocolResult(3, map[string]any{"nextCursor": "A"}),
		map[string]any{"id": 4, "error": map[string]any{"message": "cycle was not stopped"}},
	)
	_, err := c.models()
	require.ErrorContains(t, err, "repeated its cursor")
	require.Equal(t, 3, c.seq)
}

func TestAccountUpdateInvalidatesCachedStateAndPreparedThread(t *testing.T) {
	key := sessionKey{model: "model", instructions: "translate"}
	c, outgoing := protocolConnection(t,
		protocolNotification("account/updated", map[string]any{"authMode": "apikey"}),
		protocolResult(1, map[string]any{}),
		protocolResult(2, map[string]any{"config": map[string]any{}}),
		protocolResult(3, map[string]any{"thread": map[string]any{"id": "fresh"}}),
	)
	c.subscriptionCheckedAt, c.defaultModel = time.Now(), "model"
	c.prepared = &preparedThread{key: key, id: "old"}
	_, err := c.read()
	require.NoError(t, err)
	require.True(t, c.subscriptionCheckedAt.IsZero())
	require.Empty(t, c.defaultModel)
	require.False(t, c.hasPreparedSession(key))
	id, err := c.startThread(key.model, key.instructions, key.effort)
	require.NoError(t, err)
	require.Equal(t, "fresh", id)
	require.Contains(t, outgoing.String(), `"method":"thread/unsubscribe"`)
}

func TestAccountUpdateDuringRPCDoesNotRestoreStaleCaches(t *testing.T) {
	t.Run("subscription", func(t *testing.T) {
		c, _ := protocolConnection(t,
			protocolNotification("account/updated", map[string]any{}),
			protocolResult(1, map[string]any{"account": map[string]any{"type": "chatgpt"}}),
			protocolResult(2, map[string]any{"account": map[string]any{"type": "apiKey"}}),
		)
		require.NoError(t, c.requireSubscription())
		require.True(t, c.subscriptionCheckedAt.IsZero())
		require.ErrorContains(t, c.requireSubscription(), "sign in to Codex")
	})
	t.Run("default model", func(t *testing.T) {
		c, _ := protocolConnection(t,
			protocolNotification("account/updated", map[string]any{}),
			protocolResult(1, map[string]any{"data": []any{map[string]any{"model": "old", "isDefault": true}}}),
			protocolResult(2, map[string]any{"data": []any{map[string]any{"model": "new", "isDefault": true}}}),
		)
		model, err := c.resolveModel("")
		require.NoError(t, err)
		require.Equal(t, "old", model)
		require.Empty(t, c.defaultModel)
		model, err = c.resolveModel("")
		require.NoError(t, err)
		require.Equal(t, "new", model)
	})
	t.Run("prepared thread", func(t *testing.T) {
		key := sessionKey{model: "model", instructions: "translate"}
		c, _ := protocolConnection(t,
			protocolResult(1, map[string]any{"config": map[string]any{}}),
			protocolNotification("account/updated", map[string]any{}),
			protocolResult(2, map[string]any{"thread": map[string]any{"id": "old"}}),
		)
		require.NoError(t, c.prepareSession(key))
		require.False(t, c.hasPreparedSession(key))
	})
}
