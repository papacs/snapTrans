//go:build windows

package ocr

import (
	"io"
	"os"
	"os/exec"
	"snaptrans/internal/childprocess"
	"syscall"

	"golang.org/x/sys/windows"
)

func configureOCRCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}

func guardOCRProcess(process *os.Process) (io.Closer, error) {
	return childprocess.Guard(process)
}
