package translator

import (
	"context"
	"errors"
	"io"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type Options struct {
	APIKey  string
	BaseURL string
	Model   string
}

type DeepSeek struct {
	options Options
}

func NewDeepSeek(options Options) *DeepSeek {
	if strings.TrimSpace(options.BaseURL) == "" {
		options.BaseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(options.Model) == "" {
		options.Model = "deepseek-chat"
	}
	return &DeepSeek{options: options}
}

func (d *DeepSeek) Translate(ctx context.Context, sourceText string, onToken func(string)) error {
	if strings.TrimSpace(d.options.APIKey) == "" {
		return errors.New("DeepSeek API key is required")
	}
	if strings.TrimSpace(sourceText) == "" {
		return errors.New("source text is empty")
	}

	clientConfig := openai.DefaultConfig(d.options.APIKey)
	clientConfig.BaseURL = d.options.BaseURL
	client := openai.NewClientWithConfig(clientConfig)

	stream, err := client.CreateChatCompletionStream(ctx, buildTranslationRequest(d.options.Model, sourceText))
	if err != nil {
		return err
	}
	defer stream.Close()

	var translated strings.Builder
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if looksLikeMissingOCRRequest(translated.String()) {
				return errors.New("translation model did not receive usable OCR text; please select a text area and try again")
			}
			return nil
		}
		if err != nil {
			return err
		}

		for _, choice := range response.Choices {
			token := choice.Delta.Content
			if token != "" {
				translated.WriteString(token)
				onToken(token)
			}
		}
	}
}

func buildTranslationRequest(model string, sourceText string) openai.ChatCompletionRequest {
	return openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: strings.Join([]string{
					"You are a machine translation engine for OCR output.",
					"Translate the delimited OCR_TEXT into concise Simplified Chinese.",
					"Return only the translated text, with no explanations and no conversational replies.",
					"Do not leave English natural-language text unchanged.",
					"Translate short English words, labels, buttons, and menu items even when they are a single word.",
					"When OCR_TEXT has multiple lines, Return the same number of lines in the same order whenever possible. Do not merge lines or add bullets.",
					"For brand, app, product, and service names, keep the original name and add a concise Chinese meaning in parentheses when a direct Chinese name is not natural.",
					"Examples: test -> \u6d4b\u8bd5; Google Play -> Google Play (\u8c37\u6b4c\u5e94\u7528\u5546\u5e97).",
					"Preserve filenames, commands, code, URLs, version numbers, symbols, and formatting.",
					"If OCR_TEXT is already Simplified Chinese or has no natural-language text to translate, return it unchanged.",
					"Never ask the user to provide OCR text. Never say you are ready.",
				}, " "),
			},
			{
				Role: openai.ChatMessageRoleUser,
				Content: "Translate this OCR_TEXT now.\n\nOCR_TEXT_BEGIN\n" +
					strings.TrimSpace(sourceText) +
					"\nOCR_TEXT_END",
			},
		},
		Temperature: 0.1,
		Stream:      true,
	}
}

func looksLikeMissingOCRRequest(text string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	if normalized == "" {
		return false
	}

	readinessPhrases := []string{
		"please provide the ocr text",
		"provide the ocr text",
		"ocr text you would like translated",
		"i am ready to assist",
		"i'm ready to assist",
	}
	for _, phrase := range readinessPhrases {
		if strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}
