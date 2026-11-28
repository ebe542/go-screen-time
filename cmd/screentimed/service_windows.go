package main

import (
	"context"
	"errors"
	"log"
	"sync"

	"golang.org/x/sys/windows/svc"
)

const (
	serviceWaitHintMillis  = 10_000
	serviceRuntimeExitCode = 1
)

type serviceHandler struct {
	run func(context.Context, func()) error
}

var _ svc.Handler = serviceHandler{}

func (h serviceHandler) Execute(
	_ []string,
	requests <-chan svc.ChangeRequest,
	changes chan<- svc.Status,
) (bool, uint32) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status := svc.Status{
		State:      svc.StartPending,
		CheckPoint: 1,
		WaitHint:   serviceWaitHintMillis,
	}
	changes <- status

	ready := make(chan struct{})
	var readyOnce sync.Once
	var readiness <-chan struct{} = ready

	done := make(chan error, 1)
	go func() {
		done <- h.run(ctx, func() {
			readyOnce.Do(func() { close(ready) })
		})
	}()

	stop := func() {
		if status.State == svc.StopPending {
			return
		}
		readiness = nil
		status = svc.Status{
			State:      svc.StopPending,
			CheckPoint: 1,
			WaitHint:   serviceWaitHintMillis,
		}
		changes <- status
		cancel()
	}

	for {
		select {
		case <-readiness:
			readiness = nil
			status = svc.Status{
				State:   svc.Running,
				Accepts: svc.AcceptStop | svc.AcceptShutdown,
			}
			changes <- status

		case request, ok := <-requests:
			if !ok {
				requests = nil
				stop()
				continue
			}
			switch request.Cmd {
			case svc.Interrogate:
				changes <- status
			case svc.Stop, svc.Shutdown:
				stop()
			}

		case err := <-done:
			if err == nil || (ctx.Err() != nil && errors.Is(err, context.Canceled)) {
				return false, 0
			}
			log.Printf("observation service: %v", err)
			return true, serviceRuntimeExitCode
		}
	}
}
