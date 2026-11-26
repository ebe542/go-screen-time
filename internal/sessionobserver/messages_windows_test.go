package sessionobserver

import (
	"encoding/binary"
	"testing"
	"time"
)

func TestApplyWindowsSessionInfo(t *testing.T) {
	tests := []struct {
		name    string
		level   uint32
		id      uint32
		flags   uint32
		size    int
		want    SessionState
		wantErr bool
	}{
		{"active and unlocked", 1, 12, 1, 20, SessionActive, false},
		{"active and locked", 1, 12, 0, 20, SessionInactive, false},
		{"unknown lock state", 1, 12, 0xffffffff, 20, SessionUnknown, false},
		{"short response", 1, 12, 1, 16, SessionUnknown, true},
		{"unsupported information level", 2, 12, 1, 20, SessionUnknown, true},
		{"response for another session", 1, 13, 1, 20, SessionUnknown, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The level-1 prefix includes padding before the data union.
			data := make([]byte, 20)
			binary.LittleEndian.PutUint32(data[0:4], tt.level)
			binary.LittleEndian.PutUint32(data[8:12], tt.id)
			binary.LittleEndian.PutUint32(data[12:16], 0) // WTSActive
			binary.LittleEndian.PutUint32(data[16:20], tt.flags)

			session := NewSessionTracker("12", "S-1-5-21-100-200-300-1001")
			err := applyWindowsSessionInfo(session, data[:tt.size])
			if (err != nil) != tt.wantErr {
				t.Errorf("applyWindowsSessionInfo error = %v, want error %t", err, tt.wantErr)
			}
			if got := session.Snapshot().State; got != tt.want {
				t.Errorf("session state = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMonitorEventFromWindowsMessage(t *testing.T) {
	at := time.Date(2026, time.November, 27, 15, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		msg    uint32
		wParam uintptr
		lParam uintptr
		want   MonitorEvent
		found  bool
	}{
		{
			name: "heartbeat timer", msg: 0x0113, wParam: 1,
			want: MonitorEvent{At: at, Kind: MonitorHeartbeat}, found: true,
		},
		{name: "other timer", msg: 0x0113, wParam: 2},
		{
			name: "foreground change", msg: 0x8001,
			want: MonitorEvent{At: at, Kind: MonitorForegroundChanged}, found: true,
		},
		{
			name: "session locks", msg: 0x02b1, wParam: 0x7, lParam: 12,
			want: MonitorEvent{
				At: at, Kind: MonitorSessionChanged,
				Session: SessionEvent{SessionID: "12", Kind: SessionLocked},
			},
			found: true,
		},
		{name: "unhandled session event", msg: 0x02b1, wParam: 0x9, lParam: 12},
		{name: "unhandled window message", msg: 0x000f},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := monitorEventFromWindowsMessage(tt.msg, tt.wParam, tt.lParam, at)
			if found != tt.found || got != tt.want {
				t.Errorf("event = %+v, found = %t, want %+v, %t",
					got, found, tt.want, tt.found)
			}
		})
	}
}
