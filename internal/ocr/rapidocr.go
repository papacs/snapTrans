package ocr

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	resolvedExecutable, err := ResolveExecutablePath(r.ExecutablePath, cwd, executable)
	if err != nil {
		return "", err
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

	cmd := NewRapidOCRCommand(runCtx, resolvedExecutable, tempPath)
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return "", fmt.Errorf("RapidOCR timed out after %s", r.Timeout)
	}
	if err != nil {
		return "", fmt.Errorf("RapidOCR failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return ExtractTextFromJSON(output)
}

func NewRapidOCRCommand(ctx context.Context, resolvedExecutable string, imagePath string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, resolvedExecutable, "--image="+imagePath)
	cmd.Dir = filepath.Dir(resolvedExecutable)
	return cmd
}

func ResolveExecutablePath(configuredPath string, workingDirectory string, executablePath string) (string, error) {
	configuredPath = strings.Trim(strings.TrimSpace(configuredPath), `"'`)
	if configuredPath == "" {
		return "", errors.New("RapidOCR executable path is required")
	}

	candidates := make([]string, 0, 5)
	addCandidate := func(base string) {
		if base == "" {
			return
		}
		candidates = appendUniquePath(candidates, filepath.Clean(filepath.Join(base, configuredPath)))
	}

	if filepath.IsAbs(configuredPath) {
		candidates = appendUniquePath(candidates, filepath.Clean(configuredPath))
	} else {
		addCandidate(workingDirectory)
		exeDir := filepath.Dir(executablePath)
		addCandidate(exeDir)
		if filepath.Base(exeDir) == "bin" && filepath.Base(filepath.Dir(exeDir)) == "build" {
			addCandidate(filepath.Dir(filepath.Dir(exeDir)))
		}
	}

	for _, candidate := range candidates {
		resolved, ok := resolveExecutableCandidate(candidate)
		if ok {
			return resolved, nil
		}
	}

	return "", fmt.Errorf(
		"RapidOCR executable not found for %q. Put RapidOCR-json.exe next to snapTrans.exe, put it in the project root, set the RapidOCR-json_v0.2.0 folder, or set an absolute executable path. Checked: %s",
		configuredPath,
		strings.Join(candidates, "; "),
	)
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

func appendUniquePath(paths []string, next string) []string {
	for _, existing := range paths {
		if strings.EqualFold(existing, next) {
			return paths
		}
	}
	return append(paths, next)
}

func resolveExecutableCandidate(candidate string) (string, bool) {
	info, err := os.Stat(candidate)
	if err != nil {
		return "", false
	}
	if !info.IsDir() {
		return candidate, true
	}

	for _, executableName := range []string{"RapidOCR-json.exe", "rapidocr_json.exe", "rapidocr-json.exe"} {
		nested := filepath.Join(candidate, executableName)
		info, err := os.Stat(nested)
		if err == nil && !info.IsDir() {
			return nested, true
		}
	}
	return "", false
}

func ExtractTextFromJSON(raw []byte) (string, error) {
	var decoded any
	if err := decodeFirstJSONValue(raw, &decoded); err != nil {
		return "", fmt.Errorf("invalid OCR JSON: %w", err)
	}

	parts := make([]string, 0)
	collectText(decoded, &parts)
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

func decodeFirstJSONValue(raw []byte, target *any) error {
	for index, char := range raw {
		if char != '{' && char != '[' {
			continue
		}

		decoder := json.NewDecoder(strings.NewReader(string(raw[index:])))
		if err := decoder.Decode(target); err == nil {
			return nil
		}
	}

	return errors.New("no JSON object or array found in OCR output")
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
