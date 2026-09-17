package translator

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestThinkingDisabledForDeepSeekV4ModelsAcrossCompatibleEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		model    string
		want     bool
	}{
		{name: "official model", endpoint: "https://api.deepseek.com", model: "deepseek-v4-flash", want: true},
		{name: "provider-prefixed model", endpoint: "https://bifrost.heyluckyme.com/v1", model: "deepseek/deepseek-v4-flash", want: true},
		{name: "compatible gateway", endpoint: "https://gateway.example/v1", model: "deepseek-v4-pro", want: true},
		{name: "unrelated model", endpoint: "https://gateway.example/v1", model: "custom-model", want: false},
		{name: "lookalike model", endpoint: "https://gateway.example/v1", model: "deepseek-v4-flash-preview", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request, err := http.NewRequest("POST", tt.endpoint+"/chat/completions", strings.NewReader("{\"model\":\""+tt.model+"\",\"messages\":[]}"))
			require.NoError(t, err)
			transport := translationTransport{base: roundTripFunc(func(r *http.Request) (*http.Response, error) {
				data, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				var body map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &body))
				if tt.want {
					require.JSONEq(t, "{\"type\":\"disabled\"}", string(body["thinking"]))
					require.Equal(t, int64(len(data)), r.ContentLength)
				} else {
					require.NotContains(t, body, "thinking")
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
			})}
			response, err := transport.RoundTrip(request)
			require.NoError(t, err)
			response.Body.Close()
		})
	}
}

func TestIncompleteTranslationAfterRecoveryReturnsError(t *testing.T) {
	for _, recovered := range []bool{false, true} {
		t.Run(map[bool]string{false: "still missing", true: "recovered"}[recovered], func(t *testing.T) {
			var count atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n := count.Add(1)
				content := "[1] 你好"
				if n == 2 && recovered {
					content = "[2] 世界"
				}
				payload, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": content}}}})
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = w.Write([]byte("data: " + string(payload) + "\n\ndata: [DONE]\n\n"))
			}))
			defer server.Close()
			var output strings.Builder
			client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "test"})
			err := client.Translate(context.Background(), "Hello\nWorld", DirectionToChinese, func(s string) { output.WriteString(s) })
			if recovered {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "translation incomplete")
			}
			require.Equal(t, int32(2), count.Load())
			require.Contains(t, output.String(), "你好")
		})
	}
}
