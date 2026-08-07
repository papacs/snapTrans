// Package singleinstance guarantees that only one snapTrans process is
// running at a time using a Windows named mutex.
package singleinstance

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var kernel32 = windows.NewLazySystemDLL("kernel32.dll")

var (
	createMutexW = kernel32.NewProc("CreateMutexW")
	closeHandle  = kernel32.NewProc("CloseHandle")
)

var ErrAlreadyRunning = errors.New("another snapTrans instance is already running")

type Instance struct {
	handle windows.Handle
}

// Acquire creates or opens the named mutex. If the mutex already exists,
// ErrAlreadyRunning is returned and the caller should terminate.
func Acquire(name string) (*Instance, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("single instance: invalid name: %w", err)
	}

	handle, _, callErr := createMutexW.Call(0, 0, uintptr(unsafe.Pointer(namePtr)))
	if handle == 0 {
		return nil, fmt.Errorf("single instance: create mutex: %w", callErr)
	}
	if isAlreadyExistsError(callErr) {
		_, _, _ = closeHandle.Call(handle)
		return nil, ErrAlreadyRunning
	}

	return &Instance{handle: windows.Handle(handle)}, nil
}

func isAlreadyExistsError(callErr error) bool {
	var errno syscall.Errno
	return errors.As(callErr, &errno) && errno == syscall.Errno(windows.ERROR_ALREADY_EXISTS)
}

// Release closes the underlying mutex handle. It is safe to call multiple
// times and with a nil receiver.
func (i *Instance) Release() error {
	if i == nil || i.handle == 0 {
		return nil
	}
	_, _, _ = closeHandle.Call(uintptr(i.handle))
	i.handle = 0
	return nil
}
