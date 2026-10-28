package capture

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVisionCropUsesPhysicalCoordinatesWithoutUpscaling(t *testing.T) {
	frame := image.NewRGBA(image.Rect(-120, -50, 280, 150))
	draw.Draw(frame, frame.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	selection := image.Rect(-80, -30, -40, -10)
	draw.Draw(frame, selection, image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	data, err := EncodeVisionCrop(frame, selection)
	require.NoError(t, err)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, "data:image/jpeg;base64,"))
	require.NoError(t, err)
	crop, err := jpeg.Decode(bytes.NewReader(raw))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 40, 20), crop.Bounds())
	r, g, b, _ := crop.At(20, 10).RGBA()
	require.Greater(t, r, uint32(60000))
	require.Less(t, g, uint32(1000))
	require.Less(t, b, uint32(1000))
}

func TestVisionCropClipsBoundsAndLimitsUploadDimensions(t *testing.T) {
	for _, test := range []struct{ frame, selection, expected image.Rectangle }{
		{image.Rect(0, 0, 100, 80), image.Rect(-20, 20, 50, 100), image.Rect(0, 0, 50, 60)},
		{image.Rect(0, 0, 3200, 1600), image.Rect(0, 0, 3200, 1600), image.Rect(0, 0, 1600, 800)},
	} {
		data, err := EncodeVisionCrop(image.NewRGBA(test.frame), test.selection)
		require.NoError(t, err)
		raw, err := base64.StdEncoding.DecodeString(strings.SplitN(data, ",", 2)[1])
		require.NoError(t, err)
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(raw))
		require.NoError(t, err)
		require.Equal(t, test.expected.Dx(), cfg.Width)
		require.Equal(t, test.expected.Dy(), cfg.Height)
	}
	_, err := EncodeVisionCrop(nil, image.Rect(0, 0, 20, 20))
	require.Error(t, err)
	_, err = EncodeVisionCrop(image.NewRGBA(image.Rect(0, 0, 20, 20)), image.Rect(30, 30, 40, 40))
	require.Error(t, err)
}
