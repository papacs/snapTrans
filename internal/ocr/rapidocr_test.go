package ocr

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDecodeImageDataURL(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("image"))
	actual, err := DecodeImageDataURL("data:image/png;base64," + encoded)

	require.NoError(t, err)
	require.Equal(t, []byte("image"), actual)
}

func TestOneShotCancellationWinsBeforeImageOrExecutablePreparation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewRapidOCR("", time.Second).ExtractResult(ctx, "invalid image")
	require.ErrorIs(t, err, context.Canceled)
}

func TestOCRReadingOrderIsStableAcrossStaggeredRows(t *testing.T) {
	a := Block{Text: "A", X: 0.2, Y: 0, Width: 0.08, Height: 0.2}
	b := Block{Text: "B", X: 0.1, Y: 0.1, Width: 0.08, Height: 0.2}
	c := Block{Text: "C", X: 0, Y: 0.2, Width: 0.08, Height: 0.2}
	for _, blocks := range [][]Block{{a, b, c}, {c, b, a}, {b, c, a}, {c, a, b}} {
		sortBlocks(blocks)
		require.Equal(t, []Block{b, a, c}, blocks, "reading order must not depend on detector traversal order")
	}
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

func TestExtractTextFromJSONSkipsRapidOCRBannerLines(t *testing.T) {
	raw := []byte("RapidOCR-json v1.1.0\r\nOCR init completed.\r\n" +
		`{"code":100,"data":[{"box":[[31,40],[157,44],[156,93],[29,89]],"score":0.99,"text":"Hello"}]}`)

	text, err := ExtractTextFromJSON(raw)

	require.NoError(t, err)
	require.Equal(t, "Hello", text)
}

func TestExtractResultHonorsEngineStatusWithoutLeakingDiagnosticText(t *testing.T) {
	for _, raw := range []string{
		`{"code":200,"data":"Image path does not exist: C:/private/screen.png"}`,
		`{"code":299,"data":{"text":"PRIVATE_OCR_DIAGNOSTIC"}}`,
	} {
		result, err := ExtractResultFromJSON([]byte(raw), 100, 100)
		require.Error(t, err)
		require.Empty(t, result)
		require.NotContains(t, err.Error(), "private")
		require.NotContains(t, err.Error(), "PRIVATE_OCR_DIAGNOSTIC")
	}
	result, err := ExtractResultFromJSON([]byte(`{"code":101,"data":"No text found in image. Path: C:/private/screen.png"}`), 100, 100)
	require.NoError(t, err)
	require.Empty(t, result.Text)
	require.Empty(t, result.Blocks)
}

func TestExtractResultFromJSONReturnsNormalizedBlocks(t *testing.T) {
	raw := []byte(`{"code":100,"data":[
		{"box":[[20,10],[80,10],[80,30],[20,30]],"score":0.99,"text":"Neutral"},
		{"box":[[120,10],[190,10],[190,30],[120,30]],"score":0.98,"text":"Positive"}
	]}`)

	result, err := ExtractResultFromJSON(raw, 200, 100)

	require.NoError(t, err)
	require.Equal(t, "Neutral\nPositive", result.Text)
	require.Len(t, result.Blocks, 2)
	require.Equal(t, "Neutral", result.Blocks[0].Text)
	require.InDelta(t, 0.10, result.Blocks[0].X, 0.001)
	require.InDelta(t, 0.10, result.Blocks[0].Y, 0.001)
	require.InDelta(t, 0.30, result.Blocks[0].Width, 0.001)
	require.InDelta(t, 0.20, result.Blocks[0].Height, 0.001)
	require.Equal(t, "Positive", result.Blocks[1].Text)
	require.InDelta(t, 0.60, result.Blocks[1].X, 0.001)
}

func TestExtractResultClipsEdgesBeforeNormalizingBoxes(t *testing.T) {
	raw := []byte(`{"code":100,"data":[
		{"text":"left","box":[[-10,-5],[20,-5],[20,20],[-10,20]]},
		{"text":"right","box":[[90,80],[120,80],[120,110],[90,110]]},
		{"text":"outside","box":[[120,10],[140,10],[140,20],[120,20]]}
	]}`)
	result, err := ExtractResultFromJSON(raw, 100, 100)
	require.NoError(t, err)
	require.Equal(t, "left\nright", result.Text)
	require.Equal(t, []Block{
		{Text: "left", X: 0, Y: 0, Width: 0.2, Height: 0.2},
		{Text: "right", X: 0.9, Y: 0.8, Width: 0.1, Height: 0.2},
	}, result.Blocks)
	outside, err := ExtractResultFromJSON([]byte(`{"code":100,"data":[{"text":"outside","box":[120,10,140,20]}]}`), 100, 100)
	require.NoError(t, err)
	require.Empty(t, outside.Text, "padding detections entirely outside the selected image are not source text")
	require.Empty(t, outside.Blocks)
}

func TestExtractResultFromJSONPrefersLeafBlocksOverAggregateContainerText(t *testing.T) {
	raw := []byte(`{"code":100,"data":{
		"text":"I wanted the ability to audit the process and the results.\nresults are logged to a Google Sheet.",
		"box":[[0,40],[200,40],[200,90],[0,90]],
		"lines":[
			{"box":[[0,40],[200,40],[200,60],[0,60]],"text":"I wanted the ability to audit the process and the results."},
			{"box":[[0,65],[200,65],[200,85],[0,85]],"text":"results are logged to a Google Sheet."}
		]
	}}`)

	result, err := ExtractResultFromJSON(raw, 200, 100)

	require.NoError(t, err)
	require.Equal(t, "I wanted the ability to audit the process and the results.\nresults are logged to a Google Sheet.", result.Text)
	require.Len(t, result.Blocks, 2)
	require.Equal(t, "I wanted the ability to audit the process and the results.", result.Blocks[0].Text)
	require.Equal(t, "results are logged to a Google Sheet.", result.Blocks[1].Text)
}

func TestExtractResultFromJSONDropsDuplicateSiblingBlocks(t *testing.T) {
	raw := []byte(`{"code":100,"data":[
		{"box":[[20,10],[180,10],[180,30],[20,30]],"score":0.99,"text":"Each day, the pipeline"},
		{"box":[[21,11],[181,11],[181,31],[21,31]],"score":0.98,"text":"Each day, the pipeline"},
		{"box":[[20,40],[180,40],[180,60],[20,60]],"score":0.98,"text":"I wanted the ability to audit the process."}
	]}`)

	result, err := ExtractResultFromJSON(raw, 200, 100)

	require.NoError(t, err)
	require.Equal(t, "Each day, the pipeline\nI wanted the ability to audit the process.", result.Text)
	require.Len(t, result.Blocks, 2)
}

func TestResolveExecutablePathFindsRelativePathFromWorkingDirectory(t *testing.T) {
	temp := t.TempDir()
	exe := filepath.Join(temp, "rapidocr_json.exe")
	require.NoError(t, os.WriteFile(exe, []byte("bin"), 0o755))

	resolved, err := ResolveExecutablePath("./rapidocr_json.exe", temp, filepath.Join(temp, "snapTrans.exe"))

	require.NoError(t, err)
	require.Equal(t, exe, resolved)
}

func TestResolveExecutablePathFindsRelativePathNextToExecutable(t *testing.T) {
	temp := t.TempDir()
	exeDir := filepath.Join(temp, "dist")
	require.NoError(t, os.MkdirAll(exeDir, 0o755))
	rapidOCR := filepath.Join(exeDir, "rapidocr_json.exe")
	require.NoError(t, os.WriteFile(rapidOCR, []byte("bin"), 0o755))

	resolved, err := ResolveExecutablePath("./rapidocr_json.exe", filepath.Join(temp, "other"), filepath.Join(exeDir, "snapTrans.exe"))

	require.NoError(t, err)
	require.Equal(t, rapidOCR, resolved)
}

func TestResolveExecutablePathFindsProjectRootFromBuildBin(t *testing.T) {
	temp := t.TempDir()
	projectRoot := filepath.Join(temp, "snapTrans")
	buildBin := filepath.Join(projectRoot, "build", "bin")
	require.NoError(t, os.MkdirAll(buildBin, 0o755))
	rapidOCR := filepath.Join(projectRoot, "rapidocr_json.exe")
	require.NoError(t, os.WriteFile(rapidOCR, []byte("bin"), 0o755))

	resolved, err := ResolveExecutablePath("./rapidocr_json.exe", filepath.Join(temp, "other"), filepath.Join(buildBin, "snapTrans.exe"))

	require.NoError(t, err)
	require.Equal(t, rapidOCR, resolved)
}

func TestResolveExecutablePathFindsExecutableInsideConfiguredDirectory(t *testing.T) {
	temp := t.TempDir()
	rapidOCRDir := filepath.Join(temp, "RapidOCR-json_v0.2.0")
	require.NoError(t, os.MkdirAll(rapidOCRDir, 0o755))
	rapidOCR := filepath.Join(rapidOCRDir, "RapidOCR-json.exe")
	require.NoError(t, os.WriteFile(rapidOCR, []byte("bin"), 0o755))

	resolved, err := ResolveExecutablePath(rapidOCRDir, filepath.Join(temp, "other"), filepath.Join(temp, "snapTrans.exe"))

	require.NoError(t, err)
	require.Equal(t, rapidOCR, resolved)
}

func TestResolveExecutablePathFindsBundledFolderFromUnrelatedWorkingDirectory(t *testing.T) {
	temp := t.TempDir()
	bin := filepath.Join(temp, "portable app")
	bundle := filepath.Join(bin, "RapidOCR-json_v0.2.0")
	require.NoError(t, os.MkdirAll(bundle, 0o755))
	executable := filepath.Join(bundle, "RapidOCR-json.exe")
	require.NoError(t, os.WriteFile(executable, []byte("bin"), 0o755))

	resolved, err := ResolveExecutablePath("./RapidOCR-json_v0.2.0", filepath.Join(temp, "unrelated"), filepath.Join(bin, "snapTrans.exe"))

	require.NoError(t, err)
	require.Equal(t, executable, resolved)
}

func TestNewRapidOCRCommandUsesImageArgumentAndExecutableDirectory(t *testing.T) {
	executable := filepath.Join("C:", "tools", "RapidOCR-json_v0.2.0", "RapidOCR-json.exe")
	imagePath := filepath.Join("C:", "Users", "dell", "AppData", "Local", "Temp", "snaptrans.png")

	cmd := NewRapidOCRCommand(context.Background(), executable, imagePath)

	require.Equal(t, filepath.Dir(executable), cmd.Dir)
	require.Equal(t, executable, cmd.Path)
	require.Equal(t, []string{executable, "--image=" + imagePath}, cmd.Args)
}

func TestNewRapidOCRCommandWithImagePathArgUsesDocumentedShape(t *testing.T) {
	executable := filepath.Join("C:", "tools", "RapidOCR-json_v0.2.0", "RapidOCR-json.exe")
	imagePath := filepath.Join("C:", "Users", "dell", "AppData", "Local", "Temp", "snaptrans.png")

	cmd := NewRapidOCRCommandWithImagePathArg(context.Background(), executable, imagePath)

	require.Equal(t, filepath.Dir(executable), cmd.Dir)
	require.Equal(t, executable, cmd.Path)
	require.Equal(t, []string{executable, "--image_path=" + imagePath}, cmd.Args)
}

func TestResolveExecutablePathReportsCheckedLocations(t *testing.T) {
	temp := t.TempDir()
	_, err := ResolveExecutablePath("./rapidocr_json.exe", temp, filepath.Join(temp, "build", "bin", "snapTrans.exe"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "RapidOCR executable not found")
	require.True(t, strings.Contains(err.Error(), temp))
}
