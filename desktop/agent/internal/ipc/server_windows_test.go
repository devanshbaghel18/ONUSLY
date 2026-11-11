//go:build windows

package ipc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type mockSyncerForIPC struct {
	blocklist *core.BlocklistData
}

func (m *mockSyncerForIPC) Sync(ctx context.Context) (*core.BlocklistData, error) {
	return m.blocklist, nil
}

func (m *mockSyncerForIPC) GetCachedBlocklist() (*core.BlocklistData, error) {
	return m.blocklist, nil
}

type mockCredStoreForIPC struct {
	token string
}

func (m *mockCredStoreForIPC) SaveToken(t string) error { m.token = t; return nil }
func (m *mockCredStoreForIPC) GetToken() (string, error) { return m.token, nil }
func (m *mockCredStoreForIPC) ClearToken() error         { m.token = ""; return nil }

type mockIdentityStoreForIPC struct {
	deviceID string
}

func (m *mockIdentityStoreForIPC) GetDeviceID() (string, error) {
	return m.deviceID, nil
}

func getCurrentUserSID(t *testing.T) string {
	hProc, _, _ := win.ProcGetCurrentProcess.Call()
	var hToken syscall.Handle
	r, _, err := win.ProcOpenProcessToken.Call(hProc, uintptr(0x0008), uintptr(unsafe.Pointer(&hToken)))
	if r == 0 {
		t.Fatalf("OpenProcessToken failed: %v", err)
	}
	defer win.ProcCloseHandle.Call(uintptr(hToken))

	var retLen uint32
	win.ProcGetTokenInformation.Call(uintptr(hToken), 1, 0, 0, uintptr(unsafe.Pointer(&retLen)))
	if retLen == 0 {
		t.Fatalf("Failed to query TokenUser length")
	}

	buf := make([]byte, retLen)
	r2, _, err2 := win.ProcGetTokenInformation.Call(uintptr(hToken), 1, uintptr(unsafe.Pointer(&buf[0])), uintptr(retLen), uintptr(unsafe.Pointer(&retLen)))
	if r2 == 0 {
		t.Fatalf("GetTokenInformation failed: %v", err2)
	}

	tu := (*win.TOKEN_USER)(unsafe.Pointer(&buf[0]))
	sidStr, err := win.SidToString(tu.User.Sid)
	if err != nil {
		t.Fatalf("SidToString failed: %v", err)
	}
	return sidStr
}

func connectTestClient(pipeName string, sqosFlags uint32) (uintptr, error) {
	pipeNamePtr, _ := syscall.UTF16PtrFromString(pipeName)
	deadline := time.Now().Add(3 * time.Second)

	for time.Now().Before(deadline) {
		hClient, _, errOpen := win.ProcCreateFileW.Call(
			uintptr(unsafe.Pointer(pipeNamePtr)),
			uintptr(win.GENERIC_READ|win.GENERIC_WRITE),
			0,
			0,
			uintptr(win.OPEN_EXISTING),
			uintptr(sqosFlags),
			0,
		)
		if hClient != uintptr(syscall.InvalidHandle) && hClient != 0 {
			return hClient, nil
		}

		errno, ok := errOpen.(syscall.Errno)
		if ok && (errno == 2 || errno == 231) {
			win.ProcWaitNamedPipeW.Call(uintptr(unsafe.Pointer(pipeNamePtr)), 50)
			time.Sleep(20 * time.Millisecond)
			continue
		}
		return 0, errOpen
	}
	return 0, fmt.Errorf("timeout connecting to pipe %s", pipeName)
}

