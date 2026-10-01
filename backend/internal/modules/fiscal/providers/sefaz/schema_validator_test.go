package sefaz

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestXMLLintSchemaValidatorUsesPinnedSchemaAndNonet(t *testing.T) {
	dir := t.TempDir()
	schema := filepath.Join(dir, "nfe-current.xsd")
	if err := os.WriteFile(schema, []byte("<schema/>"), 0600); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.xml")
	binary := filepath.Join(dir, "fake-xmllint")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + shellQuote(argsFile) + "\ncat > " + shellQuote(stdinFile) + "\nexit 0\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}

	validator, err := newXMLLintSchemaValidator(dir, "nfe-current.xsd", binary)
	if err != nil {
		t.Fatalf("newXMLLintSchemaValidator: %v", err)
	}
	xml := []byte("<NFe><infNFe/></NFe>")
	if err := validator.Validate(context.Background(), xml); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	gotArgs := string(args)
	for _, want := range []string{"--noout", "--nonet", "--schema", schema, "-"} {
		if !strings.Contains(gotArgs, want) {
			t.Fatalf("args missing %q: %s", want, gotArgs)
		}
	}
	stdin, err := os.ReadFile(stdinFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != string(xml) {
		t.Fatalf("stdin=%q, want %q", stdin, xml)
	}
}

func TestXMLLintSchemaValidatorRejectsInvalidEntrypoint(t *testing.T) {
	dir := t.TempDir()
	if _, err := newXMLLintSchemaValidator(dir, "../nfe.xsd", "/bin/true"); err == nil {
		t.Fatal("expected traversal entrypoint to fail")
	}
}

func TestXMLLintSchemaValidatorPropagatesValidationError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nfe.xsd"), []byte("<schema/>"), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(dir, "fake-xmllint")
	script := "#!/bin/sh\necho 'schema validation failed' >&2\nexit 3\n"
	if err := os.WriteFile(binary, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	validator, err := newXMLLintSchemaValidator(dir, "nfe.xsd", binary)
	if err != nil {
		t.Fatal(err)
	}
	err = validator.Validate(context.Background(), []byte("<NFe/>"))
	if err == nil || !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
