package observationpipe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

func TestNamedPipeDeliversObservationWithWindowsIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sender, receiver := connectTestPipe(t, ctx)
	wantPeer := currentTestPeer(t)
	observation := testObservation(wantPeer)

	if err := sender.Send(ctx, observation); err != nil {
		t.Fatalf("send observation: %v", err)
	}
	gotPeer, gotObservation, err := receiver.Read(ctx)
	if err != nil {
		t.Fatalf("read observation: %v", err)
	}
	if gotPeer != wantPeer {
		t.Errorf("Windows peer = %+v, want %+v", gotPeer, wantPeer)
	}
	if !gotObservation.At.Equal(observation.At) ||
		gotObservation.SessionID != observation.SessionID ||
		gotObservation.SID != observation.SID ||
		gotObservation.ExecutablePath != observation.ExecutablePath ||
		gotObservation.State != observation.State {
		t.Errorf("observation = %+v, want %+v", gotObservation, observation)
	}
}

func TestNamedPipeRejectsClaimedIdentityThatDiffersFromWindows(t *testing.T) {
	tests := []struct {
		name   string
		change func(*tracking.Observation)
	}{
		{
			name: "different SID",
			change: func(o *tracking.Observation) {
				o.SID = "S-1-5-18"
				if o.SID == currentTestPeer(t).SID {
					o.SID = "S-1-5-19"
				}
			},
		},
		{
			name: "different session",
			change: func(o *tracking.Observation) {
				o.SessionID += "0"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			sender, receiver := connectTestPipe(t, ctx)
			observation := testObservation(currentTestPeer(t))
			tt.change(&observation)

			if err := sender.Send(ctx, observation); err != nil {
				t.Fatalf("send observation: %v", err)
			}
			peer, got, err := receiver.Read(ctx)
			if err == nil ||
				errors.Is(err, io.EOF) ||
				errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("read mismatched observation returned %v, want an identity error", err)
			}
			if peer != (PeerIdentity{}) || got != (tracking.Observation{}) {
				t.Errorf("rejected frame returned peer %+v and observation %+v", peer, got)
			}
		})
	}
}

func TestNamedPipeReportsPeerDisconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	sender, receiver := connectTestPipe(t, ctx)
	if err := sender.Close(); err != nil {
		t.Fatalf("close sender: %v", err)
	}

	peer, observation, err := receiver.Read(ctx)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("read after disconnect returned %v, want EOF", err)
	}
	if peer != (PeerIdentity{}) || observation != (tracking.Observation{}) {
		t.Errorf("disconnect returned peer %+v and observation %+v", peer, observation)
	}
}

func connectTestPipe(t *testing.T, ctx context.Context) (*Sender, *Receiver) {
	t.Helper()

	name := fmt.Sprintf(`\\.\pipe\go-screen-time-test-%d-%d`, os.Getpid(), time.Now().UnixNano())
	listener, err := Listen(name)
	if err != nil {
		t.Fatalf("listen on test pipe: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	type acceptResult struct {
		receiver *Receiver
		err      error
	}
	accepted := make(chan acceptResult, 1)
	go func() {
		receiver, err := listener.Accept(ctx)
		accepted <- acceptResult{receiver: receiver, err: err}
	}()

	sender, err := Dial(ctx, name)
	if err != nil {
		t.Fatalf("dial test pipe: %v", err)
	}
	t.Cleanup(func() { _ = sender.Close() })

	select {
	case result := <-accepted:
		if result.err != nil {
			t.Fatalf("accept test pipe: %v", result.err)
		}
		t.Cleanup(func() { _ = result.receiver.Close() })
		return sender, result.receiver
	case <-ctx.Done():
		t.Fatalf("accept test pipe: %v", ctx.Err())
		return nil, nil
	}
}

func currentTestPeer(t *testing.T) PeerIdentity {
	t.Helper()

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("read test process SID: %v", err)
	}
	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		t.Fatalf("read test process session ID: %v", err)
	}
	return PeerIdentity{
		SID:       user.User.Sid.String(),
		SessionID: strconv.FormatUint(uint64(sessionID), 10),
	}
}

func testObservation(peer PeerIdentity) tracking.Observation {
	return tracking.Observation{
		At:             time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC),
		SessionID:      peer.SessionID,
		SID:            peer.SID,
		ExecutablePath: `C:\Games\Example\game.exe`,
		State:          tracking.ObservationForeground,
	}
}
