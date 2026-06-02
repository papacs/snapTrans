package capture

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	"image/png"

	"github.com/kbinani/screenshot"
)

type Result struct {
	Image   string `json:"image"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
	OriginX int    `json:"originX"`
	OriginY int    `json:"originY"`
	Source  string `json:"source"`
}

func AllDisplays(ctx context.Context) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	count := screenshot.NumActiveDisplays()
	if count <= 0 {
		return Result{}, errors.New("no active displays found")
	}

	union := screenshot.GetDisplayBounds(0)
	for i := 1; i < count; i++ {
		union = union.Union(screenshot.GetDisplayBounds(i))
	}

	canvas := image.NewRGBA(image.Rect(0, 0, union.Dx(), union.Dy()))
	for i := 0; i < count; i++ {
		bounds := screenshot.GetDisplayBounds(i)
		img, err := screenshot.CaptureRect(bounds)
		if err != nil {
			return Result{}, err
		}

		target := image.Rect(
			bounds.Min.X-union.Min.X,
			bounds.Min.Y-union.Min.Y,
			bounds.Max.X-union.Min.X,
			bounds.Max.Y-union.Min.Y,
		)
		draw.Draw(canvas, target, img, image.Point{}, draw.Src)
	}

	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		return Result{}, err
	}

	return Result{
		Image:   "data:image/png;base64," + base64.StdEncoding.EncodeToString(buffer.Bytes()),
		Width:   canvas.Bounds().Dx(),
		Height:  canvas.Bounds().Dy(),
		OriginX: union.Min.X,
		OriginY: union.Min.Y,
		Source:  "wails",
	}, nil
}

