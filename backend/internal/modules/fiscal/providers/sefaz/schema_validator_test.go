package sefaz

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Build a tiny native fake xmllint instead of assuming POSIX /bin/sh.
// These tests must exercise the same argument, stdin, and error contract
// on developer Windows machines and CI Linux runners.
func fakeXMLLint(t *testing.T, dir, argsFile, stdinFile string, fail bool) string {
	t.Helper()
	name := "fake-xmllint"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	source := fmt.Sprintf(`package main
import (
	"fmt"
	"io"
	"os"
	"strings"
)
func main() {
	_ = os.WriteFile(%q, []byte(strings.Join(os.Args[1:], "\\n")+"\\n"), 0600)
	input, err := io.ReadAll(os.Stdin)
	if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(4) }
	_ = os.WriteFile(%q, input, 0600)
	if %t {
		fmt.Fprintln(os.Stderr, "schema validation failed")
		os.Exit(3)
	}
}`, argsFile, stdinFile, fail)
	src := filepath.Join(dir, "fake_xmllint.go")
	if err := os.WriteFile(src, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("go", "build", "-o", binary, src)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build fake xmllint: %v: %s", err, output)
	}
	return binary
}

func TestXMLLintSchemaValidatorUsesPinnedSchemaAndNonet(t *testing.T) {
	dir := t.TempDir()
	schema := filepath.Join(dir, "nfe-current.xsd")
	if err := os.WriteFile(schema, []byte("<schema/>"), 0600); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "args.txt")
	stdinFile := filepath.Join(dir, "stdin.xml")
	binary := fakeXMLLint(t, dir, argsFile, stdinFile, false)
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
	binary := fakeXMLLint(t, dir, filepath.Join(dir, "args.txt"), filepath.Join(dir, "stdin.xml"), true)
	validator, err := newXMLLintSchemaValidator(dir, "nfe.xsd", binary)
	if err != nil {
		t.Fatal(err)
	}
	err = validator.Validate(context.Background(), []byte("<NFe/>"))
	if err == nil || !strings.Contains(err.Error(), "schema validation failed") {
		t.Fatalf("expected validation error, got %v", err)
	}
}
