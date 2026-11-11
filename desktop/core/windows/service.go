//go:build windows

package windows

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	ErrNotRunningAsService = errors.New("process was not started by the Service Control Manager (error 1063)")
)

type ServiceWorkerFunc func(ctx context.Context, cancelChan <-chan struct{}, cancelEvent uintptr, sessionMonitor *ConsoleSessionMonitor) error

// WindowsService manages the SCM lifecycle, control handler callbacks, session changes,
// and bounded shutdown for the ONUSLY Windows desktop service.
type WindowsService struct {
	ServiceName    string
	sessionMonitor *ConsoleSessionMonitor
	workerFunc     ServiceWorkerFunc

	mu             sync.Mutex
	hStatus        uintptr
	hCancelEvent   uintptr
	stopChan       chan struct{}
	workerDoneChan chan struct{}
}

var (
	activeService   *WindowsService
	activeServiceMu sync.Mutex
)

func NewWindowsService(serviceName string, sessionMonitor *ConsoleSessionMonitor, worker ServiceWorkerFunc) *WindowsService {
	if sessionMonitor == nil {
		sessionMonitor = NewConsoleSessionMonitor()
	}
	return &WindowsService{
		ServiceName:    serviceName,
		sessionMonitor: sessionMonitor,
		workerFunc:     worker,
		stopChan:       make(chan struct{}, 1),
		workerDoneChan: make(chan struct{}, 1),
	}
}

// Run attempts to connect to the SCM. If invoked from an interactive terminal,
// StartServiceCtrlDispatcherW fails with 1063 (ERROR_FAILED_SERVICE_CONTROLLER_CONNECT),
// returning ErrNotRunningAsService.
func (s *WindowsService) Run() error {
	activeServiceMu.Lock()
	activeService = s
	activeServiceMu.Unlock()

	svcNamePtr, _ := syscall.UTF16PtrFromString(s.ServiceName)
	entryTable := []SERVICE_TABLE_ENTRY{
		{
			ServiceName: svcNamePtr,
			ServiceProc: syscall.NewCallback(serviceMainWrapper),
		},
		{
			ServiceName: nil,
			ServiceProc: 0,
		},
	}

	r, _, err := ProcStartServiceCtrlDispatcherW.Call(uintptr(unsafe.Pointer(&entryTable[0])))
	if r == 0 {
		errno, ok := err.(syscall.Errno)
		if ok && errno == 1063 { // ERROR_FAILED_SERVICE_CONTROLLER_CONNECT
			return ErrNotRunningAsService
		}
		return fmt.Errorf("StartServiceCtrlDispatcherW failed: %w", err)
	}

	return nil
}

func serviceMainWrapper(argc uintptr, argv uintptr) uintptr {
	activeServiceMu.Lock()
	s := activeService
	activeServiceMu.Unlock()

	if s != nil {
		s.serviceMain()
	}
	return 0
}

