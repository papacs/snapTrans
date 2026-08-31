package translator

import (
	"context"
	"encoding/json"
	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSelectedTextUsesTextOnlyPromptAndRecoversMissingRegion(t *testing.T) {
	requests := make(chan openai.ChatCompletionRequest, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			w.WriteHeader(400)
			return
		}
		requests <- request
		content := "[1] 你好"
		if len(requests) == 2 {
			content = "[2] 世界"
		}
		chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}}}})
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: " + string(chunk) + "\n\ndata: [DONE]\n\n"))
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture-model", Source: "selection", SystemPrompt: "Use a concise tone.", Glossary: "World = 世界"})
	var translated strings.Builder
	err := client.Translate(context.Background(), "Hello\nWorld", DirectionToChinese, func(s string) { translated.WriteString(s) })
	require.NoError(t, err)
	require.Len(t, requests, 2)
	first, second := <-requests, <-requests
	for _, request := range []openai.ChatCompletionRequest{first, second} {
		require.Len(t, request.Messages, 2)
		require.True(t, request.Stream)
		require.Contains(t, request.Messages[0].Content, "explicitly selected")
		require.Contains(t, request.Messages[0].Content, "Use a concise tone.")
		require.Contains(t, request.Messages[0].Content, "World = 世界")
		require.NotContains(t, request.Messages[0].Content, "OCR")
		for _, message := range request.Messages {
			require.Empty(t, message.MultiContent)
		}
	}
	require.Contains(t, first.Messages[1].Content, "[1] Hello")
	require.Contains(t, first.Messages[1].Content, "[2] World")
	require.NotContains(t, second.Messages[1].Content, "[1]")
	require.Contains(t, second.Messages[1].Content, "[2] World")
	require.Contains(t, translated.String(), "你好")
	require.Contains(t, translated.String(), "世界")
}
