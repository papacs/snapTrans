//go:build !windows

package codex

import "os/exec"

func configureCommand(_ *exec.Cmd) {}
func resolveExecutable(path string) (string, error) {
	if path == "" {
		path = "codex"
	}
	return exec.LookPath(path)
}
