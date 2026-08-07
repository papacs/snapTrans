//go:build windows

package ocr

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestRapidOCRCommandsDoNotCreateConsoleWindow(t *testing.T) {
	commands := []*exec.Cmd{
		newRapidOCRWorkerCommand(context.Background(), `C:\RapidOCR\RapidOCR-json.exe`),
		NewRapidOCRCommand(context.Background(), `C:\RapidOCR\RapidOCR-json.exe`, `C:\Temp\shot.png`),
		NewRapidOCRCommandWithImagePathArg(context.Background(), `C:\RapidOCR\RapidOCR-json.exe`, `C:\Temp\shot.png`),
	}

	for _, cmd := range commands {
		require.NotNil(t, cmd.SysProcAttr)
		require.True(t, cmd.SysProcAttr.HideWindow)
		require.NotZero(t, cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW)
	}
}
