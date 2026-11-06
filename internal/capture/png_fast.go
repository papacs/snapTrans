package capture

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"io"
	"sync"
)

// Screenshot rows only need Sub or Up filtering. Sampling their cost avoids
// computing all five PNG filters over every full-resolution desktop pixel.
// PNG framing, zlib compression and CRCs retain the original RGB values.
type screenshotPNGEncoder struct {
	compressor *zlib.Writer
	chunks     *bufio.Writer
	row        []byte
}

var screenshotPNGPools [2]sync.Pool

func encodeScreenshotPNG(ctx context.Context, frame *image.RGBA, level png.CompressionLevel) ([]byte, error) {
	compressed := level != png.NoCompression
	poolIndex, zlibLevel := 0, zlib.NoCompression
	if compressed {
		poolIndex, zlibLevel = 1, zlib.BestSpeed
	}
	pool := &screenshotPNGPools[poolIndex]
	state, _ := pool.Get().(*screenshotPNGEncoder)
	if state == nil {
		state = &screenshotPNGEncoder{}
	}
	defer func() {
		// A pooled compressor must never retain a screenshot/output buffer.
		if state.compressor != nil {
			state.compressor.Reset(io.Discard)
		}
		if state.chunks != nil {
			state.chunks.Reset(io.Discard)
		}
		pool.Put(state)
	}()

	bounds := frame.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	rowSize := 1 + width*3
	if cap(state.row) < rowSize {
		state.row = make([]byte, rowSize)
	}
	row := state.row[:rowSize]
	var output bytes.Buffer
	if !compressed {
		rawBytes := rowSize * height
		output.Grow(rawBytes + rawBytes/1000 + 4096)
	}
	output.WriteString("\x89PNG\r\n\x1a\n")
	var header [13]byte
	binary.BigEndian.PutUint32(header[:4], uint32(width))
	binary.BigEndian.PutUint32(header[4:8], uint32(height))
	header[8], header[9] = 8, 2 // 8-bit RGB, lossless and non-interlaced.
	appendPNGChunk(&output, "IHDR", header[:])
	writer := pngIDATWriter{output: &output}
	if state.chunks == nil {
		state.chunks = bufio.NewWriterSize(writer, 32<<10)
	} else {
		state.chunks.Reset(writer)
	}
	if state.compressor == nil {
		var err error
		state.compressor, err = zlib.NewWriterLevel(state.chunks, zlibLevel)
		if err != nil {
			return nil, err
		}
	} else {
		state.compressor.Reset(state.chunks)
	}
	for y := 0; y < height; y++ {
		if y%16 == 15 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		start := y * frame.Stride
		source := frame.Pix[start : start+width*4]
		var previous []byte
		if y > 0 {
			previous = frame.Pix[start-frame.Stride : start-frame.Stride+width*4]
		}
		filterScreenshotRow(row, source, previous, compressed)
		if _, err := state.compressor.Write(row); err != nil {
			return nil, err
		}
	}
	if err := state.compressor.Close(); err != nil {
		return nil, err
	}
	if err := state.chunks.Flush(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	appendPNGChunk(&output, "IEND", nil)
	return output.Bytes(), nil
}

func filterScreenshotRow(row, source, previous []byte, compressed bool) {
	row[0] = 0
	if compressed {
		row[0] = 1 // Sub works for text and horizontal color gradients.
		if len(previous) != 0 {
			subScore, upScore := 0, 0
			step := maxInt(1, len(source)/4/64) * 4
			for x := 4; x < len(source); x += step {
				for channel := 0; channel < 3; channel++ {
					subScore += signedByteMagnitude(source[x+channel] - source[x+channel-4])
					upScore += signedByteMagnitude(source[x+channel] - previous[x+channel])
				}
			}
			if upScore <= subScore {
				row[0] = 2 // Up exploits repeated UI and document rows.
			}
		}
	}
	switch row[0] {
	case 1:
		var red, green, blue byte
		for x, target := 0, 1; x < len(source); x, target = x+4, target+3 {
			row[target], row[target+1], row[target+2] = source[x]-red, source[x+1]-green, source[x+2]-blue
			red, green, blue = source[x], source[x+1], source[x+2]
		}
	case 2:
		for x, target := 0, 1; x < len(source); x, target = x+4, target+3 {
			row[target], row[target+1], row[target+2] = source[x]-previous[x], source[x+1]-previous[x+1], source[x+2]-previous[x+2]
		}
	default:
		for x, target := 0, 1; x < len(source); x, target = x+4, target+3 {
			row[target], row[target+1], row[target+2] = source[x], source[x+1], source[x+2]
		}
	}
}

func signedByteMagnitude(value byte) int {
	if value < 128 {
		return int(value)
	}
	return 256 - int(value)
}

type pngIDATWriter struct{ output *bytes.Buffer }

func (writer pngIDATWriter) Write(data []byte) (int, error) {
	appendPNGChunk(writer.output, "IDAT", data)
	return len(data), nil
}

func appendPNGChunk(output *bytes.Buffer, kind string, data []byte) {
	var header [8]byte
	binary.BigEndian.PutUint32(header[:4], uint32(len(data)))
	copy(header[4:], kind)
	output.Write(header[:])
	output.Write(data)
	checksum := crc32.Update(0, crc32.IEEETable, header[4:])
	checksum = crc32.Update(checksum, crc32.IEEETable, data)
	var footer [4]byte
	binary.BigEndian.PutUint32(footer[:], checksum)
	output.Write(footer[:])
}
