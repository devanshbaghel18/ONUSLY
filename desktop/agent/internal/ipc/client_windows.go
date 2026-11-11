//go:build windows

package ipc

import (
	"encoding/json"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// Query connects to the Windows named pipe with SecurityIdentification impersonation level,
// sends an IPCRequest and returns the decoded IPCResponse.
func Query(pipePath string, req core.IPCRequest, timeout time.Duration) (*core.IPCResponse, error) {
	if pipePath == "" || pipePath[0] == '/' {
		pipePath = DefaultWindowsNamedPipe
	}

	pipeNamePtr, err := syscall.UTF16PtrFromString(pipePath)
	if err != nil {
		return nil, fmt.Errorf("invalid pipe name %q: %w", pipePath, err)
	}

	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	flags := uint32(win.SECURITY_SQOS_PRESENT | win.SECURITY_IDENTIFICATION)
	var hClient uintptr
	var errOpen error

	deadline := time.Now().Add(timeout)
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
		return nil, fmt.Errorf("agent named pipe unreachable at %s: %w", pipePath, errOpen)
	}
	defer win.ProcCloseHandle.Call(hClient)

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
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
		return nil, fmt.Errorf("failed to write request to pipe: %w", errWrite)
	}

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
		return nil, fmt.Errorf("failed to read response from pipe: %w", errRead)
	}

	if bytesRead == 0 {
		return nil, fmt.Errorf("received empty response from agent pipe")
	}

	var resp core.IPCResponse
	if err := json.Unmarshal(readBuf[:bytesRead], &resp); err != nil {
		return nil, fmt.Errorf("failed to decode response JSON: %w", err)
	}

	return &resp, nil
}

