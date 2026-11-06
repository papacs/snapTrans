package capture

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"math/bits"
)

type scrollRow struct {
	signature uint32
	featured  bool
}

// A row that occurs exactly once in each frame retains even a one-pixel section
// number. Repeated paragraphs cannot outweigh these distinct anchors by having
// a longer overlap. A conclusive zero advance vetoes conflicting movement.
func coherentVerticalAdvance(ctx context.Context, previous, next *image.RGBA) (int, bool, error) {
	previousRows, err := fingerprintScrollRows(ctx, previous)
	if err != nil {
		return 0, false, err
	}
	nextRows, err := fingerprintScrollRows(ctx, next)
	if err != nil {
		return 0, false, err
	}
	bands, wide, err := scrollChangeDistribution(ctx, previous, next, previousRows, nextRows)
	if err != nil || bands < 3 {
		return 0, false, err
	}
	previousPositions := uniqueScrollRowPositions(previousRows)
	nextPositions := uniqueScrollRowPositions(nextRows)
	support := make(map[int]scrollAnchorSupport)
	height := len(previousRows)
	maxAdvance := height - maxInt(12, height/10)
	checked := 0
	for signature, previousY := range previousPositions {
		checked++
		if checked%64 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		nextY := nextPositions[signature]
		if previousY == 0 || nextY == 0 {
			continue
		}
		advance := previousY - nextY
		if advance < -maxAdvance || advance > maxAdvance {
			continue
		}
		value := support[advance]
		if value.count == 0 {
			value.first, value.last = nextY-1, nextY-1
		}
		value.count++
		value.first, value.last = minInt(value.first, nextY-1), maxInt(value.last, nextY-1)
		value.bands |= 1 << ((nextY - 1) * 4 / height)
		support[advance] = value
	}
	advance, found, movingAnchor := 0, false, false
	for candidate, value := range support {
		if candidate == 0 {
			continue
		}
		movingAnchor = true
		if value.count < 2 {
			continue
		}
		overlap := height - maxInt(candidate, -candidate)
		separation := maxInt(8, overlap/4)
		// A near-viewport jump may leave only one numbered text line visible.
		// Two distinct full-row signatures may be adjacent glyph rows; imposing
		// another pixel-spacing rule would discard the only surviving evidence.
		if overlap < height/4 && wide {
			separation = 0
		}
		if value.last-value.first < separation {
			continue
		}
		if found || bits.OnesCount(support[0].bands) >= 3 {
			return 0, true, nil // Conflicting anchors must not enter weak matching.
		}
		advance, found = candidate, true
	}
	if err := ctx.Err(); err != nil {
		return 0, false, err
	}
	if !found && movingAnchor {
		return 0, true, nil // Local/insufficient anchors cannot license weak aliases.
	}
	return advance, found, nil
}

type scrollAnchorSupport struct {
	count, first, last int
	bands              uint
}

func uniqueScrollRowPositions(rows []scrollRow) map[uint32]int {
	positions := make(map[uint32]int, len(rows))
	for y, row := range rows {
		if !row.featured {
			continue
		}
		if _, exists := positions[row.signature]; exists {
			positions[row.signature] = 0
		} else {
			positions[row.signature] = y + 1 // Zero marks absent or repeated rows.
		}
	}
	return positions
}

// A moving caret can have a perfect row overlap on an otherwise blank page.
// Only override the local-animation stationary guard when changed pixels span
// most of the page height. Narrow section numbers qualify when they change at
// multiple positions; a thin caret or one small animated island does not.
func scrollChangeDistribution(ctx context.Context, previous, next *image.RGBA, previousRows, nextRows []scrollRow) (int, bool, error) {
	width, height := previous.Rect.Dx(), len(previousRows)
	bands, wideChangedRow := uint(0), false
	for y, row := range previousRows {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, false, err
			}
		}
		if row.signature == nextRows[y].signature {
			continue
		}
		bands |= 1 << (y * 4 / height)
		if !wideChangedRow {
			left, right := width, -1
			previousPixels := previous.Pix[y*previous.Stride : y*previous.Stride+width*4]
			nextPixels := next.Pix[y*next.Stride : y*next.Stride+width*4]
			for x := 0; x < width; x++ {
				offset := x * 4
				if binary.LittleEndian.Uint32(previousPixels[offset:offset+4]) != binary.LittleEndian.Uint32(nextPixels[offset:offset+4]) {
					left, right = minInt(left, x), maxInt(right, x)
				}
			}
			wideChangedRow = right-left >= 2
		}
		if bits.OnesCount(bands) >= 3 && wideChangedRow {
			return bits.OnesCount(bands), true, nil
		}
	}
	return bits.OnesCount(bands), wideChangedRow, nil
}

func fingerprintScrollRows(ctx context.Context, frame *image.RGBA) ([]scrollRow, error) {
	rows := make([]scrollRow, frame.Rect.Dy())
	width := frame.Rect.Dx() * 4
	for y := range rows {
		if y%64 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		pixels := frame.Pix[y*frame.Stride : y*frame.Stride+width]
		rows[y].signature = crc32.ChecksumIEEE(pixels)
		first := binary.LittleEndian.Uint32(pixels[:4])
		for offset := 4; offset < width; offset += 4 {
			if binary.LittleEndian.Uint32(pixels[offset:offset+4]) != first {
				rows[y].featured = true
				break
			}
		}
	}
	return rows, nil
}
