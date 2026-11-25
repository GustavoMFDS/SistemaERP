//go:build windows

package main

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"
)

const esteriumServiceName = "EsteriumERP"

type esteriumWindowsService struct {
	ctx              context.Context
	run              serverRun
	startupTimeout   time.Duration
	shutdownTimeout  time.Duration
	progressInterval time.Duration
}

func (s *esteriumWindowsService) Execute(
	_ []string,
	requests <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (bool, uint32) {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	status := svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10000}
	changes <- status
	readySignal := make(chan struct{})
	var ready <-chan struct{} = readySignal
	done := make(chan uint32, 1)
	var readyOnce sync.Once
	go func() {
		done <- s.run(ctx, func() { readyOnce.Do(func() { close(readySignal) }) })
	}()

	ticker := time.NewTicker(s.progressInterval)
	defer ticker.Stop()
	start := time.NewTimer(s.startupTimeout)
	defer start.Stop()
	startDeadline := start.C
	var stopTimer *time.Timer
	var stopDeadline <-chan time.Time
	defer func() {
		if stopTimer != nil {
			stopTimer.Stop()
		}
	}()
	parentDone := s.ctx.Done()
	var forcedExit uint32
	beginStop := func(code uint32) {
		if status.State == svc.StopPending {
			return
		}
		forcedExit = code
		status = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 10000}
		changes <- status
		cancel()
		ready = nil
		startDeadline = nil
		parentDone = nil
		stopTimer = time.NewTimer(s.shutdownTimeout)
		stopDeadline = stopTimer.C
	}
	for {
		select {
		case code := <-done:
			if code == 0 && status.State != svc.StopPending && s.ctx.Err() == nil {
				code = exitUnexpectedStop
			}
			if status.State != svc.StopPending {
				status = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 10000}
				changes <- status
			}
			if forcedExit != 0 {
				code = forcedExit
			}
			return code != 0, code
		case <-ready:
			ready = nil
			startDeadline = nil
			status = svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
			changes <- status
		case <-startDeadline:
			beginStop(exitStartupTimeout)
		case <-stopDeadline:
			return true, exitShutdownTimeout
		case <-parentDone:
			beginStop(0)
		case <-ticker.C:
			if status.State == svc.StartPending || status.State == svc.StopPending {
				status.CheckPoint++
				changes <- status
			}
		case request, ok := <-requests:
			if !ok {
				requests = nil
				beginStop(0)
				continue
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- status
			case svc.Stop, svc.Shutdown:
				beginStop(0)
			}
		}
	}
}

func maybeRunWindowsService(ctx context.Context, run serverRun) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return true, err
	}
	if !isService {
		return false, nil
	}
	err = svc.Run(esteriumServiceName, &esteriumWindowsService{
		ctx: ctx, run: run, startupTimeout: 90 * time.Second,
		shutdownTimeout: 30 * time.Second, progressInterval: 2 * time.Second,
	})
	return true, err
}
