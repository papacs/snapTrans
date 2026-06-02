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
		options.BaseURL = "https://api.deepseek.com/v1"
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

	stream, err := client.CreateChatCompletionStream(ctx, openai.ChatCompletionRequest{
		Model: d.options.Model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: "Translate the user's OCR text into concise Simplified Chinese. Preserve code, numbers, and formatting. Return only the translation.",
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: sourceText,
			},
		},
		Stream: true,
	})
	if err != nil {
		return err
	}
	defer stream.Close()

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		for _, choice := range response.Choices {
			token := choice.Delta.Content
			if token != "" {
				onToken(token)
			}
		}
	}
}

