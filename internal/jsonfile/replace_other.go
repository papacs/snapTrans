//go:build !windows

package jsonfile

import "os"

func replaceFile(from, to string) error { return os.Rename(from, to) }
