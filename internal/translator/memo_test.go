package translator

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTranslationMemoExpiresBoundsMemoryAndInvalidatesSettings(t *testing.T) {
	var memo translationMemo
	now := time.Unix(1000, 0)
	key := translationMemoKey(Options{Model: "luna"}, "text", "Hello", DirectionToChinese)
	memo.put(key, "你好", DirectionToChinese, now)
	_, ok := memo.get(key, now.Add(9*time.Minute))
	require.True(t, ok)
	_, ok = memo.get(key, now.Add(10*time.Minute))
	require.False(t, ok)
	for _, changed := range []Options{{Model: "other"}, {Model: "luna", SystemPrompt: "formal"}, {Model: "luna", Glossary: "Hello=您好"}, {Model: "luna", FastTranslation: true}, {Model: "luna", Provider: "codex"}, {Model: "luna", APIKey: "rotated"}} {
		require.NotEqual(t, key, translationMemoKey(changed, "text", "Hello", DirectionToChinese))
	}
	require.NotEqual(t, key, translationMemoKey(Options{Model: "luna"}, "text", "Hello", DirectionToEnglish))
	for i := 0; i <= len(memo.entries); i++ {
		memo.put([32]byte{byte(i + 1)}, "cached", DirectionToChinese, now)
	}
	_, ok = memo.get([32]byte{1}, now)
	require.False(t, ok)
	memo.put(key, string(make([]byte, 65537)), DirectionToChinese, now)
	_, ok = memo.get(key, now)
	require.False(t, ok)
}

func TestTranslationMemoReusesOnlyTranslations(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"[1] 你好\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	options := Options{APIKey: "test", BaseURL: server.URL, Model: "test"}
	for i := 0; i < 2; i++ {
		var text string
		require.NoError(t, NewOpenAICompatible(options).Translate(context.Background(), "Hello", DirectionToChinese, func(token string) { text += token }))
		require.Equal(t, "[1] 你好", text)
	}
	require.EqualValues(t, 1, calls.Load())
	for i := 0; i < 2; i++ {
		require.NoError(t, NewOpenAICompatible(options).Reply(context.Background(), "Hello", "work", "", "zh-CN", func(string) {}))
	}
	require.EqualValues(t, 3, calls.Load())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, NewOpenAICompatible(options).Translate(ctx, "Hello", DirectionToChinese, func(string) { t.Fatal("cancelled request emitted cached output") }), context.Canceled)
}

func TestFailedPartialTranslationIsNotMemoized(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"[1] 部分\"}}]}\n\ndata: INVALID\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "test"})
	for i := 0; i < 2; i++ {
		require.Error(t, client.Translate(context.Background(), "Hello", DirectionToChinese, func(string) {}))
	}
	require.EqualValues(t, 2, calls.Load())
}

func TestImageMemoRetainsTranslationAndDirection(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"[[to-zh]]\\n你好\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "vision"})
	for i := 0; i < 2; i++ {
		var direction Direction
		var text string
		require.NoError(t, client.TranslateImage(context.Background(), testScreenshot(t), DirectionAuto, func(d Direction) { direction = d }, func(token string) { text += token }))
		require.Equal(t, DirectionToChinese, direction)
		require.Equal(t, "你好", text)
	}
	require.EqualValues(t, 1, calls.Load())
}
