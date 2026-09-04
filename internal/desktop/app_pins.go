package desktop

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/png"
	"strings"
)

type PinRequest struct {
	Image string `json:"image"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
}

func decodePinImage(data string) (image.Image, error) {
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(data, prefix) || len(data) > 24*1024*1024 {
		return nil, errors.New("pin requires a PNG under 18 MB")
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(data, prefix))
	if err != nil {
		return nil, err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	if cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return nil, errors.New("pin image must be within 16 megapixels")
	}
	return png.Decode(bytes.NewReader(raw))
}
func (a *App) PinImage(request PinRequest) error {
	a.mu.Lock()
	enabled := a.cfg.Features.Pin
	a.mu.Unlock()
	if !enabled {
		return errors.New("pinning is disabled in settings")
	}
	img, err := decodePinImage(request.Image)
	if err != nil {
		return err
	}
	return showNativePin(img, request.X, request.Y)
}

// Clamp in physical desktop coordinates, including negative monitor origins.
func fitPinRect(x, y, w, h int, work image.Rectangle) image.Rectangle {
	if w > work.Dx() {
		h = max(1, h*work.Dx()/w)
		w = work.Dx()
	}
	if h > work.Dy() {
		w = max(1, w*work.Dy()/h)
		h = work.Dy()
	}
	x = max(work.Min.X, min(x, work.Max.X-w))
	y = max(work.Min.Y, min(y, work.Max.Y-h))
	return image.Rect(x, y, x+w, y+h)
}
