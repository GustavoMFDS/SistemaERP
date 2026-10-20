//go:build windows

package main

import (
	"context"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestWindowsServiceHandlesStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := &esteriumWindowsService{ctx: ctx, stop: cancel}
	requests := make(chan svc.ChangeRequest, 1)
	statuses := make(chan svc.Status, 8)
	done := make(chan struct{})
	go func() {
		defer close(done)
		service.Execute(nil, requests, statuses)
	}()

	receive := func(expected svc.State) {
		t.Helper()
		select {
		case status := <-statuses:
			if status.State != expected {
				t.Fatalf("unexpected Windows service state: got=%v want=%v", status.State, expected)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("timeout waiting for Windows service state %v", expected)
		}
	}
	receive(svc.StartPending)
	receive(svc.Running)
	requests <- svc.ChangeRequest{Cmd: svc.Stop}
	receive(svc.StopPending)
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("Windows service did not cancel server context")
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Windows service dispatcher did not finish")
	}
}

func TestWindowsServiceCancelsWhenServerExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	statuses := make(chan svc.Status, 8)
	done := make(chan struct{})
	service := &esteriumWindowsService{ctx: ctx, stop: cancel}
	go func() {
		defer close(done)
		service.Execute(nil, make(chan svc.ChangeRequest), statuses)
	}()
	select {
	case <-statuses:
	case <-time.After(5 * time.Second):
		t.Fatal("Windows service did not start")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("service did not stop after server exit")
	}
}
