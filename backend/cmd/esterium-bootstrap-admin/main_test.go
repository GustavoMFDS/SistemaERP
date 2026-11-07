package main

import (
	"strings"
	"testing"
)

func TestReadPasswordAcceptsOneLineWithoutEchoProtocol(t *testing.T) {
	got, err := readPassword(strings.NewReader("Synthetic-Strong-Passphrase\r\n"))
	if err != nil {
		t.Fatalf("readPassword rejected a single line: %v", err)
	}
	if string(got) != "Synthetic-Strong-Passphrase" {
		t.Fatal("readPassword returned unexpected input")
	}
	clear(got)
}

func TestReadPasswordRejectsExtraLinesAndEmptyInput(t *testing.T) {
	for _, value := range []string{"", "one\ntwo\n", "one\x00two"} {
		if got, err := readPassword(strings.NewReader(value)); err == nil {
			clear(got)
			t.Fatalf("readPassword accepted invalid input %q", value)
		}
	}
}
