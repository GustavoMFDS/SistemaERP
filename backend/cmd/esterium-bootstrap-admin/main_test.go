package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/example/sistemaemgo/internal/bootstrapadmin"
	"strings"
	"testing"
	"time"
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

func TestBootstrapOutcomeDiagnostics(t *testing.T) {
	expired, stop := context.WithDeadline(context.Background(), time.Unix(1, 0))
	defer stop()
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		err  error
		want string
	}{
		{"commit", context.Background(), bootstrapadmin.ErrCommitUnconfirmed, "FAIL: first administrator outcome not confirmed: commit"},
		{"commit_precedes_deadline", expired, bootstrapadmin.ErrCommitUnconfirmed, "FAIL: first administrator outcome not confirmed: commit"},
		{"deadline", expired, errors.New("synthetic private detail"), "FAIL: first administrator outcome not confirmed: deadline"},
		{"cancel", canceled, errors.New("synthetic private detail"), "FAIL: first administrator outcome not confirmed: canceled"},
		{"wrapped_deadline", context.Background(), fmt.Errorf("synthetic private detail: %w", context.DeadlineExceeded), "FAIL: first administrator outcome not confirmed: deadline"},
		{"wrapped_cancel", context.Background(), fmt.Errorf("synthetic private detail: %w", context.Canceled), "FAIL: first administrator outcome not confirmed: canceled"},
		{"legacy", context.Background(), errors.New("invalid CNPJ"), "FAIL: first administrator was not created: invalid CNPJ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := bootstrapFailure(classifyOperationError(tc.ctx, tc.err))
			if got != tc.want || strings.Contains(got, "synthetic private detail") {
				t.Fatal("unexpected or unsafe bootstrap diagnostic")
			}
		})
	}
}
