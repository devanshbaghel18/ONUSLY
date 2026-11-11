//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

const (
	DefaultWindowsIPCPath = `\\.\pipe\OnuslyAgentIPC`
)

func getDefaultIPCPath() string {
	return DefaultWindowsIPCPath
}

func queryAgent(pipePath string, req core.IPCRequest) core.IPCResponse {
	if pipePath == "" || pipePath[0] == '/' {
		pipePath = DefaultWindowsIPCPath
	}

	pipeNamePtr, err := syscall.UTF16PtrFromString(pipePath)
	if err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   "invalid pipe name: " + err.Error(),
		}
	}

	// Connect to Windows service named pipe with SecurityIdentification SQOS
	flags := uint32(win.SECURITY_SQOS_PRESENT | win.SECURITY_IDENTIFICATION)
	var hClient uintptr
	var errOpen error

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		hClient, _, errOpen = win.ProcCreateFileW.Call(
			uintptr(unsafe.Pointer(pipeNamePtr)),
			uintptr(win.GENERIC_READ|win.GENERIC_WRITE),
			0,
			0,
			uintptr(win.OPEN_EXISTING),
			uintptr(flags),
			0,
		)
		if hClient != uintptr(syscall.InvalidHandle) && hClient != 0 {
			break
		}

		errno, ok := errOpen.(syscall.Errno)
		if ok && (errno == 2 || errno == 231) {
			// ERROR_FILE_NOT_FOUND or ERROR_PIPE_BUSY
			win.ProcWaitNamedPipeW.Call(uintptr(unsafe.Pointer(pipeNamePtr)), 100)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		break
	}

	if hClient == uintptr(syscall.InvalidHandle) || hClient == 0 {
		return core.IPCResponse{
			Success: false,
			Error:   fmt.Sprintf("agent unreachable: %v", errOpen),
		}
	}
	defer win.ProcCloseHandle.Call(hClient)

	// Encode and send IPCRequest
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   "failed to serialize request: " + err.Error(),
		}
	}

	var written uint32
	rWrite, _, errWrite := win.ProcWriteFile.Call(
		hClient,
		uintptr(unsafe.Pointer(&reqBytes[0])),
		uintptr(len(reqBytes)),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	if rWrite == 0 {
		return core.IPCResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to write to agent pipe: %v", errWrite),
		}
	}

	// Read IPCResponse (max 64KB bounded)
	var readBuf [65536]byte
	var bytesRead uint32
	rRead, _, errRead := win.ProcReadFile.Call(
		hClient,
		uintptr(unsafe.Pointer(&readBuf[0])),
		uintptr(len(readBuf)),
		uintptr(unsafe.Pointer(&bytesRead)),
		0,
	)
	if rRead == 0 {
		return core.IPCResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to read from agent pipe: %v", errRead),
		}
	}

	if bytesRead == 0 {
		return core.IPCResponse{
			Success: false,
			Error:   "empty response from agent pipe",
		}
	}

	var resp core.IPCResponse
	if err := json.Unmarshal(readBuf[:bytesRead], &resp); err != nil {
		return core.IPCResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to parse agent response: %v", err),
		}
	}

	return resp
}
