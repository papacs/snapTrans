package capture

import (
	"bytes"
	"compress/flate"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"io"
	"math"
	"strings"
	"sync"
	"time"
)

// Display describes one captured monitor in logical (96-DPI) coordinates.
// X, Y, Width and Height are relative to the union of all logical display
// rects, so the frontend can map CSS pixels to physical pixels per display
// with the Scale factor (physical pixels per logical pixel).
type Display struct {
	X      int     `json:"x"`
	Y      int     `json:"y"`
	Width  int     `json:"width"`
	Height int     `json:"height"`
	Scale  float64 `json:"scale"`
}

type Result struct {
	CaptureID       uint64        `json:"captureId,omitempty"`
	Image           string        `json:"image"`
	Frame           image.Image   `json:"-"`
	ImageBytes      []byte        `json:"-"`
	Width           int           `json:"width"`
	Height          int           `json:"height"`
	OriginX         int           `json:"originX"`
	OriginY         int           `json:"originY"`
	Displays        []Display     `json:"displays"`
	Source          string        `json:"source"`
	Mode            string        `json:"mode,omitempty"`
	ScrollFrames    int           `json:"scrollFrames,omitempty"`
	SelectedText    *SelectedText `json:"selectedText,omitempty"`
	Notice          string        `json:"notice,omitempty"`
	CaptureDuration time.Duration `json:"-"`
	EncodeDuration  time.Duration `json:"-"`
	EncodedBytes    int           `json:"-"`
	CompressionMode string        `json:"-"`
}

type physicalMonitor struct {
	Rect  image.Rectangle
	Scale float64
}

const (
	pngDataURLPrefix = "data:image/png;base64,"
)

var pngProbeWriters = sync.Pool{New: func() any {
	writer, _ := flate.NewWriter(io.Discard, flate.BestSpeed)
	return writer
}}

func AllDisplays(ctx context.Context) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	monitors, err := physicalMonitors()
	if err != nil {
		return Result{}, err
	}
	return captureMonitors(ctx, monitors)
}

// ActiveDisplay captures only the display containing the mouse cursor. This
// keeps the frozen image and the Wails overlay in one DPI coordinate space and
// avoids encoding every monitor when the user can only interact with one
// fullscreen overlay at a time.
func ActiveDisplay(ctx context.Context) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	monitors, err := physicalMonitors()
	if err != nil {
		return Result{}, err
	}
	if len(monitors) == 0 {
		return Result{}, errors.New("no active displays found")
	}

	monitor := monitors[0]
	if cursor, cursorErr := cursorPosition(); cursorErr == nil {
		if selected, ok := monitorForPoint(monitors, cursor); ok {
			monitor = selected
		}
	}

	return captureMonitors(ctx, []physicalMonitor{monitor})
}

func captureMonitors(ctx context.Context, monitors []physicalMonitor) (Result, error) {
	if len(monitors) == 0 {
		return Result{}, errors.New("no active displays found")
	}

	union := monitors[0].Rect
	for _, monitor := range monitors[1:] {
		union = union.Union(monitor.Rect)
	}

	captureStartedAt := time.Now()
	var frame image.Image
	if len(monitors) == 1 {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		img, err := ScreenRegion(monitors[0].Rect)
		if err != nil {
			return Result{}, err
		}
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		if isBlankCapture(img) {
			return Result{}, errors.New("screen capture returned a blank image")
		}
		frame = img
	} else {
		canvas := image.NewRGBA(image.Rect(0, 0, union.Dx(), union.Dy()))
		for _, monitor := range monitors {
			if err := ctx.Err(); err != nil {
				return Result{}, err
			}
			img, err := ScreenRegion(monitor.Rect)
			if err != nil {
				return Result{}, err
			}

			target := image.Rect(
				monitor.Rect.Min.X-union.Min.X,
				monitor.Rect.Min.Y-union.Min.Y,
				monitor.Rect.Max.X-union.Min.X,
				monitor.Rect.Max.Y-union.Min.Y,
			)
			draw.Draw(canvas, target, img, image.Point{}, draw.Src)
		}
		frame = canvas
	}

	displays := LogicalDisplays(monitors)
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	captureDuration := time.Since(captureStartedAt)
	encodeStartedAt := time.Now()
	compressionLevel := pngCompressionLevel(frame)
	encoded, err := encodePNGBytesContext(ctx, frame, compressionLevel)
	if err != nil {
		return Result{}, err
	}
	encodeDuration := time.Since(encodeStartedAt)

	return Result{
		ImageBytes:      encoded,
		Frame:           frame,
		Width:           union.Dx(),
		Height:          union.Dy(),
		OriginX:         union.Min.X,
		OriginY:         union.Min.Y,
		Displays:        displays,
		Source:          "wails",
		CaptureDuration: captureDuration,
		EncodeDuration:  encodeDuration,
		EncodedBytes:    len(encoded),
		CompressionMode: pngCompressionMode(compressionLevel),
	}, nil
}

