//go:build windows

package codex

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

func resolveExecutable(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		// The desktop app and npm CLI can have different versions. Prefer the
		// desktop runtime that manages the user's current subscription session.
		if local := desktopExecutable(os.Getenv("LOCALAPPDATA")); local != "" {
			return local, nil
		}
		path = "codex"
	}
	resolved, err := exec.LookPath(path)
	if err != nil {
		return "", fmt.Errorf("Codex CLI not found; install it or set the path to codex.exe in model settings: %w", err)
	}
	if strings.EqualFold(filepath.Ext(resolved), ".exe") {
		return resolved, nil
	}
	// npm shims cannot be executed directly by Go. Resolve their native binary
	// instead of invoking a command shell with a user-provided path.
	arch, triple := "x64", "x86_64-pc-windows-msvc"
	if runtime.GOARCH == "arm64" {
		arch, triple = "arm64", "aarch64-pc-windows-msvc"
	}
	base := filepath.Join(filepath.Dir(resolved), "node_modules", "@openai")
	for _, root := range []string{filepath.Join(base, "codex", "node_modules", "@openai", "codex-win32-"+arch), filepath.Join(base, "codex-win32-"+arch), filepath.Join(base, "codex")} {
		candidate := filepath.Join(root, "vendor", triple, "codex", "codex.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cannot resolve native Codex executable from %s; enter the full path to codex.exe", resolved)
}

func desktopExecutable(localAppData string) string {
	if localAppData == "" {
		return ""
	}
	root := filepath.Join(localAppData, "OpenAI", "Codex", "bin")
	entries, _ := os.ReadDir(root)
	var latest time.Time
	var path string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(root, entry.Name(), "codex.exe")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && (path == "" || info.ModTime().After(latest)) {
			path, latest = candidate, info.ModTime()
		}
	}
	return path
}
