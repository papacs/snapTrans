//go:build !windows

package main

import (
	"context"
	"time"
)

func availableScreenHeight() int {
	return 0
}

func moveWindowToDisplay(_ context.Context, _ int, _ int) {}

func waitForWindowHidden() {
	time.Sleep(16 * time.Millisecond)
}