func encodePNGDataURL(frame image.Image) (string, int, error) {
	return encodePNGDataURLWithLevel(frame, pngCompressionLevel(frame))
}

func pngCompressionLevel(frame image.Image) png.CompressionLevel {
	bounds := frame.Bounds()
	if int64(bounds.Dx())*int64(bounds.Dy()) < 1_000_000 {
		return png.NoCompression
	}
	// Probe small contiguous tiles with PNG's horizontal delta filter. Isolated
	// pixels discard spatial repetition and wrongly classify text, gradients
	// and wallpaper as noise, sending multi-megabyte images to the WebView.
	var sample [8 * 32 * 32 * 4]byte
	rgba, _ := frame.(*image.RGBA)
	tileWidth, tileHeight := minInt(32, bounds.Dx()), minInt(32, bounds.Dy())
	i := 0
	for tileY := 0; tileY < 2; tileY++ {
		for tileX := 0; tileX < 4; tileX++ {
			left := bounds.Min.X + (2*tileX+1)*(bounds.Dx()-tileWidth)/8
			top := bounds.Min.Y + (2*tileY+1)*(bounds.Dy()-tileHeight)/4
			for y := 0; y < tileHeight; y++ {
				var previous [4]byte
				for x := 0; x < tileWidth; x++ {
					var pixel [4]byte
					if rgba != nil {
						offset := rgba.PixOffset(left+x, top+y)
						copy(pixel[:], rgba.Pix[offset:offset+4])
					} else {
						r, g, b, a := frame.At(left+x, top+y).RGBA()
						pixel = [4]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8), byte(a >> 8)}
					}
					for channel := range pixel {
						sample[i] = pixel[channel] - previous[channel]
						i++
					}
					previous = pixel
				}
			}
		}
	}
	var compressed pngByteCounter
	probe := pngProbeWriters.Get().(*flate.Writer)
	probe.Reset(&compressed)
	_, err := probe.Write(sample[:i])
	closeErr := probe.Close()
	probe.Reset(io.Discard)
	pngProbeWriters.Put(probe)
	if err == nil && closeErr == nil && int(compressed)*100 <= i*45 {
		return png.BestSpeed
	}
	return png.NoCompression
}

type pngByteCounter int

func (count *pngByteCounter) Write(data []byte) (int, error) {
	*count += pngByteCounter(len(data))
	return len(data), nil
}

func pngCompressionMode(level png.CompressionLevel) string {
	if level == png.NoCompression {
		return "no-compression"
	}
	return "best-speed"
}

func encodePNGDataURLWithLevel(frame image.Image, level png.CompressionLevel) (string, int, error) {
	encoded, err := encodePNGBytesWithLevel(frame, level)
	if err != nil {
		return "", 0, err
	}

	encodedBytes := len(encoded)
	var dataURL strings.Builder
	dataURL.Grow(len(pngDataURLPrefix) + base64.StdEncoding.EncodedLen(encodedBytes))
	dataURL.WriteString(pngDataURLPrefix)
	base64Encoder := base64.NewEncoder(base64.StdEncoding, &dataURL)
	if _, err := base64Encoder.Write(encoded); err != nil {
		return "", 0, err
	}
	if err := base64Encoder.Close(); err != nil {
		return "", 0, err
	}
	return dataURL.String(), encodedBytes, nil
}

func encodePNGBytesWithLevel(frame image.Image, level png.CompressionLevel) ([]byte, error) {
	return encodePNGBytesContext(context.Background(), frame, level)
}

func encodePNGBytesContext(ctx context.Context, frame image.Image, level png.CompressionLevel) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if rgba, ok := frame.(*image.RGBA); ok && rgba.Opaque() &&
		(level == png.NoCompression || level == png.BestSpeed) && !rgba.Bounds().Empty() {
		return encodeScreenshotPNG(ctx, rgba, level)
	}
	var buffer bytes.Buffer
	if level == png.NoCompression {
		bounds := frame.Bounds()
		estimatedBytes := int64(bounds.Dx())*int64(bounds.Dy())*4 + int64(bounds.Dy()) + 4096
		if estimatedBytes > 0 && estimatedBytes <= int64(^uint(0)>>1) {
			buffer.Grow(int(estimatedBytes))
		}
	}
	encoder := png.Encoder{
		CompressionLevel: level,
	}
	if err := encoder.Encode(pngContextWriter{ctx: ctx, writer: &buffer}, frame); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

type pngContextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (writer pngContextWriter) Write(data []byte) (int, error) {
	if err := writer.ctx.Err(); err != nil {
		return 0, err
	}
	return writer.writer.Write(data)
}

