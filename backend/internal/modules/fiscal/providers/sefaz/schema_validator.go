package sefaz

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxSchemaValidationOutput = 64 << 10

type XMLLintSchemaValidator struct {
	schemaPath string
	binary     string
}

func NewXMLLintSchemaValidator(schemaDir, entrypoint string) (*XMLLintSchemaValidator, error) {
	binary, err := exec.LookPath("xmllint")
	if err != nil {
		return nil, fmt.Errorf("xmllint is required for NFC-e XSD validation: %w", err)
	}
	return newXMLLintSchemaValidator(schemaDir, entrypoint, binary)
}

func newXMLLintSchemaValidator(schemaDir, entrypoint, binary string) (*XMLLintSchemaValidator, error) {
	schemaDir = strings.TrimSpace(schemaDir)
	entrypoint = strings.TrimSpace(entrypoint)
	binary = strings.TrimSpace(binary)
	if schemaDir == "" || entrypoint == "" || binary == "" {
		return nil, fmt.Errorf("schema directory, entrypoint and validator binary are required")
	}
	if filepath.IsAbs(entrypoint) || filepath.Base(entrypoint) != entrypoint ||
		entrypoint == "." || entrypoint == ".." || strings.Contains(entrypoint, string(filepath.Separator)) {
		return nil, fmt.Errorf("NFCE schema entrypoint must be a file name inside the configured schema directory")
	}

	root, err := filepath.Abs(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("resolve NFC-e schema directory: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("resolve NFC-e schema directory symlinks: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat NFC-e schema directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("NFC-e schema path is not a directory")
	}

	schemaPath := filepath.Join(root, entrypoint)
	resolvedSchemaPath, err := filepath.EvalSymlinks(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("resolve NFC-e schema entrypoint symlinks: %w", err)
	}
	relative, err := filepath.Rel(root, resolvedSchemaPath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("NFC-e schema entrypoint resolves outside the configured schema directory")
	}
	schemaPath = resolvedSchemaPath
	schemaInfo, err := os.Stat(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("stat NFC-e schema entrypoint: %w", err)
	}
	if !schemaInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("NFC-e schema entrypoint is not a regular file")
	}
	return &XMLLintSchemaValidator{schemaPath: schemaPath, binary: binary}, nil
}

func (v *XMLLintSchemaValidator) Validate(ctx context.Context, unsignedXML []byte) error {
	if v == nil || strings.TrimSpace(v.schemaPath) == "" || strings.TrimSpace(v.binary) == "" {
		return fmt.Errorf("NFC-e schema validator is not configured")
	}
	if len(bytes.TrimSpace(unsignedXML)) == 0 {
		return fmt.Errorf("NFC-e XML is empty")
	}

	cmd := exec.CommandContext(ctx, v.binary, "--noout", "--nonet", "--schema", v.schemaPath, "-")
	cmd.Stdin = bytes.NewReader(unsignedXML)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedBuffer{buffer: &stderr, limit: maxSchemaValidationOutput}
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("NFC-e XSD validation failed: %s", message)
	}
	return nil
}

type limitedBuffer struct {
	buffer *bytes.Buffer
	limit  int
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	original := len(p)
	remaining := w.limit - w.buffer.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.buffer.Write(p)
	}
	return original, nil
}
