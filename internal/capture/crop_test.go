package capture

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkOCRCropPipeline(b *testing.B) {
	frame := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < frame.Rect.Dy(); y++ {
		for x := 0; x < frame.Rect.Dx(); x++ {
			frame.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	for _, size := range []image.Point{{505, 89}, {580, 388}, {599, 500}} {
		b.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(b *testing.B) {
			rect := image.Rect(120, 80, 120+size.X, 80+size.Y)
			b.Run("crop", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					CropForOCR(frame, rect)
				}
			})
			cropped := CropForOCR(frame, rect)
			b.Run("png", func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := EncodePNGBytes(cropped); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func TestOCRCropDoesNotAllocatePerPixel(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 800, 600))
	for _, rect := range []image.Rectangle{image.Rect(10, 20, 590, 408), image.Rect(10, 20, 515, 109)} {
		allocations := testing.AllocsPerRun(3, func() { CropForOCR(frame, rect) })
		require.Less(t, allocations, 16.0, "crop allocations must stay bounded instead of growing with pixel count")
	}
}

func TestOCRCropAtOriginalSizePreservesPixelsAndCoordinates(t *testing.T) {
	parent := image.NewRGBA(image.Rect(-80, -60, 500, 400))
	for y := parent.Rect.Min.Y; y < parent.Rect.Max.Y; y++ {
		for x := parent.Rect.Min.X; x < parent.Rect.Max.X; x++ {
			parent.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: 255})
		}
	}
	frame := parent.SubImage(image.Rect(-40, -30, 400, 350))
	for _, rect := range []image.Rectangle{image.Rect(-20, -10, 110, 100), image.Rect(120, 100, 250, 220)} {
		cropped := CropForOCR(frame, rect)
		require.Equal(t, image.Rect(0, 0, rect.Dx(), rect.Dy()), cropped.Bounds())
		for y := 0; y < rect.Dy(); y++ {
			for x := 0; x < rect.Dx(); x++ {
				require.Equal(t, frame.At(rect.Min.X+x, rect.Min.Y+y), cropped.At(x, y))
			}
		}
		original := frame.At(rect.Min.X, rect.Min.Y)
		cropped.SetRGBA(0, 0, color.RGBA{})
		require.Equal(t, original, frame.At(rect.Min.X, rect.Min.Y), "crop pixels must not alias the frozen screenshot")
	}
}

func TestOCRCropAtOriginalSizePreservesTransparentSourcePixels(t *testing.T) {
	frame := image.NewNRGBA(image.Rect(-20, -10, 180, 140))
	for y := frame.Rect.Min.Y; y < frame.Rect.Max.Y; y++ {
		for x := frame.Rect.Min.X; x < frame.Rect.Max.X; x++ {
			frame.SetNRGBA(x, y, color.NRGBA{R: uint8(x), G: uint8(y), B: uint8(x + y), A: uint8(x - y)})
		}
	}
	rect := image.Rect(-10, 0, 120, 100)
	cropped := CropForOCR(frame, rect)
	for y := 0; y < rect.Dy(); y++ {
		for x := 0; x < rect.Dx(); x++ {
			r, g, b, a := frame.At(rect.Min.X+x, rect.Min.Y+y).RGBA()
			require.Equal(t, color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}, cropped.RGBAAt(x, y))
		}
	}
}

func TestScaledRGBACropMatchesGenericBilinearPixels(t *testing.T) {
	frame := image.NewRGBA(image.Rect(-80, -60, 500, 400))
	for y := frame.Rect.Min.Y; y < frame.Rect.Max.Y; y++ {
		for x := frame.Rect.Min.X; x < frame.Rect.Max.X; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: uint8(x) / 2, G: uint8(y) / 2, B: uint8(x+y) / 2, A: 128})
		}
	}
	// Hiding the concrete type keeps the original generic sampler as a reference.
	generic := struct{ image.Image }{frame}
	for _, rect := range []image.Rectangle{image.Rect(-20, -10, 110, 50), image.Rect(120, 100, 200, 130), image.Rect(10, 20, 30, 35)} {
		cropped := CropForOCR(frame, rect)
		expected := CropForOCR(generic, rect)
		require.Equal(t, expected.Bounds(), cropped.Bounds())
		require.Equal(t, expected.Pix, cropped.Pix, "resampling must preserve every premultiplied RGBA pixel")
	}
}

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
