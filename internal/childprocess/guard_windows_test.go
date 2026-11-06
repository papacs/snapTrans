//go:build windows

package childprocess

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestGuardRejectsMissingProcess(t *testing.T) {
	_, err := Guard(nil)
	require.Error(t, err)
}

func TestGuardClosesNativeHelperAndIsIdempotent(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestGuardChild$")
	cmd.Env = append(os.Environ(), "SNAPTRANS_GUARD_TEST_CHILD=1")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	guard, err := Guard(cmd.Process)
	require.NoError(t, err)
	require.NoError(t, guard.Close())
	require.NoError(t, guard.Close())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("native helper survived closing its parent guard")
	}
}

func TestGuardChild(t *testing.T) {
	if os.Getenv("SNAPTRANS_GUARD_TEST_CHILD") == "1" {
		for {
			time.Sleep(time.Hour)
		}
	}
}
