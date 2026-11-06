package capture

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"testing"

	"github.com/stretchr/testify/require"
)

// Repeated Notepad-like four-line sections. Only the narrow left-hand number
// distinguishes sections; common paragraph text must not alias another cycle.
func repeatedSectionDocument(width, height int) *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(frame, frame.Rect, image.NewUniform(color.White), image.Point{}, draw.Src)
	for line, top := 0, 0; top+18 < height; line, top = line+1, top+27 {
		switch line % 4 {
		case 0:
			sectionText(frame, 10, top+7, fmt.Sprintf("%03d", line/4))
			sectionText(frame, 52, top+7, "Section")
		case 1:
			sectionText(frame, 10, top+7, "The same English paragraph repeats here to exercise screenshot translation.")
		case 2:
			sectionText(frame, 10, top+7, "The same English paragraph repeats here with another sentence for this section.")
		}
	}
	return frame
}

func sectionText(frame *image.RGBA, left, top int, text string) {
	for index, glyph := range []byte(text) {
		if glyph == ' ' {
			continue
		}
		for y := 0; y < 11; y++ {
			for x := 0; x < 5; x++ {
				if (int(glyph)*13+y*7+x*11)%(3+x%2) == 0 || x == 0 && y > 1 {
					frame.SetRGBA(left+index*8+x, top+y, color.RGBA{R: 28, G: 32, B: 36, A: 255})
				}
			}
		}
	}
}

func TestCoherentScrollDoesNotAppendMovingCaretOnStationaryPage(t *testing.T) {
	for _, test := range []struct {
		name  string
		width int
		tops  []int
		lefts []int
	}{{"caret", 1, []int{40}, nil}, {"small-island", 12, []int{40}, nil},
		{"thin-distributed-carets", 1, []int{10, 40, 75}, nil},
		{"thin-carets-in-different-columns", 1, []int{10, 40, 75}, []int{20, 30, 40}}} {
		t.Run(test.name, func(t *testing.T) {
			first := image.NewRGBA(image.Rect(0, 0, 80, 100))
			draw.Draw(first, first.Rect, image.NewUniform(color.White), image.Point{}, draw.Src)
			next := rgbaImage(first)
			for index, top := range test.tops {
				left := 20
				if test.lefts != nil {
					left = test.lefts[index]
				}
				draw.Draw(first, image.Rect(left, top, left+test.width, top+20), image.NewUniform(color.Black), image.Point{}, draw.Src)
				draw.Draw(next, image.Rect(left, top-5, left+test.width, top+15), image.NewUniform(color.Black), image.Point{}, draw.Src)
			}
			stitcher, err := newVerticalStitcher(first, defaultScrollPixels)
			require.NoError(t, err)
			advance, found, err := coherentVerticalAdvance(context.Background(), first, next)
			require.NoError(t, err)
			require.False(t, found, "localized animation reported advance=%d", advance)
			require.False(t, stitcher.addManual(next), "moving caret must not append a stationary page")
			require.Equal(t, 100, stitcher.totalHeight)
		})
	}
}

func TestRepeatedSectionsKeepExactSingleLineAndTwentyTwoLineAdvances(t *testing.T) {
	const width, viewport, lineHeight = 1600, 920, 27
	content := repeatedSectionDocument(width, viewport+31*lineHeight)
	stitcher, err := newVerticalStitcher(content.SubImage(image.Rect(0, 0, width, viewport)), defaultScrollPixels)
	require.NoError(t, err)
	for _, line := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 31} {
		frame := content.SubImage(image.Rect(0, line*lineHeight, width, line*lineHeight+viewport))
		before := stitcher.totalHeight
		previous := stitcher.observedFrame
		expectedAdvance := viewport + line*lineHeight - before
		appended := stitcher.addManual(frame)
		if !appended || stitcher.totalHeight != viewport+line*lineHeight {
			next := rgbaImage(frame)
			advance, score, found := verticalAdvance(previous, next)
			t.Logf("line=%d appended=%v output=%d previous=%d candidate=%d score=%g found=%v trueAdvance=%d trueScore=%g candidateFull=%g trueFull=%g stationary=%g", line, appended, stitcher.totalHeight, before, advance, score, found, expectedAdvance, sampledDifference(previous, next, expectedAdvance), directionalDifference(previous, next, advance), directionalDifference(previous, next, expectedAdvance), sampledDifference(previous, next, 0))
		}
		require.True(t, appended, "lost actual line movement at line %d", line)
		require.Equal(t, viewport+line*lineHeight, stitcher.totalHeight, "aliased repeated section at line %d", line)
	}
	require.Equal(t, 1757, stitcher.totalHeight)
	require.True(t, bytes.Equal(content.Pix, stitcher.image().Pix), "stitched rows must preserve the entire numbered document")
	final := content.SubImage(image.Rect(0, 31*lineHeight, width, 31*lineHeight+viewport))
	for index := 0; index < 10; index++ {
		require.False(t, stitcher.addManual(final), "stationary bottom added duplicate rows")
	}
}

