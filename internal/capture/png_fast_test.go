package capture

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestScreenshotPNGPreservesOpaqueSubimagePixels(t *testing.T) {
	parent := image.NewRGBA(image.Rect(-40, -30, 310, 230))
	random := rand.New(rand.NewSource(2026))
	for offset := 0; offset < len(parent.Pix); offset += 4 {
		value := random.Uint32()
		parent.Pix[offset], parent.Pix[offset+1], parent.Pix[offset+2], parent.Pix[offset+3] = byte(value), byte(value>>8), byte(value>>16), 255
	}
	frame := parent.SubImage(image.Rect(-19, -11, 247, 183)).(*image.RGBA)
	for _, level := range []png.CompressionLevel{png.NoCompression, png.BestSpeed} {
		for attempt := 0; attempt < 2; attempt++ {
			encoded, err := encodeScreenshotPNG(context.Background(), frame, level)
			require.NoError(t, err)
			decoded, err := png.Decode(bytes.NewReader(encoded))
			require.NoError(t, err, "standard PNG decoder verifies zlib and chunk checksums")
			actual := image.NewRGBA(decoded.Bounds())
			draw.Draw(actual, actual.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
			for y := 0; y < frame.Bounds().Dy(); y++ {
				source := frame.Pix[y*frame.Stride : y*frame.Stride+frame.Bounds().Dx()*4]
				require.Equal(t, source, actual.Pix[y*actual.Stride:(y+1)*actual.Stride])
			}
		}
	}
}

func TestScreenshotPNGHandlesNarrowRows(t *testing.T) {
	for _, size := range []image.Point{image.Pt(1, 1), image.Pt(1, 40), image.Pt(43, 1)} {
		frame := solidFrame(size.X, size.Y, color.RGBA{R: 19, G: 71, B: 123, A: 255})
		for _, level := range []png.CompressionLevel{png.NoCompression, png.BestSpeed} {
			encoded, err := encodePNGBytesWithLevel(frame, level)
			require.NoError(t, err)
			decoded, err := png.Decode(bytes.NewReader(encoded))
			require.NoError(t, err)
			assertSamePixels(t, frame, decoded)
		}
	}
}

func TestPNGEncodingKeepsAlphaAndSixteenBitFallback(t *testing.T) {
	transparent := image.NewRGBA(image.Rect(0, 0, 2, 1))
	transparent.SetRGBA(0, 0, color.RGBA{R: 128, A: 128})
	for _, level := range []png.CompressionLevel{png.NoCompression, png.BestSpeed} {
		encoded, err := encodePNGBytesWithLevel(transparent, level)
		require.NoError(t, err)
		decoded, err := png.Decode(bytes.NewReader(encoded))
		require.NoError(t, err)
		assertSamePixels(t, transparent, decoded)
	}

	frame := image.NewNRGBA64(image.Rect(-2, -3, 2, 1))
	for y := frame.Rect.Min.Y; y < frame.Rect.Max.Y; y++ {
		for x := frame.Rect.Min.X; x < frame.Rect.Max.X; x++ {
			frame.SetNRGBA64(x, y, color.NRGBA64{R: 0x1234, G: 0x5678, B: 0xabcd, A: uint16((x+3)*5000 + (y+4)*2000)})
		}
	}
	for _, level := range []png.CompressionLevel{png.NoCompression, png.BestSpeed} {
		encoded, err := encodePNGBytesWithLevel(frame, level)
		require.NoError(t, err)
		decoded, err := png.Decode(bytes.NewReader(encoded))
		require.NoError(t, err)
		for y := 0; y < frame.Rect.Dy(); y++ {
			for x := 0; x < frame.Rect.Dx(); x++ {
				expected := color.NRGBA64Model.Convert(frame.At(x+frame.Rect.Min.X, y+frame.Rect.Min.Y))
				actual := color.NRGBA64Model.Convert(decoded.At(x, y))
				require.Equal(t, expected, actual)
			}
		}
	}
}

func TestPNGEncodingStopsCanceledCaptureAndReusesEncoder(t *testing.T) {
	frame := solidFrame(128, 128, color.RGBA{R: 29, G: 83, B: 137, A: 255})
	for _, level := range []png.CompressionLevel{png.NoCompression, png.BestSpeed} {
		ctx := &cancelDuringPNGContext{Context: context.Background()}
		encoded, err := encodePNGBytesContext(ctx, frame, level)
		require.ErrorIs(t, err, context.Canceled)
		require.Empty(t, encoded, "a partially encoded old capture must never be published")
		encoded, err = encodePNGBytesWithLevel(frame, level)
		require.NoError(t, err)
		decoded, err := png.Decode(bytes.NewReader(encoded))
		require.NoError(t, err, "a canceled compression stream must not corrupt the next screenshot")
		assertSamePixels(t, frame, decoded)
	}
}

type cancelDuringPNGContext struct {
	context.Context
	checks int
}

func (ctx *cancelDuringPNGContext) Err() error {
	ctx.checks++
	if ctx.checks >= 2 {
		return context.Canceled
	}
	return nil
}

func BenchmarkScreenshotPNGFilters(b *testing.B) {
	for _, kind := range []string{"document", "mixed", "noise"} {
		frame := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
		random := rand.New(rand.NewSource(42))
		for y := 0; y < 1080; y++ {
			for x := 0; x < 1920; x++ {
				value := color.RGBA{R: 250, G: 250, B: 250, A: 255}
				if kind == "noise" || (kind == "mixed" && x > 1000 && y > 400) {
					value.R, value.G, value.B = byte(random.Intn(256)), byte(random.Intn(256)), byte(random.Intn(256))
				} else if x > 40 && x < 1700 && y > 40 && y%36 < 12 && x%11 < 6 {
					value.R, value.G, value.B = 30, 40, 50
				}
				frame.SetRGBA(x, y, value)
			}
		}
		for _, mode := range []string{"standard", "sampled"} {
			b.Run(kind+"/"+mode, func(b *testing.B) {
				b.ReportAllocs()
				pool := benchmarkPNGEncoderPool{}
				var result []byte
				for iteration := 0; iteration < b.N; iteration++ {
					var err error
					if mode == "sampled" {
						result, err = encodeScreenshotPNG(context.Background(), frame, png.BestSpeed)
					} else {
						var output bytes.Buffer
						encoder := png.Encoder{CompressionLevel: png.BestSpeed, BufferPool: &pool}
						err = encoder.Encode(&output, frame)
						result = output.Bytes()
					}
					if err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(len(result)), "encoded-B")
			})
		}
	}
}

// Match the previous production encoder's reuse when measuring its baseline.
type benchmarkPNGEncoderPool struct{ buffer *png.EncoderBuffer }

func (pool *benchmarkPNGEncoderPool) Get() *png.EncoderBuffer       { return pool.buffer }
func (pool *benchmarkPNGEncoderPool) Put(buffer *png.EncoderBuffer) { pool.buffer = buffer }
