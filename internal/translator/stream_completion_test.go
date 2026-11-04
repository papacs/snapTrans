package translator

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompactNumberedTranslationDoesNotTriggerRepair(t *testing.T) {
	calls := 0
	const text = "[1]你好，Alex！\n[2]下一步"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": "stop"}}})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", chunk)
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture-model"})
	var output string
	err := client.Translate(context.Background(), "Hi Alex,\nNEXT", DirectionToChinese, func(token string) { output += token })
	require.NoError(t, err)
	require.Equal(t, 1, calls, "valid compact ID formatting must not spend a second model request")
	require.Equal(t, text, output)
}

func TestIncompleteStreamsAreRejectedAndNeverMemoized(t *testing.T) {
	for _, ending := range []string{"", "length", "content_filter"} {
		t.Run(ending, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"[1] Half a translation\"}}]}\n\n")
				if ending != "" {
					fmt.Fprintf(w, "data: {\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":%q}]}\n\ndata: [DONE]\n\n", ending)
				}
			}))
			defer server.Close()
			client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "test"})
			for i := 0; i < 2; i++ {
				err := client.Translate(context.Background(), "This is a complete sentence to translate.", DirectionToChinese, func(string) {})
				require.ErrorContains(t, err, "incomplete")
			}
			require.Equal(t, 2, calls)
		})
	}
}

func TestEmptyUnfinishedStreamRetriesOnceBeforeFirstToken(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "text/event-stream")
		if calls == 1 {
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"[1] Complete translation\"},\"finish_reason\":\"stop\"}]}\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "test"})
	output := ""
	err := client.Translate(context.Background(), "A fresh sentence to exercise safe startup retry.", DirectionToChinese, func(token string) { output += token })
	require.NoError(t, err)
	require.Equal(t, 2, calls)
	require.Equal(t, "[1] Complete translation", output)
}
