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

func (d *OpenAICompatible) translationRequest(sourceText string, direction Direction) openai.ChatCompletionRequest {
	request := buildTranslationRequest(d.options.Model, sourceText, direction, d.options.SystemPrompt, d.options.Glossary)
	if d.options.Source == "selection" {
		request = selectedTextRequest(request, d.options, direction, numberedOCRLines(sourceText))
	}
	return d.prepareTranslationRequest(request, direction, numberedOCRLines(sourceText))
}
