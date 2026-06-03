package translator

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestBuildTranslationRequestNumbersOCRLinesAndForbidsReadinessReplies(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Neutral\nNegative\nPositive")

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
	request := buildTranslationRequest("deepseek-chat", "Google Play")

	require.Contains(t, request.Messages[0].Content, "Do not leave English natural-language text unchanged")
	require.Contains(t, request.Messages[0].Content, "Google Play -> Google Play (\u8c37\u6b4c\u5e94\u7528\u5546\u5e97)")
	require.Contains(t, request.Messages[1].Content, "[1] Google Play")
}

func TestBuildTranslationRequestRequiresShortEnglishWordsToTranslate(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "test")

	require.Contains(t, request.Messages[0].Content, "Translate short English words")
	require.Contains(t, request.Messages[0].Content, "test -> \u6d4b\u8bd5")
	require.Contains(t, request.Messages[1].Content, "[1] test")
}

func TestBuildTranslationRequestPreservesLineOrderForOCRBlocks(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Neutral\nNegative\nPositive")

	require.Contains(t, request.Messages[0].Content, "return exactly one output line for every input line")
	require.Contains(t, request.Messages[1].Content, "[1] Neutral\n[2] Negative\n[3] Positive")
}

func TestTryFastTranslationHandlesCommonChartLabels(t *testing.T) {
	translated, ok := TryFastTranslation("Neutral\nNegative\nPositive")

	require.True(t, ok)
	require.Equal(t, "\u4e2d\u6027\n\u8d1f\u9762\n\u6b63\u9762", translated)
}

func TestTryFastTranslationDeclinesMixedUnknownText(t *testing.T) {
	_, ok := TryFastTranslation("DeepSeek V4 Pro\nPositive")

	require.False(t, ok)
}

func TestLooksLikeMissingOCRRequestDetectsAssistantChatter(t *testing.T) {
	require.True(t, looksLikeMissingOCRRequest("I am ready to assist you. Please provide the OCR text you would like translated into Simplified Chinese."))
	require.False(t, looksLikeMissingOCRRequest("snapTrans.exe"))
}
