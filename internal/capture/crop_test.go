package capture

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOCRScaleForRectKeepsLargeSelectionsAtOne(t *testing.T) {
	require.Equal(t, 1.0, OCRScaleForRect(image.Rect(0, 0, 200, 100)))
	require.Equal(t, 1.0, OCRScaleForRect(image.Rect(0, 0, 96, 96)))
	require.Equal(t, 1.0, OCRScaleForRect(image.Rect(0, 0, 1920, 1080)))
}

func TestOCRScaleForRectEnlargesSmallSelections(t *testing.T) {
	// Short side 48 -> ceil(96/48) = 2
	require.Equal(t, 2.0, OCRScaleForRect(image.Rect(0, 0, 100, 48)))
	// Short side 20 -> ceil(96/20) = 5, capped at 5
	require.Equal(t, 5.0, OCRScaleForRect(image.Rect(0, 0, 20, 40)))
	// Short side 10 -> ceil(96/10) = 10, capped at 5
	require.Equal(t, 5.0, OCRScaleForRect(image.Rect(0, 0, 10, 100)))
	// Degenerate rect stays at 1
	require.Equal(t, 1.0, OCRScaleForRect(image.Rect(0, 0, 0, 0)))
}

func TestCropForOCRKeepsLargeSelectionSize(t *testing.T) {
	frame := solidFrame(1000, 800, color.RGBA{R: 200, G: 100, B: 50, A: 255})
	rect := image.Rect(100, 50, 300, 150)

	cropped := CropForOCR(frame, rect)

	require.Equal(t, 200, cropped.Bounds().Dx())
	require.Equal(t, 100, cropped.Bounds().Dy())
	require.Equal(t, color.RGBA{R: 200, G: 100, B: 50, A: 255}, cropped.RGBAAt(150, 80))
}

func TestCropForOCRUpscalesSmallSelection(t *testing.T) {
	frame := solidFrame(800, 600, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	// Short side 30 -> ceil(96/30) = 4
	rect := image.Rect(10, 20, 50, 50)

	cropped := CropForOCR(frame, rect)

	require.Equal(t, 160, cropped.Bounds().Dx())
	require.Equal(t, 120, cropped.Bounds().Dy())
	// Bilinear sampling of a solid color stays solid.
	for y := 0; y < cropped.Bounds().Dy(); y += 17 {
		for x := 0; x < cropped.Bounds().Dx(); x += 13 {
			require.Equal(t, color.RGBA{R: 10, G: 20, B: 30, A: 255}, cropped.RGBAAt(x, y))
		}
	}
}

func TestCropForOCRHandlesSelectionOutsideFrame(t *testing.T) {
	frame := solidFrame(100, 100, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	rect := image.Rect(50, 50, 500, 500)

	cropped := CropForOCR(frame, rect)

	// Bilinear sampling clamps to the source edges; the crop still produces
	// a valid image with the frame's solid color.
	require.Equal(t, 450, cropped.Bounds().Dx())
	require.Equal(t, 450, cropped.Bounds().Dy())
	require.Equal(t, color.RGBA{R: 1, G: 2, B: 3, A: 255}, cropped.RGBAAt(0, 0))
	require.Equal(t, color.RGBA{R: 1, G: 2, B: 3, A: 255}, cropped.RGBAAt(449, 449))
}

func TestCropForOCRPreservesLocalContrast(t *testing.T) {
	// Left half black, right half white: the boundary must survive scaling.
	frame := image.NewRGBA(image.Rect(0, 0, 200, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 200; x++ {
			if x < 100 {
				frame.SetRGBA(x, y, color.RGBA{R: 0, G: 0, B: 0, A: 255})
			} else {
				frame.SetRGBA(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
			}
		}
	}

	cropped := CropForOCR(frame, image.Rect(0, 0, 200, 100))

	left := cropped.RGBAAt(10, 50)
	require.Less(t, int(left.R), 40)
	right := cropped.RGBAAt(190, 50)
	require.Greater(t, int(right.R), 215)
	// With a 1:1 scale the boundary lands exactly on pixel 100 (center-aligned
	// sampling), so the right half stays pure white.
	middle := cropped.RGBAAt(100, 50)
	require.Greater(t, int(middle.R), 215)
}
