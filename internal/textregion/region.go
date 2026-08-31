// Package textregion describes layout without tying text to an OCR engine.
package textregion

type Block struct {
	Background string  `json:"background,omitempty"`
	Foreground string  `json:"foreground,omitempty"`
	Text       string  `json:"text"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
}
