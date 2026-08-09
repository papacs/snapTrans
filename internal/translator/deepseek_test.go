package translator

import (
	"context"
	"errors"
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestRetryStreamStartErrorOnlyRetriesEOFBeforeFirstToken(t *testing.T) {
	require.True(t, shouldRetryStreamStartError("", errors.New(`Post "https://example.test/chat/completions": EOF`)))
	require.True(t, shouldRetryStreamStartError("", errors.New("unexpected EOF")))
	require.False(t, shouldRetryStreamStartError("partial", errors.New("unexpected EOF")))
	require.False(t, shouldRetryStreamStartError("", context.Canceled))
	require.False(t, shouldRetryStreamStartError("", errors.New("401 unauthorized")))
}

func TestBuildTranslationRequestNumbersOCRLinesAndForbidsReadinessReplies(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Neutral\nNegative\nPositive", DirectionToChinese, "", "")

	require.Equal(t, "deepseek-chat", request.Model)
	require.True(t, request.Stream)
	require.Len(t, request.Messages, 2)
	require.Equal(t, openai.ChatMessageRoleSystem, request.Messages[0].Role)
	require.Contains(t, request.Messages[0].Content, "Never ask the user to provide OCR text")
	require.Contains(t, request.Messages[0].Content, "Preserve each [n] prefix")
	require.Equal(t, openai.ChatMessageRoleUser, request.Messages[1].Role)
	require.Contains(t, request.Messages[1].Content, "[1] Neutral")
	require.Contains(t, request.Messages[1].Content, "[2] Negative")
	require.Contains(t, request.Messages[1].Content, "[3] Positive")
	require.NotContains(t, request.Messages[1].Content, "OCR_TEXT_BEGIN")
	require.NotContains(t, request.Messages[1].Content, "OCR_TEXT_END")
}

func TestBuildTranslationRequestRequiresChineseMeaningForEnglishProductNames(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Google Play", DirectionToChinese, "", "")

	require.Contains(t, request.Messages[0].Content, "Do not leave English natural-language text unchanged")
	require.Contains(t, request.Messages[0].Content, "Google Play -> Google Play (\u8c37\u6b4c\u5e94\u7528\u5546\u5e97)")
	require.Contains(t, request.Messages[1].Content, "[1] Google Play")
}

func TestBuildTranslationRequestRequiresShortEnglishWordsToTranslate(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "test", DirectionToChinese, "", "")

	require.Contains(t, request.Messages[0].Content, "Translate short English words")
	require.Contains(t, request.Messages[0].Content, "test -> \u6d4b\u8bd5")
	require.Contains(t, request.Messages[1].Content, "[1] test")
}

func TestBuildTranslationRequestPreservesLineOrderForOCRBlocks(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Neutral\nNegative\nPositive", DirectionToChinese, "", "")

	require.Contains(t, request.Messages[0].Content, "return exactly one output line for every input line")
	require.Contains(t, request.Messages[1].Content, "[1] Neutral\n[2] Negative\n[3] Positive")
}

func TestTryFastTranslationHandlesCommonChartLabels(t *testing.T) {
	translated, ok := TryFastTranslation("Neutral\nNegative\nPositive", DirectionToChinese)

	require.True(t, ok)
	require.Equal(t, "\u4e2d\u6027\n\u8d1f\u9762\n\u6b63\u9762", translated)
}

func TestTryFastTranslationDeclinesMixedUnknownText(t *testing.T) {
	_, ok := TryFastTranslation("DeepSeek V4 Pro\nPositive", DirectionToChinese)

	require.False(t, ok)
}

func TestDetectDirectionChoosesByDominantScript(t *testing.T) {
	require.Equal(t, DirectionToEnglish, DetectDirection("\u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails\uff0c\u4e5f\u53ef\u4ee5\u7528\u6a21\u62df\u622a\u56fe"))
	require.Equal(t, DirectionToChinese, DetectDirection("Neutral\nNegative\nPositive"))
	require.Equal(t, DirectionToChinese, DetectDirection("30\n42\n100"))
	require.Equal(t, DirectionToChinese, DetectDirection(""))
}