// EncodePNGBytes encodes a frame to PNG bytes using the shared fast encoder
// used for full-screen captures.
func EncodePNGBytes(frame image.Image) ([]byte, error) {
	return encodePNGBytesWithLevel(frame, png.NoCompression)
}

func monitorForPoint(monitors []physicalMonitor, point image.Point) (physicalMonitor, bool) {
	if len(monitors) == 0 {
		return physicalMonitor{}, false
	}

	for _, monitor := range monitors {
		if point.In(monitor.Rect) {
			return monitor, true
		}
	}

	nearest := monitors[0]
	nearestDistance := squaredDistanceToRect(point, nearest.Rect)
	for _, monitor := range monitors[1:] {
		distance := squaredDistanceToRect(point, monitor.Rect)
		if distance < nearestDistance {
			nearest = monitor
			nearestDistance = distance
		}
	}
	return nearest, true
}

func squaredDistanceToRect(point image.Point, rect image.Rectangle) int64 {
	dx := 0
	if point.X < rect.Min.X {
		dx = rect.Min.X - point.X
	} else if point.X >= rect.Max.X {
		dx = point.X - rect.Max.X
	}

	dy := 0
	if point.Y < rect.Min.Y {
		dy = rect.Min.Y - point.Y
	} else if point.Y >= rect.Max.Y {
		dy = point.Y - rect.Max.Y
	}

	return int64(dx)*int64(dx) + int64(dy)*int64(dy)
}

// isBlankCapture samples the frame for the two failure shapes commonly
// returned by blocked screen-capture APIs: a fully transparent frame or an
// opaque near-black frame. Uniform non-black desktop backgrounds remain valid.
func isBlankCapture(frame image.Image) bool {
	bounds := frame.Bounds()
	if bounds.Empty() {
		return true
	}

	stepX := maxInt(1, bounds.Dx()/32)
	stepY := maxInt(1, bounds.Dy()/24)
	allTransparent := true
	allNearBlack := true
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			red, green, blue, alpha := frame.At(x, y).RGBA()
			if alpha > 0 {
				allTransparent = false
			}
			if red > 0x0200 || green > 0x0200 || blue > 0x0200 {
				allNearBlack = false
			}
			if !allTransparent && !allNearBlack {
				return false
			}
		}
	}
	return allTransparent || allNearBlack
}

// LogicalDisplays converts the physical monitor rects into per-display
// logical (96-DPI) rects relative to the union of all logical rects,
// together with each display's scale factor.
func LogicalDisplays(monitors []physicalMonitor) []Display {
	if len(monitors) == 0 {
		return nil
	}

	unionMin := monitors[0].Rect.Min
	for _, monitor := range monitors[1:] {
		unionMin = image.Point{
			X: minInt(unionMin.X, monitor.Rect.Min.X),
			Y: minInt(unionMin.Y, monitor.Rect.Min.Y),
		}
	}

	type logicalRect struct {
		rect  image.Rectangle
		scale float64
	}
	logical := make([]logicalRect, 0, len(monitors))
	logicalUnionMin := image.Point{X: math.MaxInt, Y: math.MaxInt}
	for _, monitor := range monitors {
		x := float64(monitor.Rect.Min.X-unionMin.X) / monitor.Scale
		y := float64(monitor.Rect.Min.Y-unionMin.Y) / monitor.Scale
		width := float64(monitor.Rect.Dx()) / monitor.Scale
		height := float64(monitor.Rect.Dy()) / monitor.Scale
		rect := piecewiseRoundRect(x, y, width, height)
		logical = append(logical, logicalRect{rect: rect, scale: monitor.Scale})

		logicalUnionMin.X = minInt(logicalUnionMin.X, rect.Min.X)
		logicalUnionMin.Y = minInt(logicalUnionMin.Y, rect.Min.Y)
	}

	displays := make([]Display, 0, len(logical))
	for _, entry := range logical {
		displays = append(displays, Display{
			X:      entry.rect.Min.X - logicalUnionMin.X,
			Y:      entry.rect.Min.Y - logicalUnionMin.Y,
			Width:  entry.rect.Dx(),
			Height: entry.rect.Dy(),
			Scale:  entry.scale,
		})
	}
	return displays
}

func intRound(value float64) int {
	return int(math.Round(value))
}

func piecewiseRoundRect(x float64, y float64, width float64, height float64) image.Rectangle {
	startX := intRound(x)
	startY := intRound(y)
	endX := intRound(x + width)
	endY := intRound(y + height)
	if endX <= startX {
		endX = startX + 1
	}
	if endY <= startY {
		endY = startY + 1
	}
	return image.Rect(startX, startY, endX, endY)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}
