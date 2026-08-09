//go:build windows

package ocr

import (
	"context"
	"os/exec"
	"testing"
	"time"

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

func TestOCRProcessGuardKillsWorkerWhenClosed(t *testing.T) {
	cmd := exec.Command("cmd.exe", "/c", "ping -t 127.0.0.1 > nul")
	configureOCRCommand(cmd)
	require.NoError(t, cmd.Start())
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()

	guard, err := guardOCRProcess(cmd.Process)
	require.NoError(t, err)
	require.NotNil(t, guard)
	require.NoError(t, guard.Close())

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("guard close did not terminate OCR child process")
	}
}