func TestRepeatedSectionsKeepLargeAdvanceWithFixedHeader(t *testing.T) {
	content := repeatedSectionDocument(1600, 1514)
	first := stickyHeaderFrame(content, 0, 920, 37)
	next := stickyHeaderFrame(content, 594, 920, 37)
	stitcher, err := newVerticalStitcher(first, defaultScrollPixels)
	require.NoError(t, err)
	require.True(t, stitcher.addManual(next))
	require.Equal(t, 1514, stitcher.totalHeight, "fixed top UI must not make a distant repeated section win")
}

func TestRepeatedSectionsTrackUpwardMovementBeforeAppendingNewRows(t *testing.T) {
	content := repeatedSectionDocument(1600, 1757)
	stitcher, err := newVerticalStitcher(content.SubImage(image.Rect(0, 0, 1600, 920)), defaultScrollPixels)
	require.NoError(t, err)
	for _, step := range []struct {
		line   int
		append bool
	}{{9, true}, {6, false}, {8, false}, {9, false}, {10, true}, {31, true}} {
		appended := stitcher.addManual(content.SubImage(image.Rect(0, step.line*27, 1600, step.line*27+920)))
		require.Equal(t, step.append, appended, "wrong direction or duplicate rows at line %d", step.line)
	}
	require.Equal(t, 1757, stitcher.totalHeight)
}

func TestRepeatedSectionsDoNotDiscardAWholeSectionScroll(t *testing.T) {
	const width, viewport = 1600, 920
	content := repeatedSectionDocument(width, viewport+108)
	stitcher, err := newVerticalStitcher(content.SubImage(image.Rect(0, 0, width, viewport)), defaultScrollPixels)
	require.NoError(t, err)
	appended, err := stitcher.addManualContext(context.Background(), content.SubImage(image.Rect(0, 108, width, 1028)))
	require.NoError(t, err)
	require.True(t, appended, "only section numbers change at the same screen positions, but the page really moved 108px")
	require.Equal(t, 1028, stitcher.totalHeight)
}

func sparseSectionDocument(width, height int) *image.RGBA {
	frame := repeatedSectionDocument(width, height)
	for line, top := 0, 0; top+18 < height; line, top = line+4, top+108 {
		draw.Draw(frame, image.Rect(10, top+7, 34, top+18), image.NewUniform(color.White), image.Point{}, draw.Src)
		frame.SetRGBA(10, top+12, color.RGBA{R: uint8(line/4 + 1), G: 40, B: 80, A: 255})
	}
	return frame
}

func TestSparseSingleRowSectionNumbersOverrideLongerRepeatedOverlap(t *testing.T) {
	content := sparseSectionDocument(1600, 1757)
	stitcher, err := newVerticalStitcher(content.SubImage(image.Rect(0, 243, 1600, 1163)), defaultScrollPixels)
	require.NoError(t, err)
	require.True(t, stitcher.addManual(content.SubImage(image.Rect(0, 837, 1600, 1757))))
	require.Equal(t, 1514, stitcher.totalHeight, "the only differing pixel row must veto the 54px periodic alias")
}

func TestSparseSingleRowSectionNumbersTrackWholePeriod(t *testing.T) {
	content := sparseSectionDocument(1600, 1028)
	stitcher, err := newVerticalStitcher(content.SubImage(image.Rect(0, 0, 1600, 920)), defaultScrollPixels)
	require.NoError(t, err)
	require.True(t, stitcher.addManual(content.SubImage(image.Rect(0, 108, 1600, 1028))))
	require.Equal(t, 1028, stitcher.totalHeight)
}

func TestFixedLeftTopIslandKeepsShortAndLargeActualAdvances(t *testing.T) {
	content := repeatedSectionDocument(1600, 1757)
	for _, footer := range []int{0, 36} {
		t.Run(fmt.Sprintf("alignment-only-footer-%d", footer), func(t *testing.T) {
			frameAt := func(offset int) *image.RGBA {
				frame := rgbaImage(content.SubImage(image.Rect(0, offset, 1600, offset+920)))
				draw.Draw(frame, image.Rect(0, 46, 160, 91), image.NewUniform(color.RGBA{R: 230, G: 230, B: 230, A: 255}), image.Point{}, draw.Src)
				sectionText(frame, 12, 57, "Fixed overlay")
				if footer > 0 {
					draw.Draw(frame, image.Rect(0, 920-footer, 1600, 920), image.NewUniform(color.Gray{Y: 220}), image.Point{}, draw.Src)
					sectionText(frame, 8, 920-footer+8, "Status")
				}
				return frame
			}
			stitcher, err := newVerticalStitcher(frameAt(0), defaultScrollPixels)
			require.NoError(t, err)
			for _, offset := range []int{27, 54, 837} {
				require.True(t, stitcher.addManual(frameAt(offset)), "lost actual movement at offset %d", offset)
				require.Equal(t, 920+offset, stitcher.totalHeight)
			}
		})
	}
}

