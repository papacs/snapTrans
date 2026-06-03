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
