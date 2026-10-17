//go:build windows

package main

import (
	"context"

	"golang.org/x/sys/windows/svc"
)

const esteriumServiceName = "EsteriumERP"

type esteriumWindowsService struct {
	ctx context.Context
	stop context.CancelFunc
}

func (s *esteriumWindowsService) Execute(
	_ []string,
	requests <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (bool, uint32) {
	changes <- svc.Status{State: svc.StartPending}
	running := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	changes <- running
	for {
		select {
		case <-s.ctx.Done():
			changes <- svc.Status{State: svc.StopPending}
			return false, 0
		case request, ok := <-requests:
			if !ok {
				s.stop()
				return false, 0
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- running
			case svc.Stop, svc.Shutdown:
				changes <- svc.Status{State: svc.StopPending}
				s.stop()
				return false, 0
			}
		}
	}
}

func maybeRunWindowsService(ctx context.Context, stop context.CancelFunc) (bool, error) {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return true, err
	}
	if !isService {
		return false, nil
	}
	err = svc.Run(esteriumServiceName, &esteriumWindowsService{ctx: ctx, stop: stop})
	return true, err
}
