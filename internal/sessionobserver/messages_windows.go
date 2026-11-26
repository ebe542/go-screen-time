package sessionobserver

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"time"
)

const (
	windowsMessageTimer          = 0x0113
	windowsMessageSessionChange  = 0x02b1
	windowsMessageForeground     = 0x8001
	windowsHeartbeatTimer        = 1
	windowsSessionInfoLevel      = 1
	windowsSessionInfoPrefixSize = 20
)

func applyWindowsSessionInfo(session *SessionTracker, data []byte) error {
	if len(data) < windowsSessionInfoPrefixSize {
		return fmt.Errorf("short Windows session info response: %d bytes", len(data))
	}

	level := binary.LittleEndian.Uint32(data[0:4])
	if level != windowsSessionInfoLevel {
		return fmt.Errorf("unsupported Windows session info level %d", level)
	}

	// The data union starts at offset 8 after the level and alignment padding.
	sessionID := binary.LittleEndian.Uint32(data[8:12])
	id := strconv.FormatUint(uint64(sessionID), 10)
	if id != session.id {
		return fmt.Errorf("windows session info is for session %s, want %s", id, session.id)
	}

	connection := int32(binary.LittleEndian.Uint32(data[12:16]))
	flags := binary.LittleEndian.Uint32(data[16:20])
	applyWindowsSessionState(session, connection, flags)
	return nil
}

func monitorEventFromWindowsMessage(
	message uint32,
	wParam, lParam uintptr,
	at time.Time,
) (MonitorEvent, bool) {
	switch message {
	case windowsMessageTimer:
		if wParam == windowsHeartbeatTimer {
			return MonitorEvent{At: at, Kind: MonitorHeartbeat}, true
		}
	case windowsMessageForeground:
		return MonitorEvent{At: at, Kind: MonitorForegroundChanged}, true
	case windowsMessageSessionChange:
		event, found := windowsSessionEvent(uint32(wParam), uint32(lParam))
		if found {
			return MonitorEvent{At: at, Kind: MonitorSessionChanged, Session: event}, true
		}
	}

	return MonitorEvent{}, false
}
