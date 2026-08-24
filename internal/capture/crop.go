package capture

import (
	"image"
	"image/color"
	"math"
)

// OCRScaleForRect mirrors the frontend's ocrScaleForRect: selections whose
// short side is smaller than 96 px are enlarged (up to 5x) so the OCR model
// sees legible glyphs. Keeping the backend and frontend crops identical
// guarantees the OCR input does not change when TranslateRegion is used.
func OCRScaleForRect(rect image.Rectangle) float64 {
	shortSide := rect.Dx()
	if rect.Dy() < shortSide {
		shortSide = rect.Dy()
	}
	if shortSide <= 0 {
		return 1
	}

	scale := math.Ceil(96.0 / float64(shortSide))
	return math.Max(1, math.Min(5, scale))
}

// CropForOCR extracts rect from frame, applying the same upscaling rule as
// the frontend crop pipeline with high-quality bilinear filtering.
func CropForOCR(frame image.Image, rect image.Rectangle) *image.RGBA {
	scale := OCRScaleForRect(rect)
	dstWidth := maxInt(1, intRound(float64(rect.Dx())*scale))
	dstHeight := maxInt(1, intRound(float64(rect.Dy())*scale))

	target := image.NewRGBA(image.Rect(0, 0, dstWidth, dstHeight))
	scaleBilinear(target, frame, rect)
	return target
}

// scaleBilinear resizes the srcRect region of src into target using
// bilinear interpolation with center-aligned source coordinates.
func scaleBilinear(target *image.RGBA, src image.Image, srcRect image.Rectangle) {
	srcWidth := srcRect.Dx()
	srcHeight := srcRect.Dy()
	if srcWidth <= 0 || srcHeight <= 0 {
		return
	}

	sampleRect := srcRect.Intersect(src.Bounds())
	if sampleRect.Empty() {
		return
	}

	dstWidth := target.Bounds().Dx()
	dstHeight := target.Bounds().Dy()
	if dstWidth <= 0 || dstHeight <= 0 {
		return
	}

	minX := sampleRect.Min.X
	minY := sampleRect.Min.Y
	maxX := sampleRect.Max.X - 1
	maxY := sampleRect.Max.Y - 1

	// Fast path: RGBA frames dominate captures, and RGBAAt avoids the
	// interface conversion cost of At for every sampled pixel.
	rgbaFrame, isRGBA := src.(*image.RGBA)

	for y := 0; y < dstHeight; y++ {
		sy := (float64(y)+0.5)*float64(srcHeight)/float64(dstHeight) - 0.5 + float64(minY)
		y0 := int(math.Floor(sy))
		fy := sy - float64(y0)
		y0c := clampInt(y0, minY, maxY)
		y1c := clampInt(y0+1, minY, maxY)

		for x := 0; x < dstWidth; x++ {
			sx := (float64(x)+0.5)*float64(srcWidth)/float64(dstWidth) - 0.5 + float64(minX)
			x0 := int(math.Floor(sx))
			fx := sx - float64(x0)
			x0c := clampInt(x0, minX, maxX)
			x1c := clampInt(x0+1, minX, maxX)

			var top rgba8
			if isRGBA {
				top = blendPair(
					blendPair(rgbaFrame.RGBAAt(x0c, y0c), rgbaFrame.RGBAAt(x1c, y0c), fx),
					blendPair(rgbaFrame.RGBAAt(x0c, y1c), rgbaFrame.RGBAAt(x1c, y1c), fx),
					fy,
				)
			} else {
				top = blendPair(
					blendPair(src.At(x0c, y0c), src.At(x1c, y0c), fx),
					blendPair(src.At(x0c, y1c), src.At(x1c, y1c), fx),
					fy,
				)
			}
			target.SetRGBA(x, y, color.RGBA{R: top.R, G: top.G, B: top.B, A: top.A})
		}
	}
}

type rgba8 struct {
	R, G, B, A uint8
}

// RGBA implements color.Color for premultiplied 8-bit RGBA values so nested
// blendPair calls can reuse the same interpolation path.
func (c rgba8) RGBA() (uint32, uint32, uint32, uint32) {
	r := uint32(c.R) * 0x101
	g := uint32(c.G) * 0x101
	b := uint32(c.B) * 0x101
	a := uint32(c.A) * 0x101
	return r, g, b, a
}

// blendPair linearly interpolates two colors in premultiplied 16-bit space
// and returns the result as 8-bit premultiplied RGBA.
func blendPair(left color.Color, right color.Color, fraction float64) rgba8 {
	lr, lg, lb, la := left.RGBA()
	rr, rg, rb, ra := right.RGBA()

	return rgba8{
		R: uint8(lerp16(lr, rr, fraction) >> 8),
		G: uint8(lerp16(lg, rg, fraction) >> 8),
		B: uint8(lerp16(lb, rb, fraction) >> 8),
		A: uint8(lerp16(la, ra, fraction) >> 8),
	}
}

func lerp16(left uint32, right uint32, fraction float64) uint32 {
	return uint32(float64(left) + (float64(right)-float64(left))*fraction)
}

func clampInt(value int, min int, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
