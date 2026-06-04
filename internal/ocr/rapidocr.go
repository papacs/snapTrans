package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Result struct {
	Text   string  `json:"text"`
	Blocks []Block `json:"blocks"`
}

type Block struct {
	Text   string  `json:"text"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

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
	result, err := r.ExtractResult(ctx, imageDataURL)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func (r *RapidOCR) ExtractResult(ctx context.Context, imageDataURL string) (Result, error) {
	if strings.TrimSpace(r.ExecutablePath) == "" {
		return Result{}, errors.New("RapidOCR executable path is required")
	}
	cwd, _ := os.Getwd()
	executable, _ := os.Executable()
	resolvedExecutable, err := ResolveExecutablePath(r.ExecutablePath, cwd, executable)
	if err != nil {
		return Result{}, err
	}

	imageBytes, err := DecodeImageDataURL(imageDataURL)
	if err != nil {
		return Result{}, err
	}
	imageWidth, imageHeight := imageDimensions(imageBytes)

	file, err := os.CreateTemp("", "snaptrans-*.png")
	if err != nil {
		return Result{}, err
	}
	tempPath := file.Name()
	defer os.Remove(tempPath)

	if _, err := file.Write(imageBytes); err != nil {
		_ = file.Close()
		return Result{}, err
	}
	if err := file.Close(); err != nil {
		return Result{}, err
	}

	runCtx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := NewRapidOCRCommand(runCtx, resolvedExecutable, tempPath)
	output, err := cmd.CombinedOutput()
	if runCtx.Err() != nil {
		return Result{}, fmt.Errorf("RapidOCR timed out after %s", r.Timeout)
	}
	if err != nil {
		return Result{}, fmt.Errorf("RapidOCR failed: %w: %s", err, strings.TrimSpace(string(output)))
	}

	return ExtractResultFromJSON(output, imageWidth, imageHeight)
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
	result, err := ExtractResultFromJSON(raw, 0, 0)
	if err != nil {
		return "", err
	}
	return result.Text, nil
}

func ExtractResultFromJSON(raw []byte, imageWidth int, imageHeight int) (Result, error) {
	var decoded any
	if err := decodeFirstJSONValue(raw, &decoded); err != nil {
		return Result{}, fmt.Errorf("invalid OCR JSON: %w", err)
	}

	parts := make([]string, 0)
	blocks := make([]Block, 0)
	collectResult(decoded, imageWidth, imageHeight, &parts, &blocks)
	sortBlocks(blocks)
	blocks = dedupeBlocks(blocks)
	if len(blocks) > 0 {
		parts = parts[:0]
		for _, block := range blocks {
			appendPart(&parts, block.Text)
		}
	}

	return Result{
		Text:   strings.TrimSpace(strings.Join(parts, "\n")),
		Blocks: blocks,
	}, nil
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

func collectResult(value any, imageWidth int, imageHeight int, parts *[]string, blocks *[]Block) bool {
	switch typed := value.(type) {
	case map[string]any:
		text := textFromMap(typed)
		hasChildText := false
		for key, child := range typed {
			if isOCRLeafKey(key) {
				continue
			}
			if collectResult(child, imageWidth, imageHeight, parts, blocks) {
				hasChildText = true
			}
		}
		if hasChildText {
			return true
		}
		if text == "" {
			return false
		}
		appendPart(parts, text)
		if block, ok := blockFromMap(typed, imageWidth, imageHeight, text); ok {
			*blocks = append(*blocks, block)
		}
		return true
	case []any:
		if text, ok := textFromArray(typed); ok {
			appendPart(parts, text)
			if block, ok := blockFromArray(typed, imageWidth, imageHeight, text); ok {
				*blocks = append(*blocks, block)
			}
			return true
		}
		hasChildText := false
		for _, child := range typed {
			if collectResult(child, imageWidth, imageHeight, parts, blocks) {
				hasChildText = true
			}
		}
		return hasChildText
	}
	return false
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

func appendPart(parts *[]string, text string) {
	text = strings.TrimSpace(text)
	if text != "" {
		*parts = append(*parts, text)
	}
}

func textFromMap(value map[string]any) string {
	for _, key := range []string{"text", "rec_text", "content"} {
		if text, ok := value[key].(string); ok {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func textFromArray(value []any) (string, bool) {
	if len(value) < 2 {
		return "", false
	}
	text, ok := value[1].(string)
	return strings.TrimSpace(text), ok && strings.TrimSpace(text) != ""
}

func blockFromMap(value map[string]any, imageWidth int, imageHeight int, text string) (Block, bool) {
	for _, key := range []string{"box", "bbox"} {
		if block, ok := blockFromBox(value[key], imageWidth, imageHeight, text); ok {
			return block, true
		}
	}
	return Block{}, false
}

func blockFromArray(value []any, imageWidth int, imageHeight int, text string) (Block, bool) {
	if len(value) == 0 {
		return Block{}, false
	}
	return blockFromBox(value[0], imageWidth, imageHeight, text)
}

func blockFromBox(value any, imageWidth int, imageHeight int, text string) (Block, bool) {
	points := pointsFromBox(value)
	if len(points) == 0 {
		return Block{}, false
	}

	minX, minY := points[0][0], points[0][1]
	maxX, maxY := minX, minY
	for _, point := range points[1:] {
		minX = math.Min(minX, point[0])
		minY = math.Min(minY, point[1])
		maxX = math.Max(maxX, point[0])
		maxY = math.Max(maxY, point[1])
	}
	if maxX <= minX || maxY <= minY {
		return Block{}, false
	}

	return normalizeBlock(text, minX, minY, maxX-minX, maxY-minY, imageWidth, imageHeight), true
}

func pointsFromBox(value any) [][2]float64 {
	items, ok := value.([]any)
	if !ok || len(items) == 0 {
		return nil
	}

	if len(items) == 4 && allNumbers(items) {
		x1 := numberValue(items[0])
		y1 := numberValue(items[1])
		x2 := numberValue(items[2])
		y2 := numberValue(items[3])
		return [][2]float64{{x1, y1}, {x2, y2}}
	}

	points := make([][2]float64, 0, len(items))
	for _, item := range items {
		point, ok := item.([]any)
		if !ok || len(point) < 2 || !isNumber(point[0]) || !isNumber(point[1]) {
			continue
		}
		points = append(points, [2]float64{numberValue(point[0]), numberValue(point[1])})
	}
	return points
}

func normalizeBlock(text string, x float64, y float64, width float64, height float64, imageWidth int, imageHeight int) Block {
	if imageWidth > 0 {
		x /= float64(imageWidth)
		width /= float64(imageWidth)
	}
	if imageHeight > 0 {
		y /= float64(imageHeight)
		height /= float64(imageHeight)
	}

	return Block{
		Text:   text,
		X:      clampFloat(x, 0, 1),
		Y:      clampFloat(y, 0, 1),
		Width:  clampFloat(width, 0, 1),
		Height: clampFloat(height, 0, 1),
	}
}

func sortBlocks(blocks []Block) {
	sort.SliceStable(blocks, func(left int, right int) bool {
		a := blocks[left]
		b := blocks[right]
		rowTolerance := math.Min(a.Height, b.Height) * 0.7
		if math.Abs(a.Y-b.Y) > rowTolerance {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
}

func dedupeBlocks(blocks []Block) []Block {
	result := make([]Block, 0, len(blocks))
	for _, block := range blocks {
		duplicate := false
		for _, existing := range result {
			if normalizedBlockText(block.Text) == normalizedBlockText(existing.Text) && blockOverlapRatio(block, existing) >= 0.72 {
				duplicate = true
				break
			}
		}
		if !duplicate {
			result = append(result, block)
		}
	}
	return result
}

func normalizedBlockText(text string) string {
	return strings.ToLower(strings.Join(strings.Fields(text), " "))
}

func blockOverlapRatio(left Block, right Block) float64 {
	intersectionWidth := math.Min(left.X+left.Width, right.X+right.Width) - math.Max(left.X, right.X)
	intersectionHeight := math.Min(left.Y+left.Height, right.Y+right.Height) - math.Max(left.Y, right.Y)
	if intersectionWidth <= 0 || intersectionHeight <= 0 {
		return 0
	}

	intersectionArea := intersectionWidth * intersectionHeight
	leftArea := left.Width * left.Height
	rightArea := right.Width * right.Height
	minArea := math.Min(leftArea, rightArea)
	if minArea <= 0 {
		return 0
	}
	return intersectionArea / minArea
}

func imageDimensions(imageBytes []byte) (int, int) {
	config, _, err := image.DecodeConfig(bytes.NewReader(imageBytes))
	if err != nil {
		return 0, 0
	}
	return config.Width, config.Height
}

func isOCRLeafKey(key string) bool {
	return key == "text" || key == "rec_text" || key == "content" || key == "box" || key == "bbox"
}

func allNumbers(items []any) bool {
	for _, item := range items {
		if !isNumber(item) {
			return false
		}
	}
	return true
}

func isNumber(value any) bool {
	switch value.(type) {
	case float64, float32, int, int64, int32, uint, uint64, uint32:
		return true
	default:
		return false
	}
}

func numberValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case int32:
		return float64(typed)
	case uint:
		return float64(typed)
	case uint64:
		return float64(typed)
	case uint32:
		return float64(typed)
	default:
		return 0
	}
}

func clampFloat(value float64, min float64, max float64) float64 {
	return math.Min(math.Max(value, min), max)
}
