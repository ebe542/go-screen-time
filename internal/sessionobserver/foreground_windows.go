package sessionobserver

import (
	"errors"
	"fmt"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	getForegroundWindow      = user32.NewProc("GetForegroundWindow")
	getWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
)

// WindowsForegroundSource reads the foreground process in the current desktop.
type WindowsForegroundSource struct{}

// ForegroundProcess returns the current foreground process.
// A zero result indicates that no stable foreground window was observed.
func (WindowsForegroundSource) ForegroundProcess() (ForegroundProcess, error) {
	window, _, _ := getForegroundWindow.Call()
	if window == 0 {
		return ForegroundProcess{}, nil
	}

	var pid uint32
	thread, _, callErr := getWindowThreadProcessID.Call(window, uintptr(unsafe.Pointer(&pid)))
	if thread == 0 || pid == 0 {
		if callErr == windows.ERROR_SUCCESS {
			return ForegroundProcess{}, errors.New("foreground window has no process ID")
		}
		return ForegroundProcess{}, fmt.Errorf("get foreground window process ID: %w", callErr)
	}

	process, err := foregroundProcessForPID(pid)
	if err != nil {
		return ForegroundProcess{}, err
	}

	currentWindow, _, _ := getForegroundWindow.Call()
	if currentWindow != window {
		return ForegroundProcess{}, nil
	}
	return process, nil
}

func foregroundProcessForPID(pid uint32) (ForegroundProcess, error) {
	if pid == 0 {
		return ForegroundProcess{}, errors.New("process ID must be nonzero")
	}

	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ForegroundProcess{}, fmt.Errorf("open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)

	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(process, 0, &buffer[0], &size); err != nil {
		return ForegroundProcess{}, fmt.Errorf("read image path for process %d: %w", pid, err)
	}

	var token windows.Token
	if err := windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token); err != nil {
		return ForegroundProcess{}, fmt.Errorf("open token for process %d: %w", pid, err)
	}
	defer token.Close()

	user, err := token.GetTokenUser()
	if err != nil {
		return ForegroundProcess{}, fmt.Errorf("read user SID for process %d: %w", pid, err)
	}

	sessionID, err := sessionIDFromToken(token)
	if err != nil {
		return ForegroundProcess{}, fmt.Errorf("read session ID for process %d: %w", pid, err)
	}

	return ForegroundProcess{
		SessionID:      strconv.FormatUint(uint64(sessionID), 10),
		SID:            user.User.Sid.String(),
		ExecutablePath: windows.UTF16ToString(buffer[:size]),
	}, nil
}
