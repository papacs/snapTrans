//go:build ignore

// Explicit Windows-only diagnostics. Build this file directly; it is excluded
// from go test ./.... Only metrics are emitted, never captured pixel data.
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lxn/win"
	"golang.org/x/sys/windows"
	"snaptrans/internal/capture"
)

var (
	captureDiagnostics bool
	captureOracle      bool
	knownNotepadPID    uint
	user32             = windows.NewLazySystemDLL("user32.dll")
	setDPI             = user32.NewProc("SetThreadDpiAwarenessContext")
	getDPI             = user32.NewProc("GetThreadDpiAwarenessContext")
	getWindowDPI       = user32.NewProc("GetDpiForWindow")
	equalDPI           = user32.NewProc("AreDpiAwarenessContextsEqual")
	enumMonitors       = user32.NewProc("EnumDisplayMonitors")
	enumWindows        = user32.NewProc("EnumWindows")
	getMonitor         = user32.NewProc("GetMonitorInfoW")
	enumMode           = user32.NewProc("EnumDisplaySettingsW")
	guiResources       = user32.NewProc("GetGuiResources")
	setLastError       = windows.NewLazySystemDLL("kernel32.dll").NewProc("SetLastError")
)

type monitorInfo struct {
	win.MONITORINFO
	Device [32]uint16
}

