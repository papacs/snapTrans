package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
	"snaptrans/internal/config"
)

func TestReplyInputModeRespectsTextSelectionAndExplicitOverride(t *testing.T) {
	cfg := config.Default()
	cfg.DirectImageInput = true
	region := &TranslateRegionRequest{Width: 100, Height: 80}
	require.Equal(t, "image", replyInputMode(cfg, TextActionRequest{Region: region}))
	require.Equal(t, "ocr", replyInputMode(cfg, TextActionRequest{Text: "selected text"}))
	require.Equal(t, "ocr", replyInputMode(cfg, TextActionRequest{Region: region, InputMode: "ocr"}))
}

func TestInvalidTranslationInputModePreservesCurrentRequest(t *testing.T) {
	app := NewApp(nil)
	app.cfg = config.Default()
	app.cfg.DirectImageInput = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.processing = cancel
	request := TranslateRegionRequest{Width: 100, Height: 80, InputMode: "unknown"}
	require.ErrorContains(t, app.TranslateRegion(request, "to-zh", 1), "input mode")
	require.NoError(t, ctx.Err())
}

func TestImageReplyBypassesUnavailableOCR(t *testing.T) {
	requests := make(chan openai.ChatCompletionRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request openai.ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"A reply\"}}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	app := NewApp(nil)
	app.cfg = config.Default()
	app.cfg.DirectImageInput = true
	app.cfg.APIKey, app.cfg.BaseURL = "test", server.URL
	app.cfg.RapidOCRPath = "missing-ocr.exe"
	app.frame = image.NewRGBA(image.Rect(0, 0, 100, 80))
	t.Cleanup(app.cancelTextAction)
	require.NoError(t, app.StartTextAction(TextActionRequest{ID: "image-reply", Action: "reply", Region: &TranslateRegionRequest{Width: 100, Height: 80}}))
	select {
	case request := <-requests:
		require.Empty(t, request.Messages[1].Content)
		require.Len(t, request.Messages[1].MultiContent, 2)
		require.Contains(t, request.Messages[1].MultiContent[1].ImageURL.URL, "data:image/jpeg;base64,")
	case <-time.After(3 * time.Second):
		t.Fatal("image mode never reached the model; OCR must not be required")
	}
}

func TestInvalidImageReplyPreservesCurrentRequest(t *testing.T) {
	app := NewApp(nil)
	app.cfg = config.Default()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	app.extensionProcessing = cancel
	for _, request := range []TextActionRequest{
		{ID: "bad-mode", Action: "reply", Text: "message", InputMode: "unknown"},
		{ID: "no-image", Action: "reply", Text: "message", InputMode: "image"},
	} {
		require.Error(t, app.StartTextAction(request))
		require.NoError(t, ctx.Err())
	}
}
