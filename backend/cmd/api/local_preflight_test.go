package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckConfigArgument(t *testing.T) {
	cases := []struct {
		name string
		args []string
		check bool
		bad bool
	}{
		{name: "default server", args: nil},
		{name: "check configuration", args: []string{"--check-config"}, check: true},
		{name: "unknown option", args: []string{"--unsafe-mode"}, bad: true},
		{name: "extra arguments", args: []string{"--check-config", "extra"}, bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			check, err := checkConfigArgument(tc.args)
			if check != tc.check || (err != nil) != tc.bad {
				t.Fatalf("check=%v err=%v, want check=%v error=%v", check, err, tc.check, tc.bad)
			}
		})
	}
}

func TestPreflightFrontendBundle(t *testing.T) {
	if err := preflightFrontendBundle(""); err != nil {
		t.Fatalf("API-only startup should remain allowed: %v", err)
	}
	if err := preflightFrontendBundle(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("accepted missing frontend bundle")
	}
	dir := t.TempDir()
	if err := preflightFrontendBundle(dir); err == nil {
		t.Fatal("accepted a frontend folder without index.html")
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>ok</html>"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := preflightFrontendBundle(dir); err != nil {
		t.Fatalf("rejected existing SPA: %v", err)
	}
}
