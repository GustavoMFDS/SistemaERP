//go:build windows

package main

import (
	"context"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/example/sistemaemgo/internal/app"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// Only the disposable CI test binary can exercise this synthetic service.
// Normal unit-test execution never connects to SCM or writes an event.
func TestNativeStartupEventServiceFixture(t *testing.T) {
	name := os.Getenv("ESTERIUM_STARTUP_EVENT_CI_SERVICE")
	if name == "" {
		t.Skip("disposable native SCM fixture not requested")
	}
	if !regexp.MustCompile("^EstStartupEventCI[a-f0-9]{32}$").MatchString(name) {
		t.Fatal("invalid fixture identity")
	}
	isService, err := svc.IsWindowsService()
	if err != nil || !isService {
		t.Fatal("fixture must be launched by SCM")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil || user.User.Sid.String() != "S-1-5-19" {
		t.Fatal("fixture requires LocalService")
	}
	ctx := withStartupDiagnosticSink(context.Background(), writeWindowsStartupDiagnostic)
	run := func(ctx context.Context, _ func()) uint32 {
		message := redisStartupDiagnosticMessage(app.StartupRedisSecurity, "rate-limit-atomic", "deadline")
		if err := emitStartupDiagnostic(ctx, message); err != nil {
			return 99
		}
		return 31
	}
	if err := svc.Run(name, &esteriumWindowsService{
		ctx: ctx, run: run, startupTimeout: 30 * time.Second,
		shutdownTimeout: 10 * time.Second, progressInterval: time.Second,
	}); err != nil {
		t.Fatal("fixture service dispatcher failed")
	}
}
