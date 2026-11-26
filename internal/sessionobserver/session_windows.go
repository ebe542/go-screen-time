package sessionobserver

import (
	"fmt"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wtsActive       int32 = 0
	wtsDisconnected int32 = 4

	wtsLocked   uint32 = 0
	wtsUnlocked uint32 = 1
)

func currentWindowsSessionIdentity() (id, sid string, err error) {
	token := windows.GetCurrentProcessToken()

	user, err := token.GetTokenUser()
	if err != nil {
		return "", "", fmt.Errorf("read current Windows user SID: %w", err)
	}

	sessionID, err := sessionIDFromToken(token)
	if err != nil {
		return "", "", fmt.Errorf("read current Windows session ID: %w", err)
	}

	return strconv.FormatUint(uint64(sessionID), 10), user.User.Sid.String(), nil
}

func sessionIDFromToken(token windows.Token) (uint32, error) {
	var sessionID, returnedSize uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenSessionId,
		(*byte)(unsafe.Pointer(&sessionID)),
		uint32(unsafe.Sizeof(sessionID)),
		&returnedSize,
	)
	return sessionID, err
}

func applyWindowsSessionState(session *SessionTracker, connection int32, flags uint32) {
	session.active = factUnknown
	switch connection {
	case wtsActive:
		session.active = factYes
	case wtsDisconnected:
		session.active = factNo
	}

	session.unlocked = factUnknown
	switch flags {
	case wtsLocked:
		session.unlocked = factNo
	case wtsUnlocked:
		session.unlocked = factYes
	}
}

func windowsSessionEvent(code, sessionID uint32) (SessionEvent, bool) {
	const (
		consoleConnect    = 0x1
		consoleDisconnect = 0x2
		remoteConnect     = 0x3
		remoteDisconnect  = 0x4
		sessionLogoff     = 0x6
		sessionLock       = 0x7
		sessionUnlock     = 0x8
	)

	var kind SessionEventKind
	switch code {
	case consoleConnect, remoteConnect:
		kind = SessionActivated
	case consoleDisconnect, remoteDisconnect, sessionLogoff:
		kind = SessionDeactivated
	case sessionLock:
		kind = SessionLocked
	case sessionUnlock:
		kind = SessionUnlocked
	default:
		return SessionEvent{}, false
	}

	return SessionEvent{
		SessionID: strconv.FormatUint(uint64(sessionID), 10),
		Kind:      kind,
	}, true
}
