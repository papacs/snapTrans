//go:build !windows

package ocr

import "os/exec"

func configureOCRCommand(_ *exec.Cmd) {}