func TestMovingSmallIconAndFlashingBarsDoNotEstablishPageMovement(t *testing.T) {
	first := image.NewRGBA(image.Rect(0, 0, 80, 100))
	draw.Draw(first, first.Rect, image.NewUniform(color.White), image.Point{}, draw.Src)
	next := rgbaImage(first)
	for y := 0; y < 12; y++ {
		shade := image.NewUniform(color.RGBA{R: uint8(20 + y*7), G: 40, B: 60, A: 255})
		draw.Draw(first, image.Rect(20, 40+y, 32, 41+y), shade, image.Point{}, draw.Src)
		draw.Draw(next, image.Rect(20, 35+y, 32, 36+y), shade, image.Point{}, draw.Src)
	}
	draw.Draw(first, image.Rect(0, 0, 80, 2), image.NewUniform(color.Black), image.Point{}, draw.Src)
	draw.Draw(next, image.Rect(0, 98, 80, 100), image.NewUniform(color.Black), image.Point{}, draw.Src)
	stitcher, err := newVerticalStitcher(first, defaultScrollPixels)
	require.NoError(t, err)
	require.False(t, stitcher.addManual(next))
	require.Equal(t, 100, stitcher.totalHeight)
}

func TestShortOverlapKeepsTwoAdjacentUniqueNumberRows(t *testing.T) {
	content := sparseSectionDocument(1600, 1757)
	for section, top := 0, 0; top+18 < content.Rect.Dy(); section, top = section+1, top+108 {
		content.SetRGBA(10, top+13, color.RGBA{R: uint8(section + 1), G: 90, B: 120, A: 255})
	}
	frameAt := func(offset int) *image.RGBA {
		frame := rgbaImage(content.SubImage(image.Rect(0, offset, 1600, offset+920)))
		draw.Draw(frame, image.Rect(0, 46, 160, 91), image.NewUniform(color.Gray{Y: 230}), image.Point{}, draw.Src)
		draw.Draw(frame, image.Rect(0, 884, 1600, 920), image.NewUniform(color.Gray{Y: 220}), image.Point{}, draw.Src)
		return frame
	}
	stitcher, err := newVerticalStitcher(frameAt(0), defaultScrollPixels)
	require.NoError(t, err)
	for _, offset := range []int{27, 54, 837} {
		require.True(t, stitcher.addManual(frameAt(offset)), "two distinct adjacent number rows must establish short overlap at %d", offset)
		require.Equal(t, 920+offset, stitcher.totalHeight)
	}
}

func TestConflictingUniqueAnchorsCannotFallBackToWeakMatching(t *testing.T) {
	first := image.NewRGBA(image.Rect(0, 0, 80, 100))
	draw.Draw(first, first.Rect, image.NewUniform(color.White), image.Point{}, draw.Src)
	next := rgbaImage(first)
	for index, y := range []int{10, 40, 60, 90} {
		shade := image.NewUniform(color.RGBA{R: uint8(20 + index*30), G: 40, B: 60, A: 255})
		advance := 5 + index%2*5
		draw.Draw(first, image.Rect(20, y, 23, y+1), shade, image.Point{}, draw.Src)
		draw.Draw(next, image.Rect(20, y-advance, 23, y-advance+1), shade, image.Point{}, draw.Src)
	}
	stitcher, err := newVerticalStitcher(first, defaultScrollPixels)
	require.NoError(t, err)
	observed := stitcher.observedFrame
	advance, conclusive, err := coherentVerticalAdvance(context.Background(), first, next)
	require.NoError(t, err)
	require.True(t, conclusive)
	require.Zero(t, advance)
	require.False(t, stitcher.addManual(next))
	require.Same(t, observed, stitcher.observedFrame, "ambiguous motion must preserve the last confirmed position")
	require.Zero(t, stitcher.observedTop)
	require.Equal(t, 100, stitcher.totalHeight)
}

func BenchmarkRepeatedSectionAdvance(b *testing.B) {
	content := repeatedSectionDocument(1600, 1757)
	previous := rgbaImage(content.SubImage(image.Rect(0, 9*27, 1600, 9*27+920)))
	next := rgbaImage(content.SubImage(image.Rect(0, 31*27, 1600, 31*27+920)))
	b.Run("original-sampled-alias", func(b *testing.B) {
		b.ReportAllocs()
		var advance int
		for index := 0; index < b.N; index++ {
			advance, _, _ = verticalAdvance(previous, next)
		}
		b.ReportMetric(float64(advance), "advance-px")
	})
	b.Run("full-row-coherence", func(b *testing.B) {
		b.ReportAllocs()
		var advance int
		for index := 0; index < b.N; index++ {
			var found bool
			var err error
			advance, found, err = coherentVerticalAdvance(context.Background(), previous, next)
			if err != nil || !found || advance != 594 {
				b.Fatalf("wrong coherent advance=%d found=%v err=%v", advance, found, err)
			}
		}
		b.ReportMetric(float64(advance), "advance-px")
	})
}
