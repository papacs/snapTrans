package ocr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

type RapidOCR struct {
	ExecutablePath string
	Timeout        time.Duration
}

func NewRapidOCR(path string, timeout time.Duration) *RapidOCR {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	return &RapidOCR{
		ExecutablePath: path,
		Timeout:        timeout,
	}
}

func (r *RapidOCR) ExtractText(ctx context.Context, imageDataURL string) (string, error) {
	if strings.TrimSpace(r.ExecutablePath) == "" {
		return "", errors.New("RapidOCR executable path is required")
	}

	imageBytes, err := DecodeImageDataURL(imageDataURL)
	if err != nil {
		return "", err
	}

	file, err := os.CreateTemp("", "snaptrans-*.png")
	if err != nil {
		return "", err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if _, err := file.Write(imageBytes); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, r.ExecutablePath, "--image_dir", tempPath)
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return "", fmt.Errorf("RapidOCR timed out after %s", r.Timeout)
	}
	if err != nil {
		return "", fmt.Errorf("RapidOCR failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return ExtractTextFromJSON(output)
}

func DecodeImageDataURL(input string) ([]byte, error) {
	value := strings.TrimSpace(input)
	if comma := strings.Index(value, ","); comma >= 0 {
		value = value[comma+1:]
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("invalid base64 image: %w", err)
	}
	return decoded, nil
}

func ExtractTextFromJSON(raw []byte) (string, error) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return "", fmt.Errorf("invalid OCR JSON: %w", err)
	}

	parts := make([]string, 0)
	collectText(decoded, &parts)
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

func collectText(value any, parts *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"text", "rec_text", "content"} {
			if text, ok := typed[key].(string); ok {
				appendText(parts, text)
			}
		}
		for key, child := range typed {
			if key == "text" || key == "rec_text" || key == "content" {
				continue
			}
			collectText(child, parts)
		}
	case []any:
		if len(typed) >= 2 {
			if text, ok := typed[1].(string); ok {
				appendText(parts, text)
			}
		}
		for _, child := range typed {
			collectText(child, parts)
		}
	}
}

func appendText(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	for _, existing := range *parts {
		if existing == text {
			return
		}
	}
	*parts = append(*parts, text)
}

