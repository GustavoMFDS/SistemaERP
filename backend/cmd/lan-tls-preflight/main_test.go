package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRefuseInvalidInputs(t *testing.T) {
	now := time.Now()
	if err := run(nil, now); err == nil {
		t.Fatal("accepted missing inputs")
	}
	root := filepath.Join(t.TempDir(), "root.pem")
	if err := os.WriteFile(root, []byte("synthetic invalid root"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-root-file", root, "-root-file-sha256", "bad"}, now); err == nil {
		t.Fatal("accepted mismatched root pin")
	}
	if err := run([]string{"-root-file", root, "unexpected"}, now); err == nil {
		t.Fatal("accepted extra arguments")
	}
	if _, err := readInput("relative.pem"); err == nil {
		t.Fatal("accepted relative input")
	}
}
