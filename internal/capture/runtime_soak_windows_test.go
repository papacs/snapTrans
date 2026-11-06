//go:build windows

package capture

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// Opt-in component soak: real desktop pixels stay in this test process and are
// never saved, logged or sent anywhere. Normal unit-test runs skip this work.
func requireCaptureSoak(t *testing.T, phase string) {
	t.Helper()
	mode := os.Getenv("SNAPTRANS_CAPTURE_SOAK")
	if mode != "all" && mode != phase {
		t.Skip("set SNAPTRANS_CAPTURE_SOAK=" + phase + " to run this bounded runtime check")
	}
}

type soakTiming struct {
	Count  int     `json:"count"`
	MeanMS float64 `json:"meanMs"`
	P50MS  float64 `json:"p50Ms"`
	P95MS  float64 `json:"p95Ms"`
	MaxMS  float64 `json:"maxMs"`
}

func summarizeSoakDurations(values []time.Duration) soakTiming {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var total time.Duration
	for _, value := range ordered {
		total += value
	}
	return soakTiming{
		Count: len(values), MeanMS: float64(total) / float64(len(values)) / float64(time.Millisecond),
		P50MS: float64(ordered[(len(ordered)-1)/2]) / float64(time.Millisecond),
		P95MS: float64(ordered[(len(ordered)-1)*95/100]) / float64(time.Millisecond),
		MaxMS: float64(ordered[len(ordered)-1]) / float64(time.Millisecond),
	}
}

type soakMemory struct {
	Iteration int    `json:"iteration"`
	HeapBytes uint64 `json:"heapBytes"`
	HeapSys   uint64 `json:"heapSysBytes"`
	GDI       uint64 `json:"gdiObjects"`
}

func captureSoakMemory(iteration int) soakMemory {
	runtime.GC()
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	getGUIResources := windows.NewLazySystemDLL("user32.dll").NewProc("GetGuiResources")
	gdi, _, _ := getGUIResources.Call(^uintptr(0), 0)
	return soakMemory{Iteration: iteration, HeapBytes: memory.HeapAlloc, HeapSys: memory.HeapSys, GDI: uint64(gdi)}
}

func logCaptureSoak(t *testing.T, phase string, result any) {
	t.Helper()
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	t.Logf("CAPTURE_SOAK_%s %s", phase, encoded)
}

