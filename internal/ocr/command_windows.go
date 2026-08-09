//go:build windows

package ocr

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

func configureOCRCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: windows.CREATE_NO_WINDOW,
	}
}

type ocrProcessGuard struct {
	handle windows.Handle
}

func (guard *ocrProcessGuard) Close() error {
	if guard == nil || guard.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(guard.handle)
	guard.handle = 0
	return err
}

// guardOCRProcess assigns RapidOCR to a kill-on-close Job Object. Windows
// closes this handle when snapTrans exits—even after an abnormal termination—
// preventing orphan OCR workers from continuously consuming CPU.
func guardOCRProcess(process *os.Process) (io.Closer, error) {
	if process == nil {
		return nil, fmt.Errorf("process is nil")
	}

	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJobOnError := true
	defer func() {
		if closeJobOnError {
			_ = windows.CloseHandle(job)
		}
	}()

	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	); err != nil {
		return nil, err
	}

	processHandle, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(process.Pid),
	)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(processHandle)

	if err := windows.AssignProcessToJobObject(job, processHandle); err != nil {
		return nil, err
	}

	closeJobOnError = false
	return &ocrProcessGuard{handle: job}, nil
}
