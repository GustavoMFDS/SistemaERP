package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi"
)

type startupMode uint8

const (
	startupServe startupMode = iota
	startupConfigCheck
	startupDependencyCheck
)

// The preflight flags must never be combined with a service startup or
// unknown arguments that could silently enable unsafe modes.
func parseStartupMode(args []string) (startupMode, error) {
	if len(args) == 0 {
		return startupServe, nil
	}
	if len(args) == 1 {
		switch args[0] {
		case "--check-config":
			return startupConfigCheck, nil
		case "--check-dependencies":
			return startupDependencyCheck, nil
		}
	}
	return startupServe, errors.New("only --check-config or --check-dependencies is supported")
}

// Validate a local UI bundle before opening the database or any listener.
func preflightFrontendBundle(webDir string) error {
	if strings.TrimSpace(webDir) == "" {
		return nil
	}
	_, err := httpapi.WithLocalFrontend(http.NotFoundHandler(), webDir)
	return err
}
