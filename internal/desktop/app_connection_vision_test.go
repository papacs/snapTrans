package desktop

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	_ "image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
)

func TestVisionConnectionTestUsesSyntheticImageAndValidatesAnswer(t *testing.T) {
	for _, answer := range []string{"red", "I cannot see images"} {
		t.Run(answer, func(t *testing.T) {
			var request openai.ChatCompletionRequest
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = json.NewDecoder(r.Body).Decode(&request)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": answer}}}})
			}))
			defer server.Close()
			cfg := config.Default()
			cfg.DirectImageInput, cfg.APIKey, cfg.BaseURL = true, "test", server.URL
			err := NewApp(nil).TestConnection(cfg)
			if answer == "red" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "image")
			}
			require.Len(t, request.Messages[0].MultiContent, 2)
			probe := request.Messages[0].MultiContent[1].ImageURL.URL
			raw, decodeErr := base64.StdEncoding.DecodeString(strings.SplitN(probe, ",", 2)[1])
			require.NoError(t, decodeErr)
			img, _, decodeErr := image.Decode(bytes.NewReader(raw))
			require.NoError(t, decodeErr)
			r, g, b, _ := img.At(0, 0).RGBA()
			require.Greater(t, r, g)
			require.Greater(t, r, b)
		})
	}
}
