//go:build windows

package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

type serviceResult struct {
	specific bool
	code     uint32
}

func startFixture(t *testing.T, run serverRun, startup, shutdown time.Duration) (chan svc.ChangeRequest, chan svc.Status, chan serviceResult) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	service := &esteriumWindowsService{ctx: ctx, run: run, startupTimeout: startup, shutdownTimeout: shutdown, progressInterval: 20 * time.Millisecond}
	requests := make(chan svc.ChangeRequest, 4)
	statuses := make(chan svc.Status, 128)
	done := make(chan serviceResult, 1)
	go func() {
		specific, code := service.Execute(nil, requests, statuses)
		done <- serviceResult{specific, code}
	}()
	return requests, statuses, done
}

func receiveState(t *testing.T, statuses <-chan svc.Status, expected svc.State) svc.Status {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		select {
		case status := <-statuses:
			if status.State == expected {
				return status
			}
			if status.State != svc.StartPending && status.State != svc.StopPending {
				t.Fatalf("unexpected status: got %v want %v", status.State, expected)
			}
		case <-timeout:
			t.Fatalf("timeout waiting for state %v", expected)
		}
	}
}

func receiveResult(t *testing.T, done <-chan serviceResult, expected uint32) {
	t.Helper()
	select {
	case result := <-done:
		if result.code != expected || result.specific != (expected != 0) {
			t.Fatalf("unexpected exit: %+v, expected %d", result, expected)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("service did not exit")
	}
}

func TestWindowsServiceReportsPendingUntilReadyAndWaitsForStop(t *testing.T) {
	release := make(chan struct{})
	stopped := make(chan struct{})
	allowExit := make(chan struct{})
	var releaseOnce sync.Once
	releaseWorker := func() { releaseOnce.Do(func() { close(allowExit) }) }
	t.Cleanup(releaseWorker)
	requests, statuses, done := startFixture(t, func(ctx context.Context, ready func()) uint32 {
		select {
		case <-release:
		case <-ctx.Done():
			return 0
		}
		ready()
		<-ctx.Done()
		close(stopped)
		<-allowExit
		return 0
	}, time.Second, time.Second)
	first := receiveState(t, statuses, svc.StartPending)
	progress := receiveState(t, statuses, svc.StartPending)
	if first.CheckPoint == 0 || first.WaitHint == 0 || progress.CheckPoint <= first.CheckPoint {
		t.Fatal("start pending did not report progress")
	}
	close(release)
	running := receiveState(t, statuses, svc.Running)
	if running.Accepts&svc.AcceptStop == 0 {
		t.Fatal("running state did not accept Stop")
	}
	requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
	receiveState(t, statuses, svc.Running)
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	receiveState(t, statuses, svc.StopPending)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("worker not cancelled")
	}
	select {
	case <-done:
		t.Fatal("SCM returned before worker completed")
	default:
	}
	releaseWorker()
	receiveResult(t, done, 0)
}

func TestWindowsServiceStartupFailureNeverReportsRunning(t *testing.T) {
	_, statuses, done := startFixture(t, func(context.Context, func()) uint32 { return exitApp }, time.Second, time.Second)
	receiveState(t, statuses, svc.StartPending)
	receiveState(t, statuses, svc.StopPending)
	receiveResult(t, done, exitApp)
}

func TestWindowsServiceStartupTimeoutCancelsWorker(t *testing.T) {
	_, statuses, done := startFixture(t, func(ctx context.Context, ready func()) uint32 {
		<-ctx.Done()
		// A delayed ready notification after cancellation must not panic.
		ready()
		return 0
	}, 60*time.Millisecond, time.Second)
	receiveState(t, statuses, svc.StartPending)
	receiveState(t, statuses, svc.StopPending)
	receiveResult(t, done, exitStartupTimeout)
}

func TestWindowsServiceShutdownTimeout(t *testing.T) {
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	requests, statuses, done := startFixture(t, func(ctx context.Context, ready func()) uint32 {
		ready()
		<-ctx.Done()
		<-release
		return 0
	}, time.Second, 60*time.Millisecond)
	receiveState(t, statuses, svc.StartPending)
	receiveState(t, statuses, svc.Running)
	requests <- svc.ChangeRequest{Cmd: svc.Shutdown}
	receiveState(t, statuses, svc.StopPending)
	receiveResult(t, done, exitShutdownTimeout)
}

func TestWindowsServiceUnexpectedServerFailure(t *testing.T) {
	fail := make(chan struct{})
	_, statuses, done := startFixture(t, func(ctx context.Context, ready func()) uint32 {
		ready()
		select {
		case <-fail:
			return exitServe
		case <-ctx.Done():
			return 0
		}
	}, time.Second, time.Second)
	receiveState(t, statuses, svc.StartPending)
	receiveState(t, statuses, svc.Running)
	close(fail)
	receiveState(t, statuses, svc.StopPending)
	receiveResult(t, done, exitServe)
}

func TestWindowsServiceUnexpectedCleanExitBeforeReady(t *testing.T) {
	_, statuses, done := startFixture(t, func(context.Context, func()) uint32 { return 0 }, time.Second, time.Second)
	receiveState(t, statuses, svc.StartPending)
	receiveState(t, statuses, svc.StopPending)
	receiveResult(t, done, exitUnexpectedStop)
}
