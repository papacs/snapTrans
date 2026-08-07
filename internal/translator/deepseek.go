package translator

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"

	openai "github.com/sashabaranov/go-openai"
)

type Options struct {
	APIKey       string
	BaseURL      string
	Model        string
	SystemPrompt string
	Glossary     string
}

type Direction string

const (
	DirectionToChinese Direction = "to-zh"
	DirectionToEnglish Direction = "to-en"
	DirectionAuto      Direction = "auto"
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
	case DirectionAuto:
		return DirectionAuto
	default:
		return DirectionToChinese
	}
}

// DetectDirection guesses the translation direction from the OCR text:
// Chinese-dominated text is translated to English, otherwise to Chinese.
func DetectDirection(text string) Direction {
	hanCount := 0
	otherCount := 0
	for _, char := range text {
		if unicode.IsSpace(char) {
			continue
		}
		if unicode.Is(unicode.Han, char) {
			hanCount++
		} else {
			otherCount++
		}
	}
	total := hanCount + otherCount
	if total > 0 && float64(hanCount)/float64(total) > 0.35 {
		return DirectionToEnglish
	}
	return DirectionToChinese
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

// Ping verifies the configured API key, base URL, and model with a minimal
// chat completion request.
func (d *OpenAICompatible) Ping(ctx context.Context) error {
	if strings.TrimSpace(d.options.APIKey) == "" {
		return errors.New("LLM API key is required")
	}

	clientConfig := openai.DefaultConfig(d.options.APIKey)
	clientConfig.BaseURL = d.options.BaseURL
	client := openai.NewClientWithConfig(clientConfig)

	_, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: d.options.Model,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleUser, Content: "ping"},
		},
		MaxTokens: 1,
	})
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	return nil
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

	translated, err := streamTranslationRequest(ctx, client, buildTranslationRequest(d.options.Model, sourceText, direction, d.options.SystemPrompt, d.options.Glossary), onToken)
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
	retry, err := streamTranslationRequest(ctx, client, buildMissingTranslationRequest(d.options.Model, missing, direction, d.options.SystemPrompt, d.options.Glossary), onToken)
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

