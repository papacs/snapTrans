package main

import (
	"errors"
	"image"
	"snaptrans/internal/capture"
)

// Kept separate from workflow startup so clipping can be tested without a
// native event loop or racing a background OCR process.
func cropTranslationRegion(frame image.Image, region TranslateRegionRequest) (*image.RGBA, image.Rectangle, error) {
	rect := image.Rect(region.X, region.Y, region.X+region.Width, region.Y+region.Height).Intersect(frame.Bounds())
	if rect.Empty() {
		return nil, rect, errors.New("selection is outside the captured frame")
	}
	return capture.CropForOCR(frame, rect), rect, nil
}
