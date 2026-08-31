package main

import (
	"bytes"
	"encoding/base64"
	"github.com/stretchr/testify/require"
	"image"
	"image/png"
	"testing"
)

func TestPinCoordinatesStayInsideNegativeOriginMonitor(t *testing.T) {
	work := image.Rect(-1920, -200, 0, 880)
	r := fitPinRect(-2100, -400, 800, 600, work)
	require.Equal(t, image.Rect(-1920, -200, -1120, 400), r)
	r = fitPinRect(-200, 700, 4000, 2000, work)
	require.True(t, r.In(work))
	require.Equal(t, 1920, r.Dx())
	require.Equal(t, 960, r.Dy())
}
func TestPinOnlyAcceptsBoundedPNGs(t *testing.T) {
	_, err := decodePinImage("https://example.com/image.png")
	require.Error(t, err)
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 5, 3))))
	img, err := decodePinImage("data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes()))
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 5, 3), img.Bounds())
}
