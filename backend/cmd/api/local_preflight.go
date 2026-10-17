package main

import (
	"errors"
	"net/http"
	"strings"

	"github.com/example/sistemaemgo/internal/httpapi"
)

// --check-config must not open a socket, connect to PostgreSQL or Redis,
// modify a database, print secrets or start a Windows service.
func checkConfigArgument(args []string) (bool, error) {
	if len(args) == 0 {
		return false, nil
	}
	if len(args) == 1 && args[0] == "--check-config" {
		return true, nil
	}
	return false, errors.New("only --check-config is supported")
}

// Validate a local UI bundle before opening the database or any listener.
// On normal startup this allows configuration errors to be detected early.
func preflightFrontendBundle(webDir string) error {
	if strings.TrimSpace(webDir) == "" {
		return nil
	}
	_, err := httpapi.WithLocalFrontend(http.NotFoundHandler(), webDir)
	return err
}
