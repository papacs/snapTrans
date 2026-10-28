package translator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFastTranslationDoesNotChangeReplyEffortOrUnknownModelCapabilities(t *testing.T) {
	client := NewOpenAICompatible(Options{Provider: "codex", Model: "gpt-6-luna", CodexReasoningEffort: "high", FastTranslation: true})
	require.Equal(t, "none", client.forTranslation().options.CodexReasoningEffort)
	require.Equal(t, "high", client.options.CodexReasoningEffort)
	for _, options := range []Options{
		{Provider: "codex", Model: "unknown-model", CodexReasoningEffort: "high", FastTranslation: true},
		{Provider: "codex", Model: "gpt-6-luna", CodexReasoningEffort: "high"},
		{Provider: "openai-compatible", Model: "gpt-6-luna", CodexReasoningEffort: "high", FastTranslation: true},
	} {
		require.Equal(t, "high", NewOpenAICompatible(options).forTranslation().options.CodexReasoningEffort)
	}
}

func TestFastRequestKeepsTermsContextDirectionAndOriginalIDs(t *testing.T) {
	client := NewOpenAICompatible(Options{Provider: "codex", FastTranslation: true, Glossary: "release=发布", SystemPrompt: "Use a formal tone."})
	request := buildMissingTranslationRequest("gpt-6-luna", []numberedOCRLine{{Index: 2, Text: "release"}}, DirectionToChinese, "", "")
	request = client.prepareTranslationRequest(request, DirectionToChinese, "[2] release")
	require.Equal(t, "[2] release", request.Messages[1].Content)
	for _, required := range []string{"Simplified Chinese", "each [n] ID", "never merge, omit or reorder", "adjacent lines", "untrusted data", "release=发布", "formal tone"} {
		require.Contains(t, request.Messages[0].Content, required)
	}
	request = client.prepareTranslationRequest(request, DirectionToEnglish, "[1] 发布")
	require.Contains(t, request.Messages[0].Content, "concise English")
}
