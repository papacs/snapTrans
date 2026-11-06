package codex

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type guardCloseFunc func() error

func (close guardCloseFunc) Close() error { return close() }

func TestClosingConnectionReleasesNativeProcessGuardOnce(t *testing.T) {
	c, err := start(context.Background(), fakeExecutable(t, "fast"))
	require.NoError(t, err)
	t.Cleanup(c.close)
	require.NotNil(t, c.processGuard)
	original := c.processGuard
	closes := 0
	c.processGuard = guardCloseFunc(func() error { closes++; return original.Close() })
	c.close()
	c.close()
	require.Equal(t, 1, closes)
	require.Nil(t, c.processGuard)
	require.NotNil(t, c.cmd.ProcessState)
	_, err = os.Stat(c.dir)
	require.True(t, os.IsNotExist(err), "the private runtime directory must be released with the process")
}