func TestDesktopCaptureEncodingRuntimeSoak(t *testing.T) {
	requireCaptureSoak(t, "native")
	monitors, err := physicalMonitors()
	require.NoError(t, err)
	require.NotEmpty(t, monitors)
	monitor := monitors[0]
	if point, err := cursorPosition(); err == nil {
		if active, found := monitorForPoint(monitors, point); found {
			monitor = active
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	captureTimes, encodeTimes, decodeTimes := []time.Duration{}, []time.Duration{}, []time.Duration{}
	memory := []soakMemory{}
	encodedSizes := []int{}
	for index := -2; index < 40; index++ {
		captureTime, encodeTime, decodeTime, size := runtimeDesktopCaptureOnce(t, ctx, monitor)
		if index < 0 {
			continue
		}
		captureTimes = append(captureTimes, captureTime)
		encodeTimes = append(encodeTimes, encodeTime)
		decodeTimes = append(decodeTimes, decodeTime)
		encodedSizes = append(encodedSizes, size)
		if index == 0 || (index+1)%10 == 0 {
			memory = append(memory, captureSoakMemory(index+1))
		}
	}
	logCaptureSoak(t, "native", map[string]any{
		"width": monitor.Rect.Dx(), "height": monitor.Rect.Dy(), "iterations": 40,
		"capture": summarizeSoakDurations(captureTimes), "encode": summarizeSoakDurations(encodeTimes),
		"decode": summarizeSoakDurations(decodeTimes), "encodedBytes": encodedSizes, "memory": memory,
		"pixelsVerified": true, "desktopImagesSaved": false,
	})
	require.LessOrEqual(t, int64(memory[len(memory)-1].HeapBytes)-int64(memory[0].HeapBytes), int64(32<<20), "repeated captures must release frames and PNG outputs")
	require.LessOrEqual(t, int64(memory[len(memory)-1].GDI)-int64(memory[0].GDI), int64(4), "desktop capture must release native GDI resources")
}

func runtimeDesktopCaptureOnce(t *testing.T, ctx context.Context, monitor physicalMonitor) (time.Duration, time.Duration, time.Duration, int) {
	t.Helper()
	result, err := captureMonitors(ctx, []physicalMonitor{monitor})
	require.NoError(t, err)
	decodeStarted := time.Now()
	decoded, err := png.Decode(bytes.NewReader(result.ImageBytes))
	require.NoError(t, err)
	decodeDuration := time.Since(decodeStarted)
	source, ok := result.Frame.(*image.RGBA)
	require.True(t, ok)
	actual, ok := decoded.(*image.RGBA)
	require.True(t, ok)
	require.Equal(t, source.Bounds().Size(), actual.Bounds().Size())
	for row := 0; row < source.Bounds().Dy(); row++ {
		width := source.Bounds().Dx() * 4
		require.True(t, bytes.Equal(source.Pix[row*source.Stride:row*source.Stride+width], actual.Pix[row*actual.Stride:row*actual.Stride+width]), "lossless desktop PNG pixels differ")
	}
	return result.CaptureDuration, result.EncodeDuration, decodeDuration, len(result.ImageBytes)
}

func TestManualScrollProductionRuntimeSoak(t *testing.T) {
	requireCaptureSoak(t, "scroll")
	const width, viewportHeight, advance, observations = 720, 600, 13, 180
	content := sparseScrollText(width, viewportHeight+observations*advance+1)
	times := []time.Duration{}
	memory := []soakMemory{captureSoakMemory(0)}
	for cycle := 0; cycle < 3; cycle++ {
		runtimeScrollSessionOnce(t, content, width, viewportHeight, advance, observations, &times)
		memory = append(memory, captureSoakMemory((cycle+1)*observations))
		t.Logf("CAPTURE_SOAK_scroll_progress sessions=%d observations=%d", cycle+1, (cycle+1)*observations)
	}
	logCaptureSoak(t, "scroll", map[string]any{
		"sessions": 3, "observations": 3 * observations, "viewportWidth": width, "viewportHeight": viewportHeight,
		"productionMaxFrames": defaultScrollFrames, "productionMaxPixels": defaultScrollPixels,
		"acceptedFramesPerSession": defaultScrollFrames, "step": summarizeSoakDurations(times), "memory": memory,
		"pixelsVerified": true, "previewVerified": true,
	})
	require.LessOrEqual(t, int64(memory[len(memory)-1].HeapBytes)-int64(memory[0].HeapBytes), int64(16<<20), "finished scrolling sessions must release their strips and output")
	runtime.KeepAlive(content)
}

func runtimeScrollSessionOnce(t *testing.T, content *image.RGBA, width, height, advance, observations int, times *[]time.Duration) {
	t.Helper()
	offset, captured := 0, 0
	session, err := startManualScrollCapture(context.Background(), image.Rect(0, 0, width, height), ManualScrollOptions{}, func(image.Rectangle) (image.Image, error) {
		frame := content.SubImage(image.Rect(0, offset, width, offset+height))
		offset += advance
		captured++
		return frame, nil
	})
	require.NoError(t, err)
	defer session.Cancel()
	for index := 0; index < observations; index++ {
		started := time.Now()
		step, err := session.CaptureNext()
		if index < defaultScrollFrames-1 {
			*times = append(*times, time.Since(started))
		}
		require.NoError(t, err)
		if index < defaultScrollFrames-1 {
			require.True(t, step.Appended, "text movement was lost at sample %d", index)
			require.Equal(t, height+(index+1)*advance, step.Height, "scroll distance drifted at sample %d", index)
			if index%30 == 0 {
				preview, err := png.Decode(bytes.NewReader(step.PreviewImageBytes))
				require.NoError(t, err)
				expected := session.stitcher.thumbnail(240, 4096)
				assertSoakPixels(t, expected, preview)
			}
		} else {
			require.True(t, step.LimitReached)
			require.False(t, step.Appended)
		}
	}
	require.Equal(t, defaultScrollFrames, captured, "capture work must stop at the production cap")
	result, err := session.Finish()
	require.NoError(t, err)
	require.Equal(t, defaultScrollFrames, result.ScrollFrames)
	expectedHeight := height + (defaultScrollFrames-1)*advance
	require.Equal(t, expectedHeight, result.Height)
	actual, err := png.Decode(bytes.NewReader(result.ImageBytes))
	require.NoError(t, err)
	assertSoakPixels(t, content.SubImage(image.Rect(0, 0, width, expectedHeight)).(*image.RGBA), actual)
	storedPixels := 0
	for _, strip := range session.stitcher.frames {
		storedPixels += len(strip.Pix) / 4
	}
	require.Equal(t, width*expectedHeight, storedPixels, "store the output once rather than full viewports per sample")
}

func assertSoakPixels(t *testing.T, expected *image.RGBA, actual image.Image) {
	t.Helper()
	rgba, ok := actual.(*image.RGBA)
	require.True(t, ok)
	require.Equal(t, expected.Rect.Size(), rgba.Rect.Size())
	width := expected.Rect.Dx() * 4
	for row := 0; row < expected.Rect.Dy(); row++ {
		require.True(t, bytes.Equal(expected.Pix[row*expected.Stride:row*expected.Stride+width], rgba.Pix[row*rgba.Stride:row*rgba.Stride+width]), "scroll pixels differ at row %d", row)
	}
}

func TestCanceledCaptureEncodingRuntimeSoak(t *testing.T) {
	requireCaptureSoak(t, "cancel")
	frame := patternedScrollContent(1920, 1080)
	times := []time.Duration{}
	for iteration := 0; iteration < 24; iteration++ {
		ctx, cancel := context.WithCancel(context.Background())
		started := make(chan struct{})
		observed := &soakEncodingContext{Context: ctx, started: started}
		finished := make(chan error, 1)
		go func() {
			output, err := encodePNGBytesContext(observed, frame, png.BestSpeed)
			if err == nil && len(output) > 0 {
				finished <- nil
				return
			}
			finished <- err
		}()
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			cancel()
			t.Fatal("PNG encoding did not start")
		}
		cancelStarted := time.Now()
		cancel()
		select {
		case err := <-finished:
			times = append(times, time.Since(cancelStarted))
			require.ErrorIs(t, err, context.Canceled)
		case <-time.After(5 * time.Second):
			t.Fatal("canceled capture kept encoding")
		}
	}
	logCaptureSoak(t, "cancel", map[string]any{"iterations": len(times), "cancelToReturn": summarizeSoakDurations(times), "partialImagesPublished": 0})
}

type soakEncodingContext struct {
	context.Context
	checks  int
	started chan struct{}
	once    sync.Once
}

func (ctx *soakEncodingContext) Err() error {
	ctx.checks++
	if ctx.checks >= 2 {
		ctx.once.Do(func() { close(ctx.started) })
	}
	return ctx.Context.Err()
}

func BenchmarkScrollMatcherSparse720(b *testing.B) {
	content := sparseScrollText(720, 613)
	previous := content.SubImage(image.Rect(0, 0, 720, 600))
	next := content.SubImage(image.Rect(0, 13, 720, 613))
	for _, candidate := range []struct {
		name  string
		match func(image.Image, image.Image) (int, float64, bool)
	}{{"reference", referenceScrollAdvance}, {"rgba", verticalAdvance}} {
		b.Run(candidate.name, func(b *testing.B) {
			b.ReportAllocs()
			for index := 0; index < b.N; index++ {
				advance, _, found := candidate.match(previous, next)
				if !found || advance != 13 {
					b.Fatalf("lost exact overlap: advance=%d found=%v", advance, found)
				}
			}
		})
	}
}

func referenceScrollAdvance(previous, next image.Image) (int, float64, bool) {
	height := previous.Bounds().Dy()
	bestAdvance, bestScore := 0, 1e9
	for advance := 1; advance <= height-maxInt(12, height/10); advance++ {
		score := sampledDifferenceKeepingRowsGeneric(previous, next, advance, 7)
		if score < bestScore {
			bestAdvance, bestScore = advance, score
		}
	}
	return bestAdvance, bestScore, bestAdvance > 0
}
