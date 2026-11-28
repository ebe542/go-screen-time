package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"golang.org/x/sys/windows/svc"
)

func TestServiceHandlerWaitsForReadinessAndShutdown(t *testing.T) {
	for _, tt := range []struct {
		name    string
		command svc.Cmd
	}{
		{"stop", svc.Stop},
		{"shutdown", svc.Shutdown},
	} {
		t.Run(tt.name, func(t *testing.T) {
			requests := make(chan svc.ChangeRequest, 2)
			statuses := make(chan svc.Status, 8)
			allowReady := make(chan struct{}, 1)
			allowClose := make(chan struct{}, 1)
			closing := make(chan struct{})
			done := make(chan serviceTestResult, 1)

			handler := serviceHandler{
				run: func(ctx context.Context, ready func()) error {
					select {
					case <-allowReady:
						ready()
					case <-ctx.Done():
						return ctx.Err()
					}
					<-ctx.Done()
					close(closing)
					<-allowClose
					return ctx.Err()
				},
			}
			go func() {
				specific, code := handler.Execute(nil, requests, statuses)
				done <- serviceTestResult{specific: specific, code: code}
			}()
			t.Cleanup(func() {
				select {
				case requests <- svc.ChangeRequest{Cmd: svc.Stop}:
				default:
				}
				select {
				case allowClose <- struct{}{}:
				default:
				}
			})

			assertServiceState(t, statuses, svc.StartPending)
			requests <- svc.ChangeRequest{Cmd: svc.Interrogate}
			assertServiceState(t, statuses, svc.StartPending)

			allowReady <- struct{}{}
			running := assertServiceState(t, statuses, svc.Running)
			const wantAccepts = svc.AcceptStop | svc.AcceptShutdown
			if running.Accepts&wantAccepts != wantAccepts {
				t.Errorf("running service accepts %v, want stop and shutdown", running.Accepts)
			}

			requests <- svc.ChangeRequest{Cmd: tt.command}
			assertServiceState(t, statuses, svc.StopPending)
			select {
			case <-closing:
			case <-time.After(5 * time.Second):
				t.Fatal("service runner was not cancelled")
			}

			select {
			case result := <-done:
				t.Fatalf("handler returned before the runner finished: %+v", result)
			default:
			}

			allowClose <- struct{}{}
			select {
			case result := <-done:
				if result.specific || result.code != 0 {
					t.Errorf("service exit = %+v, want success", result)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("handler did not finish after the runner closed")
			}
		})
	}
}

func TestServiceHandlerReportsStartupFailure(t *testing.T) {
	startErr := errors.New("could not open child profiles")
	requests := make(chan svc.ChangeRequest)
	statuses := make(chan svc.Status, 8)
	done := make(chan serviceTestResult, 1)

	handler := serviceHandler{
		run: func(context.Context, func()) error {
			return startErr
		},
	}
	go func() {
		specific, code := handler.Execute(nil, requests, statuses)
		done <- serviceTestResult{specific: specific, code: code}
	}()

	assertServiceState(t, statuses, svc.StartPending)
	select {
	case result := <-done:
		if result.code == 0 {
			t.Errorf("startup failure returned success: %+v", result)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("handler did not return after startup failure")
	}

	for len(statuses) > 0 {
		if status := <-statuses; status.State == svc.Running {
			t.Error("service reported running after startup failure")
		}
	}
}

type serviceTestResult struct {
	specific bool
	code     uint32
}

func assertServiceState(t *testing.T, statuses <-chan svc.Status, want svc.State) svc.Status {
	t.Helper()

	select {
	case got := <-statuses:
		if got.State != want {
			t.Fatalf("service state = %v, want %v", got.State, want)
		}
		return got
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for service state %v", want)
		return svc.Status{}
	}
}