func sendAndReceive(hPipe uintptr, req core.IPCRequest) (*core.IPCResponse, error) {
	reqBytes, _ := json.Marshal(req)
	var written uint32
	rWrite, _, errWrite := win.ProcWriteFile.Call(
		hPipe,
		uintptr(unsafe.Pointer(&reqBytes[0])),
		uintptr(len(reqBytes)),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	if rWrite == 0 {
		return nil, fmt.Errorf("WriteFile failed: %v", errWrite)
	}

	var readBuf [4096]byte
	var bytesRead uint32
	rRead, _, errRead := win.ProcReadFile.Call(
		hPipe,
		uintptr(unsafe.Pointer(&readBuf[0])),
		uintptr(len(readBuf)),
		uintptr(unsafe.Pointer(&bytesRead)),
		0,
	)
	if rRead == 0 || bytesRead == 0 {
		return nil, fmt.Errorf("ReadFile failed: %v", errRead)
	}

	var resp core.IPCResponse
	if err := json.Unmarshal(readBuf[:bytesRead], &resp); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}
	return &resp, nil
}

// 1. Authorized Paired SID Acceptance Test
func TestWindowsNamedPipe_AuthorizedPairedSID(t *testing.T) {
	pipeName := fmt.Sprintf(`\\.\pipe\OnuslyTestPipe_Auth_%d_%d`, os.Getpid(), time.Now().UnixNano())
	currentUserSID := getCurrentUserSID(t)

	syncer := &mockSyncerForIPC{
		blocklist: &core.BlocklistData{
			UserID:  "test-user",
			Version: "v1.0",
			Targets: []core.TargetItem{
				{GoalTitle: "No Distractions", Status: "active", Domains: []string{"distraction.com"}},
			},
		},
	}
	credStore := &mockCredStoreForIPC{token: "test.jwt.token"}
	idStore := &mockIdentityStoreForIPC{deviceID: "dev-123"}

	srv := NewServer(pipeName, syncer, credStore, idStore)
	srv.SetOwnerSID(currentUserSID)
	srv.SetSessionValidator(func(clientSessionID uint32) error {
		return nil // Accept current session in unit test
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server Start failed: %v", err)
	}
	defer srv.Stop()

	hClient, err := connectTestClient(pipeName, win.SECURITY_SQOS_PRESENT|win.SECURITY_IDENTIFICATION)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer win.ProcCloseHandle.Call(hClient)

	// Test Ping
	respPing, err := sendAndReceive(hClient, core.IPCRequest{Action: "ping"})
	if err != nil {
		t.Fatalf("Ping request failed: %v", err)
	}
	if !respPing.Success {
		t.Fatalf("Ping failed: %s", respPing.Error)
	}

	// Connect second client for get-blocklist
	hClient2, err := connectTestClient(pipeName, win.SECURITY_SQOS_PRESENT|win.SECURITY_IDENTIFICATION)
	if err != nil {
		t.Fatalf("Failed to connect client 2: %v", err)
	}
	defer win.ProcCloseHandle.Call(hClient2)

	respBlocklist, err := sendAndReceive(hClient2, core.IPCRequest{Action: "get-blocklist"})
	if err != nil {
		t.Fatalf("get-blocklist failed: %v", err)
	}
	if !respBlocklist.Success || respBlocklist.Blocklist == nil || len(respBlocklist.Blocklist.Targets) != 1 {
		t.Fatalf("Unexpected blocklist response: %+v", respBlocklist)
	}
}

// 2. Unauthorized Wrong SID Rejection Test (Direct regression test for W-07)
func TestWindowsNamedPipe_UnauthorizedWrongSID(t *testing.T) {
	pipeName := fmt.Sprintf(`\\.\pipe\OnuslyTestPipe_WrongSID_%d_%d`, os.Getpid(), time.Now().UnixNano())

	// Configure server with a different paired owner SID
	bogusOwnerSID := "S-1-5-21-999999999-888888888-777777777-1234"

	srv := NewServer(pipeName, &mockSyncerForIPC{}, &mockCredStoreForIPC{}, &mockIdentityStoreForIPC{})
	srv.SetOwnerSID(bogusOwnerSID)
	srv.SetSessionValidator(func(clientSessionID uint32) error {
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server Start failed: %v", err)
	}
	defer srv.Stop()

	// Client connects with actual user token (which will not match bogusOwnerSID)
	hClient, err := connectTestClient(pipeName, win.SECURITY_SQOS_PRESENT|win.SECURITY_IDENTIFICATION)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer win.ProcCloseHandle.Call(hClient)

	resp, err := sendAndReceive(hClient, core.IPCRequest{Action: "ping"})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Invariant: MUST fail closed with unauthorized error
	if resp.Success {
		t.Fatalf("Security failure: Server accepted request from unauthorized caller SID!")
	}
	if resp.Error == "" {
		t.Fatalf("Expected error message for unauthorized caller, got empty string")
	}
}

// 3. Wrong Session Rejection Test
func TestWindowsNamedPipe_WrongSessionRejection(t *testing.T) {
	pipeName := fmt.Sprintf(`\\.\pipe\OnuslyTestPipe_WrongSess_%d_%d`, os.Getpid(), time.Now().UnixNano())
	currentUserSID := getCurrentUserSID(t)

	srv := NewServer(pipeName, &mockSyncerForIPC{}, &mockCredStoreForIPC{}, &mockIdentityStoreForIPC{})
	srv.SetOwnerSID(currentUserSID)
	// Simulate wrong session rejection (e.g. RDP or detached session)
	srv.SetSessionValidator(func(clientSessionID uint32) error {
		return fmt.Errorf("session %d is an unsupported remote session", clientSessionID)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server Start failed: %v", err)
	}
	defer srv.Stop()

	hClient, err := connectTestClient(pipeName, win.SECURITY_SQOS_PRESENT|win.SECURITY_IDENTIFICATION)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer win.ProcCloseHandle.Call(hClient)

	resp, err := sendAndReceive(hClient, core.IPCRequest{Action: "ping"})
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}

	// Invariant: MUST fail closed when session is invalid
	if resp.Success {
		t.Fatalf("Security failure: Server accepted request from invalid session!")
	}
}

// 4. Guaranteed RevertToSelf Invariant Test
func TestWindowsNamedPipe_GuaranteedRevertToSelf(t *testing.T) {
	pipeName := fmt.Sprintf(`\\.\pipe\OnuslyTestPipe_Revert_%d_%d`, os.Getpid(), time.Now().UnixNano())
	currentUserSID := getCurrentUserSID(t)

	srv := NewServer(pipeName, &mockSyncerForIPC{}, &mockCredStoreForIPC{}, &mockIdentityStoreForIPC{})
	srv.SetOwnerSID(currentUserSID)
	srv.SetSessionValidator(func(clientSessionID uint32) error { return nil })

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Server Start failed: %v", err)
	}
	defer srv.Stop()

	// Verify no thread token on calling thread before IPC
	hThread, _, _ := win.ProcGetCurrentThread.Call()
	var hTokenBefore syscall.Handle
	rBefore, _, _ := win.ProcOpenThreadToken.Call(hThread, uintptr(0x0008), 1, uintptr(unsafe.Pointer(&hTokenBefore)))
	if rBefore != 0 {
		win.ProcCloseHandle.Call(uintptr(hTokenBefore))
		t.Fatalf("Thread token was active before IPC request!")
	}

	hClient, err := connectTestClient(pipeName, win.SECURITY_SQOS_PRESENT|win.SECURITY_IDENTIFICATION)
	if err != nil {
		t.Fatalf("Failed to connect client: %v", err)
	}
	defer win.ProcCloseHandle.Call(hClient)

	_, _ = sendAndReceive(hClient, core.IPCRequest{Action: "ping"})

	// Verify no thread token leaked after IPC execution
	var hTokenAfter syscall.Handle
	rAfter, _, _ := win.ProcOpenThreadToken.Call(hThread, uintptr(0x0008), 1, uintptr(unsafe.Pointer(&hTokenAfter)))
	if rAfter != 0 {
		win.ProcCloseHandle.Call(uintptr(hTokenAfter))
		t.Fatalf("Thread token leaked after IPC request! RevertToSelf was not called or failed!")
	}
}
