// Package observationpipe transports foreground observations over a local Windows pipe.
package observationpipe

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

const (
	localPipePrefix = `\\.\pipe\`
	maxFrameSize    = 64 * 1024

	// Interactive users can read and write data without creating pipe instances.
	pipeSecurity = "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;OW)(A;;GR;;;IU)(A;;0x2;;;IU)"
)

// PeerIdentity contains the account and session reported by Windows.
type PeerIdentity struct {
	SID       string
	SessionID string
}

// Listener accepts local helper connections.
type Listener struct {
	listener net.Listener
}

// Sender writes observations to the service.
type Sender struct {
	conn net.Conn
}

// Receiver reads and authenticates observations from one helper.
type Receiver struct {
	conn net.Conn
}

// Listen creates a local named pipe.
func Listen(name string) (*Listener, error) {
	if err := validatePipeName(name); err != nil {
		return nil, err
	}
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: pipeSecurity,
		InputBufferSize:    maxFrameSize + 4,
		OutputBufferSize:   maxFrameSize + 4,
	})
	if err != nil {
		return nil, err
	}
	return &Listener{listener: listener}, nil
}

// Accept waits for one connection. Cancelling ctx closes the listener.
func (l *Listener) Accept(ctx context.Context) (*Receiver, error) {
	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(finished)
		_ = l.Close()
	})

	conn, err := l.listener.Accept()
	if !stop() {
		<-finished
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		if conn != nil {
			_ = conn.Close()
		}
		return nil, ctxErr
	}
	if err != nil {
		return nil, err
	}
	return &Receiver{conn: conn}, nil
}

func (l *Listener) Close() error { return l.listener.Close() }

// Dial connects to a local pipe and allows the peer to identify the caller.
func Dial(ctx context.Context, name string) (*Sender, error) {
	if err := validatePipeName(name); err != nil {
		return nil, err
	}
	conn, err := winio.DialPipeAccessImpLevel(
		ctx,
		name,
		windows.GENERIC_READ|windows.FILE_WRITE_DATA,
		winio.PipeImpLevelIdentification,
	)
	if err != nil {
		return nil, err
	}
	return &Sender{conn: conn}, nil
}

func (s *Sender) Close() error { return s.conn.Close() }

func (r *Receiver) Close() error { return r.conn.Close() }

func validatePipeName(name string) error {
	if len(name) <= len(localPipePrefix) ||
		!strings.HasPrefix(strings.ToLower(name), localPipePrefix) {
		return errors.New("observation pipe requires a local Windows pipe name")
	}
	return nil
}

// Send writes one complete observation frame.
func (s *Sender) Send(ctx context.Context, observation tracking.Observation) error {
	payload, err := json.Marshal(observation)
	if err != nil {
		return err
	}
	if len(payload) > maxFrameSize {
		return errors.New("observation frame is too large")
	}

	frame := make([]byte, 4+len(payload))
	binary.LittleEndian.PutUint32(frame[:4], uint32(len(payload)))
	copy(frame[4:], payload)

	return withPipeDeadline(ctx, s.conn.SetWriteDeadline, func() error {
		n, err := s.conn.Write(frame)
		if err == nil && n != len(frame) {
			return io.ErrShortWrite
		}
		return err
	})
}

// Read returns a complete observation and its Windows-authenticated identity.
// A read error ends the stream and the caller should close the receiver.
func (r *Receiver) Read(ctx context.Context) (PeerIdentity, tracking.Observation, error) {
	var payload []byte
	err := withPipeDeadline(ctx, r.conn.SetReadDeadline, func() error {
		var header [4]byte
		if _, err := io.ReadFull(r.conn, header[:]); err != nil {
			return err
		}
		size := binary.LittleEndian.Uint32(header[:])
		if size == 0 || size > maxFrameSize {
			return errors.New("invalid observation frame size")
		}

		payload = make([]byte, size)
		_, err := io.ReadFull(r.conn, payload)
		if errors.Is(err, io.EOF) {
			return io.ErrUnexpectedEOF
		}
		return err
	})
	if err != nil {
		return PeerIdentity{}, tracking.Observation{}, err
	}

	handle, ok := r.conn.(interface{ Fd() uintptr })
	if !ok {
		return PeerIdentity{}, tracking.Observation{}, errors.New("pipe handle unavailable")
	}
	peer, err := readPipePeer(windows.Handle(handle.Fd()))
	if err != nil {
		return PeerIdentity{}, tracking.Observation{}, err
	}

	var observation tracking.Observation
	if err := json.Unmarshal(payload, &observation); err != nil {
		return PeerIdentity{}, tracking.Observation{}, fmt.Errorf("decode observation: %w", err)
	}
	if observation.SID != peer.SID || observation.SessionID != peer.SessionID {
		return PeerIdentity{}, tracking.Observation{}, errors.New("observation identity does not match Windows peer")
	}

	return peer, observation, nil
}

func withPipeDeadline(
	ctx context.Context,
	setDeadline func(time.Time) error,
	run func() error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	deadline, _ := ctx.Deadline()
	if err := setDeadline(deadline); err != nil {
		return err
	}

	finished := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		defer close(finished)
		_ = setDeadline(time.Now())
	})
	defer func() {
		if !stop() {
			<-finished
		}
		_ = setDeadline(time.Time{})
	}()

	err := run()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