func buildTranslationRequest(model string, sourceText string, direction Direction, customPrompt string, glossary string) openai.ChatCompletionRequest {
	numberedLines := numberedOCRLines(sourceText)
	spec := translationPromptSpecForDirection(direction)
	return openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role: openai.ChatMessageRoleSystem,
				Content: systemPrompt(spec, customPrompt, glossary),
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

func buildMissingTranslationRequest(model string, lines []numberedOCRLine, direction Direction, customPrompt string, glossary string) openai.ChatCompletionRequest {
	spec := translationPromptSpecForDirection(direction)
	return openai.ChatCompletionRequest{
		Model: model,
		Messages: []openai.ChatCompletionMessage{
			{
				Role:    openai.ChatMessageRoleSystem,
				Content: systemPrompt(spec, customPrompt, glossary),
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

// systemPrompt assembles the base translation instructions together with an
// optional user-supplied prompt and glossary.
func systemPrompt(spec translationPromptSpec, customPrompt string, glossary string) string {
	parts := []string{
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
	}
	if glossary := strings.TrimSpace(glossary); glossary != "" {
		parts = append(parts, "Terminology glossary (source term -> target term): use these translations for the listed terms.\n"+glossary)
	}
	if prompt := strings.TrimSpace(customPrompt); prompt != "" {
		parts = append(parts, "Additional user instructions: "+prompt)
	}
	return strings.Join(parts, " ")
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
	"about":         "\u5173\u4e8e",
	"accept":        "\u63a5\u53d7",
	"add":           "\u6dfb\u52a0",
	"advanced":      "\u9ad8\u7ea7",
	"apply":         "\u5e94\u7528",
	"back":          "\u8fd4\u56de",
	"browse":        "\u6d4f\u89c8",
	"cancel":        "\u53d6\u6d88",
	"clear":         "\u6e05\u9664",
	"close":         "\u5173\u95ed",
	"confirm":       "\u786e\u8ba4",
	"copy":          "\u590d\u5236",
	"cut":           "\u526a\u5207",
	"delete":        "\u5220\u9664",
	"deny":          "\u62d2\u7edd",
	"disable":       "\u7981\u7528",
	"download":      "\u4e0b\u8f7d",
	"edit":          "\u7f16\u8f91",
	"enable":        "\u542f\u7528",
	"error":         "\u9519\u8bef",
	"exit":          "\u9000\u51fa",
	"failed":        "\u5931\u8d25",
	"file":          "\u6587\u4ef6",
	"help":          "\u5e2e\u52a9",
	"home":          "\u9996\u9875",
	"ignore":        "\u5ffd\u7565",
	"info":          "\u4fe1\u606f",
	"install":       "\u5b89\u88c5",
	"login":         "\u767b\u5f55",
	"logout":        "\u6ce8\u9500",
	"menu":          "\u83dc\u5355",
	"more":          "\u66f4\u591a",
	"negative":      "\u8d1f\u9762",
	"neutral":       "\u4e2d\u6027",
	"next":          "\u4e0b\u4e00\u6b65",
	"no":            "\u5426",
	"none":          "\u65e0",
	"ok":            "\u786e\u5b9a",
	"open":          "\u6253\u5f00",
	"options":       "\u9009\u9879",
	"paste":         "\u7c98\u8d34",
	"positive":      "\u6b63\u9762",
	"previous":      "\u4e0a\u4e00\u6b65",
	"quit":          "\u9000\u51fa",
	"refresh":       "\u5237\u65b0",
	"remove":        "\u79fb\u9664",
	"rename":        "\u91cd\u547d\u540d",
	"reset":         "\u91cd\u7f6e",
	"retry":         "\u91cd\u8bd5",
	"save":          "\u4fdd\u5b58",
	"search":        "\u641c\u7d22",
	"select":        "\u9009\u62e9",
	"settings":      "\u8bbe\u7f6e",
	"skip":          "\u8df3\u8fc7",
	"start":         "\u5f00\u59cb",
	"stop":          "\u505c\u6b62",
	"submit":        "\u63d0\u4ea4",
	"success":       "\u6210\u529f",
	"test":          "\u6d4b\u8bd5",
	"update":        "\u66f4\u65b0",
	"upload":        "\u4e0a\u4f20",
	"view":          "\u67e5\u770b",
	"warning":       "\u8b66\u544a",
	"yes":           "\u662f",
}

var fastTranslationsToEnglish = map[string]string{
	"about":    "About",
	"accept":   "Accept",
	"add":      "Add",
	"advanced": "Advanced",
	"apply":    "Apply",
	"back":     "Back",
	"browse":   "Browse",
	"cancel":   "Cancel",
	"clear":    "Clear",
	"close":    "Close",
	"confirm":  "Confirm",
	"copy":     "Copy",
	"cut":      "Cut",
	"delete":   "Delete",
	"deny":     "Deny",
	"disable":  "Disable",
	"download": "Download",
	"edit":     "Edit",
	"enable":   "Enable",
	"error":    "Error",
	"exit":     "Exit",
	"failed":   "Failed",
	"file":     "File",
	"help":     "Help",
	"home":     "Home",
	"ignore":   "Ignore",
	"info":     "Info",
	"install":  "Install",
	"login":    "Log in",
	"logout":   "Log out",
	"menu":     "Menu",
	"more":     "More",
	"negative": "Negative",
	"neutral":  "Neutral",
	"next":     "Next",
	"no":       "No",
	"none":     "None",
	"ok":       "OK",
	"open":     "Open",
	"options":  "Options",
	"paste":    "Paste",
	"positive": "Positive",
	"previous": "Previous",
	"quit":     "Quit",
	"refresh":  "Refresh",
	"remove":   "Remove",
	"rename":   "Rename",
	"reset":    "Reset",
	"retry":    "Retry",
	"save":     "Save",
	"search":   "Search",
	"select":   "Select",
	"settings": "Settings",
	"skip":     "Skip",
	"start":    "Start",
	"stop":     "Stop",
	"submit":   "Submit",
	"success":  "Success",
	"test":     "test",
	"update":   "Update",
	"upload":   "Upload",
	"view":     "View",
	"warning":  "Warning",
	"yes":      "Yes",
	"\u5173\u4e8e": "\u5173\u4e8e",
	"\u63a5\u53d7": "Accept",
	"\u6dfb\u52a0": "Add",
	"\u9ad8\u7ea7": "Advanced",
	"\u5e94\u7528": "Apply",
	"\u8fd4\u56de": "Back",
	"\u6d4f\u89c8": "Browse",
	"\u53d6\u6d88": "Cancel",
	"\u6e05\u9664": "Clear",
	"\u5173\u95ed": "Close",
	"\u786e\u8ba4": "Confirm",
	"\u590d\u5236": "Copy",
	"\u526a\u5207": "Cut",
	"\u5220\u9664": "Delete",
	"\u62d2\u7edd": "Deny",
	"\u7981\u7528": "Disable",
	"\u4e0b\u8f7d": "Download",
	"\u7f16\u8f91": "Edit",
	"\u542f\u7528": "Enable",
	"\u9519\u8bef": "Error",
	"\u9000\u51fa": "Exit",
	"\u5931\u8d25": "Failed",
	"\u6587\u4ef6": "File",
	"\u5e2e\u52a9": "Help",
	"\u9996\u9875": "Home",
	"\u5ffd\u7565": "Ignore",
	"\u4fe1\u606f": "Info",
	"\u5b89\u88c5": "Install",
	"\u767b\u5f55": "Log in",
	"\u6ce8\u9500": "Log out",
	"\u83dc\u5355": "Menu",
	"\u66f4\u591a": "More",
	"\u8d1f\u9762": "Negative",
	"\u4e2d\u6027": "Neutral",
	"\u4e0b\u4e00\u6b65": "Next",
	"\u5426":   "No",
	"\u65e0":   "None",
	"\u786e\u5b9a": "OK",
	"\u6253\u5f00": "Open",
	"\u9009\u9879": "Options",
	"\u7c98\u8d34": "Paste",
	"\u6b63\u9762": "Positive",
	"\u4e0a\u4e00\u6b65": "Previous",
	"\u5237\u65b0": "Refresh",
	"\u79fb\u9664": "Remove",
	"\u91cd\u547d\u540d": "Rename",
	"\u91cd\u7f6e": "Reset",
	"\u91cd\u8bd5": "Retry",
	"\u4fdd\u5b58": "Save",
	"\u641c\u7d22": "Search",
	"\u9009\u62e9": "Select",
	"\u8bbe\u7f6e": "Settings",
	"\u8df3\u8fc7": "Skip",
	"\u5f00\u59cb": "Start",
	"\u505c\u6b62": "Stop",
	"\u63d0\u4ea4": "Submit",
	"\u6210\u529f": "Success",
	"\u6d4b\u8bd5": "test",
	"\u66f4\u65b0": "Update",
	"\u4e0a\u4f20": "Upload",
	"\u67e5\u770b": "View",
	"\u8b66\u544a": "Warning",
	"\u662f":   "Yes",
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
