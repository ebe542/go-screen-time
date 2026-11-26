package sessionobserver

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ebe542/go-screen-time/internal/tracking"
)

const (
	wtsSessionInfoEx        = 25
	wtsNotifyForThisSession = 0
	windowsEventForeground  = 0x0003
	windowsMessageClose     = 0x0010
	windowsMessageStop      = 0x8002
	windowsHeartbeatMillis  = 5000
)

var (
	wtsAPI = windows.NewLazySystemDLL("wtsapi32.dll")

	querySessionInformation       = wtsAPI.NewProc("WTSQuerySessionInformationW")
	registerSessionNotification   = wtsAPI.NewProc("WTSRegisterSessionNotification")
	unregisterSessionNotification = wtsAPI.NewProc("WTSUnRegisterSessionNotification")

	monitorKernel32       = windows.NewLazySystemDLL("kernel32.dll")
	getModuleHandle       = monitorKernel32.NewProc("GetModuleHandleW")
	registerWindow        = user32.NewProc("RegisterClassExW")
	createWindow          = user32.NewProc("CreateWindowExW")
	destroyWindow         = user32.NewProc("DestroyWindow")
	defaultWindowProc     = user32.NewProc("DefWindowProcW")
	getWindowMessage      = user32.NewProc("GetMessageW")
	dispatchMessage       = user32.NewProc("DispatchMessageW")
	postWindowMessage     = user32.NewProc("PostMessageW")
	setWindowTimer        = user32.NewProc("SetTimer")
	killWindowTimer       = user32.NewProc("KillTimer")
	setWindowEventHook    = user32.NewProc("SetWinEventHook")
	removeWindowEventHook = user32.NewProc("UnhookWinEvent")
)

type monitorWindowClass struct {
	size, style                       uint32
	procedure                         uintptr
	classExtra, windowExtra           int32
	instance, icon, cursor            uintptr
	background, menu, name, smallIcon uintptr
}

type monitorWindowMessage struct {
	window         uintptr
	message        uint32
	wParam, lParam uintptr
	timestamp      uint32
	x, y           int32
	private        uint32
}

type registeredWindowClass struct {
	name     *uint16
	instance uintptr
}

type windowsMonitorRoute struct {
	ctx    context.Context
	cancel context.CancelFunc
	window uintptr
	events chan MonitorEvent
	ready  bool
}

var (
	monitorRoutes sync.Map // OS thread ID to *windowsMonitorRoute.

	monitorWindowCallback = windows.NewCallback(windowsMonitorWindowProc)
	foregroundCallback    = windows.NewCallback(windowsForegroundChanged)

	getMonitorWindowClass = sync.OnceValues(registerMonitorWindowClass)
)

func readWindowsSessionInfo(sessionID uint32) ([]byte, error) {
	var buffer *byte
	var size uint32
	ok, _, callErr := querySessionInformation.Call(
		0, uintptr(sessionID), wtsSessionInfoEx,
		uintptr(unsafe.Pointer(&buffer)),
		uintptr(unsafe.Pointer(&size)),
	)
	if ok == 0 {
		return nil, fmt.Errorf("query Windows session %d: %w", sessionID, callErr)
	}
	if buffer == nil {
		return nil, errors.New("Windows returned no session information")
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))

	return append([]byte(nil), unsafe.Slice(buffer, size)...), nil
}

func registerMonitorWindowClass() (registeredWindowClass, error) {
	name, err := windows.UTF16PtrFromString("GoScreenTimeObservationWindow")
	if err != nil {
		return registeredWindowClass{}, err
	}
	instance, _, callErr := getModuleHandle.Call(0)
	if instance == 0 {
		return registeredWindowClass{}, fmt.Errorf("get module handle: %w", callErr)
	}

	class := monitorWindowClass{
		procedure: monitorWindowCallback,
		instance:  instance,
		name:      uintptr(unsafe.Pointer(name)),
	}
	class.size = uint32(unsafe.Sizeof(class))
	atom, _, callErr := registerWindow.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return registeredWindowClass{}, fmt.Errorf("register monitor window class: %w", callErr)
	}
	return registeredWindowClass{name: name, instance: instance}, nil
}

