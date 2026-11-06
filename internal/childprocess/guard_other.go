//go:build !windows

package childprocess

import (
	"io"
	"os"
)

type processGuard struct{}

func (processGuard) Close() error { return nil }

func Guard(_ *os.Process) (io.Closer, error) { return processGuard{}, nil }
