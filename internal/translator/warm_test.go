package translator

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTranslationRepairRetainsFullContextAndExactWarmInstructions(t *testing.T) {
	sourceText := "Use the same\nAPI key for every model.\nNEXT"
	for _, fast := range []bool{false, true} {
		for _, source := range []string{"", "selection"} {
			client := NewOpenAICompatible(Options{Provider: "codex", Model: "gpt-6-luna", FastTranslation: fast,
				Source: source, Glossary: "API key=API 密钥", SystemPrompt: "Use a formal tone."}).forTranslation()
			first := client.translationRequest(sourceText, DirectionToChinese)
			repair := client.missingTranslationRequest(sourceText, []numberedOCRLine{{Index: 2, Text: "API key for every model."}}, DirectionToChinese)
			require.Equal(t, first.Messages[0], repair.Messages[0], "a repair must retain the same source mode, glossary, and warm session key")
			contextJSON, err := json.Marshal(sourceText)
			require.NoError(t, err)
			require.Contains(t, repair.Messages[1].Content, string(contextJSON))
			require.Equal(t, map[int]bool{2: true}, parseNumberedTranslationIndices(repair.Messages[1].Content), "only the missing ID is a numbered translation target")
			require.NotContains(t, repair.Messages[1].Content, "[1] Use the same")
			require.NotContains(t, repair.Messages[1].Content, "[3] NEXT")
		}
	}
}

func TestTranslationWarmInstructionsMatchActualRequestWithoutSourceText(t *testing.T) {
	for _, fast := range []bool{false, true} {
		for _, source := range []string{"", "selection"} {
			for _, direction := range []Direction{DirectionToChinese, DirectionToEnglish} {
				options := Options{Provider: "codex", Model: "gpt-6-luna", FastTranslation: fast,
					CodexReasoningEffort: "high", Source: source, Glossary: "release=发布", SystemPrompt: "Use a formal tone."}
				original := NewOpenAICompatible(options)
				originalOptions := original.options
				client := original.forTranslation()
				warm := client.translationRequest("", direction)
				actual := client.translationRequest("PRIVATE_CAPTURE_TEXT\nA second captured line", direction)
				require.Equal(t, actual.Model, warm.Model)
				require.Equal(t, actual.Messages[0], warm.Messages[0])
				require.NotContains(t, warm.Messages[0].Content, "PRIVATE_CAPTURE_TEXT")
				require.Contains(t, warm.Messages[0].Content, "release=发布")
				require.Contains(t, warm.Messages[0].Content, "Use a formal tone.")
				require.NotEqual(t, actual.Messages[1].Content, warm.Messages[1].Content)
				require.Equal(t, originalOptions, original.options, "warming must not mutate settings")
				if fast {
					require.Equal(t, "low", client.options.CodexReasoningEffort)
				} else {
					require.Equal(t, "high", client.options.CodexReasoningEffort)
				}
			}
		}
	}
}

func TestTranslationWarmInstructionsChangeWithDirectionAndSourceMode(t *testing.T) {
	client := NewOpenAICompatible(Options{Provider: "codex", FastTranslation: true})
	chinese := client.translationRequest("", DirectionToChinese).Messages[0].Content
	english := client.translationRequest("", DirectionToEnglish).Messages[0].Content
	require.NotEqual(t, chinese, english)
	client.options.Source = "selection"
	require.Equal(t, chinese, client.translationRequest("", DirectionToChinese).Messages[0].Content)
	client.options.FastTranslation = false
	selected := client.translationRequest("", DirectionToChinese).Messages[0].Content
	client.options.Source = ""
	require.NotEqual(t, selected, client.translationRequest("", DirectionToChinese).Messages[0].Content)
}

func TestAPITranslationWarmupOnlyRequestsModels(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests++
		require.Equal(t, http.MethodGet, request.Method)
		require.Equal(t, "/models", request.URL.Path)
		require.Equal(t, int64(0), request.ContentLength)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL})
	require.NoError(t, client.WarmTranslation(context.Background(), DirectionToChinese))
	require.Equal(t, 1, requests)
}
