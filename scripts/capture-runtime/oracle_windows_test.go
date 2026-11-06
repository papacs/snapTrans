//go:build ignore

package main

import (
	"image"
	"image/color"
	"testing"
)

func oracleDocument() *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, 80, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 80; x++ {
			frame.SetRGBA(x, y, color.RGBA{R: uint8(y), G: uint8(x), B: uint8(y + x), A: 255})
		}
	}
	return frame
}

func TestOracleUsesStableNativePositionsAndExactPixels(t *testing.T) {
	content := oracleDocument()
	frameAt := func(offset int) *image.RGBA {
		return copyOracleRows(content.SubImage(image.Rect(0, offset, 80, offset+100)).(*image.RGBA), 0)
	}
	oracle := &notepadOracle{}
	oracle.observe(frameAt(0), 0, 10)
	if len(oracle.strips) != 0 {
		t.Fatal("one raster is not a stable baseline")
	}
	oracle.observe(frameAt(0), 0, 10)
	oracle.observe(frameAt(20), 2, 10)
	if oracle.height != 100 {
		t.Fatal("one new-position raster was incorrectly accepted")
	}
	oracle.observe(frameAt(20), 2, 10)
	oracle.observe(frameAt(30), 3, 10)
	oracle.observe(frameAt(30), 3, 10)
	if oracle.height != 130 || oracle.accepted != 2 {
		t.Fatalf("wrong independent native tail assembly: height=%d appends=%d", oracle.height, oracle.accepted)
	}
	export := copyOracleRows(content.SubImage(image.Rect(0, 0, 80, 130)).(*image.RGBA), 0)
	if report := oracle.compare(export); report["pixelEqual"] != true {
		t.Fatalf("correct export did not match independent oracle: %v", report)
	}
	export.SetRGBA(12, 117, color.RGBA{R: 255, A: 255})
	if report := oracle.compare(export); report["pixelEqual"] != false || report["mismatchingPixels"] != 1 || report["mismatchingRows"] != 1 {
		t.Fatalf("oracle missed an incorrect exported pixel: %v", report)
	}
}

func TestOracleRejectsMovementBeforeStableBaseline(t *testing.T) {
	content := oracleDocument()
	oracle := &notepadOracle{}
	oracle.observe(copyOracleRows(content.SubImage(image.Rect(0, 0, 80, 100)).(*image.RGBA), 0), 0, 10)
	frame := copyOracleRows(content.SubImage(image.Rect(0, 10, 80, 110)).(*image.RGBA), 0)
	oracle.observe(frame, 1, 10)
	oracle.observe(frame, 1, 10)
	if oracle.problem == "" || oracle.compare(frame)["pixelEqual"] == true {
		t.Fatal("moving baseline must not be reported as verified")
	}
}

func TestOracleStateRequiresPureKnownEditorAndMeasuredHeight(t *testing.T) {
	editor := map[string]any{"verifiedEditControl15": true, "roiWithinEditorClient": true, "firstVisibleLineZeroBased": 295, "adjacentVisibleLineHeight": 27}
	metadata := map[string]any{"foregroundIsTaskNotepad": true, "editor": editor}
	if position, height, ok := oracleState(metadata); !ok || position != 295 || height != 27 {
		t.Fatal("valid controlled Notepad metadata was rejected")
	}
	editor["roiWithinEditorClient"] = false
	if _, _, ok := oracleState(metadata); ok {
		t.Fatal("mixed footer ROI must not be called a content oracle")
	}
}