type monitorMode struct {
	Device string `json:"device"`
	Left   int    `json:"left"`
	Top    int    `json:"top"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (mode monitorMode) rect() image.Rectangle {
	return image.Rect(mode.Left, mode.Top, mode.Left+mode.Width, mode.Top+mode.Height)
}

type timings struct {
	Count  int     `json:"count"`
	MeanMS float64 `json:"meanMs"`
	P50MS  float64 `json:"p50Ms"`
	P95MS  float64 `json:"p95Ms"`
	MaxMS  float64 `json:"maxMs"`
}

func summarize(values []time.Duration) timings {
	if len(values) == 0 {
		return timings{}
	}
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	var total time.Duration
	for _, value := range ordered {
		total += value
	}
	return timings{len(ordered), float64(total) / float64(len(ordered)) / 1e6,
		float64(ordered[(len(ordered)-1)/2]) / 1e6, float64(ordered[(len(ordered)-1)*95/100]) / 1e6,
		float64(ordered[len(ordered)-1]) / 1e6}
}

func emit(event string, data any) {
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"event": event, "utc": time.Now().UTC().Format(time.RFC3339Nano), "data": data})
}

func main() {
	mode := flag.String("mode", "inspect", "inspect, native, or scroll; inspect never captures pixels")
	iterations := flag.Int("iterations", 40, "native captures, from 1 to 60")
	rectValue := flag.String("rect", "", "scroll physical desktop rectangle: x,y,width,height")
	duration := flag.Duration("duration", 45*time.Second, "scroll observation window, at most 180s")
	interval := flag.Duration("interval", 250*time.Millisecond, "delay between scroll observations, at least 100ms")
	samples := flag.Int("samples", 90, "scroll observations, from 1 to 360")
	flag.BoolVar(&captureDiagnostics, "diagnostics", false, "emit numerical row-overlap diagnostics on changed scroll frames")
	flag.BoolVar(&captureOracle, "notepad-oracle", false, "compare export with independent stable native-position strips; extra diagnostic memory")
	flag.UintVar(&knownNotepadPID, "notepad-pid", 0, "inspect only: verified fixture Notepad PID; no activation or text reads")
	flag.Parse()
	if *iterations < 1 || *iterations > 60 || *samples < 1 || *samples > 360 ||
		*duration <= 0 || *duration > 180*time.Second || *interval < 100*time.Millisecond {
		emit("error", "invalid bounds for diagnostic run")
		os.Exit(2)
	}
	// ScreenRegion contains synchronous driver calls. Bound this disposable
	// helper process even if a driver never returns to a context checkpoint.
	hardLimit := 3 * time.Minute
	if *mode == "scroll" && *duration+30*time.Second > hardLimit {
		hardLimit = *duration + 30*time.Second
	}
	deadline := time.AfterFunc(hardLimit, func() { emit("error", "diagnostic process deadline exceeded"); os.Exit(124) })
	defer deadline.Stop()
	if err := run(*mode, *iterations, *rectValue, *duration, *interval, *samples); err != nil {
		emit("error", err.Error())
		os.Exit(1)
	}
}

func run(mode string, iterations int, rectValue string, duration, interval time.Duration, samples int) (runError error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pmv2 := ^uintptr(3) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4).
	previous, _, err := setDPI.Call(pmv2)
	if previous == 0 {
		return fmt.Errorf("SetThreadDpiAwarenessContext(PMV2) failed: %v", err)
	}
	defer func() {
		if restored, _, err := setDPI.Call(previous); restored == 0 && runError == nil {
			runError = fmt.Errorf("restoring thread DPI context failed: %v", err)
		}
	}()
	current, _, _ := getDPI.Call()
	if equal, _, _ := equalDPI.Call(current, pmv2); equal == 0 {
		return errors.New("locked test thread is not actually PMV2 aware")
	}
	monitors, err := physicalModes()
	if err != nil {
		return err
	}
	calibration, err := calibrateGDI()
	if err != nil {
		return err
	}
	emit("environment", map[string]any{"mode": mode, "pid": os.Getpid(), "pmv2Verified": true,
		"dpiScope": "current locked native thread only", "monitors": monitors, "gdiCalibration": calibration,
		"pixelFilesSaved": false, "documentTextOCRVerified": false})
	limit := 175 * time.Second
	if mode == "scroll" {
		limit = duration + 25*time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), limit)
	defer cancel()
	switch mode {
	case "inspect":
		if captureDiagnostics && rectValue != "" {
			rect, err := parseRect(rectValue)
			if err != nil {
				return err
			}
			var metadata map[string]any
			if knownNotepadPID != 0 {
				metadata = inspectNotepadEditorByPID(uint32(knownNotepadPID), rect)
			} else {
				metadata, _ = notepadForeground(rect, 0)
			}
			emit("known_notepad_geometry", metadata)
		}
		return nil
	case "native":
		return runNative(ctx, monitors, iterations)
	case "scroll":
		rect, err := parseRect(rectValue)
		if err != nil {
			return err
		}
		valid := false
		for _, monitor := range monitors {
			valid = valid || rect.In(monitor.rect())
		}
		if !valid {
			return errors.New("scroll rectangle must stay within one verified physical monitor")
		}
		return runScroll(ctx, rect, duration, interval, samples)
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
}

func physicalModes() ([]monitorMode, error) {
	var modes []monitorMode
	var callbackError error
	callback := syscall.NewCallback(func(monitor, _, _, _ uintptr) uintptr {
		var info monitorInfo
		info.CbSize = uint32(unsafe.Sizeof(info))
		if ok, _, err := getMonitor.Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
			callbackError = fmt.Errorf("GetMonitorInfoW failed: %v", err)
			return 0
		}
		var settings win.DEVMODE
		settings.DmSize = uint16(unsafe.Sizeof(settings))
		if ok, _, err := enumMode.Call(uintptr(unsafe.Pointer(&info.Device[0])), 0xffffffff, uintptr(unsafe.Pointer(&settings))); ok == 0 {
			callbackError = fmt.Errorf("EnumDisplaySettingsW failed: %v", err)
			return 0
		}
		width, height := int(info.RcMonitor.Right-info.RcMonitor.Left), int(info.RcMonitor.Bottom-info.RcMonitor.Top)
		if width != int(settings.DmPelsWidth) || height != int(settings.DmPelsHeight) {
			callbackError = fmt.Errorf("monitor bounds %dx%d differ from physical display mode %dx%d", width, height, settings.DmPelsWidth, settings.DmPelsHeight)
			return 0
		}
		modes = append(modes, monitorMode{windows.UTF16ToString(info.Device[:]), int(info.RcMonitor.Left), int(info.RcMonitor.Top), width, height})
		return 1
	})
	ok, _, callError := enumMonitors.Call(0, 0, callback, 0)
	if callbackError != nil {
		return nil, callbackError
	}
	if ok == 0 || len(modes) == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors failed: %v", callError)
	}
	return modes, nil
}

func gdiCount() (uint64, error) {
	_, _, _ = setLastError.Call(0)
	count, _, err := guiResources.Call(^uintptr(0), 0)
	if count == 0 && err != nil && err != syscall.Errno(0) {
		return 0, fmt.Errorf("GetGuiResources failed: %v", err)
	}
	return uint64(count), nil
}

func calibrateGDI() (map[string]uint64, error) {
	before, err := gdiCount()
	if err != nil {
		return nil, err
	}
	region := win.CreateRectRgn(0, 0, 1, 1)
	if region == 0 {
		return nil, errors.New("could not allocate GDI calibration region")
	}
	during, err := gdiCount()
	deleted := win.DeleteObject(win.HGDIOBJ(region))
	if err != nil {
		return nil, err
	}
	if !deleted || during <= before {
		return nil, errors.New("GDI resource counter did not detect its calibration allocation")
	}
	after, err := gdiCount()
	return map[string]uint64{"before": before, "during": during, "after": after}, err
}

type memoryState struct {
	Iteration int    `json:"iteration"`
	HeapBytes uint64 `json:"heapBytes"`
	HeapSys   uint64 `json:"heapSysBytes"`
	GDI       uint64 `json:"gdiObjects"`
}

func memoryAt(iteration int) (memoryState, error) {
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	gdi, err := gdiCount()
	return memoryState{iteration, stats.HeapAlloc, stats.HeapSys, gdi}, err
}

type contentStats struct {
	Samples          int     `json:"samples"`
	ColorCount       int     `json:"sampledColorCount"`
	LumaRange        int     `json:"lumaRange"`
	DarkFraction     float64 `json:"darkFraction"`
	LightFraction    float64 `json:"lightFraction"`
	NonBlackFraction float64 `json:"nonBlackFraction"`
	NonFlat          bool    `json:"nonFlatCenter"`
}

func diagnose(frame *image.RGBA) contentStats {
	width, height := frame.Rect.Dx(), frame.Rect.Dy()
	left, right, top, bottom := width/6, width-width/6, height/5, height-height/5
	stepX, stepY := max(1, (right-left)/128), max(1, (bottom-top)/96)
	colors := make(map[uint32]struct{})
	minimum, maximum, dark, light, nonBlack, count := 255, 0, 0, 0, 0, 0
	for y := top; y < bottom; y += stepY {
		for x := left; x < right; x += stepX {
			offset := y*frame.Stride + x*4
			red, green, blue := frame.Pix[offset], frame.Pix[offset+1], frame.Pix[offset+2]
			if len(colors) < 4096 {
				colors[uint32(red)<<16|uint32(green)<<8|uint32(blue)] = struct{}{}
			}
			luma := (int(red)*77 + int(green)*150 + int(blue)*29) >> 8
			minimum, maximum = min(minimum, luma), max(maximum, luma)
			if luma < 64 {
				dark++
			}
			if luma > 224 {
				light++
			}
			if max(red, max(green, blue)) > 2 {
				nonBlack++
			}
			count++
		}
	}
	return contentStats{count, len(colors), maximum - minimum, float64(dark) / float64(count), float64(light) / float64(count), float64(nonBlack) / float64(count), len(colors) > 1 && maximum-minimum > 32}
}

func runNative(ctx context.Context, monitors []monitorMode, iterations int) error {
	before, err := memoryAt(0)
	if err != nil {
		return err
	}
	memory := []memoryState{before}
	captures, encodes, decodes := []time.Duration{}, []time.Duration{}, []time.Duration{}
	digests := map[[32]byte]struct{}{}
	nonFlat := 0
	for index := 1; index <= iterations; index++ {
		result, err := capture.ActiveDisplay(ctx)
		if err != nil {
			return err
		}
		matched := false
		for _, monitor := range monitors {
			matched = matched || image.Rect(result.OriginX, result.OriginY, result.OriginX+result.Width, result.OriginY+result.Height) == monitor.rect()
		}
		if !matched {
			return errors.New("production capture bounds differ from verified physical monitor mode")
		}
		frame, ok := result.Frame.(*image.RGBA)
		if !ok {
			return errors.New("production capture returned a non-RGBA frame")
		}
		content := diagnose(frame)
		if content.NonFlat {
			nonFlat++
		}
		digests[sha256.Sum256(frame.Pix)] = struct{}{}
		started := time.Now()
		decoded, err := png.Decode(bytes.NewReader(result.ImageBytes))
		if err != nil {
			return err
		}
		decodeTime := time.Since(started)
		rgba, ok := decoded.(*image.RGBA)
		if !ok || rgba.Rect.Size() != frame.Rect.Size() {
			return errors.New("decoded PNG bounds or format mismatch")
		}
		for y := 0; y < frame.Rect.Dy(); y++ {
			if !bytes.Equal(frame.Pix[y*frame.Stride:y*frame.Stride+frame.Rect.Dx()*4], rgba.Pix[y*rgba.Stride:y*rgba.Stride+rgba.Rect.Dx()*4]) {
				return errors.New("decoded PNG differs from native raster")
			}
		}
		captures, encodes, decodes = append(captures, result.CaptureDuration), append(encodes, result.EncodeDuration), append(decodes, decodeTime)
		emit("native_sample", map[string]any{"iteration": index, "width": result.Width, "height": result.Height, "originX": result.OriginX, "originY": result.OriginY, "displays": result.Displays, "pngBytes": len(result.ImageBytes), "pngMode": result.CompressionMode, "captureMs": float64(result.CaptureDuration) / 1e6, "encodeMs": float64(result.EncodeDuration) / 1e6, "decodeMs": float64(decodeTime) / 1e6, "center": content})
		// Drop this iteration's pixel owners before measuring retained Go heap.
		result, frame, rgba, decoded = capture.Result{}, nil, nil, nil
		if index%10 == 0 || index == iterations {
			checkpoint, err := memoryAt(index)
			if err != nil {
				return err
			}
			memory = append(memory, checkpoint)
		}
	}
	emit("native_summary", map[string]any{"iterations": iterations, "capture": summarize(captures), "encode": summarize(encodes), "decode": summarize(decodes), "nonFlatCenterFrames": nonFlat, "distinctRasters": len(digests), "pngPixelsVerified": true, "documentTextOCRVerified": false, "memory": memory})
	if nonFlat == 0 {
		return errors.New("all center samples are flat or low contrast; expected document pixels are unconfirmed")
	}
	return resourceBounds(memory, 32<<20)
}

func runScroll(ctx context.Context, rect image.Rectangle, duration, interval time.Duration, samples int) error {
	before, err := memoryAt(0)
	if err != nil {
		return err
	}
	var latest *image.RGBA
	var previousDiagnostic *image.RGBA
	var taskNotepad win.HWND
	var captureTime time.Duration
	var oracle *notepadOracle
	if captureOracle {
		oracle = &notepadOracle{}
	}
	frames := 0
	session, err := capture.StartManualScrollCaptureWithSource(ctx, rect, capture.ManualScrollOptions{}, func(bounds image.Rectangle) (image.Image, error) {
		foregroundBefore := win.HWND(0)
		var beforeMetadata map[string]any
		if captureDiagnostics || captureOracle {
			foregroundBefore = win.GetForegroundWindow()
		}
		if captureOracle {
			var expected win.HWND
			beforeMetadata, expected = notepadForeground(bounds, taskNotepad)
			taskNotepad = expected
			if beforeMetadata["foregroundIsTaskNotepad"] != true {
				return nil, errors.New("oracle source is no longer the known foreground Notepad")
			}
		}
		start := time.Now()
		frame, err := capture.ScreenRegion(bounds)
		captureTime = time.Since(start)
		if err == nil && (captureDiagnostics || captureOracle) {
			changed := previousDiagnostic == nil || !bytes.Equal(previousDiagnostic.Pix, frame.Pix)
			if changed || captureOracle {
				foreground, expected := notepadForeground(bounds, taskNotepad)
				taskNotepad = expected
				foreground["stableDuringNativeCapture"] = foregroundBefore == win.GetForegroundWindow()
				if captureOracle {
					beforePos, _, beforeValid := oracleState(beforeMetadata)
					position, lineHeight, valid := oracleState(foreground)
					if beforeValid && valid && beforePos == position && foreground["stableDuringNativeCapture"] == true {
						oracle.observe(frame, position, lineHeight)
					} else if !beforeValid || !valid {
						oracle.problem = "oracle requires a known Notepad, measured line height, and ROI entirely inside Edit client"
					} else {
						oracle.stableSamples = 0
					}
				}
				if changed && captureDiagnostics {
					diagnostic := map[string]any{"foreground": foreground, "sourceCapture": frames + 1}
					if previousDiagnostic != nil {
						diagnostic["overlap"] = diagnoseScrollOverlap(previousDiagnostic, frame)
					}
					emit("scroll_frame_diagnostic", diagnostic)
				}
			}
			previousDiagnostic = frame
		}
		latest = frame
		frames++
		return frame, err
	})
	if err != nil {
		return err
	}
	defer func() {
		if session != nil {
			session.Cancel()
		}
	}()
	initial := diagnose(latest)
	digests := map[[32]byte]struct{}{sha256.Sum256(latest.Pix): {}}
	latest = nil
	emit("scroll_started", map[string]any{"rect": rect, "durationSeconds": duration.Seconds(), "intervalMs": float64(interval) / 1e6, "initialCenter": initial, "uiDrivenByHarness": false, "numericalOverlapDiagnostics": captureDiagnostics, "controlledNotepadOracle": captureOracle})
	steps, nativeTimes := []time.Duration{}, []time.Duration{}
	appended, observations := 0, 0
	until := time.Now().Add(duration)
	for observations < samples && time.Now().Before(until) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		start := time.Now()
		snapshot, err := session.CaptureNext()
		elapsed := time.Since(start)
		if err != nil {
			return err
		}
		observations++
		var content *contentStats
		if latest != nil {
			value := diagnose(latest)
			content = &value
			digests[sha256.Sum256(latest.Pix)] = struct{}{}
			latest = nil
			nativeTimes = append(nativeTimes, captureTime)
		}
		steps = append(steps, elapsed)
		if snapshot.Appended {
			appended++
		}
		emit("scroll_sample", map[string]any{"observation": observations, "frames": snapshot.Frames, "width": snapshot.Width, "height": snapshot.Height, "appended": snapshot.Appended, "limitReached": snapshot.LimitReached, "stepMs": float64(elapsed) / 1e6, "nativeMs": float64(captureTime) / 1e6, "center": content})
		if snapshot.LimitReached {
			break
		}
	}
	result, err := session.Finish()
	if err != nil {
		return err
	}
	decoded, err := png.Decode(bytes.NewReader(result.ImageBytes))
	if err != nil {
		return err
	}
	if decoded.Bounds().Dx() != result.Width || decoded.Bounds().Dy() != result.Height || result.ScrollFrames != 1+appended {
		return errors.New("scroll export dimensions or accepted-frame count mismatch")
	}
	width, height, pngBytes, scrollFrames := result.Width, result.Height, len(result.ImageBytes), result.ScrollFrames
	var oracleReport map[string]any
	if oracle != nil {
		oracleReport = oracle.compare(decoded)
		oracle = nil
	}
	result, decoded = capture.Result{}, nil
	previousDiagnostic = nil
	session.Cancel()
	session = nil
	after, err := memoryAt(observations)
	if err != nil {
		return err
	}
	emit("scroll_summary", map[string]any{"observations": observations, "sourceCaptures": frames, "appended": appended, "frames": scrollFrames, "width": width, "height": height, "pngBytes": pngBytes, "native": summarize(nativeTimes), "step": summarize(steps), "distinctRasters": len(digests), "exportPNGDecoded": true, "documentOrderVerified": false, "uiHoleAndHotkeyVerified": false, "memory": []memoryState{before, after}, "controlledNotepadOracle": oracleReport, "extraOracleMemoryAndTiming": captureOracle})
	if captureOracle && oracleReport["pixelEqual"] != true {
		return errors.New("export differs from independent stable Notepad position oracle")
	}
	if appended < 2 || len(digests) < 2 {
		return errors.New("insufficient observed raster motion/appends; real scrolling is inconclusive")
	}
	return resourceBounds([]memoryState{before, after}, 32<<20)
}

type notepadOracle struct {
	pending                         *image.RGBA
	pendingPosition, stableSamples  int
	initialPosition, latestPosition int
	height, lineHeight, accepted    int
	strips                          []*image.RGBA
	problem                         string
}

func oracleState(metadata map[string]any) (position, lineHeight int, valid bool) {
	editor, ok := metadata["editor"].(map[string]any)
	if !ok || metadata["foregroundIsTaskNotepad"] != true || editor["verifiedEditControl15"] != true || editor["roiWithinEditorClient"] != true {
		return 0, 0, false
	}
	position, posOK := editor["firstVisibleLineZeroBased"].(int)
	lineHeight, heightOK := editor["adjacentVisibleLineHeight"].(int)
	return position, lineHeight, posOK && heightOK && position >= 0 && lineHeight > 0
}

func (oracle *notepadOracle) observe(frame *image.RGBA, position, lineHeight int) {
	if oracle.problem != "" {
		return
	}
	if oracle.pending == nil {
		oracle.initialPosition = position
	}
	if oracle.pending != nil && position == oracle.pendingPosition && bytes.Equal(oracle.pending.Pix, frame.Pix) {
		oracle.stableSamples++
	} else {
		oracle.pendingPosition, oracle.stableSamples = position, 1
	}
	oracle.pending = frame
	if oracle.stableSamples < 2 {
		return
	}
	if len(oracle.strips) == 0 {
		if position != oracle.initialPosition {
			oracle.problem = "fixture moved before a stable initial raster was observed"
			return
		}
		oracle.strips = append(oracle.strips, copyOracleRows(frame, 0))
		oracle.height, oracle.lineHeight, oracle.latestPosition = frame.Rect.Dy(), lineHeight, position
		emit("oracle_stable_position", map[string]any{"firstVisibleLine": position, "measuredLineHeight": lineHeight, "expectedHeight": oracle.height})
		return
	}
	if lineHeight != oracle.lineHeight || position < oracle.initialPosition {
		oracle.problem = "font geometry changed or source moved above the initial viewport"
		return
	}
	top := (position - oracle.initialPosition) * lineHeight
	if bottom := top + frame.Rect.Dy(); bottom > oracle.height {
		extension := bottom - oracle.height
		if extension > frame.Rect.Dy() || int64(frame.Rect.Dx())*int64(bottom) > 40_000_000 {
			oracle.problem = "native viewport movement exceeded bounded oracle overlap/storage"
			return
		}
		oracle.strips = append(oracle.strips, copyOracleRows(frame, frame.Rect.Dy()-extension))
		oracle.height, oracle.accepted = bottom, oracle.accepted+1
		emit("oracle_stable_position", map[string]any{"firstVisibleLine": position, "measuredLineHeight": lineHeight, "nativeExtension": extension, "expectedHeight": oracle.height})
	}
	oracle.latestPosition = position
}

func copyOracleRows(frame *image.RGBA, first int) *image.RGBA {
	result := image.NewRGBA(image.Rect(0, 0, frame.Rect.Dx(), frame.Rect.Dy()-first))
	draw.Draw(result, result.Rect, frame, image.Pt(frame.Rect.Min.X, frame.Rect.Min.Y+first), draw.Src)
	return result
}

func (oracle *notepadOracle) compare(decoded image.Image) map[string]any {
	report := map[string]any{"pixelEqual": false, "problem": oracle.problem, "stableSamplesRequired": 2,
		"initialFirstVisibleLine": oracle.initialPosition, "finalFirstVisibleLine": oracle.latestPosition, "measuredLineHeight": oracle.lineHeight,
		"expectedHeight": oracle.height, "nativeAppendCount": oracle.accepted, "diagnosticExtraMemory": true}
	if oracle.problem != "" || len(oracle.strips) == 0 {
		return report
	}
	if decoded.Bounds().Dx() != oracle.strips[0].Rect.Dx() || decoded.Bounds().Dy() != oracle.height {
		report["problem"] = "export dimensions differ from native position oracle"
		return report
	}
	actual, ok := decoded.(*image.RGBA)
	if !ok {
		actual = image.NewRGBA(image.Rect(0, 0, decoded.Bounds().Dx(), decoded.Bounds().Dy()))
		draw.Draw(actual, actual.Rect, decoded, decoded.Bounds().Min, draw.Src)
	}
	mismatchingRows, mismatchingPixels, outputY := 0, 0, 0
	for _, strip := range oracle.strips {
		for y := 0; y < strip.Rect.Dy(); y++ {
			width := strip.Rect.Dx() * 4
			wanted := strip.Pix[y*strip.Stride : y*strip.Stride+width]
			got := actual.Pix[outputY*actual.Stride : outputY*actual.Stride+width]
			if !bytes.Equal(wanted, got) {
				mismatchingRows++
				for x := 0; x < width; x += 4 {
					if binary.LittleEndian.Uint32(wanted[x:]) != binary.LittleEndian.Uint32(got[x:]) {
						mismatchingPixels++
					}
				}
			}
			outputY++
		}
	}
	report["mismatchingRows"], report["mismatchingPixels"] = mismatchingRows, mismatchingPixels
	report["pixelEqual"] = mismatchingPixels == 0
	return report
}

type diagnosticRow struct {
	signature uint32
	featured  bool
}

// Examine only already captured frames. Report counts and candidate offsets,
// never pixels, row signatures, or hashes that could reconstruct the document.
func diagnoseScrollOverlap(previous, next *image.RGBA) map[string]any {
	width, height := previous.Rect.Dx(), previous.Rect.Dy()
	changedRows, changedBands, widestChangedRow := 0, uint(0), 0
	firstChangedX, lastChangedX := width, -1
	for y := 0; y < height; y++ {
		left, right := width, -1
		for x := 0; x < width; x++ {
			offset := x * 4
			if binary.LittleEndian.Uint32(previous.Pix[y*previous.Stride+offset:]) != binary.LittleEndian.Uint32(next.Pix[y*next.Stride+offset:]) {
				if x < left {
					left = x
				}
				right = x
			}
		}
		if right >= left {
			changedRows++
			changedBands |= 1 << (y * 4 / height)
			if right-left+1 > widestChangedRow {
				widestChangedRow = right - left + 1
			}
			if left < firstChangedX {
				firstChangedX = left
			}
			if right > lastChangedX {
				lastChangedX = right
			}
		}
	}
	var crops []map[string]any
	bestQualityOriginalAdvance := 0
	for _, crop := range []int{0, 1, 4, 8, 16, 32, 80} {
		if crop*2 >= width {
			continue
		}
		leftRows := diagnosticRows(previous, crop)
		rightRows := diagnosticRows(next, crop)
		stationaryMatches, stationaryFeatures := diagnosticRowMatches(leftRows, rightRows, 0)
		bestAdvance, bestMatches, bestFeatures := 0, 0, 0
		bestCoherentAdvance, bestCoherentMatches, bestCoherentFeatures := 0, 0, 0
		stationaryRatio := float64(stationaryMatches) / float64(stationaryFeatures)
		type candidate struct{ advance, matches, features int }
		var qualityCandidates []candidate
		for advance := 1; advance <= height-height/10; advance++ {
			matches, features := diagnosticRowMatches(leftRows, rightRows, advance)
			if features >= 32 && matches >= 8 {
				qualityCandidates = append(qualityCandidates, candidate{advance, matches, features})
			}
			if matches > bestMatches {
				bestAdvance, bestMatches, bestFeatures = advance, matches, features
			}
			if features >= 8 && float64(matches)/float64(features) >= .94 && float64(matches)/float64(features)-stationaryRatio >= .05 && matches > bestCoherentMatches {
				bestCoherentAdvance, bestCoherentMatches, bestCoherentFeatures = advance, matches, features
			}
		}
		sort.Slice(qualityCandidates, func(left, right int) bool {
			a, b := qualityCandidates[left], qualityCandidates[right]
			if a.matches*b.features != b.matches*a.features {
				return a.matches*b.features > b.matches*a.features
			}
			return a.matches > b.matches
		})
		var topQuality []map[string]any
		for rank, value := range qualityCandidates {
			if rank == 8 {
				break
			}
			topQuality = append(topQuality, map[string]any{"advance": value.advance, "matches": value.matches, "features": value.features,
				"ratio": float64(value.matches) / float64(value.features)})
		}
		if crop == 0 && len(qualityCandidates) > 0 {
			bestQualityOriginalAdvance = qualityCandidates[0].advance
		}
		matches27, features27 := diagnosticRowMatches(leftRows, rightRows, 27)
		var candidates []map[string]any
		for _, advance := range []int{26, 27, 28, 54, 55, 108, 594, 702, 756, 783, 810} {
			matches, features := diagnosticRowMatches(leftRows, rightRows, advance)
			ratio := 0.0
			if features > 0 {
				ratio = float64(matches) / float64(features)
			}
			candidates = append(candidates, map[string]any{"advance": advance, "matches": matches, "features": features, "ratio": ratio})
		}
		crops = append(crops, map[string]any{"cropEachSide": crop, "stationaryMatches": stationaryMatches, "stationaryFeatures": stationaryFeatures,
			"advance27Matches": matches27, "advance27Features": features27,
			"candidates":                  candidates,
			"topQualityAtLeast32Features": topQuality,
			"mostMatchingAdvance":         bestAdvance, "mostMatchingRows": bestMatches, "mostMatchingFeatures": bestFeatures,
			"legacy94PercentAdvance": bestCoherentAdvance, "legacy94PercentMatches": bestCoherentMatches, "legacy94PercentFeatures": bestCoherentFeatures})
	}
	var mismatchDistributions []map[string]any
	for _, advance := range []int{27, 594, 702, 783, bestQualityOriginalAdvance} {
		yBins, xBins := make([]int, 20), make([]int, 20)
		mismatchingRows := 0
		for y := height / 20; y < height-advance-height/20; y++ {
			rowChanged := false
			for x := 0; x < width; x++ {
				if binary.LittleEndian.Uint32(previous.Pix[(y+advance)*previous.Stride+x*4:]) != binary.LittleEndian.Uint32(next.Pix[y*next.Stride+x*4:]) {
					xBins[x*len(xBins)/width]++
					rowChanged = true
				}
			}
			if rowChanged {
				mismatchingRows++
				yBins[y*len(yBins)/height]++
			}
		}
		mismatchDistributions = append(mismatchDistributions, map[string]any{"advance": advance,
			"yMismatchRowBins": yBins, "xMismatchingPixelBins": xBins, "mismatchingRows": mismatchingRows,
			"yBinsRelativeTo": "next frame; previous y = next y + advance", "binCount": 20})
	}
	return map[string]any{"changedRows": changedRows, "changedBandBits": changedBands, "widestChangedRow": widestChangedRow,
		"firstChangedX": firstChangedX, "lastChangedX": lastChangedX, "crops": crops, "mismatchDistributions": mismatchDistributions,
		"uniqueRowAnchors": diagnosticUniqueAnchors(previous, next)}
}

func diagnosticUniqueAnchors(previous, next *image.RGBA) []map[string]any {
	positions := func(rows []diagnosticRow) map[uint32]int {
		result := make(map[uint32]int)
		for y, row := range rows {
			if row.featured {
				if _, exists := result[row.signature]; exists {
					result[row.signature] = 0
				} else {
					result[row.signature] = y + 1
				}
			}
		}
		return result
	}
	left, right := positions(diagnosticRows(previous, 0)), positions(diagnosticRows(next, 0))
	pairs := map[int][][2]int{}
	for signature, y := range left {
		if other := right[signature]; y > 0 && other > 0 {
			pairs[y-other] = append(pairs[y-other], [2]int{y - 1, other - 1})
		}
	}
	var advances []int
	for advance := range pairs {
		advances = append(advances, advance)
	}
	sort.Ints(advances)
	var result []map[string]any
	for _, advance := range advances {
		values := pairs[advance]
		sort.Slice(values, func(left, right int) bool { return values[left][1] < values[right][1] })
		first, last := values[0][1], values[len(values)-1][1]
		count := len(values)
		if len(values) > 32 {
			values = values[:32]
		}
		result = append(result, map[string]any{"advance": advance, "uniqueRowPairCount": count, "nextYFirst": first, "nextYLast": last,
			"nextYSpan": last - first, "previousNextYCoordinates": values, "coordinatesTruncated": count > len(values)})
	}
	return result
}

func notepadForeground(roi image.Rectangle, expected win.HWND) (map[string]any, win.HWND) {
	hwnd := win.GetForegroundWindow()
	var pid uint32
	win.GetWindowThreadProcessId(hwnd, &pid)
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return map[string]any{"foregroundIsTaskNotepad": false, "verificationError": "cannot query foreground process"}, expected
	}
	defer windows.CloseHandle(process)
	path := make([]uint16, 1024)
	length := uint32(len(path))
	if err := windows.QueryFullProcessImageName(process, 0, &path[0], &length); err != nil || !strings.EqualFold(filepath.Base(windows.UTF16ToString(path[:length])), "notepad.exe") {
		return map[string]any{"foregroundIsTaskNotepad": false, "otherWindowMetadataOmitted": true}, expected
	}
	if expected == 0 {
		expected = hwnd // Root has prepared this foreground synthetic fixture.
	}
	if hwnd != expected {
		return map[string]any{"foregroundIsTaskNotepad": false, "differentNotepadWindow": true}, expected
	}
	var bounds, client win.RECT
	var origin win.POINT
	if !win.GetWindowRect(hwnd, &bounds) || !win.GetClientRect(hwnd, &client) || !win.ClientToScreen(hwnd, &origin) {
		return map[string]any{"foregroundIsTaskNotepad": true, "verificationError": "cannot query known Notepad geometry"}, expected
	}
	physicalClient := image.Rect(int(origin.X+client.Left), int(origin.Y+client.Top), int(origin.X+client.Right), int(origin.Y+client.Bottom))
	dpi, _, _ := getWindowDPI.Call(uintptr(hwnd))
	return map[string]any{"foregroundIsTaskNotepad": true, "notepadPid": pid,
		"windowRect":         image.Rect(int(bounds.Left), int(bounds.Top), int(bounds.Right), int(bounds.Bottom)),
		"physicalClientRect": physicalClient, "roiWithinClient": roi.In(physicalClient),
		"wsMaximize": uint32(win.GetWindowLong(hwnd, win.GWL_STYLE))&uint32(win.WS_MAXIMIZE) != 0,
		"isZoomed":   win.IsZoomed(hwnd), "windowDpi": dpi, "editor": notepadEditor(hwnd, roi)}, expected
}

func notepadEditor(parent win.HWND, roi image.Rectangle) map[string]any {
	editor := win.GetDlgItem(parent, 15)
	var class [256]uint16
	length, err := win.GetClassName(editor, &class[0], len(class))
	if editor == 0 || err != nil || !strings.EqualFold(windows.UTF16ToString(class[:length]), "Edit") {
		return map[string]any{"verifiedEditControl15": false}
	}
	var bounds, client win.RECT
	var origin win.POINT
	if !win.GetWindowRect(editor, &bounds) || !win.GetClientRect(editor, &client) || !win.ClientToScreen(editor, &origin) {
		return map[string]any{"verifiedEditControl15": true, "geometryError": true}
	}
	physicalClient := image.Rect(int(origin.X+client.Left), int(origin.Y+client.Top), int(origin.X+client.Right), int(origin.Y+client.Bottom))
	var formatting win.RECT
	win.SendMessage(editor, win.EM_GETRECT, 0, uintptr(unsafe.Pointer(&formatting)))
	runtime.KeepAlive(&formatting)
	firstVisible := int(win.SendMessage(editor, win.EM_GETFIRSTVISIBLELINE, 0, 0))
	firstIndex := win.SendMessage(editor, win.EM_LINEINDEX, uintptr(firstVisible), 0)
	nextIndex := win.SendMessage(editor, win.EM_LINEINDEX, uintptr(firstVisible+1), 0)
	firstPosition := win.SendMessage(editor, win.EM_POSFROMCHAR, firstIndex, 0)
	nextPosition := win.SendMessage(editor, win.EM_POSFROMCHAR, nextIndex, 0)
	lineHeight := int(int16(nextPosition>>16)) - int(int16(firstPosition>>16))
	if firstIndex == ^uintptr(0) || nextIndex == ^uintptr(0) || uint32(firstPosition) == ^uint32(0) || uint32(nextPosition) == ^uint32(0) {
		lineHeight = 0
	}
	scroll := win.SCROLLINFO{CbSize: uint32(unsafe.Sizeof(win.SCROLLINFO{})), FMask: win.SIF_ALL}
	scrollOK := win.GetScrollInfo(editor, win.SB_VERT, &scroll)
	return map[string]any{"verifiedEditControl15": true, "physicalClientRect": physicalClient, "roiWithinEditorClient": roi.In(physicalClient),
		"windowRect":                image.Rect(int(bounds.Left), int(bounds.Top), int(bounds.Right), int(bounds.Bottom)),
		"formatRectInClient":        image.Rect(int(formatting.Left), int(formatting.Top), int(formatting.Right), int(formatting.Bottom)),
		"firstVisibleLineZeroBased": firstVisible, "verticalScrollInfoOK": scrollOK,
		"adjacentVisibleLineHeight": lineHeight,
		"verticalScroll":            map[string]any{"min": scroll.NMin, "max": scroll.NMax, "page": scroll.NPage, "pos": scroll.NPos}}
}

func inspectNotepadEditorByPID(pid uint32, roi image.Rectangle) map[string]any {
	var known win.HWND
	callback := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		var owner uint32
		win.GetWindowThreadProcessId(win.HWND(hwnd), &owner)
		if owner == pid {
			var class [256]uint16
			length, err := win.GetClassName(win.HWND(hwnd), &class[0], len(class))
			if err == nil && strings.EqualFold(windows.UTF16ToString(class[:length]), "Notepad") {
				known = win.HWND(hwnd)
				return 0
			}
		}
		return 1
	})
	enumWindows.Call(callback, 0)
	if known == 0 {
		return map[string]any{"verifiedKnownNotepadFound": false}
	}
	return map[string]any{"verifiedKnownNotepadFound": true, "knownNotepadPid": pid,
		"foregroundIsKnownNotepad": win.GetForegroundWindow() == known, "editor": notepadEditor(known, roi)}
}

func diagnosticRows(frame *image.RGBA, crop int) []diagnosticRow {
	rows := make([]diagnosticRow, frame.Rect.Dy())
	for y := range rows {
		pixels := frame.Pix[y*frame.Stride+crop*4 : y*frame.Stride+(frame.Rect.Dx()-crop)*4]
		rows[y].signature = crc32.ChecksumIEEE(pixels)
		first := binary.LittleEndian.Uint32(pixels)
		for offset := 4; offset < len(pixels); offset += 4 {
			if binary.LittleEndian.Uint32(pixels[offset:]) != first {
				rows[y].featured = true
				break
			}
		}
	}
	return rows
}

func diagnosticRowMatches(previous, next []diagnosticRow, advance int) (matches, features int) {
	edge := len(previous) / 20
	for y := edge; y < len(previous)-advance-edge; y++ {
		left, right := previous[y+advance], next[y]
		if !left.featured && !right.featured {
			continue
		}
		features++
		if left.signature == right.signature {
			matches++
		}
	}
	return matches, features
}

func resourceBounds(memory []memoryState, heapLimit int64) error {
	first, last := memory[0], memory[len(memory)-1]
	if int64(last.HeapBytes)-int64(first.HeapBytes) > heapLimit || last.GDI > first.GDI+4 {
		return errors.New("retained heap or GDI resources exceed diagnostic bound")
	}
	return nil
}

func parseRect(value string) (image.Rectangle, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, errors.New("scroll requires -rect=x,y,width,height in physical pixels")
	}
	var fields [4]int
	for index, part := range parts {
		number, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return image.Rectangle{}, err
		}
		fields[index] = number
	}
	if fields[2] < 32 || fields[3] < 32 || fields[2] > 16384 || fields[3] > 16384 || fields[0] < -1_000_000 || fields[0] > 1_000_000 || fields[1] < -1_000_000 || fields[1] > 1_000_000 {
		return image.Rectangle{}, errors.New("invalid physical scroll bounds")
	}
	return image.Rect(fields[0], fields[1], fields[0]+fields[2], fields[1]+fields[3]), nil
}
