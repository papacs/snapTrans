package translator

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"

	openai "github.com/sashabaranov/go-openai"
)

func connectionVisionProbe() (openai.ChatCompletionMessage, error) {
	probe := image.NewRGBA(image.Rect(0, 0, 32, 32))
	draw.Draw(probe, probe.Bounds(), image.NewUniform(color.RGBA{R: 255, A: 255}), image.Point{}, draw.Src)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, probe); err != nil {
		return openai.ChatCompletionMessage{}, err
	}
	return visionMessage("Name the dominant color of this test image in English. Reply with only one word.", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(encoded.Bytes()))
}

func validateVisionProbeAnswer(answer string) error {
	if !strings.EqualFold(strings.Trim(answer, " \r\n\t.\"'`*"), "red") {
		return errors.New("image connection test failed: the model could not identify the test image; choose a model with image support or disable direct image input")
	}
	return nil
}
