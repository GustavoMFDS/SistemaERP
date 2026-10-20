package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStartupMode(t *testing.T) {
	cases := []struct {
		name string
		args []string
		mode startupMode
		bad  bool
	}{
		{name: "default server", args: nil, mode: startupServe},
		{name: "config only", args: []string{"--check-config"}, mode: startupConfigCheck},
		{name: "live dependencies", args: []string{"--check-dependencies"}, mode: startupDependencyCheck},
		{name: "unknown option", args: []string{"--unsafe-mode"}, bad: true},
		{name: "extra arguments", args: []string{"--check-config", "extra"}, bad: true},
		{name: "multiple checks", args: []string{"--check-config", "--check-dependencies"}, bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mode, err := parseStartupMode(tc.args)
			if mode != tc.mode || (err != nil) != tc.bad {
				t.Fatalf("mode=%v err=%v, want mode=%v error=%v", mode, err, tc.mode, tc.bad)
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
