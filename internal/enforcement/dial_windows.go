//go:build windows

package enforcement

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

func queryLocalAgent(req core.IPCRequest, timeout time.Duration) (*core.IPCResponse, error) {
	pipePath := os.Getenv("ONUSLY_IPC_PATH")
	if pipePath == "" {
		pipePath = `\\.\pipe\OnuslyAgentIPC`
	}

	pipeNamePtr, err := syscall.UTF16PtrFromString(pipePath)
	if err != nil {
		return nil, fmt.Errorf("invalid pipe name %q: %w", pipePath, err)
	}

	if timeout <= 0 {
		timeout = 3 * time.Second
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
			win.ProcWaitNamedPipeW.Call(uintptr(unsafe.Pointer(pipeNamePtr)), 100)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		break
	}

	if hClient == uintptr(syscall.InvalidHandle) || hClient == 0 {
		return nil, fmt.Errorf("desktop agent unreachable on %s: %w", pipePath, errOpen)
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
		return nil, fmt.Errorf("failed to write to agent pipe: %w", errWrite)
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
		return nil, fmt.Errorf("failed to read from agent pipe: %w", errRead)
	}

	if bytesRead == 0 {
		return nil, fmt.Errorf("empty response from agent pipe")
	}

	var resp core.IPCResponse
	if err := json.Unmarshal(readBuf[:bytesRead], &resp); err != nil {
		return nil, fmt.Errorf("failed to parse agent response: %w", err)
	}

	return &resp, nil
}