// RunWindowsMonitor observes the current interactive Windows session.
// It runs until the context is cancelled or the observation receiver fails.
func RunWindowsMonitor(
	ctx context.Context,
	emit func(tracking.Observation, error) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	id, sid, err := currentWindowsSessionIdentity()
	if err != nil {
		return err
	}
	sessionID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		return fmt.Errorf("parse Windows session ID: %w", err)
	}
	session := NewSessionTracker(id, sid)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	class, err := getMonitorWindowClass()
	if err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	route := &windowsMonitorRoute{
		ctx: runCtx, cancel: cancel, events: make(chan MonitorEvent, 32),
	}
	threadID := windows.GetCurrentThreadId()
	monitorRoutes.Store(threadID, route)
	defer monitorRoutes.Delete(threadID)

	window, _, callErr := createWindow.Call(
		0, uintptr(unsafe.Pointer(class.name)), 0, 0,
		0, 0, 0, 0, 0, 0, class.instance, 0,
	)
	if window == 0 {
		return fmt.Errorf("create monitor window: %w", callErr)
	}
	route.window = window
	defer destroyWindow.Call(window)

	ok, _, callErr := registerSessionNotification.Call(window, wtsNotifyForThisSession)
	if ok == 0 {
		return fmt.Errorf("register Windows session notifications: %w", callErr)
	}
	defer unregisterSessionNotification.Call(window)

	hook, _, callErr := setWindowEventHook.Call(
		windowsEventForeground, windowsEventForeground, 0, foregroundCallback, 0, 0, 0,
	)
	if hook == 0 {
		return fmt.Errorf("register Windows foreground events: %w", callErr)
	}
	defer removeWindowEventHook.Call(hook)

	timer, _, callErr := setWindowTimer.Call(window, windowsHeartbeatTimer, windowsHeartbeatMillis, 0)
	if timer == 0 {
		return fmt.Errorf("start foreground confirmation timer: %w", callErr)
	}
	defer killWindowTimer.Call(window, timer)

	data, startupErr := readWindowsSessionInfo(uint32(sessionID))
	if startupErr == nil {
		startupErr = applyWindowsSessionInfo(session, data)
	}
	route.events <- MonitorEvent{At: time.Now(), Kind: MonitorStart}
	route.ready = true

	wakeDone := make(chan struct{})
	stopWake := context.AfterFunc(runCtx, func() {
		postWindowMessage.Call(window, windowsMessageStop, 0, 0)
		close(wakeDone)
	})
	defer func() {
		if !stopWake() {
			<-wakeDone
		}
	}()

	done := make(chan error, 1)
	go func() {
		monitor := NewMonitor(session, WindowsForegroundSource{})
		first := true
		done <- RunMonitor(runCtx, monitor, route.events,
			func(observation tracking.Observation, readErr error) error {
				if first {
					first = false
					readErr = errors.Join(startupErr, readErr)
				}
				return emit(observation, readErr)
			},
		)
		cancel()
	}()

	pumpErr := runWindowsMessageLoop(runCtx)
	cancel()
	monitorErr := <-done
	if pumpErr != nil {
		return pumpErr
	}
	return monitorErr
}

func runWindowsMessageLoop(ctx context.Context) error {
	for ctx.Err() == nil {
		var message monitorWindowMessage
		result, _, callErr := getWindowMessage.Call(
			uintptr(unsafe.Pointer(&message)), 0, 0, 0,
		)
		switch int32(result) {
		case -1:
			return fmt.Errorf("read Windows monitor message: %w", callErr)
		case 0:
			return nil
		default:
			dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
		}
	}
	return nil
}

func windowsMonitorWindowProc(window, message, wParam, lParam uintptr) uintptr {
	value, found := monitorRoutes.Load(windows.GetCurrentThreadId())
	if found {
		route := value.(*windowsMonitorRoute)
		if window == route.window {
			switch message {
			case windowsMessageStop:
				return 0
			case windowsMessageClose:
				route.cancel()
				return 0
			}

			event, handled := monitorEventFromWindowsMessage(
				uint32(message), wParam, lParam, time.Now(),
			)
			if handled {
				if route.ready {
					select {
					case route.events <- event:
					case <-route.ctx.Done():
					}
				}
				return 0
			}
		}
	}

	result, _, _ := defaultWindowProc.Call(window, message, wParam, lParam)
	return result
}

func windowsForegroundChanged(_, _, _, _, _, _, _ uintptr) uintptr {
	value, found := monitorRoutes.Load(windows.GetCurrentThreadId())
	if found {
		route := value.(*windowsMonitorRoute)
		if route.ready {
			postWindowMessage.Call(route.window, windowsMessageForeground, 0, 0)
		}
	}
	return 0
}
