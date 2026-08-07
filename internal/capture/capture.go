package capture

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	"image/png"
	"math"

	"github.com/kbinani/screenshot"
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
	Image    string    `json:"image"`
	Width    int       `json:"width"`
	Height   int       `json:"height"`
	OriginX  int       `json:"originX"`
	OriginY  int       `json:"originY"`
	Displays []Display `json:"displays"`
	Source   string    `json:"source"`
}

type physicalMonitor struct {
	Rect  image.Rectangle
	Scale float64
}

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

	canvas := image.NewRGBA(image.Rect(0, 0, union.Dx(), union.Dy()))
	for _, monitor := range monitors {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		img, err := screenshot.CaptureRect(monitor.Rect)
		if err != nil {
			return Result{}, err
		}
		if len(monitors) == 1 && isBlankCapture(img) {
			return Result{}, errors.New("screen capture returned a blank image")
		}

		target := image.Rect(
			monitor.Rect.Min.X-union.Min.X,
			monitor.Rect.Min.Y-union.Min.Y,
			monitor.Rect.Max.X-union.Min.X,
			monitor.Rect.Max.Y-union.Min.Y,
		)
		draw.Draw(canvas, target, img, image.Point{}, draw.Src)
	}

	displays := LogicalDisplays(monitors)

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		return Result{}, err
	}

	return Result{
		Image:    "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes()),
		Width:    canvas.Bounds().Dx(),
		Height:   canvas.Bounds().Dy(),
		OriginX:  union.Min.X,
		OriginY:  union.Min.Y,
		Displays: displays,
		Source:   "wails",
	}, nil
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
