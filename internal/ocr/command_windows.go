//go:build windows

package ocr

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureOCRCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}
