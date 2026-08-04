package translator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

type Options struct {
	APIKey  string
	BaseURL string
	Model   string
}

type Direction string

const (
	DirectionToChinese Direction = "to-zh"
	DirectionToEnglish Direction = "to-en"
)

// OpenAICompatible streams translation through any OpenAI-compatible API,
// including LiteLLM, DeepSeek, and self-hosted gateways.
type OpenAICompatible struct {
	options Options
}

func NormalizeDirection(value string) Direction {
	switch Direction(strings.TrimSpace(value)) {
	case DirectionToEnglish:
		return DirectionToEnglish
	default:
		return DirectionToChinese
	}
}

func NewOpenAICompatible(options Options) *OpenAICompatible {
	if strings.TrimSpace(options.BaseURL) == "" {
		options.BaseURL = "https://api.deepseek.com"
	}
	if strings.TrimSpace(options.Model) == "" {
		options.Model = "deepseek-chat"
	}
	return &OpenAICompatible{options: options}
}

func (d *OpenAICompatible) Translate(ctx context.Context, sourceText string, direction Direction, onToken func(string)) error {
	if strings.TrimSpace(d.options.APIKey) == "" {
		return errors.New("LLM API key is required")
	}
	if strings.TrimSpace(sourceText) == "" {
		return errors.New("source text is empty")
	}

	clientConfig := openai.DefaultConfig(d.options.APIKey)
	clientConfig.BaseURL = d.options.BaseURL
	client := openai.NewClientWithConfig(clientConfig)
	direction = NormalizeDirection(string(direction))

	translated, err := streamTranslationRequest(ctx, client, buildTranslationRequest(d.options.Model, sourceText, direction), onToken)
	if err != nil {
		return err
	}
	if looksLikeMissingOCRRequest(translated) {
		return errors.New("translation model did not receive usable OCR text; please select a text area and try again")
	}

	missing := missingNumberedOCRLines(sourceText, translated)
	if len(missing) == 0 {
		return nil
	}

	if strings.TrimSpace(translated) != "" {
		onToken("\n")
	}
	retry, err := streamTranslationRequest(ctx, client, buildMissingTranslationRequest(d.options.Model, missing, direction), onToken)
	if err != nil {
		return err
	}
	if looksLikeMissingOCRRequest(retry) {
		return errors.New("translation model did not receive usable OCR text; please select a text area and try again")
	}

	return nil
}

func streamTranslationRequest(ctx context.Context, client *openai.Client, request openai.ChatCompletionRequest, onToken func(string)) (string, error) {
	stream, err := client.CreateChatCompletionStream(ctx, request)
	if err != nil {
		return "", err
	}
	defer stream.Close()

	var translated strings.Builder
	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return translated.String(), nil
		}
		if err != nil {
			return translated.String(), err
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

func parseNumberedTranslationIndices(translatedText string) map[int]bool {
	indices := make(map[int]bool)
	pattern := regexp.MustCompile(`(?m)^\s*\[(\d+)\]\s+\S+`)
	for _, match := range pattern.FindAllStringSubmatch(translatedText, -1) {
		var index int
		if _, err := fmt.Sscanf(match[1], "%d", &index); err == nil && index > 0 {
			indices[index] = true
		}
	}
	return indices
}

func TryFastTranslation(sourceText string, direction Direction) (string, bool) {
	lines := nonEmptyOCRLines(sourceText)
	translated := make([]string, 0, len(lines))
	translations := fastTranslationsToChinese
	if NormalizeDirection(string(direction)) == DirectionToEnglish {
		translations = fastTranslationsToEnglish
	}
	for _, line := range lines {
		value, ok := translations[strings.ToLower(line)]
		if !ok {
			return "", false
		}
		translated = append(translated, value)
	}
	if len(translated) == 0 {
		return "", false
	}
	return strings.Join(translated, "\n"), true
}

func buildTranslationRequest(model string, sourceText string, direction Direction) openai.ChatCompletionRequest {
	numberedLines := numberedOCRLines(sourceText)
	spec := translationPromptSpecForDirection(direction)
	return openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: strings.Join([]string{
					"You are a machine translation engine for OCR output.",
					"Translate each numbered OCR line into " + spec.targetDescription + ".",
					"Return only the translated text, with no explanations and no conversational replies.",
					"Preserve each [n] prefix exactly and return exactly one output line for every input line.",
					"Do not add, omit, merge, reorder, or renumber lines.",
					"Never output delimiter names such as OCR_TEXT_BEGIN or OCR_TEXT_END.",
					spec.doNotLeaveUnchanged,
					spec.shortTextRule,
					spec.brandNameRule,
					"Examples: " + spec.examples + ".",
					"Preserve filenames, commands, code, URLs, version numbers, symbols, and formatting.",
					spec.targetLanguageUnchangedRule,
					"Never ask the user to provide OCR text. Never say you are ready.",
				}, " "),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Translate these numbered OCR lines into " + spec.userTarget + " now. Keep every [n] prefix.\n\n" + numberedLines,
			},
		},
		Temperature: 0.1,
		Stream:      true,
	}
}

