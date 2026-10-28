package capture

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/draw"
	"image/jpeg"
	"math"
)

// EncodeVisionCrop keeps the physical selection and avoids OCR upscaling.
// Bound the long edge and JPEG payload to reduce upload and vision prefill.
func EncodeVisionCrop(frame image.Image, rect image.Rectangle) (string, error) {
	if frame == nil {
		return "", errors.New("no screenshot is available")
	}
	rect = rect.Intersect(frame.Bounds())
	if rect.Empty() {
		return "", errors.New("selection is outside the captured frame")
	}
	ratio := math.Min(1, 1600.0/float64(maxInt(rect.Dx(), rect.Dy())))
	target := image.NewRGBA(image.Rect(0, 0, maxInt(1, int(math.Round(float64(rect.Dx())*ratio))), maxInt(1, int(math.Round(float64(rect.Dy())*ratio)))))
	if ratio == 1 {
		draw.Draw(target, target.Bounds(), frame, rect.Min, draw.Src)
	} else {
		scaleBilinear(target, frame, rect)
	}
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, target, &jpeg.Options{Quality: 90}); err != nil {
		return "", err
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
