//go:build windows

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"syscall"
	"time"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

const (
	DefaultWindowsNamedPipe = `\\.\pipe\OnuslyAgentIPC`
)

// Start initiates the Windows Named-Pipe IPC server.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New("ipc server already running")
	}

	pipeName := s.socketPath
	if pipeName == "" || pipeName[0] == '/' {
		pipeName = DefaultWindowsNamedPipe
	}
	s.socketPath = pipeName

	// Create cancellation event (manual-reset, initially non-signaled)
	hCancel, _, errCancel := win.ProcCreateEventW.Call(0, 1, 0, 0)
	if hCancel == 0 {
		s.mu.Unlock()
		return fmt.Errorf("failed to create IPC cancel event: %w", errCancel)
	}
	s.hCancelEvent = hCancel
	s.running = true
	s.mu.Unlock()

	go s.namedPipeAcceptLoop(ctx)
	return nil
}

// Stop terminates the Windows Named-Pipe server and cancels pending I/O.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}
	s.running = false
	close(s.stopChan)

	if s.hCancelEvent != 0 {
		win.ProcSetEvent.Call(s.hCancelEvent)
		win.ProcCloseHandle.Call(s.hCancelEvent)
		s.hCancelEvent = 0
	}

	return nil
}

func (s *Server) namedPipeAcceptLoop(ctx context.Context) {
	pipeNamePtr, err := syscall.UTF16PtrFromString(s.socketPath)
	if err != nil {
		log.Printf("[IPC Windows] Invalid pipe name %q: %v", s.socketPath, err)
		return
	}

	for {
		s.mu.Lock()
		if !s.running {
			s.mu.Unlock()
			return
		}
		cancelEvt := s.hCancelEvent
		s.mu.Unlock()

		// 1. Build explicit SDDL DACL:
		// Grant Full Control (GA) to Service SID (if found), Administrators (BA), SYSTEM (SY), LocalService (LS)
		// Grant Read+Write (GRGW) to Interactive Users (IU)
		sddl := "D:(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;LS)(A;;GRGW;;;IU)"
		sddlPtr, _ := syscall.UTF16PtrFromString(sddl)

		var pSD uintptr
		rConv, _, _ := win.ProcConvertStringSDToSDW.Call(
			uintptr(unsafe.Pointer(sddlPtr)),
			win.SDDL_REVISION_1,
			uintptr(unsafe.Pointer(&pSD)),
			0,
		)

		var sa win.SECURITY_ATTRIBUTES
		var pSA uintptr
		if rConv != 0 && pSD != 0 {
			defer win.ProcLocalFree.Call(pSD)
			sa.NLength = uint32(unsafe.Sizeof(sa))
			sa.LpSecurityDescriptor = pSD
			sa.BInheritHandle = 0
			pSA = uintptr(unsafe.Pointer(&sa))
		}

		openMode := uint32(win.PIPE_ACCESS_DUPLEX | win.FILE_FLAG_FIRST_PIPE_INSTANCE | win.FILE_FLAG_OVERLAPPED)
		pipeMode := uint32(win.PIPE_TYPE_MESSAGE | win.PIPE_READMODE_MESSAGE | win.PIPE_WAIT)

		hPipe, _, errCreate := win.ProcCreateNamedPipeW.Call(
			uintptr(unsafe.Pointer(pipeNamePtr)),
			uintptr(openMode),
			uintptr(pipeMode),
			1, // Max instances = 1
			4096,
			4096,
			5000,
			pSA,
		)
		if hPipe == uintptr(syscall.InvalidHandle) || hPipe == 0 {
			select {
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			default:
				log.Printf("[IPC Windows] CreateNamedPipeW error: %v (retrying in 200ms)", errCreate)
				time.Sleep(200 * time.Millisecond)
				continue
			}
		}

		// 2. Overlapped ConnectNamedPipe
		hConnEvent, _, _ := win.ProcCreateEventW.Call(0, 1, 0, 0)
		var ovConn win.OVERLAPPED
		ovConn.HEvent = syscall.Handle(hConnEvent)

		rConn, _, errConn := win.ProcConnectNamedPipe.Call(hPipe, uintptr(unsafe.Pointer(&ovConn)))
		connected := false

		if rConn != 0 {
			connected = true
		} else {
			errno, ok := errConn.(syscall.Errno)
			if ok && errno == 535 { // ERROR_PIPE_CONNECTED
				connected = true
			} else if ok && errno == 997 { // ERROR_IO_PENDING
				var waitHandles [2]uintptr
				waitHandles[0] = hConnEvent
				waitCount := uintptr(1)
				if cancelEvt != 0 {
					waitHandles[1] = cancelEvt
					waitCount = 2
				}

				waitRes, _, _ := win.ProcWaitForMultipleObjects.Call(
					waitCount,
					uintptr(unsafe.Pointer(&waitHandles[0])),
					0, // bWaitAll = FALSE
					0xFFFFFFFF, // INFINITE
				)

				if waitRes == win.WAIT_OBJECT_0 {
					var bytesXfer uint32
					win.ProcGetOverlappedResult.Call(hPipe, uintptr(unsafe.Pointer(&ovConn)), uintptr(unsafe.Pointer(&bytesXfer)), 1)
					connected = true
				} else {
					// Cancelled or shutdown
					win.ProcCancelIoEx.Call(hPipe, uintptr(unsafe.Pointer(&ovConn)))
					var bytesCancelled uint32
					win.ProcGetOverlappedResult.Call(hPipe, uintptr(unsafe.Pointer(&ovConn)), uintptr(unsafe.Pointer(&bytesCancelled)), 1)
					win.ProcCloseHandle.Call(hConnEvent)
					win.ProcCloseHandle.Call(hPipe)
					return
				}
			}
		}
		win.ProcCloseHandle.Call(hConnEvent)

		if connected {
			s.handleNamedPipeClient(ctx, hPipe)
			win.ProcFlushFileBuffers.Call(hPipe)
		}

		win.ProcDisconnectNamedPipe.Call(hPipe)
		win.ProcCloseHandle.Call(hPipe)
	}
}