func buildMissingTranslationRequest(model string, lines []numberedOCRLine, direction Direction) openai.ChatCompletionRequest {
	spec := translationPromptSpecForDirection(direction)
	return openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: strings.Join([]string{
					"You are a machine translation engine for OCR output.",
					"Translate only the missing numbered OCR lines into " + spec.targetDescription + ".",
					"Return only the translated text, with no explanations and no conversational replies.",
					"Preserve each original [n] prefix exactly.",
					"Do not add, omit, merge, reorder, or renumber lines.",
					"Never output delimiter names such as OCR_TEXT_BEGIN or OCR_TEXT_END.",
					"Preserve filenames, commands, code, URLs, version numbers, symbols, and formatting.",
				}, " "),
			},
			{
				Role:    openai.ChatMessageRoleUser,
				Content: "Translate only these missing numbered OCR lines into " + spec.userTarget + " now. Keep every original [n] prefix.\n\n" + numberedOCRLineText(lines),
			},
		},
		Temperature: 0.1,
		Stream:      true,
	}
}

func numberedOCRLines(sourceText string) string {
	lines := numberedOCRLineEntries(sourceText)
	return numberedOCRLineText(lines)
}

func numberedOCRLineText(lines []numberedOCRLine) string {
	numbered := make([]string, 0, len(lines))
	for _, line := range lines {
		numbered = append(numbered, fmt.Sprintf("[%d] %s", line.Index, line.Text))
	}
	return strings.Join(numbered, "\n")
}

type numberedOCRLine struct {
	Index int
	Text  string
}

func missingNumberedOCRLines(sourceText string, translatedText string) []numberedOCRLine {
	seen := parseNumberedTranslationIndices(translatedText)
	sourceLines := numberedOCRLineEntries(sourceText)
	missing := make([]numberedOCRLine, 0)
	for _, line := range sourceLines {
		if !seen[line.Index] {
			missing = append(missing, line)
		}
	}
	return missing
}

func numberedOCRLineEntries(sourceText string) []numberedOCRLine {
	lines := nonEmptyOCRLines(sourceText)
	numbered := make([]numberedOCRLine, 0, len(lines))
	for index, line := range lines {
		numbered = append(numbered, numberedOCRLine{Index: index + 1, Text: line})
	}
	return numbered
}

func nonEmptyOCRLines(sourceText string) []string {
	lines := strings.Split(strings.ReplaceAll(sourceText, "\r", ""), "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

type translationPromptSpec struct {
	targetDescription           string
	userTarget                  string
	doNotLeaveUnchanged         string
	shortTextRule               string
	examples                    string
	targetLanguageUnchangedRule string
	brandNameRule               string
}

func translationPromptSpecForDirection(direction Direction) translationPromptSpec {
	if NormalizeDirection(string(direction)) == DirectionToEnglish {
		return translationPromptSpec{
			targetDescription:           "concise English",
			userTarget:                  "English",
			doNotLeaveUnchanged:         "Do not leave Simplified Chinese natural-language text unchanged.",
			shortTextRule:               "Translate short Simplified Chinese words, labels, buttons, and menu items even when they are a single word.",
			examples:                    "\u6d4b\u8bd5 -> test; \u53d6\u6d88 -> Cancel",
			targetLanguageUnchangedRule: "If an input line is already English or has no natural-language text to translate, return it unchanged after the same [n] prefix.",
			brandNameRule:               "For brand, app, product, and service names, keep the original name unless the name itself has a standard English form.",
		}
	}

	return translationPromptSpec{
		targetDescription:           "concise Simplified Chinese",
		userTarget:                  "Simplified Chinese",
		doNotLeaveUnchanged:         "Do not leave English natural-language text unchanged.",
		shortTextRule:               "Translate short English words, labels, buttons, and menu items even when they are a single word.",
		examples:                    "test -> \u6d4b\u8bd5; Google Play -> Google Play (\u8c37\u6b4c\u5e94\u7528\u5546\u5e97)",
		targetLanguageUnchangedRule: "If an input line is already Simplified Chinese or has no natural-language text to translate, return it unchanged after the same [n] prefix.",
		brandNameRule:               "For brand, app, product, and service names, keep the original name and add a concise Chinese meaning in parentheses when a direct Chinese name is not natural.",
	}
}

var fastTranslationsToChinese = map[string]string{
	"cancel":   "\u53d6\u6d88",
	"close":    "\u5173\u95ed",
	"copy":     "\u590d\u5236",
	"negative": "\u8d1f\u9762",
	"neutral":  "\u4e2d\u6027",
	"positive": "\u6b63\u9762",
	"save":     "\u4fdd\u5b58",
	"test":     "\u6d4b\u8bd5",
}

var fastTranslationsToEnglish = map[string]string{
	"cancel":       "Cancel",
	"close":        "Close",
	"copy":         "Copy",
	"negative":     "Negative",
	"neutral":      "Neutral",
	"positive":     "Positive",
	"save":         "Save",
	"test":         "test",
	"\u53d6\u6d88": "Cancel",
	"\u5173\u95ed": "Close",
	"\u590d\u5236": "Copy",
	"\u8d1f\u9762": "Negative",
	"\u4e2d\u6027": "Neutral",
	"\u6b63\u9762": "Positive",
	"\u4fdd\u5b58": "Save",
	"\u6d4b\u8bd5": "test",
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
