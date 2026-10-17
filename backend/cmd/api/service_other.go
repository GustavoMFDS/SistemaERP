//go:build !windows

package main

import "context"

// Native Windows service dispatch is not needed on Linux or macOS.
func maybeRunWindowsService(_ context.Context, _ context.CancelFunc) (bool, error) {
	return false, nil
}
