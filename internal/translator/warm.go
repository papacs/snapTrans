package translator

import (
	"context"

	"snaptrans/internal/codex"

	openai "github.com/sashabaranov/go-openai"
)

// WarmTranslation prepares an unused conversation with the exact translation
// instructions. It never submits a user turn or includes captured content.
func (d *OpenAICompatible) WarmTranslation(ctx context.Context, direction Direction) error {
	if d.options.Provider != "codex" {
		return d.Warm(ctx)
	}
	d = d.forTranslation()
	request := d.translationRequest("", direction)
	return codex.WarmSession(ctx, d.options.CodexPath, request.Model, request.Messages[0].Content, d.options.CodexReasoningEffort)
}

// WarmReply prepares only an unused thread for a known reply shape. Replies
// retain the configured reasoning effort even when fast translation is enabled.
func (d *OpenAICompatible) WarmReply(ctx context.Context, scene, locale string, withImage bool) error {
	prompt, err := replyPrompt(scene, locale, withImage)
	if err != nil {
		return err
	}
	if d.options.Provider != "codex" {
		return d.Warm(ctx)
	}
	return codex.WarmSessionRequested(ctx, d.options.CodexPath, d.options.Model, prompt, d.options.CodexReasoningEffort)
}

func (d *OpenAICompatible) translationRequest(sourceText string, direction Direction) openai.ChatCompletionRequest {
	request := buildTranslationRequest(d.options.Model, sourceText, direction, d.options.SystemPrompt, d.options.Glossary)
	if d.options.Source == "selection" {
		request = selectedTextRequest(request, d.options, direction, numberedOCRLines(sourceText))
	}
	return d.prepareTranslationRequest(request, direction, numberedOCRLines(sourceText))
}