func TestNormalizeDirectionSupportsAuto(t *testing.T) {
	require.Equal(t, DirectionAuto, NormalizeDirection("auto"))
	require.Equal(t, DirectionToEnglish, NormalizeDirection("to-en"))
	require.Equal(t, DirectionToChinese, NormalizeDirection("garbage"))
}

func TestLooksLikeMissingOCRRequestDetectsAssistantChatter(t *testing.T) {
	require.True(t, looksLikeMissingOCRRequest("I am ready to assist you. Please provide the OCR text you would like translated into Simplified Chinese."))
	require.False(t, looksLikeMissingOCRRequest("snapTrans.exe"))
}

func TestMissingNumberedOCRLinesReturnsOriginalMissingLines(t *testing.T) {
	missing := missingNumberedOCRLines("Neutral\nNegative\nPositive", "[1] \u4e2d\u6027\n[3] \u6b63\u9762")

	require.Equal(t, []numberedOCRLine{{Index: 2, Text: "Negative"}}, missing)
}

func TestBuildMissingTranslationRequestPreservesOriginalLineNumbers(t *testing.T) {
	request := buildMissingTranslationRequest("deepseek-chat", []numberedOCRLine{{Index: 2, Text: "Negative"}}, DirectionToChinese, "", "")

	require.True(t, request.Stream)
	require.Contains(t, request.Messages[1].Content, "Translate only these missing numbered OCR lines")
	require.Contains(t, request.Messages[1].Content, "[2] Negative")
	require.NotContains(t, request.Messages[1].Content, "[1] Neutral")
}

func TestBuildTranslationRequestSupportsChineseToEnglishDirection(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "\u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails", DirectionToEnglish, "", "")

	require.Contains(t, request.Messages[0].Content, "concise English")
	require.Contains(t, request.Messages[0].Content, "Do not leave Simplified Chinese natural-language text unchanged")
	require.Contains(t, request.Messages[0].Content, "If an input line is already English")
	require.Contains(t, request.Messages[1].Content, "[1] \u5982\u679c\u5c1a\u672a\u5b89\u88c5 Wails")
}

func TestBuildTranslationRequestIncludesCustomPromptAndGlossary(t *testing.T) {
	request := buildTranslationRequest(
		"deepseek-chat",
		"Hello",
		DirectionToChinese,
		"Keep the tone informal.",
		"Hello -> \u60a8\u597d",
	)

	require.Contains(t, request.Messages[0].Content, "Additional user instructions: Keep the tone informal.")
	require.Contains(t, request.Messages[0].Content, "Hello -> \u60a8\u597d")
}

func TestBuildTranslationRequestOmitsGlossaryWhenEmpty(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Hello", DirectionToChinese, "Keep it short.", "")

	require.Contains(t, request.Messages[0].Content, "Keep it short.")
	require.NotContains(t, request.Messages[0].Content, "Terminology glossary")
}

func TestTryFastTranslationKeepsCommonEnglishLabelsWhenTargetIsEnglish(t *testing.T) {
	translated, ok := TryFastTranslation("Neutral\nNegative\nPositive", DirectionToEnglish)

	require.True(t, ok)
	require.Equal(t, "Neutral\nNegative\nPositive", translated)
}

func TestTryFastTranslationCoversCommonUiWords(t *testing.T) {
	translated, ok := TryFastTranslation("Settings\nSearch\nDownload\nCancel", DirectionToChinese)

	require.True(t, ok)
	require.Equal(t, "\u8bbe\u7f6e\n\u641c\u7d22\n\u4e0b\u8f7d\n\u53d6\u6d88", translated)
}

func TestTryFastTranslationChineseToEnglish(t *testing.T) {
	translated, ok := TryFastTranslation("\u8bbe\u7f6e\n\u4fdd\u5b58", DirectionToEnglish)

	require.True(t, ok)
	require.Equal(t, "Settings\nSave", translated)
}

func TestTryFastTranslationDeclinesSentenceText(t *testing.T) {
	_, ok := TryFastTranslation("This is a longer sentence that should go to the API.", DirectionToChinese)

	require.False(t, ok)
}
