package desktop

import (
	"context"
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"snaptrans/internal/config"
	"snaptrans/internal/translator"
	"strings"
	"time"
)

type TextActionRequest struct {
	ID     string `json:"id"`
	Text   string `json:"text"`
	Action string `json:"action"`
}
type TextActionEvent struct {
	ID    string `json:"id"`
	Token string `json:"token,omitempty"`
	Error string `json:"error,omitempty"`
	Done  bool   `json:"done"`
}

func actionEnabled(f config.Features, action string) bool {
	switch action {
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
	a.mu.Unlock()
	if !actionEnabled(cfg.Features, request.Action) {
		return errors.New("this text action is disabled in settings")
	}
	if strings.TrimSpace(request.ID) == "" || len(request.ID) > 128 || strings.TrimSpace(request.Text) == "" || len(request.Text) > 100000 {
		return errors.New("invalid text action request (maximum 100 KB)")
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
		client := translator.NewOpenAICompatible(translator.Options{APIKey: cfg.APIKey, BaseURL: cfg.BaseURL, Model: cfg.Model})
		err := client.TextAction(ctx, request.Text, request.Action, cfg.UILanguage, func(token string) { emit(TextActionEvent{ID: request.ID, Token: token}) })
		if errors.Is(ctx.Err(), context.Canceled) {
			return
		}
		event := TextActionEvent{ID: request.ID, Done: true}
		if err != nil {
			event.Error = err.Error()
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
