package desktop

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"image"
	"snaptrans/internal/capture"
	"snaptrans/internal/config"
	"snaptrans/internal/translator"
	"strings"
	"time"
)

type TextActionRequest struct {
	ID        string                  `json:"id"`
	Text      string                  `json:"text"`
	Action    string                  `json:"action"`
	Scene     string                  `json:"scene"`
	Intent    string                  `json:"intent"`
	Region    *TranslateRegionRequest `json:"region,omitempty"`
	InputMode string                  `json:"inputMode,omitempty"`
}
type TextActionEvent struct {
	ID     string `json:"id"`
	Token  string `json:"token,omitempty"`
	Error  string `json:"error,omitempty"`
	Done   bool   `json:"done"`
	Source string `json:"source,omitempty"`
	Stage  string `json:"stage,omitempty"`
	Image  string `json:"image,omitempty"`
}

func actionEnabled(f config.Features, action string) bool {
	switch action {
	case "reply":
		return true
	case "explain", "summarize":
		return f.TextActions
	case "meme":
		return f.MemeExplanation
	case "learning":
		return f.LearningCards
	default:
		return false
	}
}
func (a *App) StartTextAction(request TextActionRequest) error {
	a.mu.Lock()
	cfg := a.cfg.WithDefaults()
	frame := a.frame
	a.mu.Unlock()
	if !actionEnabled(cfg.Features, request.Action) {
		return errors.New("this text action is disabled in settings")
	}
	if strings.TrimSpace(request.ID) == "" || len(request.ID) > 128 || (strings.TrimSpace(request.Text) == "" && (request.Action != "reply" || request.Region == nil)) || len(request.Text) > 100000 || len(request.Intent) > 8000 {
		return errors.New("invalid text action request (maximum 100 KB)")
	}
	if request.Action == "reply" {
		mode := replyInputMode(cfg, request)
		if mode != "ocr" && mode != "image" {
			return errors.New("unknown reply input mode")
		}
		if mode == "image" && (request.Region == nil || frame == nil) {
			return errors.New("direct image input needs a captured region; capture again or choose OCR text")
		}
		if request.Scene != "" && request.Scene != "work" && request.Scene != "social" && request.Scene != "general" {
			return errors.New("unknown reply scene")
		}
		if request.Region != nil && !validRegion(*request.Region) {
			return errors.New("invalid reply region")
		}
		if strings.TrimSpace(request.Text) == "" && frame == nil {
			return errors.New("no captured frame is available; capture again")
		}
	}
	a.mu.Lock()
	if a.extensionProcessing != nil {
		a.extensionProcessing()
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TranslationTimeoutSeconds)*time.Second)
	a.extensionProcessing = cancel
	a.extensionRequestID = request.ID
	a.mu.Unlock()
	emit := func(event TextActionEvent) {
		if ctx.Err() == nil && a.ctx != nil {
			runtime.EventsEmit(a.ctx, "text-action", event)
		}
	}
	go func() {
		defer cancel()
		defer func() {
			a.mu.Lock()
			defer a.mu.Unlock()
			if a.extensionRequestID == request.ID {
				a.extensionProcessing = nil
				a.extensionRequestID = ""
			}
		}()
		client := translator.NewOpenAICompatible(translationOptions(cfg))
		started := time.Now()
		preparedAt := started
		inputMode := "text"
		if request.Action == "reply" && request.Region != nil {
			inputMode = replyInputMode(cfg, request)
		}
		first := true
		onToken := func(token string) {
			if first && token != "" {
				first = false
				if a.log != nil {
					a.log.Infof("action=%s input_mode=%s prepare_ms=%d model_first_token_ms=%d first_token_ms=%d", request.Action, inputMode, preparedAt.Sub(started).Milliseconds(), time.Since(preparedAt).Milliseconds(), time.Since(started).Milliseconds())
				}
			}
			emit(TextActionEvent{ID: request.ID, Token: token})
		}
		var err error
		if request.Action == "reply" {
			if replyInputMode(cfg, request) == "image" {
				var dataURL string
				region := request.Region
				dataURL, err = capture.EncodeVisionCrop(frame, image.Rect(region.X, region.Y, region.X+region.Width, region.Y+region.Height))
				if err == nil {
					emit(TextActionEvent{ID: request.ID, Stage: "writing", Image: dataURL})
					preparedAt = time.Now()
					err = client.Reply(ctx, request.Text, request.Scene, request.Intent, cfg.UILanguage, onToken, dataURL)
				}
			} else {
				if strings.TrimSpace(request.Text) == "" {
					emit(TextActionEvent{ID: request.ID, Stage: "ocr"})
				}
				var source string
				source, err = a.replySource(ctx, cfg, request, frame)
				if err == nil {
					emit(TextActionEvent{ID: request.ID, Stage: "writing", Source: source})
					preparedAt = time.Now()
					err = client.Reply(ctx, source, request.Scene, request.Intent, cfg.UILanguage, onToken)
				}
			}
		} else {
			err = client.TextAction(ctx, request.Text, request.Action, cfg.UILanguage, onToken)
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		event := TextActionEvent{ID: request.ID, Done: true}
		if err != nil {
			event.Error = err.Error()
			if a.log != nil {
				a.log.Errorf("action=%s input_mode=%s error=%v", request.Action, inputMode, err)
			}
		}
		// Timeouts still need a terminal event; explicit cancellation does not.
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "text-action", event)
		}
	}()
	return nil
}
func (a *App) CancelTextAction(id string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if id == a.extensionRequestID && a.extensionProcessing != nil {
		a.extensionProcessing()
		a.extensionProcessing = nil
		a.extensionRequestID = ""
	}
	return nil
}
func (a *App) cancelTextAction() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.extensionProcessing != nil {
		a.extensionProcessing()
		a.extensionProcessing = nil
		a.extensionRequestID = ""
	}
}
func (a *App) SetHistoryFavorite(id string, favorite bool) error {
	a.mu.Lock()
	enabled := a.cfg.Features.HistoryTools
	a.mu.Unlock()
	if !enabled {
		return errors.New("history tools are disabled")
	}
	return a.historyStore.SetFavorite(id, favorite)
}
func (a *App) SaveLearningCard(source, meaning, example string) error {
	a.mu.Lock()
	enabled := a.cfg.Features.LearningCards
	a.mu.Unlock()
	if !enabled {
		return errors.New("learning cards are disabled")
	}
	return a.historyStore.AddLearning(source, meaning, example)
}
func (a *App) DeleteSavedEntry(id string) error { return a.historyStore.DeleteSaved(id) }
