package translator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openai "github.com/sashabaranov/go-openai"
	"github.com/stretchr/testify/require"
)

func writeBoundaryChunk(w http.ResponseWriter, text, finish string) {
	w.Header().Set("Content-Type", "text/event-stream")
	chunk, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]string{"content": text}, "finish_reason": finish}}})
	fmt.Fprintf(w, "data: %s\n\n", chunk)
}

func TestNumberedCoverageMatchesVisibleFrontendSyntax(t *testing.T) {
	source := "one\ntwo\nthree\nfour\nfive\nsix\nseven"
	output := "\ufeff\"[1] 第一项\"\n2. 第二项\n3) 第三项\n4、第四项\n5:第五项\n6：第六项\n7-第七项"
	require.Empty(t, missingNumberedOCRLines(source, output))
	for _, body := range []string{`""`, "“”", "```", "OCR_TEXT_BEGIN", "OCR TEXT END", "---BEGIN---", "__END__", "```END", `OCR_TEXT_BEGIN "" OCR_TEXT_END`} {
		require.Equal(t, []numberedOCRLine{{Index: 1, Text: "one"}}, missingNumberedOCRLines("one", "[1] "+body), "a visually empty body is still missing")
	}
	require.Empty(t, missingNumberedOCRLines("one", `OCR_TEXT_BEGIN "[1] 第一项" OCR_TEXT_END`), "marker removal may expose enclosing quotes")
	require.Empty(t, missingNumberedOCRLines("one\ntwo", "[1] Begin\n[2] End"), "Begin and End can be valid translated labels")
	require.Empty(t, missingNumberedOCRLines("one\ntwo", "[1] Begin :\n[2] \"End :\""), "a single punctuation mark and whitespace are not decorated separators")
	for _, separator := range []string{"\n", "\r\n", "\r"} {
		require.Empty(t, missingNumberedOCRLines("one\ntwo", "[1]第一行"+separator+"[2]第二行"), "all supported response line endings must keep their region boundaries")
	}
}

func TestNumberedQuotedReadinessTextIsAValidTranslation(t *testing.T) {
	require.False(t, looksLikeMissingOCRRequest("[1] I am ready to assist."))
	require.True(t, looksLikeMissingOCRRequest("I am ready to assist. Please provide the OCR text."))
}

func TestMalformedNumberedBodiesAreNeverMemoized(t *testing.T) {
	for _, body := range []string{`""`, "```", "OCR_TEXT_BEGIN", `OCR_TEXT_BEGIN "" OCR_TEXT_END`} {
		t.Run(body, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				writeBoundaryChunk(w, "[1] "+body, "stop")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
			for i := 0; i < 2; i++ {
				require.ErrorContains(t, client.Translate(context.Background(), "A region still needs a real translation.", DirectionToChinese, func(string) {}), "incomplete")
			}
			require.Equal(t, 4, calls, "each call may repair once, but unusable output must never be cached")
		})
	}
}

func TestCompletedHTTPStreamDoesNotWaitForHungTerminator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBoundaryChunk(w, "[1] 完整译文", "stop")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var output string
	err := client.Translate(ctx, "Complete translation before stream cleanup.", DirectionToChinese, func(token string) { output += token })
	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "cleanup must finish before the overall request deadline")
	require.Equal(t, "[1] 完整译文", output)
}

func TestEmptyCompletedStreamsAreErrorsWithoutRepairsOrCaching(t *testing.T) {
	for _, body := range []string{"", " \n\t"} {
		for _, action := range []string{"translation", "reply", "explain"} {
			t.Run(fmt.Sprintf("%q/%s", body, action), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					writeBoundaryChunk(w, body, "stop")
					fmt.Fprint(w, "data: [DONE]\n\n")
				}))
				defer server.Close()
				client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
				for i := 0; i < 2; i++ {
					var err error
					switch action {
					case "translation":
						err = client.Translate(context.Background(), "A real sentence without model output.", DirectionToChinese, func(string) {})
					case "reply":
						err = client.Reply(context.Background(), "Hello.", "general", "", "en", func(string) {})
					default:
						err = client.TextAction(context.Background(), "Hello.", "explain", "en", func(string) {})
					}
					require.ErrorContains(t, err, "no text")
				}
				require.Equal(t, 2, calls)
			})
		}
	}
}