func (s *WindowsService) serviceMain() {
	svcNamePtr, _ := syscall.UTF16PtrFromString(s.ServiceName)
	cbHandler := syscall.NewCallback(serviceHandlerExWrapper)

	h, _, errReg := ProcRegisterServiceCtrlHandlerExW.Call(
		uintptr(unsafe.Pointer(svcNamePtr)),
		cbHandler,
		0,
	)
	if h == 0 {
		log.Printf("[SCM Error] RegisterServiceCtrlHandlerExW failed: %v", errReg)
		return
	}
	s.hStatus = h

	// 1. Report SERVICE_START_PENDING promptly with checkpoint 1, wait hint 5000ms
	s.setStatus(SERVICE_START_PENDING, 0, 1, 5000, 0)

	// Create cancellation event (manual-reset, non-signaled)
	hCancel, _, _ := ProcCreateEventW.Call(0, 1, 0, 0)
	s.hCancelEvent = hCancel
	defer func() {
		if s.hCancelEvent != 0 {
			ProcCloseHandle.Call(s.hCancelEvent)
			s.hCancelEvent = 0
		}
	}()

	cancelChan := make(chan struct{})

	// 2. Report SERVICE_RUNNING promptly (< 50ms) accepting STOP, SHUTDOWN, SESSIONCHANGE, POWEREVENT
	controls := uint32(SERVICE_ACCEPT_STOP | SERVICE_ACCEPT_SHUTDOWN | SERVICE_ACCEPT_SESSIONCHANGE | SERVICE_ACCEPT_POWEREVENT)
	s.setStatus(SERVICE_RUNNING, controls, 0, 0, 0)

	// 3. Launch background service worker
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	go func() {
		defer close(s.workerDoneChan)
		if s.workerFunc != nil {
			_ = s.workerFunc(ctx, cancelChan, s.hCancelEvent, s.sessionMonitor)
		}
	}()

	// 4. Block until stop control signal is received
	<-s.stopChan

	// 5. Bounded Shutdown Protocol
	// Report SERVICE_STOP_PENDING promptly
	s.setStatus(SERVICE_STOP_PENDING, 0, 1, 5000, 0)

	// Signal cancellation across Go channels and Win32 event
	close(cancelChan)
	cancelCtx()
	if s.hCancelEvent != 0 {
		ProcSetEvent.Call(s.hCancelEvent)
	}

	// Bounded wait for worker to complete cleanup (max 3500ms)
	select {
	case <-s.workerDoneChan:
		log.Printf("[SCM] All workers exited cleanly.")
	case <-time.After(3500 * time.Millisecond):
		log.Printf("[SCM Warning] Worker shutdown exceeded deadline; forcing completion.")
	}

	// 6. Report SERVICE_STOPPED with exit code 0
	s.setStatus(SERVICE_STOPPED, 0, 0, 0, 0)
}

func serviceHandlerExWrapper(control uintptr, eventType uintptr, eventData uintptr, context uintptr) uintptr {
	activeServiceMu.Lock()
	s := activeService
	activeServiceMu.Unlock()

	if s != nil {
		return s.handleControl(uint32(control), uint32(eventType), eventData)
	}
	return 0x00000078 // ERROR_CALL_NOT_IMPLEMENTED
}

func (s *WindowsService) handleControl(control uint32, eventType uint32, eventData uintptr) uintptr {
	switch control {
	case SERVICE_CONTROL_STOP, SERVICE_CONTROL_SHUTDOWN:
		select {
		case s.stopChan <- struct{}{}:
		default:
		}
		return 0

	case SERVICE_CONTROL_SESSIONCHANGE:
		var sessionID uint32
		if eventData != 0 {
			p := *(*unsafe.Pointer)(unsafe.Pointer(&eventData))
			notif := (*WTSSESSION_NOTIFICATION)(p)
			sessionID = notif.SessionID
		}
		if s.sessionMonitor != nil {
			s.sessionMonitor.OnSessionChange(eventType, sessionID)
		}
		return 0

	case SERVICE_CONTROL_POWEREVENT:
		switch eventType {
		case PBT_APMSUSPEND:
			log.Printf("[SCM Power] System entering sleep/suspend.")
		case PBT_APMRESUMEAUTOMATIC, PBT_APMRESUMESUSPEND:
			log.Printf("[SCM Power] System resumed from sleep/suspend.")
			if s.sessionMonitor != nil {
				s.sessionMonitor.Refresh()
			}
		}
		return 0

	case SERVICE_CONTROL_INTERROGATE:
		return 0

	default:
		return 0x00000078 // ERROR_CALL_NOT_IMPLEMENTED
	}
}

func (s *WindowsService) setStatus(currentState uint32, controlsAccepted uint32, checkPoint uint32, waitHint uint32, exitCode uint32) {
	if s.hStatus == 0 {
		return
	}
	st := SERVICE_STATUS{
		ServiceType:      SERVICE_WIN32_OWN_PROCESS,
		CurrentState:     currentState,
		ControlsAccepted: controlsAccepted,
		Win32ExitCode:    exitCode,
		CheckPoint:       checkPoint,
		WaitHint:         waitHint,
	}
	ProcSetServiceStatus.Call(s.hStatus, uintptr(unsafe.Pointer(&st)))
}
