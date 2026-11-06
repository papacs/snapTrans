//go:build windows

// Package childprocess ties long-lived native helpers to the desktop app.
package childprocess

import (
	"errors"
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

type processGuard struct{ handle windows.Handle }

func (guard *processGuard) Close() error {
	if guard == nil || guard.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(guard.handle)
	guard.handle = 0
	return err
}

// Guard uses a kill-on-close Job Object so OCR and API helpers also terminate
// when Windows closes the parent's handles after an abnormal app exit.
func Guard(process *os.Process) (io.Closer, error) {
	if process == nil {
		return nil, errors.New("process is nil")
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	closeJob := true
	defer func() {
		if closeJob {
			_ = windows.CloseHandle(job)
		}
	}()
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return nil, err
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(job, handle); err != nil {
		return nil, err
	}
	closeJob = false
	return &processGuard{handle: job}, nil
}
