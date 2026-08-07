//go:build !windows

package main

import "context"

func availableScreenHeight() int {
	return 0
}

func moveWindowToDisplay(_ context.Context, _ int, _ int) {}
