// Package observationservice routes Windows helper observations to the controller.
package observationservice

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebe542/go-screen-time/internal/control"
	"github.com/ebe542/go-screen-time/internal/managedapps"
	"github.com/ebe542/go-screen-time/internal/observationpipe"
	"github.com/ebe542/go-screen-time/internal/tracking"
)

// Report describes the result of processing one pipe event.
type Report struct {
	ConnectionID uint64
	Observation  tracking.Observation
	Decision     managedapps.AccessDecision
	Disconnected bool
	Err          error
}

type pipeEvent struct {
	connectionID uint64
	peer         observationpipe.PeerIdentity
	observation  tracking.Observation
	err          error
}

// Serve processes observations until the context is cancelled or a
// listener, session selection, or controller operation fails.
func Serve(
	ctx context.Context,
	listener *observationpipe.Listener,
	intake *control.ObservationIntake,
	activeSession func() (string, error),
	now func() time.Time,
	report func(Report),
) error {
	runCtx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() {
		cancel()
		workers.Wait()
	}()

	events := make(chan pipeEvent)
	listenerErrors := make(chan error, 1)

	workers.Go(func() {
		var nextID uint64
		for {
			receiver, err := listener.Accept(runCtx)
			if err != nil {
				listenerErrors <- err
				return
			}

			nextID++
			id := nextID
			workers.Go(func() {
				readHelper(runCtx, receiver, id, events)
			})
		}
	})

	registered := make(map[uint64]bool)
	for {
		select {
		case <-runCtx.Done():
			return runCtx.Err()
		case err := <-listenerErrors:
			if runCtx.Err() != nil {
				return runCtx.Err()
			}
			return fmt.Errorf("accept observation connection: %w", err)
		case event := <-events:
			if err := runCtx.Err(); err != nil {
				return err
			}
			at := now()
			sessionID, err := activeSession()
			if err != nil {
				return fmt.Errorf("read active console session: %w", err)
			}
			if err := intake.SetActiveSession(sessionID, at); err != nil {
				return fmt.Errorf("select active console session: %w", err)
			}

			if event.err != nil {
				if err := intake.Disconnect(event.connectionID, at); err != nil {
					return fmt.Errorf("disconnect observation helper: %w", err)
				}
				delete(registered, event.connectionID)

				result := Report{ConnectionID: event.connectionID, Disconnected: true}
				if !errors.Is(event.err, io.EOF) {
					result.Err = event.err
				}
				if report != nil {
					report(result)
				}
				continue
			}

			if !registered[event.connectionID] {
				connection := control.ObservationConnection{
					ID:        event.connectionID,
					SessionID: event.peer.SessionID,
					SID:       event.peer.SID,
				}
				if err := intake.Connect(connection, at); err != nil {
					return fmt.Errorf("register observation helper: %w", err)
				}
				registered[event.connectionID] = true
			}

			observation := event.observation
			observation.At = at
			decision, err := intake.Accept(event.connectionID, observation)
			if err != nil {
				return fmt.Errorf("process foreground observation: %w", err)
			}
			if report != nil {
				report(Report{
					ConnectionID: event.connectionID,
					Observation:  observation,
					Decision:     decision,
				})
			}
		}
	}
}

func readHelper(
	ctx context.Context,
	receiver *observationpipe.Receiver,
	id uint64,
	events chan<- pipeEvent,
) {
	defer receiver.Close()

	var firstPeer observationpipe.PeerIdentity
	for {
		peer, observation, err := receiver.Read(ctx)
		if err == nil {
			if firstPeer == (observationpipe.PeerIdentity{}) {
				firstPeer = peer
			} else if peer != firstPeer {
				err = errors.New("windows identity changed on an existing observation connection")
			}
		}

		select {
		case <-ctx.Done():
			return
		case events <- pipeEvent{
			connectionID: id,
			peer:         peer,
			observation:  observation,
			err:          err,
		}:
		}
		if err != nil {
			return
		}
	}
}