func TestEOFTextInHTTPErrorDoesNotRetryTheAPIRequest(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":{"message":"upstream unexpected EOF","type":"rate_limit_error"}}`)
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	err := client.Translate(context.Background(), "A request rejected by a gateway.", DirectionToChinese, func(string) {})
	var apiErr *openai.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusTooManyRequests, apiErr.HTTPStatusCode)
	require.Equal(t, 1, calls)
	require.False(t, shouldRetryStreamStartError("openai-compatible", "", &openai.RequestError{HTTPStatusCode: 503, Err: io.ErrUnexpectedEOF}))
}

func TestWhitespaceAlreadyEmittedIsNotRetried(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeBoundaryChunk(w, " ", "")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	var output string
	err := client.Translate(context.Background(), "Keep the stream retry boundary exact.", DirectionToChinese, func(token string) { output += token })
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Equal(t, " ", output)
	require.Equal(t, 1, calls)
}

func TestHTTPStreamCancellationAfterFinalTokenIsNotSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBoundaryChunk(w, "A complete reply.", "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := client.Reply(ctx, "Hello.", "general", "", "en", func(string) { cancel() })
	require.ErrorIs(t, err, context.Canceled)
}

func TestCachedTextTranslationReturnsCancellation(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		writeBoundaryChunk(w, "[1] 你好", "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	require.NoError(t, client.Translate(context.Background(), "Hello.", DirectionToChinese, func(string) {}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.ErrorIs(t, client.Translate(ctx, "Hello.", DirectionToChinese, func(string) { cancel() }), context.Canceled)
	require.Equal(t, 1, calls)
}

func TestVisionCancellationDuringDirectionDoesNotEmitTranslation(t *testing.T) {
	for _, cached := range []bool{false, true} {
		t.Run(fmt.Sprintf("cached=%t", cached), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				writeBoundaryChunk(w, "[[to-zh]]\n你好", "stop")
				fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "vision-fixture"})
			image := testScreenshot(t)
			if cached {
				require.NoError(t, client.TranslateImage(context.Background(), image, DirectionAuto, func(Direction) {}, func(string) {}))
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var output string
			err := client.TranslateImage(ctx, image, DirectionAuto, func(Direction) { cancel() }, func(token string) { output += token })
			require.ErrorIs(t, err, context.Canceled)
			require.Empty(t, output)
			require.Equal(t, 1, calls)
		})
	}
}

func TestWarmReportsHTTPFailuresWithoutResponseBody(t *testing.T) {
	for _, status := range []int{401, 429, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "untrusted response body", status)
			}))
			defer server.Close()
			client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL})
			err := client.Warm(context.Background())
			require.ErrorContains(t, err, fmt.Sprint(status))
			require.NotContains(t, err.Error(), "untrusted response body")
		})
	}
}

func TestCompletedHTTPStreamPreservesExplicitTailErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBoundaryChunk(w, "A reply.", "stop")
		fmt.Fprint(w, "data: {\"error\":{\"message\":\"provider terminal error\"}}\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	err := client.Reply(context.Background(), "Hello.", "general", "", "en", func(string) {})
	require.ErrorContains(t, err, "provider terminal error")
}

func TestCompletedHTTPStreamIgnoresAdditionalContentAfterStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeBoundaryChunk(w, "A complete reply.", "stop")
		writeBoundaryChunk(w, "Unexpected content after completion.", "")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	client := NewOpenAICompatible(Options{APIKey: "fixture", BaseURL: server.URL, Model: "fixture"})
	var output string
	require.NoError(t, client.Reply(context.Background(), "Hello.", "general", "", "en", func(token string) { output += token }))
	require.Equal(t, "A complete reply.", output)
}

func TestStreamRetryDoesNotTreatCancellationAsEOF(t *testing.T) {
	require.False(t, shouldRetryStreamStartError("openai-compatible", "", fmt.Errorf("unexpected EOF: %w", context.Canceled)))
	require.False(t, shouldRetryStreamStartError("openai-compatible", "\n", errors.New("unexpected EOF")))
	require.False(t, shouldRetryStreamStartError("openai-compatible", "", &openai.APIError{Message: "unexpected EOF"}))
	require.True(t, shouldRetryStreamStartError("openai-compatible", "", fmt.Errorf("network stream: %w", io.ErrUnexpectedEOF)))
}
