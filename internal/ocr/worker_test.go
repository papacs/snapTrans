package ocr

import (
	"context"
	"errors"
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
