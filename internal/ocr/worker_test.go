package ocr

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReadOneResultSkipsInitBanner(t *testing.T) {
	reader := strings.NewReader("RapidOCR-json v1.1.0\r\nOCR init completed.\r\n" +
		`{"code":100,"data":[{"box":[[31,40],[157,44],[156,93],[29,89]],"score":0.99,"text":"Hello"}]}` + "\n")

	raw, err := readOneResult(reader)

	require.NoError(t, err)
	require.Contains(t, string(raw), "Hello")
}

func TestReadOneResultReturnsFirstJsonLine(t *testing.T) {
	reader := strings.NewReader("ignored line\n" + `{"code":200,"data":"ok"}` + "\n" + "trailing\n")

	raw, err := readOneResult(reader)

	require.NoError(t, err)
	require.Equal(t, `{"code":200,"data":"ok"}`, string(raw))
}

func TestReadOneResultAccumulatesMultilineJson(t *testing.T) {
	reader := strings.NewReader("OCR init completed.\n{\n  \"code\": 100,\n  \"data\": [\n    {\"text\": \"Hello\"}\n  ]\n}\n")

	raw, err := readOneResult(reader)

	require.NoError(t, err)
	require.Contains(t, string(raw), "Hello")
}

func TestReadOneResultSkipsNonJsonPrefixLines(t *testing.T) {
	reader := strings.NewReader("banner line\n" + `{"code":100,"data":[{"text":"ok"}]}` + "\n")

	raw, err := readOneResult(reader)

	require.NoError(t, err)
	require.Contains(t, string(raw), "ok")
}

func TestJSONBalancedIgnoresBracesInsideStrings(t *testing.T) {
	require.True(t, jsonBalanced(`{"text":"a{b}c","box":[1,2]}`))
	require.True(t, jsonBalanced(`{"text":"a}b"}`))
	require.False(t, jsonBalanced(`{"text":"unfinished"`))
	require.False(t, jsonBalanced(`{"text":"a}b"`))
}

func TestReadOneResultReportsEOFWithoutResult(t *testing.T) {
	_, err := readOneResult(strings.NewReader("nothing useful here\n"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "process exited")
}

func TestReadOneResultPreservesTextThatLooksLikeTheStartupBanner(t *testing.T) {
	for _, raw := range []string{
		`{"code":100,"data":[{"text":"OCR init completed."}]}`,
		"{\n\"code\":100,\n\"data\":[{\"text\":\"init completed successfully\"}]\n}",
	} {
		actual, err := readOneResult(strings.NewReader("OCR init completed.\n" + raw + "\n"))
		require.NoError(t, err, "recognized text must never be mistaken for engine logging")
		result, err := ExtractResultFromJSON(actual, 100, 100)
		require.NoError(t, err)
		require.Contains(t, result.Text, "init completed")
	}
}

func TestWorkerRunParsesImageDimensionsForBlockNormalization(t *testing.T) {
	worker := NewRapidOCRWorker("", time.Second)
	require.NotNil(t, worker)
	require.Equal(t, time.Second, worker.timeout)
}

func TestWorkerRunGateSerializesRequestsAndHonorsCancellation(t *testing.T) {
	worker := NewRapidOCRWorker("", time.Second)
	require.NoError(t, worker.acquireRun(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, worker.acquireRun(ctx), context.Canceled)

	worker.releaseRun()
	require.NoError(t, worker.acquireRun(context.Background()))
	worker.releaseRun()
}

func TestWorkerStartGateDoesNotLeakWhenWaitingContextIsCancelled(t *testing.T) {
	worker := NewRapidOCRWorker("", time.Second)
	require.NoError(t, worker.acquireStart(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.True(t, errors.Is(worker.acquireStart(ctx), context.Canceled))

	worker.releaseStart()
	require.NoError(t, worker.acquireStart(context.Background()))
	worker.releaseStart()
}

func TestCancelledWorkerRequestDoesNotAcquireAvailableGate(t *testing.T) {
	gate := newWorkerGate()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for i := 0; i < 64; i++ {
		err := acquireWorkerGate(ctx, gate)
		if err == nil {
			releaseWorkerGate(gate)
		}
		require.ErrorIs(t, err, context.Canceled)
		require.Len(t, gate, 1)
	}
}

type testOCRInput struct {
	io.Writer
	io.Closer
}

func TestWorkerOwnTimeoutPreservesDeadlineIdentity(t *testing.T) {
	reader, writer := io.Pipe()
	defer reader.Close()
	worker := NewRapidOCRWorker("", 30*time.Millisecond)
	worker.ready = true
	worker.stdin = testOCRInput{Writer: io.Discard, Closer: writer}
	worker.stdout = reader
	defer worker.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := worker.Run(ctx, "aW1hZ2U=")
	require.ErrorIs(t, err, context.DeadlineExceeded, "a worker timeout must not trigger another expensive fallback")
	require.NoError(t, ctx.Err(), "the worker's deadline must be distinguishable from the outer deadline")
}
