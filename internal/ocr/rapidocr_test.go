package ocr

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeImageDataURL(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("image"))
	actual, err := DecodeImageDataURL("data:image/png;base64," + encoded)

	require.NoError(t, err)
	require.Equal(t, []byte("image"), actual)
}

func TestExtractTextFromJSONReadsCommonRapidOCRShapes(t *testing.T) {
	raw := []byte(`{
		"result": [
			{"text": "Hello"},
			{"rec_text": "World"},
			[[], "Nested text", 0.98]
		]
	}`)

	text, err := ExtractTextFromJSON(raw)

	require.NoError(t, err)
	require.Equal(t, "Hello\nWorld\nNested text", text)
}

