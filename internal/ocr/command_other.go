//go:build !windows

package ocr

import (
	"io"
	"os"
	"os/exec"
)

func configureOCRCommand(_ *exec.Cmd) {}

func guardOCRProcess(_ *os.Process) (io.Closer, error) {
	return noopProcessGuard{}, nil
}

type noopProcessGuard struct{}

func (noopProcessGuard) Close() error { return nil }
