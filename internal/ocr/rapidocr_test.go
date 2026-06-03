package ocr

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
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

func TestExtractTextFromJSONSkipsRapidOCRBannerLines(t *testing.T) {
	raw := []byte("RapidOCR-json v1.1.0\r\nOCR init completed.\r\n" +
		`{"code":100,"data":[{"box":[[31,40],[157,44],[156,93],[29,89]],"score":0.99,"text":"Hello"}]}`)

	text, err := ExtractTextFromJSON(raw)

	require.NoError(t, err)
	require.Equal(t, "Hello", text)
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

func TestNewRapidOCRCommandUsesImageArgumentAndExecutableDirectory(t *testing.T) {
	executable := filepath.Join("C:", "tools", "RapidOCR-json_v0.2.0", "RapidOCR-json.exe")
	imagePath := filepath.Join("C:", "Users", "dell", "AppData", "Local", "Temp", "snaptrans.png")

	cmd := NewRapidOCRCommand(context.Background(), executable, imagePath)

	require.Equal(t, filepath.Dir(executable), cmd.Dir)
	require.Equal(t, executable, cmd.Path)
	require.Equal(t, []string{executable, "--image=" + imagePath}, cmd.Args)
}

func TestResolveExecutablePathReportsCheckedLocations(t *testing.T) {
	temp := t.TempDir()
	_, err := ResolveExecutablePath("./rapidocr_json.exe", temp, filepath.Join(temp, "build", "bin", "snapTrans.exe"))

	require.Error(t, err)
	require.Contains(t, err.Error(), "RapidOCR executable not found")
	require.True(t, strings.Contains(err.Error(), temp))
}