func (s *Server) handleNamedPipeClient(ctx context.Context, hPipe uintptr) {
	// Authenticate caller on every request
	callerSID, err := s.authenticateCaller(hPipe)
	if err != nil {
		log.Printf("[IPC Windows Security Rejection] %v", err)
		resp := core.IPCResponse{
			Success: false,
			Error:   "unauthorized: " + err.Error(),
		}
		respBytes, _ := json.Marshal(resp)
		var written uint32
		win.ProcWriteFile.Call(hPipe, uintptr(unsafe.Pointer(&respBytes[0])), uintptr(len(respBytes)), uintptr(unsafe.Pointer(&written)), 0)
		return
	}

	// Bounded overlapped ReadFile (5s deadline)
	var readBuf [4096]byte
	var bytesRead uint32
	var ovRead win.OVERLAPPED
	hReadEvent, _, _ := win.ProcCreateEventW.Call(0, 1, 0, 0)
	defer win.ProcCloseHandle.Call(hReadEvent)
	ovRead.HEvent = syscall.Handle(hReadEvent)

	rRead, _, errRead := win.ProcReadFile.Call(
		hPipe,
		uintptr(unsafe.Pointer(&readBuf[0])),
		uintptr(len(readBuf)),
		uintptr(unsafe.Pointer(&bytesRead)),
		uintptr(unsafe.Pointer(&ovRead)),
	)

	if rRead == 0 {
		errno, ok := errRead.(syscall.Errno)
		if ok && errno == 997 { // ERROR_IO_PENDING
			waitRes, _, _ := win.ProcWaitForSingleObject.Call(hReadEvent, 5000)
			if waitRes == win.WAIT_OBJECT_0 {
				win.ProcGetOverlappedResult.Call(hPipe, uintptr(unsafe.Pointer(&ovRead)), uintptr(unsafe.Pointer(&bytesRead)), 1)
			} else {
				win.ProcCancelIoEx.Call(hPipe, uintptr(unsafe.Pointer(&ovRead)))
				var bytesCancelled uint32
				win.ProcGetOverlappedResult.Call(hPipe, uintptr(unsafe.Pointer(&ovRead)), uintptr(unsafe.Pointer(&bytesCancelled)), 1)
				return
			}
		} else {
			return
		}
	}

	if bytesRead == 0 {
		return
	}

	var req core.IPCRequest
	if err := json.Unmarshal(readBuf[:bytesRead], &req); err != nil {
		resp := core.IPCResponse{
			Success: false,
			Error:   "invalid json request",
		}
		respBytes, _ := json.Marshal(resp)
		var written uint32
		win.ProcWriteFile.Call(hPipe, uintptr(unsafe.Pointer(&respBytes[0])), uintptr(len(respBytes)), uintptr(unsafe.Pointer(&written)), 0)
		return
	}

	resp := s.dispatch(ctx, req, callerSID)
	respBytes, err := json.Marshal(resp)
	if err == nil {
		var written uint32
		win.ProcWriteFile.Call(hPipe, uintptr(unsafe.Pointer(&respBytes[0])), uintptr(len(respBytes)), uintptr(unsafe.Pointer(&written)), 0)
	}
}

