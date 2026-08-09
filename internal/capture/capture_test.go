package capture

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/kbinani/screenshot"
	"github.com/stretchr/testify/require"
)

func TestEncodePNGDataURLPreservesScreenshotPixels(t *testing.T) {
	frame := image.NewRGBA(image.Rect(0, 0, 2, 2))
	frame.SetRGBA(0, 0, color.RGBA{R: 0x12, G: 0x34, B: 0x56, A: 0xff})
	frame.SetRGBA(1, 0, color.RGBA{R: 0xab, G: 0xcd, B: 0xef, A: 0xff})
	frame.SetRGBA(0, 1, color.RGBA{R: 0xfe, G: 0xdc, B: 0xba, A: 0xff})
	frame.SetRGBA(1, 1, color.RGBA{R: 0x65, G: 0x43, B: 0x21, A: 0xff})

	dataURL, encodedBytes, err := encodePNGDataURL(frame)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(dataURL, pngDataURLPrefix))
	require.Positive(t, encodedBytes)

	encoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, pngDataURLPrefix))
	require.NoError(t, err)
	require.Len(t, encoded, encodedBytes)

	decoded, err := png.Decode(bytes.NewReader(encoded))
	require.NoError(t, err)
	require.Equal(t, frame.Bounds(), decoded.Bounds())
	for y := frame.Bounds().Min.Y; y < frame.Bounds().Max.Y; y++ {
		for x := frame.Bounds().Min.X; x < frame.Bounds().Max.X; x++ {
			require.Equal(
				t,
				color.RGBAModel.Convert(frame.At(x, y)),
				color.RGBAModel.Convert(decoded.At(x, y)),
			)
		}
	}
}

func TestCaptureResultDoesNotExposeInternalMetrics(t *testing.T) {
	payload, err := json.Marshal(Result{
		Image:           "data:image/png;base64,AA==",
		Width:           1,
		Height:          1,
		CaptureDuration: 12 * time.Millisecond,
		EncodeDuration:  34 * time.Millisecond,
		EncodedBytes:    56,
	})
	require.NoError(t, err)
	serialized := strings.ToLower(string(payload))
	require.NotContains(t, serialized, "captureduration")
	require.NotContains(t, serialized, "encodeduration")
	require.NotContains(t, serialized, "encodedbytes")
	require.NotContains(t, serialized, "compressionmode")
}

func TestPNGCompressionLevelPrioritizesLocalCaptureLatency(t *testing.T) {
	require.Equal(t, png.NoCompression, pngCompressionLevel(image.Rect(0, 0, 1920, 1080)))
	require.Equal(t, png.NoCompression, pngCompressionLevel(image.Rect(0, 0, 2560, 1440)))
	require.Equal(t, png.NoCompression, pngCompressionLevel(image.Rect(0, 0, 3840, 2160)))
}

func BenchmarkScreenshotEncoding1080p(b *testing.B) {
	frame := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := 0; y < frame.Bounds().Dy(); y++ {
		for x := 0; x < frame.Bounds().Dx(); x++ {
			frame.SetRGBA(x, y, color.RGBA{
				R: uint8(x % 256),
				G: uint8(y % 256),
				B: uint8((x + y) % 256),
				A: 0xff,
			})
		}
	}

	b.Run("png-best-speed", func(b *testing.B) {
		benchmarkPNGEncoding(b, frame, png.BestSpeed)
	})
	b.Run("png-no-compression", func(b *testing.B) {
		benchmarkPNGEncoding(b, frame, png.NoCompression)
	})
	b.Run("jpeg-quality-95", func(b *testing.B) {
		var buffer bytes.Buffer
		b.ReportAllocs()
		b.SetBytes(int64(len(frame.Pix)))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			buffer.Reset()
			if err := jpeg.Encode(&buffer, frame, &jpeg.Options{Quality: 95}); err != nil {
				b.Fatal(err)
			}
		}
		b.ReportMetric(float64(buffer.Len())/(1024*1024), "encoded-MiB")
	})
}

func BenchmarkActiveDisplayEncoding(b *testing.B) {
	monitors, err := physicalMonitors()
	require.NoError(b, err)
	require.NotEmpty(b, monitors)

	monitor := monitors[0]
	if cursor, cursorErr := cursorPosition(); cursorErr == nil {
		if selected, ok := monitorForPoint(monitors, cursor); ok {
			monitor = selected
		}
	}
	frame, err := screenshot.CaptureRect(monitor.Rect)
	require.NoError(b, err)

	b.Run("adaptive", func(b *testing.B) {
		benchmarkPNGEncoding(b, frame, pngCompressionLevel(frame.Bounds()))
	})
	b.Run("png-best-speed", func(b *testing.B) {
		benchmarkPNGEncoding(b, frame, png.BestSpeed)
	})
	b.Run("png-no-compression", func(b *testing.B) {
		benchmarkPNGEncoding(b, frame, png.NoCompression)
	})
}

