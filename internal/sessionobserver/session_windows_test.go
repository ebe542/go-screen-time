package sessionobserver

import (
	"os"
	"strconv"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCurrentWindowsSessionIdentity(t *testing.T) {
	gotID, gotSID, err := currentWindowsSessionIdentity()
	if err != nil {
		t.Fatalf("read current session identity: %v", err)
	}

	var sessionID uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &sessionID); err != nil {
		t.Fatalf("read test process session ID: %v", err)
	}
	if want := strconv.FormatUint(uint64(sessionID), 10); gotID != want {
		t.Errorf("session ID = %q, want %q", gotID, want)
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("read test process SID: %v", err)
	}
	if want := user.User.Sid.String(); gotSID != want {
		t.Errorf("SID = %q, want %q", gotSID, want)
	}
}

func TestApplyWindowsSessionState(t *testing.T) {
	const (
		wtsActive       = int32(0)
		wtsDisconnected = int32(4)
		wtsLocked       = uint32(0)
		wtsUnlocked     = uint32(1)
		wtsUnknownLock  = uint32(0xffffffff)
	)

	tests := []struct {
		name            string
		connection      int32
		flags           uint32
		want            SessionState
		wantAfterUnlock SessionState
	}{
		{"active and unlocked", wtsActive, wtsUnlocked, SessionActive, SessionActive},
		{"active and locked", wtsActive, wtsLocked, SessionInactive, SessionActive},
		{"disconnected and unlocked", wtsDisconnected, wtsUnlocked, SessionInactive, SessionInactive},
		{"unknown lock state", wtsActive, wtsUnknownLock, SessionUnknown, SessionActive},
		{"unknown connection state", -1, wtsUnlocked, SessionUnknown, SessionUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := NewSessionTracker("12", "S-1-5-21-100-200-300-1001")
			applyWindowsSessionState(session, tt.connection, tt.flags)

			if got := session.Snapshot().State; got != tt.want {
				t.Errorf("initial state = %v, want %v", got, tt.want)
			}

			session.Apply(SessionEvent{SessionID: "12", Kind: SessionUnlocked})
			if got := session.Snapshot().State; got != tt.wantAfterUnlock {
				t.Errorf("state after unlock = %v, want %v", got, tt.wantAfterUnlock)
			}
		})
	}
}

func TestWindowsSessionEvent(t *testing.T) {
	tests := []struct {
		name string
		code uint32
		want SessionEventKind
	}{
		{"console connect", 0x1, SessionActivated},
		{"console disconnect", 0x2, SessionDeactivated},
		{"remote connect", 0x3, SessionActivated},
		{"remote disconnect", 0x4, SessionDeactivated},
		{"logoff", 0x6, SessionDeactivated},
		{"lock", 0x7, SessionLocked},
		{"unlock", 0x8, SessionUnlocked},
		{"unhandled event", 0x9, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := windowsSessionEvent(tt.code, 12)
			if tt.want == 0 {
				if found {
					t.Errorf("unhandled event produced %+v", got)
				}
				return
			}

			want := SessionEvent{SessionID: "12", Kind: tt.want}
			if !found || got != want {
				t.Errorf("event = %+v, found = %t, want %+v", got, found, want)
			}
		})
	}
}
