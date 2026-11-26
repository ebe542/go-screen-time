package sessionobserver

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

var _ ForegroundSource = WindowsForegroundSource{}

func TestForegroundProcessForPIDReadsCurrentProcess(t *testing.T) {
	pid := uint32(os.Getpid())

	got, err := foregroundProcessForPID(pid)
	if err != nil {
		t.Fatalf("read test process %d: %v", pid, err)
	}

	wantPath, err := os.Executable()
	if err != nil {
		t.Fatalf("get test executable path: %v", err)
	}
	if !strings.EqualFold(filepath.Clean(got.ExecutablePath), filepath.Clean(wantPath)) {
		t.Errorf("executable path = %q, want %q", got.ExecutablePath, wantPath)
	}

	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatalf("get test process user: %v", err)
	}
	if wantSID := user.User.Sid.String(); got.SID != wantSID {
		t.Errorf("SID = %q, want %q", got.SID, wantSID)
	}

	var sessionID uint32
	if err := windows.ProcessIdToSessionId(pid, &sessionID); err != nil {
		t.Fatalf("get test process session ID: %v", err)
	}
	if wantSessionID := strconv.FormatUint(uint64(sessionID), 10); got.SessionID != wantSessionID {
		t.Errorf("session ID = %q, want %q", got.SessionID, wantSessionID)
	}
}

func TestForegroundProcessForPIDRejectsZero(t *testing.T) {
	got, err := foregroundProcessForPID(0)
	if err == nil {
		t.Fatal("reading process ID zero succeeded")
	}
	if got != (ForegroundProcess{}) {
		t.Errorf("failed process lookup returned %+v, want zero value", got)
	}
}
