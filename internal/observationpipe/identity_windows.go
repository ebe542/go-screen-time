package observationpipe

import (
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var impersonatePipeClient = windows.NewLazySystemDLL("advapi32.dll").
	NewProc("ImpersonateNamedPipeClient")

func readPipePeer(handle windows.Handle) (PeerIdentity, error) {
	type result struct {
		peer PeerIdentity
		err  error
	}

	done := make(chan result, 1)
	go func() {
		peer, err := readPipePeerOnThread(handle)
		done <- result{peer: peer, err: err}
	}()

	got := <-done
	return got.peer, got.err
}

func readPipePeerOnThread(handle windows.Handle) (peer PeerIdentity, err error) {
	runtime.LockOSThread()
	canUnlock := true
	defer func() {
		if canUnlock {
			runtime.UnlockOSThread()
		}
	}()

	ok, _, callErr := impersonatePipeClient.Call(uintptr(handle))
	if ok == 0 {
		return PeerIdentity{}, fmt.Errorf("identify named-pipe client: %w", callErr)
	}

	canUnlock = false
	defer func() {
		if revertErr := windows.RevertToSelf(); revertErr != nil {
			peer = PeerIdentity{}
			err = errors.Join(err, fmt.Errorf("revert pipe client identity: %w", revertErr))
			return
		}
		canUnlock = true
	}()

	var token windows.Token
	if err := windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token); err != nil {
		return PeerIdentity{}, fmt.Errorf("open pipe client token: %w", err)
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		return PeerIdentity{}, fmt.Errorf("read pipe client SID: %w", err)
	}

	var sessionID, returned uint32
	size := uint32(unsafe.Sizeof(sessionID))
	if err := windows.GetTokenInformation(
		token,
		windows.TokenSessionId,
		(*byte)(unsafe.Pointer(&sessionID)),
		size,
		&returned,
	); err != nil {
		return PeerIdentity{}, fmt.Errorf("read pipe client session ID: %w", err)
	}
	if returned != size {
		return PeerIdentity{}, errors.New("unexpected pipe client session ID size")
	}

	return PeerIdentity{
		SID:       user.User.Sid.String(),
		SessionID: strconv.FormatUint(uint64(sessionID), 10),
	}, nil
}
