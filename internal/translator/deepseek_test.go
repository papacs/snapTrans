package translator

import (
	"testing"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func TestBuildTranslationRequestWrapsOCRTextAndForbidsReadinessReplies(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "snapTrans.exe")

	require.Equal(t, "deepseek-chat", request.Model)
	require.True(t, request.Stream)
	require.Len(t, request.Messages, 2)
	require.Equal(t, openai.ChatMessageRoleSystem, request.Messages[0].Role)
	require.Contains(t, request.Messages[0].Content, "Never ask the user to provide OCR text")
	require.Equal(t, openai.ChatMessageRoleUser, request.Messages[1].Role)
	require.Contains(t, request.Messages[1].Content, "OCR_TEXT_BEGIN")
	require.Contains(t, request.Messages[1].Content, "snapTrans.exe")
	require.Contains(t, request.Messages[1].Content, "OCR_TEXT_END")
}

func TestBuildTranslationRequestRequiresChineseMeaningForEnglishProductNames(t *testing.T) {
	request := buildTranslationRequest("deepseek-chat", "Google Play")

	require.Contains(t, request.Messages[0].Content, "Do not leave English natural-language text unchanged")
	require.Contains(t, request.Messages[0].Content, "Google Play -> Google Play（谷歌应用商店）")
	require.Contains(t, request.Messages[1].Content, "Google Play")
}

func TestLooksLikeMissingOCRRequestDetectsAssistantChatter(t *testing.T) {
	require.True(t, looksLikeMissingOCRRequest("I am ready to assist you. Please provide the OCR text you would like translated into Simplified Chinese."))
	require.False(t, looksLikeMissingOCRRequest("snapTrans.exe"))
}
