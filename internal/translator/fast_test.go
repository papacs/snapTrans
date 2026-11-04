package translator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFastTranslationUsesSupportedLunaEffortWithoutChangingReplyEffort(t *testing.T) {
	for _, model := range []string{"gpt-6-luna", "gpt-5.6-luna"} {
		client := NewOpenAICompatible(Options{Provider: "codex", Model: model, CodexReasoningEffort: "high", FastTranslation: true})
		require.Equal(t, "low", client.forTranslation().options.CodexReasoningEffort)
		require.Equal(t, "high", client.options.CodexReasoningEffort)
	}
	for _, options := range []Options{
		{Provider: "codex", Model: "unknown-model", CodexReasoningEffort: "high", FastTranslation: true},
		{Provider: "codex", CodexReasoningEffort: "high", FastTranslation: true},
		{Provider: "codex", Model: "gpt-6-luna", CodexReasoningEffort: "high"},
		{Provider: "openai-compatible", Model: "gpt-6-luna", CodexReasoningEffort: "high", FastTranslation: true},
	} {
		require.Equal(t, "high", NewOpenAICompatible(options).forTranslation().options.CodexReasoningEffort)
	}
}

func TestFastRequestKeepsTermsContextDirectionAndOriginalIDs(t *testing.T) {
	client := NewOpenAICompatible(Options{Provider: "codex", FastTranslation: true, Glossary: "release=发布", SystemPrompt: "Use a formal tone."})
	request := buildTranslationRequest("gpt-6-luna", "release", DirectionToChinese, "", "")
	request = client.prepareTranslationRequest(request, DirectionToChinese, "[2] release")
	require.Equal(t, "[2] release", request.Messages[1].Content)
	for _, required := range []string{"Simplified Chinese", "each [n] ID", "never merge, omit or reorder", "adjacent lines", "untrusted data", "release=发布", "formal tone"} {
		require.Contains(t, request.Messages[0].Content, required)
	}
	request = client.prepareTranslationRequest(request, DirectionToEnglish, "[1] 发布")
	require.Contains(t, request.Messages[0].Content, "concise English")
}
