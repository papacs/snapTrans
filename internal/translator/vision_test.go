package translator

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"snaptrans/internal/capture"
	"snaptrans/internal/codex"
)

func testScreenshot(t *testing.T) string {
	t.Helper()
	var data bytes.Buffer
	require.NoError(t, png.Encode(&data, image.NewRGBA(image.Rect(0, 0, 10, 10))))
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(data.Bytes())
}

func TestImageTranslationStreamsDirectionWithoutOCR(t *testing.T) {
	var request openai.ChatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		w.Header().Set("Content-Type", "text/event-stream")
		for _, token := range []string{"[[to-", "en]]\n", "Hello", " world"} {
			data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": token}}}})
			fmt.Fprintf(w, "data: %s\n\n", data)
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "vision-model"})
	var output string
	var direction Direction
	err := client.TranslateImage(context.Background(), testScreenshot(t), DirectionAuto, func(value Direction) { direction = value }, func(token string) { output += token })
	require.NoError(t, err)
	require.Equal(t, DirectionToEnglish, direction)
	require.Equal(t, "Hello world", output)
	require.Len(t, request.Messages[1].MultiContent, 2)
	require.Equal(t, openai.ChatMessagePartTypeImageURL, request.Messages[1].MultiContent[1].Type)
	require.Contains(t, request.Messages[0].Content, "untrusted data")
}

func TestVisionRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"https://example.com/image.png", "data:image/png;base64,broken", "data:text/html;base64,dGVzdA=="} {
		_, err := visionMessage("text", input)
		require.Error(t, err)
	}
}

func TestImageTranslationHandlesHeaderOnlyAndMissingHeaders(t *testing.T) {
	for _, test := range []struct {
		name      string
		tokens    []string
		want      string
		wantError bool
	}{
		{"header only", []string{"[[to-", "zh]]"}, "", true},
		{"no header", []string{"你好"}, "你好", false},
		{"leading newlines", []string{"\n", "[[to-zh]]\r\n", "你好"}, "你好", false},
		{"header and no translation", []string{"[[to-en]]\n"}, "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				for _, token := range test.tokens {
					data, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]any{"content": token}}}})
					fmt.Fprintf(w, "data: %s\n\n", data)
				}
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			client := NewOpenAICompatible(Options{APIKey: "test", BaseURL: server.URL, Model: "vision"})
			var output string
			err := client.TranslateImage(context.Background(), testScreenshot(t), DirectionAuto, func(Direction) {}, func(token string) { output += token })
			if test.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.want, output)
		})
	}
}

func TestLocalImageTranslation(t *testing.T) {
	path := os.Getenv("SNAPTRANS_VISION_TRANSLATION_IMAGE")
	if path == "" {
		t.Skip("optional synthetic screenshot translation")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	frame, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	dataURL, err := capture.EncodeVisionCrop(frame, frame.Bounds())
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer codex.CloseIdle()
	client := NewOpenAICompatible(Options{Provider: "codex", Model: "gpt-6-luna", CodexReasoningEffort: "low"})
	var output string
	var direction Direction
	err = client.TranslateImage(ctx, dataURL, DirectionAuto, func(value Direction) { direction = value }, func(token string) { output += token })
	require.NoError(t, err)
	require.Equal(t, DirectionToChinese, direction)
	require.NotEmpty(t, output)
	require.NotContains(t, output, "[[to-")
	t.Logf("direct image translation resolved=%s output=%s", direction, output)
}

func TestLocalInputModes(t *testing.T) {
	path := os.Getenv("SNAPTRANS_VISION_BENCHMARK_IMAGE")
	if path == "" {
		t.Skip("optional synthetic screenshot benchmark")
	}
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	frame, _, err := image.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	prepared := time.Now()
	dataURL, err := capture.EncodeVisionCrop(frame, frame.Bounds())
	require.NoError(t, err)
	t.Logf("image_prepare_ms=%d upload_bytes=%d", time.Since(prepared).Milliseconds(), len(dataURL)*3/4)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	defer codex.CloseIdle()
	require.NoError(t, codex.Warm(ctx, ""))
	for _, scenario := range []struct {
		name, effort string
		image        bool
	}{
		{"text-low", "low", false}, {"text-none", "none", false}, {"image-none", "none", true}, {"text-none-repeat", "none", false}, {"image-none-repeat", "none", true},
	} {
		client := NewOpenAICompatible(Options{Provider: "codex", Model: "gpt-6-luna", CodexReasoningEffort: scenario.effort})
		text := "Manager: Can you finish this urgent request today?\nMe: Today's tasks are already fully booked.\nPlease confirm which task should take priority."
		var images []string
		if scenario.image {
			images = []string{dataURL}
			text = ""
		}
		started := time.Now()
		var first time.Duration
		var output string
		err := client.Reply(ctx, text, "work", "Ask for task priority, without promising a deadline.", "en", func(token string) {
			if first == 0 {
				first = time.Since(started)
			}
			output += token
		}, images...)
		require.NoError(t, err)
		require.NotEmpty(t, output)
		t.Logf("case=%s first_token_ms=%d total_ms=%d reply=%s", scenario.name, first.Milliseconds(), time.Since(started).Milliseconds(), output)
	}
}
