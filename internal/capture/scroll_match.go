package capture

import (
	"encoding/binary"
	"image"
)

// Captured frames are RGBA. Reading their sampled pixels directly avoids
// repeated image.Image method calls in every candidate overlap comparison.
// The sample positions, RGB distances and row trimming match the generic path.
func sampledRGBADifferenceKeepingRows(previous, next *image.RGBA, advance, keptTenths int) float64 {
	width, height := previous.Rect.Dx(), previous.Rect.Dy()
	if width != next.Rect.Dx() || height != next.Rect.Dy() || advance < 0 || advance >= height {
		return 1e9
	}
	left, right, bottom := width/20, width-width/20, height-advance
	if right <= left || bottom <= 0 {
		return 1e9
	}
	stepX, stepY := maxInt(1, (right-left)/28), maxInt(1, bottom/28)
	// floor(bottom/28) spacing yields at most 55 sampled rows.
	var storage [64]float64
	rows := storage[:0]
	samples := ((right-left-1)/stepX + 1) * 3
	for y := 0; y < bottom; y += stepY {
		previousOffset := (y+advance)*previous.Stride + left*4
		nextOffset := y*next.Stride + left*4
		firstPrevious := binary.LittleEndian.Uint32(previous.Pix[previousOffset : previousOffset+4])
		firstNext := binary.LittleEndian.Uint32(next.Pix[nextOffset : nextOffset+4])
		difference, hasFeature := 0, false
		for x := left; x < right; x += stepX {
			previousPixel := binary.LittleEndian.Uint32(previous.Pix[previousOffset : previousOffset+4])
			nextPixel := binary.LittleEndian.Uint32(next.Pix[nextOffset : nextOffset+4])
			hasFeature = hasFeature || previousPixel != firstPrevious || nextPixel != firstNext
			difference += absByteDifference(byte(previousPixel), byte(nextPixel))
			difference += absByteDifference(byte(previousPixel>>8), byte(nextPixel>>8))
			difference += absByteDifference(byte(previousPixel>>16), byte(nextPixel>>16))
			previousOffset += stepX * 4
			nextOffset += stepX * 4
		}
		if !hasFeature {
			previousOffset = minInt(height-1, y+advance+1)*previous.Stride + left*4
			nextOffset = minInt(height-1, y+1)*next.Stride + left*4
			hasFeature = binary.LittleEndian.Uint32(previous.Pix[previousOffset:previousOffset+4]) != firstPrevious ||
				binary.LittleEndian.Uint32(next.Pix[nextOffset:nextOffset+4]) != firstNext
		}
		if difference > 0 || hasFeature {
			rows = append(rows, float64(difference)/float64(samples))
		}
	}
	return trimmedScrollRowScore(rows, advance, keptTenths)
}
