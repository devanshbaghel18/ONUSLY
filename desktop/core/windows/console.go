//go:build windows

package windows

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	ErrConsoleDetached    = errors.New("physical console is detached (session 0xFFFFFFFF)")
	ErrRDPUnsupported     = errors.New("remote desktop session is not supported (physical console required)")
	ErrSessionUserUnknown = errors.New("failed to resolve interactive user for physical console session")
)

type ConsoleSessionInfo struct {
	SessionID         uint32
	IsAttached        bool
	IsPhysicalConsole bool
	UserName          string
	DomainName        string
	UserSID           string
}

// GetActiveConsoleSession inspects WTS to determine the active physical console session and user SID.
func GetActiveConsoleSession() (*ConsoleSessionInfo, error) {
	rConsole, _, _ := ProcWTSGetActiveConsoleSessionId.Call()
	consoleSessionID := uint32(rConsole)

	if consoleSessionID == 0xFFFFFFFF {
		return &ConsoleSessionInfo{
			SessionID:         0xFFFFFFFF,
			IsAttached:        false,
			IsPhysicalConsole: false,
		}, ErrConsoleDetached
	}

	info := &ConsoleSessionInfo{
		SessionID:         consoleSessionID,
		IsAttached:        true,
		IsPhysicalConsole: true,
	}

	// 1. Query client protocol type (WTSClientProtocolType = 16)
	// Protocol 0 = Console (local hardware), 2 = RDP
	var pProto *uint16
	var bytesReturned uint32
	rP, _, _ := ProcWTSQuerySessionInformationW.Call(
		0,
		uintptr(consoleSessionID),
		16, // WTSClientProtocolType
		uintptr(unsafe.Pointer(&pProto)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if rP != 0 && pProto != nil {
		protoVal := *(*uint16)(unsafe.Pointer(pProto))
		ProcWTSFreeMemory.Call(uintptr(unsafe.Pointer(pProto)))
		if protoVal != 0 {
			info.IsPhysicalConsole = false
			return info, ErrRDPUnsupported
		}
	}

	// 2. Query user name (WTSUserName = 5)
	var pUser *uint16
	rU, _, _ := ProcWTSQuerySessionInformationW.Call(
		0,
		uintptr(consoleSessionID),
		5, // WTSUserName
		uintptr(unsafe.Pointer(&pUser)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if rU != 0 && pUser != nil {
		info.UserName = PtrToUTF16String(pUser)
		ProcWTSFreeMemory.Call(uintptr(unsafe.Pointer(pUser)))
	}

	// 3. Query domain name (WTSDomainName = 7)
	var pDomain *uint16
	rD, _, _ := ProcWTSQuerySessionInformationW.Call(
		0,
		uintptr(consoleSessionID),
		7, // WTSDomainName
		uintptr(unsafe.Pointer(&pDomain)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if rD != 0 && pDomain != nil {
		info.DomainName = PtrToUTF16String(pDomain)
		ProcWTSFreeMemory.Call(uintptr(unsafe.Pointer(pDomain)))
	}

	if info.UserName == "" {
		return info, ErrSessionUserUnknown
	}

	// 4. Resolve SID via LookupAccountNameW
	fullAcc := info.UserName
	if info.DomainName != "" {
		fullAcc = info.DomainName + "\\" + info.UserName
	}
	pAcc, err := syscall.UTF16PtrFromString(fullAcc)
	if err != nil {
		return info, err
	}

	var sidBuf [256]byte
	sidLen := uint32(len(sidBuf))
	var domBuf [256]uint16
	domLen := uint32(len(domBuf))
	var sidUse uint32

	rL, _, errL := ProcLookupAccountNameW.Call(
		0,
		uintptr(unsafe.Pointer(pAcc)),
		uintptr(unsafe.Pointer(&sidBuf[0])),
		uintptr(unsafe.Pointer(&sidLen)),
		uintptr(unsafe.Pointer(&domBuf[0])),
		uintptr(unsafe.Pointer(&domLen)),
		uintptr(unsafe.Pointer(&sidUse)),
	)
	if rL == 0 {
		return info, fmt.Errorf("LookupAccountNameW failed for %s: %v", fullAcc, errL)
	}

	var strSid *uint16
	rC, _, errC := ProcConvertSidToStringSidW.Call(
		uintptr(unsafe.Pointer(&sidBuf[0])),
		uintptr(unsafe.Pointer(&strSid)),
	)
	if rC == 0 || strSid == nil {
		return info, fmt.Errorf("ConvertSidToStringSidW failed: %v", errC)
	}
	defer ProcLocalFree.Call(uintptr(unsafe.Pointer(strSid)))

	info.UserSID = PtrToUTF16String(strSid)
	return info, nil
}

// ConsoleSessionMonitor provides thread-safe reconciliation of the active console session.
type ConsoleSessionMonitor struct {
	mu           sync.RWMutex
	current      *ConsoleSessionInfo
	onChangeChan chan struct{}
}

func NewConsoleSessionMonitor() *ConsoleSessionMonitor {
	m := &ConsoleSessionMonitor{
		onChangeChan: make(chan struct{}, 1),
	}
	m.Refresh()
	return m
}

func (m *ConsoleSessionMonitor) Refresh() *ConsoleSessionInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, _ := GetActiveConsoleSession()
	m.current = info

	select {
	case m.onChangeChan <- struct{}{}:
	default:
	}

	return info
}

func (m *ConsoleSessionMonitor) OnSessionChange(eventType uint32, sessionID uint32) {
	// Reconcile on any console connect, disconnect, logon, logoff, or lock
	m.Refresh()
}

func (m *ConsoleSessionMonitor) GetCurrent() *ConsoleSessionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.current
}

// ValidateClientSession verifies that the caller's session matches the active physical console session.
func (m *ConsoleSessionMonitor) ValidateClientSession(clientSessionID uint32) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.current == nil || !m.current.IsAttached {
		return ErrConsoleDetached
	}
	if !m.current.IsPhysicalConsole {
		return ErrRDPUnsupported
	}
	if clientSessionID != m.current.SessionID {
		return fmt.Errorf("client session %d does not match active console session %d", clientSessionID, m.current.SessionID)
	}
	return nil
}