// authenticateCaller enforces the 8-step caller authentication and isolation pipeline,
// and returns the validated caller's SID string.
func (s *Server) authenticateCaller(hPipe uintptr) (string, error) {
	// 1. Obtain client session ID via named-pipe session API
	var clientSessionID uint32
	rSess, _, errSess := win.ProcGetNamedPipeClientSessionId.Call(hPipe, uintptr(unsafe.Pointer(&clientSessionID)))
	if rSess == 0 {
		return "", fmt.Errorf("GetNamedPipeClientSessionId failed: %w", errSess)
	}

	// 2. Validate client session
	s.mu.RLock()
	valFn := s.sessionValidator
	s.mu.RUnlock()

	var activeConsoleID uint32
	if valFn != nil {
		if err := valFn(clientSessionID); err != nil {
			return "", err
		}
	} else {
		rConsole, _, _ := win.ProcWTSGetActiveConsoleSessionId.Call()
		activeConsoleID = uint32(rConsole)
		if activeConsoleID == 0xFFFFFFFF {
			return "", win.ErrConsoleDetached
		}
		if clientSessionID != activeConsoleID {
			return "", fmt.Errorf("caller session %d does not match active console session %d", clientSessionID, activeConsoleID)
		}
	}

	// 3. Impersonate named-pipe client
	rImp, _, errImp := win.ProcImpersonateNamedPipeClient.Call(hPipe)
	if rImp == 0 {
		return "", fmt.Errorf("ImpersonateNamedPipeClient failed: %w", errImp)
	}

	// 8. Guaranteed RevertToSelf on all return paths
	defer func() {
		rRev, _, _ := win.ProcRevertToSelf.Call()
		if rRev == 0 {
			log.Printf("[IPC Security] CRITICAL: RevertToSelf failed!")
		}
	}()

	// 4. Open thread token via GetCurrentThread
	hThread, _, _ := win.ProcGetCurrentThread.Call()
	var hThreadToken syscall.Handle
	rTok, _, errTok := win.ProcOpenThreadToken.Call(
		hThread,
		uintptr(0x0008), // TOKEN_QUERY
		1,               // OpenAsSelf = TRUE
		uintptr(unsafe.Pointer(&hThreadToken)),
	)
	if rTok == 0 {
		return "", fmt.Errorf("OpenThreadToken failed: %w", errTok)
	}
	defer win.ProcCloseHandle.Call(uintptr(hThreadToken))

	// 5. Query TokenImpersonationLevel (must equal SecurityIdentification = 1)
	var impLevel uint32
	var retLen uint32
	win.ProcGetTokenInformation.Call(
		uintptr(hThreadToken),
		9, // TokenImpersonationLevel
		uintptr(unsafe.Pointer(&impLevel)),
		uintptr(unsafe.Sizeof(impLevel)),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if impLevel != 1 {
		return "", fmt.Errorf("expected TokenImpersonationLevel=1 (SecurityIdentification), got %d", impLevel)
	}

	// 6. Query TokenUser and compare binary SID with expected paired owner SID
	retLen = 0
	win.ProcGetTokenInformation.Call(uintptr(hThreadToken), 1, 0, 0, uintptr(unsafe.Pointer(&retLen)))
	if retLen == 0 {
		return "", fmt.Errorf("failed to query TokenUser buffer length")
	}

	uBuf := make([]byte, retLen)
	rU, _, errU := win.ProcGetTokenInformation.Call(
		uintptr(hThreadToken),
		1,
		uintptr(unsafe.Pointer(&uBuf[0])),
		uintptr(retLen),
		uintptr(unsafe.Pointer(&retLen)),
	)
	if rU == 0 {
		return "", fmt.Errorf("GetTokenInformation(TokenUser) failed: %w", errU)
	}

	tu := (*win.TOKEN_USER)(unsafe.Pointer(&uBuf[0]))
	rValid, _, _ := win.ProcIsValidSid.Call(tu.User.Sid)
	if rValid == 0 {
		return "", fmt.Errorf("client token SID is invalid")
	}

	clientSidStr, errStr := win.SidToString(tu.User.Sid)
	if errStr != nil {
		return "", fmt.Errorf("failed to convert client SID to string: %w", errStr)
	}

	expectedOwnerSID := s.GetOwnerSID()
	if expectedOwnerSID != "" {
		pExpectedSID, errExp := win.StringToSid(expectedOwnerSID)
		if errExp != nil {
			return "", fmt.Errorf("failed to parse expected owner SID: %w", errExp)
		}
		defer win.ProcLocalFree.Call(pExpectedSID)

		if !win.EqualSid(tu.User.Sid, pExpectedSID) {
			return "", fmt.Errorf("caller SID %s does not match paired owner SID", clientSidStr)
		}
	}

	// 7. Post-check active console session to mitigate TOCTOU race
	if valFn == nil {
		rConsolePost, _, _ := win.ProcWTSGetActiveConsoleSessionId.Call()
		if uint32(rConsolePost) != activeConsoleID {
			return "", fmt.Errorf("active console session changed from %d to %d during authorization", activeConsoleID, rConsolePost)
		}
	}

	return clientSidStr, nil
}