func benchmarkPNGEncoding(b *testing.B, frame image.Image, level png.CompressionLevel) {
	b.ReportAllocs()
	b.SetBytes(int64(frame.Bounds().Dx() * frame.Bounds().Dy() * 4))
	b.ResetTimer()
	encodedBytes := 0
	for i := 0; i < b.N; i++ {
		var err error
		if _, encodedBytes, err = encodePNGDataURLWithLevel(frame, level); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(encodedBytes)/(1024*1024), "encoded-MiB")
}

func TestLogicalDisplaysSingleDisplay(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 2560, 1440), Scale: 1.5},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 1)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 0, displays[0].Y)
	require.Equal(t, 1707, displays[0].Width)
	require.Equal(t, 960, displays[0].Height)
	require.InDelta(t, 1.5, displays[0].Scale, 0.0001)
}

func TestLogicalDisplaysMixedDPIKeepsPerDisplayScale(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 2560, 1440), Scale: 2.0},
		{Rect: image.Rect(2560, 0, 3840, 1080), Scale: 1.0},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 2)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 0, displays[0].Y)
	require.Equal(t, 1280, displays[0].Width)
	require.Equal(t, 720, displays[0].Height)
	require.InDelta(t, 2.0, displays[0].Scale, 0.0001)

	require.Equal(t, 2560, displays[1].X)
	require.Equal(t, 0, displays[1].Y)
	require.Equal(t, 1280, displays[1].Width)
	require.Equal(t, 1080, displays[1].Height)
	require.InDelta(t, 1.0, displays[1].Scale, 0.0001)
}

func TestLogicalDisplaysNegativeCoordinates(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(-1920, 0, 0, 1080), Scale: 1.0},
		{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1.0},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 2)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 1920, displays[1].X)
	require.Equal(t, 1920, displays[0].Width)
	require.Equal(t, 1920, displays[1].Width)
}

func TestLogicalDisplaysOrderedBelowPrimary(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1.25},
		{Rect: image.Rect(0, 1080, 3840, 2160), Scale: 2.0},
	}

	displays := LogicalDisplays(monitors)

	require.Len(t, displays, 2)
	require.Equal(t, 0, displays[0].X)
	require.Equal(t, 0, displays[0].Y)
	require.Equal(t, 1536, displays[0].Width)
	require.Equal(t, 864, displays[0].Height)

	require.Equal(t, 0, displays[1].X)
	require.Equal(t, 540, displays[1].Y)
	require.Equal(t, 1920, displays[1].Width)
	require.Equal(t, 540, displays[1].Height)
}

func TestLogicalDisplaysEmpty(t *testing.T) {
	require.Nil(t, LogicalDisplays(nil))
}

func TestMonitorForPointSelectsDisplayContainingCursor(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(-1920, 0, 0, 1080), Scale: 1.0},
		{Rect: image.Rect(0, 0, 2560, 1440), Scale: 1.5},
	}

	monitor, ok := monitorForPoint(monitors, image.Pt(-640, 300))

	require.True(t, ok)
	require.Equal(t, monitors[0], monitor)
}

func TestMonitorForPointUsesNearestDisplayForCoordinateGap(t *testing.T) {
	monitors := []physicalMonitor{
		{Rect: image.Rect(0, 0, 1920, 1080), Scale: 1.0},
		{Rect: image.Rect(2200, 0, 3480, 1024), Scale: 1.0},
	}

	monitor, ok := monitorForPoint(monitors, image.Pt(2100, 400))

	require.True(t, ok)
	require.Equal(t, monitors[1], monitor)
}

func TestMonitorForPointRejectsEmptyMonitorList(t *testing.T) {
	_, ok := monitorForPoint(nil, image.Pt(10, 10))
	require.False(t, ok)
}

func TestIsBlankCaptureDetectsOpaqueBlackFrame(t *testing.T) {
	frame := solidFrame(64, 48, color.RGBA{A: 255})
	require.True(t, isBlankCapture(frame))
}

func TestIsBlankCaptureDetectsTransparentFrame(t *testing.T) {
	frame := solidFrame(64, 48, color.RGBA{})
	require.True(t, isBlankCapture(frame))
}

func TestIsBlankCaptureKeepsLegitimateUniformColor(t *testing.T) {
	frame := solidFrame(64, 48, color.RGBA{R: 18, G: 86, B: 160, A: 255})
	require.False(t, isBlankCapture(frame))
}

func solidFrame(width int, height int, fill color.RGBA) *image.RGBA {
	frame := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			frame.SetRGBA(x, y, fill)
		}
	}
	return frame
}
