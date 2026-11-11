//go:build windows
// +build windows

package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

const (
	SERVICE_WIN32_OWN_PROCESS = 0x00000010
	SERVICE_STOPPED           = 0x00000001
	SERVICE_START_PENDING     = 0x00000002
	SERVICE_STOP_PENDING      = 0x00000003
	SERVICE_RUNNING           = 0x00000004

	SERVICE_ACCEPT_STOP          = 0x00000001
	SERVICE_ACCEPT_SHUTDOWN      = 0x00000004
	SERVICE_ACCEPT_SESSIONCHANGE = 0x00000080
	SERVICE_ACCEPT_POWEREVENT    = 0x00000040

	SERVICE_CONTROL_STOP        = 0x00000001
	SERVICE_CONTROL_SHUTDOWN    = 0x00000005
	SERVICE_CONTROL_INTERROGATE = 0x00000004

	// Named Pipe & SQOS constants
	SECURITY_SQOS_PRESENT   = 0x00100000
	SECURITY_IDENTIFICATION = 0x00010000

	PIPE_ACCESS_DUPLEX            = 0x00000003
	FILE_FLAG_FIRST_PIPE_INSTANCE = 0x00080000
	FILE_FLAG_OVERLAPPED          = 0x40000000
	PIPE_TYPE_MESSAGE             = 0x00000004
	PIPE_READMODE_MESSAGE         = 0x00000002
	PIPE_WAIT                     = 0x00000000

	GENERIC_READ              = 0x80000000
	GENERIC_WRITE             = 0x40000000
	OPEN_EXISTING             = 3
	FILE_SHARE_READ           = 0x00000001
	SDDL_REVISION_1           = 1
	DACL_SECURITY_INFORMATION = 0x00000004

	WAIT_OBJECT_0 = 0x00000000
	WAIT_TIMEOUT  = 0x00000102
)

var (
	modadvapi32 = syscall.NewLazyDLL("advapi32.dll")
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modwtsapi32 = syscall.NewLazyDLL("wtsapi32.dll")
	modcrypt32  = syscall.NewLazyDLL("crypt32.dll")

	procStartServiceCtrlDispatcherW   = modadvapi32.NewProc("StartServiceCtrlDispatcherW")
	procRegisterServiceCtrlHandlerExW = modadvapi32.NewProc("RegisterServiceCtrlHandlerExW")
	procSetServiceStatus              = modadvapi32.NewProc("SetServiceStatus")
	procOpenProcessToken              = modadvapi32.NewProc("OpenProcessToken")
	procOpenThreadToken               = modadvapi32.NewProc("OpenThreadToken")
	procGetTokenInformation           = modadvapi32.NewProc("GetTokenInformation")
	procConvertSidToStringSidW        = modadvapi32.NewProc("ConvertSidToStringSidW")
	procConvertStringSidToSidW        = modadvapi32.NewProc("ConvertStringSidToSidW")
	procConvertStringSDToSDW          = modadvapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	procSetFileSecurityW              = modadvapi32.NewProc("SetFileSecurityW")
	procLookupPrivilegeNameW          = modadvapi32.NewProc("LookupPrivilegeNameW")
	procLookupAccountNameW            = modadvapi32.NewProc("LookupAccountNameW")
	procIsValidSid                    = modadvapi32.NewProc("IsValidSid")
	procEqualSid                      = modadvapi32.NewProc("EqualSid")
	procImpersonateNamedPipeClient    = modadvapi32.NewProc("ImpersonateNamedPipeClient")
	procRevertToSelf                  = modadvapi32.NewProc("RevertToSelf")

	procGetCurrentProcess            = modkernel32.NewProc("GetCurrentProcess")
	procGetCurrentProcessId          = modkernel32.NewProc("GetCurrentProcessId")
	procGetCurrentThread             = modkernel32.NewProc("GetCurrentThread")
	procProcessIdToSessionId         = modkernel32.NewProc("ProcessIdToSessionId")
	procWTSGetActiveConsoleSessionId = modkernel32.NewProc("WTSGetActiveConsoleSessionId")
	procGetNamedPipeClientSessionId  = modkernel32.NewProc("GetNamedPipeClientSessionId")

	procWTSEnumerateSessionsW       = modwtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInformationW = modwtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory               = modwtsapi32.NewProc("WTSFreeMemory")

	procLocalFree = modkernel32.NewProc("LocalFree")

	procCreateNamedPipeW    = modkernel32.NewProc("CreateNamedPipeW")
	procWaitNamedPipeW      = modkernel32.NewProc("WaitNamedPipeW")
	procConnectNamedPipe    = modkernel32.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe = modkernel32.NewProc("DisconnectNamedPipe")
	procCreateFileW         = modkernel32.NewProc("CreateFileW")
	procReadFile            = modkernel32.NewProc("ReadFile")
	procWriteFile           = modkernel32.NewProc("WriteFile")
	procCloseHandle         = modkernel32.NewProc("CloseHandle")
	procCreateEventW           = modkernel32.NewProc("CreateEventW")
	procSetEvent               = modkernel32.NewProc("SetEvent")
	procResetEvent             = modkernel32.NewProc("ResetEvent")
	procWaitForSingleObject    = modkernel32.NewProc("WaitForSingleObject")
	procWaitForMultipleObjects = modkernel32.NewProc("WaitForMultipleObjects")
	procGetOverlappedResult    = modkernel32.NewProc("GetOverlappedResult")
	procCancelIoEx             = modkernel32.NewProc("CancelIoEx")

	procCryptProtectData   = modcrypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = modcrypt32.NewProc("CryptUnprotectData")

	hServiceStatus     uintptr
	hGlobalCancelEvent uintptr
	stopChan           = make(chan struct{}, 1)
	workerDoneChan     = make(chan struct{}, 1)
)

type SERVICE_TABLE_ENTRY struct {
	ServiceName *uint16
	ServiceProc uintptr
}

type SERVICE_STATUS struct {
	ServiceType             uint32
	CurrentState            uint32
	ControlsAccepted        uint32
	Win32ExitCode           uint32
	ServiceSpecificExitCode uint32
	CheckPoint              uint32
	WaitHint                uint32
}

type OVERLAPPED struct {
	Internal     uintptr
	InternalHigh uintptr
	Offset       uint32
	OffsetHigh   uint32
	HEvent       syscall.Handle
}

type SECURITY_ATTRIBUTES struct {
	NLength              uint32
	_                    uint32 // padding for 8-byte alignment on x64
	LpSecurityDescriptor uintptr
	BInheritHandle       int32
	_                    uint32 // alignment padding to 8-byte multiple
}

type SID_AND_ATTRIBUTES struct {
	Sid        uintptr // PSID (8 bytes on x64)
	Attributes uint32
	_          uint32 // Alignment padding to 8 bytes on x64
}

type TOKEN_USER struct {
	User SID_AND_ATTRIBUTES
}

type TOKEN_MANDATORY_LABEL struct {
	Label SID_AND_ATTRIBUTES
}

type TOKEN_ELEVATION struct {
	TokenIsElevated uint32
}

type SessionInfo struct {
	SessionID       uint32 `json:"session_id"`
	WinStationName  string `json:"winstation_name"`
	ConnectState    int32  `json:"connect_state"`
	UserName        string `json:"user_name,omitempty"`
	DomainName      string `json:"domain_name,omitempty"`
	QueryUserStatus string `json:"query_user_status"`
	ResolvedSID     string `json:"resolved_sid,omitempty"`
}

type PrivilegeInfo struct {
	Name       string `json:"name"`
	Attributes uint32 `json:"attributes"`
	Enabled    bool   `json:"enabled"`
}

type HTTPSDiagnostics struct {
	TargetURL             string   `json:"target_url"`
	DNSSuccess            bool     `json:"dns_success"`
	ResolvedIPs           []string `json:"resolved_ips,omitempty"`
	DNSError              string   `json:"dns_error,omitempty"`
	TCPConnectSuccess     bool     `json:"tcp_connect_success"`
	TCPError              string   `json:"tcp_error,omitempty"`
	TLSHandshakeSuccess   bool     `json:"tls_handshake_success"`
	NegotiatedTLSVersion  string   `json:"negotiated_tls_version,omitempty"`
	NegotiatedCipherSuite string   `json:"negotiated_cipher_suite,omitempty"`
	TLSError              string   `json:"tls_error,omitempty"`
	HTTPResponseReceived  bool     `json:"http_response_received"`
	HTTPStatusCode        int      `json:"http_status_code,omitempty"`
	HTTPStatusText        string   `json:"http_status_text,omitempty"`
	HTTPError             string   `json:"http_error,omitempty"`
	BackendHealthy        bool     `json:"backend_healthy"`
	CloudflareOriginDown  bool     `json:"cloudflare_origin_down"`
}

type ProbeResult struct {
	Timestamp                string           `json:"timestamp"`
	ExecutionMode            string           `json:"execution_mode"`
	Architecture             string           `json:"architecture"`
	ProcessID                uint32           `json:"process_id"`
	TokenUserSID             string           `json:"token_user_sid"`
	TokenUserName            string           `json:"token_user_name"`
	IsLocalService           bool             `json:"is_local_service"`
	ServiceSIDFound          string           `json:"service_sid_found"`
	Privileges               []PrivilegeInfo  `json:"privileges"`
	SeImpersonateStripped    bool             `json:"se_impersonate_stripped"`
	ActiveConsoleSessionID   uint32           `json:"active_console_session_id"`
	IsConsoleDetached        bool             `json:"is_console_detached"`
	Sessions                 []SessionInfo    `json:"sessions"`
	WTSQueryConsoleSuccess   bool             `json:"wts_query_console_success"`
	WTSQueryLastError        uint32           `json:"wts_query_last_error"`
	DPAPIMachineScopeWorks   bool             `json:"dpapi_machine_scope_works"`
	OutboundHTTPSSuccess     bool             `json:"outbound_https_success"`
	OutboundHTTPSStatusCode  int              `json:"outbound_https_status_code"`
	HTTPSDetails             HTTPSDiagnostics `json:"https_details"`
	NamedPipeSessionAPIAvail bool             `json:"named_pipe_session_api_avail"`

	// Named Pipe Impersonation & SQOS Validation
	NamedPipeAuthTestSuccess    bool   `json:"named_pipe_auth_test_success"`
	ElevatedClientAuthSuccess   bool   `json:"elevated_client_auth_success"`
	UnelevatedClientAuthSuccess bool   `json:"unelevated_client_auth_success"`
	NamedPipeClientSQOSUsed     string `json:"named_pipe_client_sqos_used"`
	NamedPipeClientTokenUserSID string `json:"named_pipe_client_token_user_sid"`
	NamedPipeClientImpLevel     uint32 `json:"named_pipe_client_imp_level"`
	NamedPipeClientSIDValid     bool   `json:"named_pipe_client_sid_valid"`
	NamedPipeBinarySIDMatched   bool   `json:"named_pipe_binary_sid_matched"`
	NamedPipeRevertToSelfWorks  bool   `json:"named_pipe_revert_to_self_works"`
	NamedPipeReadTimeoutHandled bool   `json:"named_pipe_read_timeout_handled"`

	// Filesystem Service SID Isolation
	ServiceSIDPositiveAccess bool `json:"service_sid_positive_access"`

	Notes []string `json:"notes"`
}

func getCurrentProcessHandle() uintptr {
	h, _, _ := procGetCurrentProcess.Call()
	if h == 0 {
		return ^uintptr(0) // Win32 (HANDLE)-1 fallback
	}
	return h
}

func getCurrentThreadHandle() uintptr {
	h, _, _ := procGetCurrentThread.Call()
	if h == 0 {
		return ^uintptr(1) // Win32 (HANDLE)-2 fallback
	}
	return h
}

func ptrToUTF16String(p *uint16) string {
	if p == nil {
		return ""
	}
	s := unsafe.Slice(p, 1024)
	for i, c := range s {
		if c == 0 {
			return syscall.UTF16ToString(s[:i])
		}
	}
	return syscall.UTF16ToString(s)
}

func setStatusWithCode(currentState uint32, controlsAccepted uint32, checkPoint uint32, waitHint uint32, exitCode uint32) {
	if hServiceStatus == 0 {
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
	procSetServiceStatus.Call(hServiceStatus, uintptr(unsafe.Pointer(&st)))
}

func setStatus(currentState uint32, controlsAccepted uint32, checkPoint uint32, waitHint uint32) {
	setStatusWithCode(currentState, controlsAccepted, checkPoint, waitHint, 0)
}

func serviceHandlerExCallback(control uintptr, eventType uintptr, eventData uintptr, context uintptr) uintptr {
	switch uint32(control) {
	case SERVICE_CONTROL_STOP, SERVICE_CONTROL_SHUTDOWN:
		select {
		case stopChan <- struct{}{}:
		default:
		}
		return 0
	case SERVICE_CONTROL_INTERROGATE:
		return 0
	default:
		return 0x00000078 // ERROR_CALL_NOT_IMPLEMENTED
	}
}

func serviceMainCallback(argc uintptr, argv uintptr) uintptr {
	svcName, _ := syscall.UTF16PtrFromString("OnuslyValidationProbeService")
	cbHandler := syscall.NewCallback(serviceHandlerExCallback)
	h, _, _ := procRegisterServiceCtrlHandlerExW.Call(uintptr(unsafe.Pointer(svcName)), cbHandler, 0)
	hServiceStatus = h

	// 1. Report SERVICE_START_PENDING promptly with checkpoint 1, wait hint 5000ms
	setStatus(SERVICE_START_PENDING, 0, 1, 5000)

	// 2. Report SERVICE_RUNNING promptly so SCM and Start-Service return immediately (< 50ms)
	setStatus(SERVICE_RUNNING, SERVICE_ACCEPT_STOP|SERVICE_ACCEPT_SHUTDOWN, 0, 0)

	cancelChan := make(chan struct{})
	hEvt, _, _ := procCreateEventW.Call(0, 1, 0, 0) // manual-reset event, non-signaled
	hGlobalCancelEvent = hEvt
	defer func() {
		if hGlobalCancelEvent != 0 {
			procCloseHandle.Call(hGlobalCancelEvent)
			hGlobalCancelEvent = 0
		}
	}()

	// 3. Execute diagnostics asynchronously in background worker
	go runProbeWorker("SCM_SERVICE", cancelChan)

	// 4. Wait for stop control signal
	<-stopChan

	// 5. Report SERVICE_STOP_PENDING
	setStatusWithCode(SERVICE_STOP_PENDING, 0, 1, 5000, 0)

	// Signal cancellation to worker & set Win32 cancel event for overlapped I/O
	close(cancelChan)
	if hGlobalCancelEvent != 0 {
		procSetEvent.Call(hGlobalCancelEvent)
	}

	// Bounded wait for worker to complete cleanup (max 3500ms)
	workerExitedCleanly := false
	select {
	case <-workerDoneChan:
		workerExitedCleanly = true
	case <-time.After(3500 * time.Millisecond):
		workerExitedCleanly = false
	}

	// Synchronize with result writer: wait up to 1000ms for any in-flight write to complete,
	// and permanently close writer before reporting SERVICE_STOPPED.
	writerClosedCleanly := globalResultWriter.CloseWithDeadline(1000 * time.Millisecond)

	// 6. Record shutdown status and report SERVICE_STOPPED to SCM
	dataDir := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe")

	type ShutdownStatus struct {
		Timestamp           string `json:"timestamp"`
		CleanShutdown       bool   `json:"clean_shutdown"`
		WorkerTimedOut      bool   `json:"worker_timed_out"`
		WriterTimedOut      bool   `json:"writer_timed_out"`
		AllWorkersExited    bool   `json:"all_workers_exited"`
		ResultWritesBlocked bool   `json:"result_writes_blocked"`
		Win32ExitCode       uint32 `json:"win32_exit_code"`
		Notes               string `json:"notes"`
	}

	cleanShutdown := workerExitedCleanly && writerClosedCleanly
	var exitCode uint32 = 0
	var notes string

	if cleanShutdown {
		exitCode = 0
		notes = "Service shutdown cleanly: worker exited within 3.5s and result writer closed cleanly."
	} else if !workerExitedCleanly && !writerClosedCleanly {
		exitCode = 1053 // ERROR_SERVICE_REQUEST_TIMEOUT
		notes = "SHUTDOWN TIMEOUT: Both worker goroutine and result writer timed out during service shutdown."
	} else if !workerExitedCleanly {
		exitCode = 1053
		notes = "WORKER SHUTDOWN TIMEOUT: Worker goroutine did not terminate within 3.5s deadline. Service stopped with failure exit code 1053."
	} else {
		exitCode = 1053
		notes = "WRITER SHUTDOWN TIMEOUT: In-flight result write did not complete within 1s deadline. Service stopped with failure exit code 1053."
	}

	stLog := ShutdownStatus{
		Timestamp:           time.Now().UTC().Format(time.RFC3339),
		CleanShutdown:       cleanShutdown,
		WorkerTimedOut:      !workerExitedCleanly,
		WriterTimedOut:      !writerClosedCleanly,
		AllWorkersExited:    workerExitedCleanly,
		ResultWritesBlocked: true,
		Win32ExitCode:       exitCode,
		Notes:               notes,
	}

	globalResultWriter.WriteShutdownStatus(dataDir, stLog)
	setStatusWithCode(SERVICE_STOPPED, 0, 0, 0, exitCode)
	return 0
}

func negServiceMainCallback(argc uintptr, argv uintptr) uintptr {
	svcName, _ := syscall.UTF16PtrFromString("OnuslyGenericLSTestService")
	cbHandler := syscall.NewCallback(serviceHandlerExCallback)
	h, _, _ := procRegisterServiceCtrlHandlerExW.Call(uintptr(unsafe.Pointer(svcName)), cbHandler, 0)
	hServiceStatus = h

	setStatus(SERVICE_RUNNING, SERVICE_ACCEPT_STOP|SERVICE_ACCEPT_SHUTDOWN, 0, 0)
	runNegativeAccessTest()
	setStatus(SERVICE_STOPPED, 0, 0, 0)
	return 0
}

func runProbeWorker(mode string, cancelChan <-chan struct{}) {
	defer func() {
		select {
		case workerDoneChan <- struct{}{}:
		default:
		}
	}()

	result := runProbeTests(mode, cancelChan)

	// Check if cancelled before writing result
	select {
	case <-cancelChan:
		return
	default:
		writeProbeResults(result)
	}
}

func main() {
	// Explicit 64-bit architecture validation
	if unsafe.Sizeof(uintptr(0)) != 8 {
		fmt.Println("FATAL: ONUSLY Windows probe strictly targets 64-bit Windows (x64). 32-bit execution is rejected.")
		os.Exit(1)
	}

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--client", "--client-elevated":
			runDiagnosticClient("ELEVATED_ADMIN", false)
			return
		case "--client-unelevated":
			runDiagnosticClient("UNELEVATED_CONSOLE_USER", false)
			return
		case "--client-mismatch":
			runDiagnosticClient("MISMATCH_TEST", true)
			return
		case "--test-negative-access":
			cbNegMain := syscall.NewCallback(negServiceMainCallback)
			negSvcName, _ := syscall.UTF16PtrFromString("OnuslyGenericLSTestService")
			table := [2]SERVICE_TABLE_ENTRY{
				{ServiceName: negSvcName, ServiceProc: cbNegMain},
				{ServiceName: nil, ServiceProc: 0},
			}
			r1, _, err := syscall.Syscall(procStartServiceCtrlDispatcherW.Addr(), 1, uintptr(unsafe.Pointer(&table[0])), 0, 0)
			if r1 == 0 && err == 1063 { // ERROR_FAILED_SERVICE_CONTROLLER_CONNECT
				runNegativeAccessTest()
			}
			return
		}
	}

	svcName, _ := syscall.UTF16PtrFromString("OnuslyValidationProbeService")
	cbServiceMain := syscall.NewCallback(serviceMainCallback)
	table := [2]SERVICE_TABLE_ENTRY{
		{ServiceName: svcName, ServiceProc: cbServiceMain},
		{ServiceName: nil, ServiceProc: 0},
	}

	r1, _, err := syscall.Syscall(procStartServiceCtrlDispatcherW.Addr(), 1, uintptr(unsafe.Pointer(&table[0])), 0, 0)
	if r1 == 0 {
		// Win32 Error 1063: ERROR_FAILED_SERVICE_CONTROLLER_CONNECT
		// Executed from interactive console! Run standalone test mode.
		if err == 1063 {
			res := runProbeTests("STANDALONE_CONSOLE", nil)
			writeProbeResults(res)
			return
		}
		fmt.Printf("StartServiceCtrlDispatcherW failed: %v\n", err)
	}
}

func runProbeTests(mode string, cancelChan <-chan struct{}) ProbeResult {
	result := ProbeResult{
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
		ExecutionMode: mode,
		Architecture:  "x64 (AMD64 / 64-bit Windows)",
		ProcessID:     uint32(os.Getpid()),
		Notes:         []string{},
	}

	inspectProcessToken(&result)
	inspectSessions(&result)
	testNamedPipeAPI(&result)
	testNamedPipeReadTimeout(&result)
	testNamedPipeServerAuth(&result, cancelChan)
	testServiceSIDPositiveAccess(&result)
	testDPAPI(&result)
	testOutboundHTTPS(&result, cancelChan)

	return result
}

func writeProbeResults(result ProbeResult) {
	dataDir := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe")
	if !globalResultWriter.WriteResults(dataDir, result) {
		// Shutdown completed; reject late writes
		return
	}

	outBytes, err := json.MarshalIndent(result, "", "  ")
	if err == nil {
		fmt.Println(string(outBytes))
	}
	fmt.Printf("\n[Probe Complete] Report atomically saved to %s\n", filepath.Join(dataDir, "probe_results.json"))
}

func inspectProcessToken(r *ProbeResult) {
	var hToken syscall.Handle
	r1, _, _ := procOpenProcessToken.Call(
		getCurrentProcessHandle(),
		uintptr(0x0008|0x0020), // TOKEN_QUERY | TOKEN_QUERY_SOURCE
		uintptr(unsafe.Pointer(&hToken)),
	)
	if r1 == 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("OpenProcessToken failed: %v", syscall.GetLastError()))
		return
	}
	defer syscall.CloseHandle(hToken)

	// TokenUser (1)
	var reqLen uint32
	procGetTokenInformation.Call(uintptr(hToken), 1, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
	if reqLen > 0 {
		buf := make([]byte, reqLen)
		r1, _, _ := procGetTokenInformation.Call(uintptr(hToken), 1, uintptr(unsafe.Pointer(&buf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
		if r1 != 0 {
			tu := (*TOKEN_USER)(unsafe.Pointer(&buf[0]))
			rVal, _, _ := procIsValidSid.Call(tu.User.Sid)
			if rVal != 0 {
				var strSid *uint16
				procConvertSidToStringSidW.Call(tu.User.Sid, uintptr(unsafe.Pointer(&strSid)))
				if strSid != nil {
					r.TokenUserSID = ptrToUTF16String(strSid)
					procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
				}
				if r.TokenUserSID == "S-1-5-19" {
					r.IsLocalService = true
					r.TokenUserName = "NT AUTHORITY\\LocalService"
				}
			} else {
				r.Notes = append(r.Notes, "Process TokenUser SID failed IsValidSid check")
			}
		}
	}

	// TokenGroups (2) to check for Service SID (S-1-5-80-...)
	reqLen = 0
	procGetTokenInformation.Call(uintptr(hToken), 2, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
	if reqLen > 0 {
		buf := make([]byte, reqLen)
		r1, _, _ := procGetTokenInformation.Call(uintptr(hToken), 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
		if r1 != 0 {
			count := *(*uint32)(unsafe.Pointer(&buf[0]))
			offset := uintptr(8) // GroupCount is 4 bytes, 4 bytes padding on x64 due to 8-byte PSID pointer alignment
			if uintptr(len(buf)) >= offset+uintptr(count)*16 {
				groups := unsafe.Slice((*SID_AND_ATTRIBUTES)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0]))+offset)), count)
				for i := uint32(0); i < count && i < 1024; i++ {
					sa := &groups[i]
					var strSid *uint16
					procConvertSidToStringSidW.Call(sa.Sid, uintptr(unsafe.Pointer(&strSid)))
					if strSid != nil {
						s := ptrToUTF16String(strSid)
						procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
						if len(s) > 9 && s[:9] == "S-1-5-80-" {
							r.ServiceSIDFound = s
						}
					}
				}
			} else {
				r.Notes = append(r.Notes, fmt.Sprintf("TokenGroups buffer bounds check failed: len=%d, required=%d", len(buf), offset+uintptr(count)*16))
			}
		}
	}

	// TokenPrivileges (3)
	reqLen = 0
	procGetTokenInformation.Call(uintptr(hToken), 3, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
	if reqLen > 0 {
		buf := make([]byte, reqLen)
		r1, _, _ := procGetTokenInformation.Call(uintptr(hToken), 3, uintptr(unsafe.Pointer(&buf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
		if r1 != 0 {
			count := *(*uint32)(unsafe.Pointer(&buf[0]))
			type LUID_AND_ATTRIBUTES struct {
				Luid struct {
					LowPart  uint32
					HighPart int32
				}
				Attributes uint32
			}
			// PrivilegeCount is 4 bytes; LUID_AND_ATTRIBUTES has 4-byte struct alignment, no padding on x64!
			offset := uintptr(4)
			if uintptr(len(buf)) >= offset+uintptr(count)*12 {
				privs := unsafe.Slice((*LUID_AND_ATTRIBUTES)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0]))+offset)), count)
				hasImpersonate := false
				for i := uint32(0); i < count && i < 1024; i++ {
					la := &privs[i]
					var nameBuf [256]uint16
					nameLen := uint32(len(nameBuf))
					procLookupPrivilegeNameW.Call(0, uintptr(unsafe.Pointer(&la.Luid)), uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(unsafe.Pointer(&nameLen)))
					pName := syscall.UTF16ToString(nameBuf[:nameLen])
					pInfo := PrivilegeInfo{
						Name:       pName,
						Attributes: la.Attributes,
						Enabled:    (la.Attributes & 0x00000002) != 0, // SE_PRIVILEGE_ENABLED
					}
					r.Privileges = append(r.Privileges, pInfo)
					if pName == "SeImpersonatePrivilege" {
						hasImpersonate = true
					}
				}
				r.SeImpersonateStripped = !hasImpersonate
			} else {
				r.Notes = append(r.Notes, fmt.Sprintf("TokenPrivileges buffer bounds check failed: len=%d, required=%d", len(buf), offset+uintptr(count)*12))
			}
		}
	}
}

func inspectSessions(r *ProbeResult) {
	r1, _, _ := procWTSGetActiveConsoleSessionId.Call()
	r.ActiveConsoleSessionID = uint32(r1)
	if r.ActiveConsoleSessionID == 0xFFFFFFFF {
		r.IsConsoleDetached = true
	}

	type WTS_SESSION_INFOW struct {
		SessionId       uint32
		pWinStationName *uint16
		State           int32
	}
	var pSessions *WTS_SESSION_INFOW
	var count uint32
	r1, _, err := procWTSEnumerateSessionsW.Call(0, 0, 1, uintptr(unsafe.Pointer(&pSessions)), uintptr(unsafe.Pointer(&count)))
	if r1 == 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("WTSEnumerateSessionsW failed: %v", err))
		return
	}
	defer procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pSessions)))

	sessionList := unsafe.Slice(pSessions, count)
	for i := uint32(0); i < count; i++ {
		s := &sessionList[i]
		info := SessionInfo{
			SessionID:      s.SessionId,
			ConnectState:   s.State,
			WinStationName: ptrToUTF16String(s.pWinStationName),
		}

		var pUser *uint16
		var bytesReturned uint32
		rQ, _, errQ := procWTSQuerySessionInformationW.Call(0, uintptr(s.SessionId), 5, uintptr(unsafe.Pointer(&pUser)), uintptr(unsafe.Pointer(&bytesReturned)))
		if rQ != 0 && pUser != nil {
			info.UserName = ptrToUTF16String(pUser)
			info.QueryUserStatus = "SUCCESS"
			procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pUser)))
			if s.SessionId == r.ActiveConsoleSessionID {
				r.WTSQueryConsoleSuccess = true
			}
		} else {
			info.QueryUserStatus = fmt.Sprintf("FAILED (error %v)", errQ)
			if s.SessionId == r.ActiveConsoleSessionID {
				r.WTSQueryConsoleSuccess = false
			}
		}

		var pDomain *uint16
		rD, _, _ := procWTSQuerySessionInformationW.Call(0, uintptr(s.SessionId), 7, uintptr(unsafe.Pointer(&pDomain)), uintptr(unsafe.Pointer(&bytesReturned)))
		if rD != 0 && pDomain != nil {
			info.DomainName = ptrToUTF16String(pDomain)
			procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pDomain)))
		}

		if info.UserName != "" {
			fullAccount := info.UserName
			if info.DomainName != "" {
				fullAccount = info.DomainName + "\\" + info.UserName
			}
			pAcc, _ := syscall.UTF16PtrFromString(fullAccount)
			var sidBuf [256]byte
			sidLen := uint32(len(sidBuf))
			var domBuf [256]uint16
			domLen := uint32(len(domBuf))
			var sidUse uint32
			rL, _, _ := procLookupAccountNameW.Call(0, uintptr(unsafe.Pointer(pAcc)), uintptr(unsafe.Pointer(&sidBuf[0])), uintptr(unsafe.Pointer(&sidLen)), uintptr(unsafe.Pointer(&domBuf[0])), uintptr(unsafe.Pointer(&domLen)), uintptr(unsafe.Pointer(&sidUse)))
			if rL != 0 {
				var strSid *uint16
				procConvertSidToStringSidW.Call(uintptr(unsafe.Pointer(&sidBuf[0])), uintptr(unsafe.Pointer(&strSid)))
				if strSid != nil {
					info.ResolvedSID = ptrToUTF16String(strSid)
					procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
				}
			}
		}

		r.Sessions = append(r.Sessions, info)
	}
}

func testDPAPI(r *ProbeResult) {
	type DATA_BLOB struct {
		cbData uint32
		pbData *byte
	}
	plaintext := []byte("ONUSLY_DPAPI_PROBE_SECRET")
	inBlob := DATA_BLOB{
		cbData: uint32(len(plaintext)),
		pbData: &plaintext[0],
	}
	var outBlob DATA_BLOB
	// CRYPTPROTECT_LOCAL_MACHINE = 0x4
	r1, _, _ := procCryptProtectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, 0, 0, 0,
		0x4,
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r1 == 0 {
		r.DPAPIMachineScopeWorks = false
		r.Notes = append(r.Notes, fmt.Sprintf("CryptProtectData failed: %v", syscall.GetLastError()))
		return
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	// Unprotect
	var unprotectBlob DATA_BLOB
	r2, _, _ := procCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&outBlob)),
		0, 0, 0, 0,
		0,
		uintptr(unsafe.Pointer(&unprotectBlob)),
	)
	if r2 != 0 {
		defer procLocalFree.Call(uintptr(unsafe.Pointer(unprotectBlob.pbData)))
		r.DPAPIMachineScopeWorks = true
	} else {
		r.DPAPIMachineScopeWorks = false
	}
}

func testOutboundHTTPS(r *ProbeResult, cancelChan <-chan struct{}) {
	targetURL := "https://api.onusly.com/health"
	diag := HTTPSDiagnostics{
		TargetURL: targetURL,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	if cancelChan != nil {
		go func() {
			select {
			case <-cancelChan:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	u, err := url.Parse(targetURL)
	if err != nil {
		diag.DNSError = err.Error()
		r.HTTPSDetails = diag
		return
	}

	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "443"
	}

	// 1. DNS Resolution
	ips, errDNS := net.DefaultResolver.LookupIP(ctx, "ip4", host)
	if errDNS != nil {
		diag.DNSSuccess = false
		diag.DNSError = errDNS.Error()
		r.HTTPSDetails = diag
		r.Notes = append(r.Notes, fmt.Sprintf("DNS resolution failed for %s: %v", host, errDNS))
		return
	}
	diag.DNSSuccess = true
	for _, ip := range ips {
		diag.ResolvedIPs = append(diag.ResolvedIPs, ip.String())
	}

	// 2. TCP Connectivity
	dialer := net.Dialer{Timeout: 3 * time.Second}
	targetAddr := net.JoinHostPort(diag.ResolvedIPs[0], port)
	rawConn, errTCP := dialer.DialContext(ctx, "tcp", targetAddr)
	if errTCP != nil {
		diag.TCPConnectSuccess = false
		diag.TCPError = errTCP.Error()
		r.HTTPSDetails = diag
		r.Notes = append(r.Notes, fmt.Sprintf("TCP connect failed to %s: %v", targetAddr, errTCP))
		return
	}
	defer rawConn.Close()
	diag.TCPConnectSuccess = true

	// 3. TLS Handshake & Negotiated Version Inspection
	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	tlsConn := tls.Client(rawConn, tlsConfig)
	errTLS := tlsConn.HandshakeContext(ctx)
	if errTLS != nil {
		diag.TLSHandshakeSuccess = false
		diag.TLSError = errTLS.Error()
		r.HTTPSDetails = diag
		r.Notes = append(r.Notes, fmt.Sprintf("TLS handshake failed with %s: %v", host, errTLS))
		return
	}
	diag.TLSHandshakeSuccess = true

	cs := tlsConn.ConnectionState()
	switch cs.Version {
	case tls.VersionTLS13:
		diag.NegotiatedTLSVersion = "TLS 1.3"
	case tls.VersionTLS12:
		diag.NegotiatedTLSVersion = "TLS 1.2"
	default:
		diag.NegotiatedTLSVersion = fmt.Sprintf("0x%04x", cs.Version)
	}
	diag.NegotiatedCipherSuite = tls.CipherSuiteName(cs.CipherSuite)

	// 4. HTTP Request & Response over established TLS connection
	req, errReq := http.NewRequestWithContext(ctx, "GET", u.Path, nil)
	if errReq != nil {
		diag.HTTPError = errReq.Error()
		r.HTTPSDetails = diag
		return
	}
	req.Host = host
	req.Header.Set("User-Agent", "ONUSLY-Windows-Validation-Probe/1.0")
	req.Header.Set("Accept", "application/json")

	if errWrite := req.Write(tlsConn); errWrite != nil {
		diag.HTTPError = errWrite.Error()
		r.HTTPSDetails = diag
		return
	}

	resp, errResp := http.ReadResponse(bufio.NewReader(tlsConn), req)
	if errResp != nil {
		diag.HTTPError = errResp.Error()
		r.HTTPSDetails = diag
		return
	}
	defer resp.Body.Close()

	diag.HTTPResponseReceived = true
	diag.HTTPStatusCode = resp.StatusCode
	diag.HTTPStatusText = resp.Status

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	respBody := string(bodyBytes)

	// Distinguish backend health from Cloudflare edge response:
	// HTTP 521: Web Server Is Down (Cloudflare reached, origin server not answering)
	// HTTP 522/523: Origin Connection Timed Out / Unreachable
	if resp.StatusCode == 200 {
		diag.BackendHealthy = true
	} else if resp.StatusCode == 521 || resp.StatusCode == 522 || resp.StatusCode == 523 {
		diag.BackendHealthy = false
		diag.CloudflareOriginDown = true
		r.Notes = append(r.Notes, fmt.Sprintf("Cloudflare edge reached over %s (%s), but origin returned HTTP %d (Backend origin server down)", diag.NegotiatedTLSVersion, diag.NegotiatedCipherSuite, resp.StatusCode))
	} else {
		diag.BackendHealthy = false
		r.Notes = append(r.Notes, fmt.Sprintf("HTTP response received (%s), body: %s", resp.Status, respBody))
	}

	r.OutboundHTTPSSuccess = diag.TLSHandshakeSuccess && diag.HTTPResponseReceived
	r.OutboundHTTPSStatusCode = diag.HTTPStatusCode
	r.HTTPSDetails = diag
}

func testNamedPipeAPI(r *ProbeResult) {
	if err := procGetNamedPipeClientSessionId.Find(); err == nil {
		r.NamedPipeSessionAPIAvail = true
	} else {
		r.NamedPipeSessionAPIAvail = false
		r.Notes = append(r.Notes, "GetNamedPipeClientSessionId proc not found in kernel32")
	}
}

func testNamedPipeReadTimeout(r *ProbeResult) {
	pipeName := fmt.Sprintf(`\\.\pipe\OnuslyProbeTimeoutPipe_%d`, os.Getpid())
	pipeNamePtr, _ := syscall.UTF16PtrFromString(pipeName)

	openMode := uint32(PIPE_ACCESS_DUPLEX | FILE_FLAG_FIRST_PIPE_INSTANCE | FILE_FLAG_OVERLAPPED)
	pipeMode := uint32(PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT)

	hServerPipe, _, _ := procCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(pipeNamePtr)),
		uintptr(openMode),
		uintptr(pipeMode),
		1,
		4096,
		4096,
		1000,
		0,
	)
	if hServerPipe == uintptr(syscall.InvalidHandle) || hServerPipe == 0 {
		return
	}
	defer procCloseHandle.Call(hServerPipe)

	clientDone := make(chan struct{})

	// Spawn client that connects but sends NO bytes, exercising server read timeout
	go func() {
		defer close(clientDone)
		time.Sleep(30 * time.Millisecond) // Let server enter ConnectNamedPipe

		flags := uint32(SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION)
		hClient, _, _ := procCreateFileW.Call(
			uintptr(unsafe.Pointer(pipeNamePtr)),
			uintptr(GENERIC_READ|GENERIC_WRITE),
			0,
			0,
			uintptr(OPEN_EXISTING),
			uintptr(flags),
			0,
		)
		if hClient != uintptr(syscall.InvalidHandle) && hClient != 0 {
			defer procCloseHandle.Call(hClient)
			time.Sleep(300 * time.Millisecond) // Hold connection open past 100ms server timeout
		}
	}()

	hConnEvent, _, _ := procCreateEventW.Call(0, 1, 0, 0)
	if hConnEvent == 0 {
		return
	}
	defer procCloseHandle.Call(hConnEvent)

	var ovConn OVERLAPPED
	ovConn.HEvent = syscall.Handle(hConnEvent)

	rConn, _, errConn := procConnectNamedPipe.Call(hServerPipe, uintptr(unsafe.Pointer(&ovConn)))
	if rConn == 0 {
		errno, ok := errConn.(syscall.Errno)
		if ok && errno == 997 { // ERROR_IO_PENDING
			procWaitForSingleObject.Call(hConnEvent, 2000)
			var bytesXfer uint32
			procGetOverlappedResult.Call(hServerPipe, uintptr(unsafe.Pointer(&ovConn)), uintptr(unsafe.Pointer(&bytesXfer)), 1)
		}
	}
	defer procDisconnectNamedPipe.Call(hServerPipe)

	// Issue overlapped ReadFile with 100ms deadline
	var readBuf [256]byte
	var bytesRead uint32
	var ovRead OVERLAPPED
	hReadEvent, _, _ := procCreateEventW.Call(0, 1, 0, 0)
	if hReadEvent == 0 {
		return
	}
	defer procCloseHandle.Call(hReadEvent)
	ovRead.HEvent = syscall.Handle(hReadEvent)

	rRead, _, errRead := procReadFile.Call(
		hServerPipe,
		uintptr(unsafe.Pointer(&readBuf[0])),
		uintptr(len(readBuf)),
		uintptr(unsafe.Pointer(&bytesRead)),
		uintptr(unsafe.Pointer(&ovRead)),
	)

	if rRead == 0 {
		errno, ok := errRead.(syscall.Errno)
		if ok && errno == 997 { // ERROR_IO_PENDING
			waitRes, _, _ := procWaitForSingleObject.Call(hReadEvent, 100)
			if waitRes == WAIT_TIMEOUT {
				// Cancel pending I/O
				procCancelIoEx.Call(hServerPipe, uintptr(unsafe.Pointer(&ovRead)))
				// Bounded wait for cancellation to drain from kernel before releasing ovRead
				var bytesCancelled uint32
				procGetOverlappedResult.Call(hServerPipe, uintptr(unsafe.Pointer(&ovRead)), uintptr(unsafe.Pointer(&bytesCancelled)), 1)
				r.NamedPipeReadTimeoutHandled = true
			}
		}
	}

	<-clientDone
}

func testNamedPipeServerAuth(r *ProbeResult, cancelChan <-chan struct{}) {
	pipeName := `\\.\pipe\OnuslyValidationProbePipe`
	pipeNamePtr, _ := syscall.UTF16PtrFromString(pipeName)

	openMode := uint32(PIPE_ACCESS_DUPLEX | FILE_FLAG_FIRST_PIPE_INSTANCE | FILE_FLAG_OVERLAPPED)
	pipeMode := uint32(PIPE_TYPE_MESSAGE | PIPE_READMODE_MESSAGE | PIPE_WAIT)

	// Clean and define server readiness marker files
	dataDir := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe")
	_ = os.MkdirAll(dataDir, 0755)
	markerC1 := filepath.Join(dataDir, "pipe_server_ready_c1.marker")
	markerC2 := filepath.Join(dataDir, "pipe_server_ready_c2.marker")
	_ = os.Remove(markerC1)
	_ = os.Remove(markerC2)
	defer os.Remove(markerC1)
	defer os.Remove(markerC2)

	// Construct explicit SDDL:
	// Grant Full Control (GA) to Service SID (if found), Administrators (BA), SYSTEM (SY), LocalService (LS)
	// Grant Read+Write (GRGW) to Interactive Users (IU)
	sddl := "D:(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;LS)(A;;GRGW;;;IU)"
	if r.ServiceSIDFound != "" {
		sddl = fmt.Sprintf("D:(A;;GA;;;%s)(A;;GA;;;BA)(A;;GA;;;SY)(A;;GA;;;LS)(A;;GRGW;;;IU)", r.ServiceSIDFound)
	}
	sddlPtr, _ := syscall.UTF16PtrFromString(sddl)

	var pSD uintptr
	rConv, _, errConv := procConvertStringSDToSDW.Call(
		uintptr(unsafe.Pointer(sddlPtr)),
		SDDL_REVISION_1,
		uintptr(unsafe.Pointer(&pSD)),
		0,
	)
	var pSA uintptr
	var sa SECURITY_ATTRIBUTES
	if rConv != 0 && pSD != 0 {
		defer procLocalFree.Call(pSD)
		sa.NLength = uint32(unsafe.Sizeof(sa))
		sa.LpSecurityDescriptor = pSD
		sa.BInheritHandle = 0
		pSA = uintptr(unsafe.Pointer(&sa))
	} else {
		r.Notes = append(r.Notes, fmt.Sprintf("ConvertStringSDToSDW for pipe failed: %v", errConv))
	}

	hServerPipe, _, errCreate := procCreateNamedPipeW.Call(
		uintptr(unsafe.Pointer(pipeNamePtr)),
		uintptr(openMode),
		uintptr(pipeMode),
		1,
		4096,
		4096,
		5000,
		pSA,
	)
	if hServerPipe == uintptr(syscall.InvalidHandle) || hServerPipe == 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("CreateNamedPipeW failed: %v", errCreate))
		return
	}
	defer procCloseHandle.Call(hServerPipe)

	// In standalone mode, spawn an in-process diagnostic client
	if r.ExecutionMode == "STANDALONE_CONSOLE" {
		go func() {
			time.Sleep(50 * time.Millisecond)
			runDiagnosticClient("STANDALONE_ELEVATED", false)
		}()
	}

	r.NamedPipeClientSQOSUsed = "SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION"

	// Helper to accept and process one client connection
	handleClient := func(timeoutMs uint32) (bool, string) {
		hEvent, _, _ := procCreateEventW.Call(0, 1, 0, 0)
		if hEvent == 0 {
			return false, ""
		}
		defer procCloseHandle.Call(hEvent)

		var ov OVERLAPPED
		ov.HEvent = syscall.Handle(hEvent)

		rConn, _, errConn := procConnectNamedPipe.Call(hServerPipe, uintptr(unsafe.Pointer(&ov)))
		if rConn == 0 {
			errno, ok := errConn.(syscall.Errno)
			if ok && errno == 535 { // ERROR_PIPE_CONNECTED
				// Connected immediately
			} else if ok && errno == 997 { // ERROR_IO_PENDING
				var waitHandles [2]uintptr
				waitHandles[0] = hEvent
				waitCount := uintptr(1)
				if hGlobalCancelEvent != 0 {
					waitHandles[1] = hGlobalCancelEvent
					waitCount = 2
				}

				waitRes, _, _ := procWaitForMultipleObjects.Call(
					waitCount,
					uintptr(unsafe.Pointer(&waitHandles[0])),
					0, // bWaitAll = FALSE
					uintptr(timeoutMs),
				)

				if waitRes == WAIT_OBJECT_0 {
					var bytesTransferred uint32
					procGetOverlappedResult.Call(hServerPipe, uintptr(unsafe.Pointer(&ov)), uintptr(unsafe.Pointer(&bytesTransferred)), 1)
				} else {
					procCancelIoEx.Call(hServerPipe, uintptr(unsafe.Pointer(&ov)))
					var bytesCancelled uint32
					// Wait for overlapped cancellation to complete before releasing ov memory from stack
					procGetOverlappedResult.Call(hServerPipe, uintptr(unsafe.Pointer(&ov)), uintptr(unsafe.Pointer(&bytesCancelled)), 1)
					return false, ""
				}
			} else {
				return false, ""
			}
		}

		defer procDisconnectNamedPipe.Call(hServerPipe)

		// Read bounded message (max 4096 bytes with 5s timeout)
		var readBuf [4096]byte
		var bytesRead uint32
		var ovRead OVERLAPPED
		hReadEvent, _, _ := procCreateEventW.Call(0, 1, 0, 0)
		defer procCloseHandle.Call(hReadEvent)
		ovRead.HEvent = syscall.Handle(hReadEvent)

		rRead, _, errRead := procReadFile.Call(
			hServerPipe,
			uintptr(unsafe.Pointer(&readBuf[0])),
			uintptr(len(readBuf)),
			uintptr(unsafe.Pointer(&bytesRead)),
			uintptr(unsafe.Pointer(&ovRead)),
		)
		if rRead == 0 {
			errno, ok := errRead.(syscall.Errno)
			if ok && errno == 997 { // ERROR_IO_PENDING
				var waitHandles [2]uintptr
				waitHandles[0] = hReadEvent
				waitCount := uintptr(1)
				if hGlobalCancelEvent != 0 {
					waitHandles[1] = hGlobalCancelEvent
					waitCount = 2
				}

				waitRes, _, _ := procWaitForMultipleObjects.Call(
					waitCount,
					uintptr(unsafe.Pointer(&waitHandles[0])),
					0, // bWaitAll = FALSE
					5000,
				)

				if waitRes == WAIT_OBJECT_0 {
					procGetOverlappedResult.Call(hServerPipe, uintptr(unsafe.Pointer(&ovRead)), uintptr(unsafe.Pointer(&bytesRead)), 1)
				} else {
					procCancelIoEx.Call(hServerPipe, uintptr(unsafe.Pointer(&ovRead)))
					var bytesCancelled uint32
					// Wait for overlapped cancellation to complete before releasing ovRead memory from stack
					procGetOverlappedResult.Call(hServerPipe, uintptr(unsafe.Pointer(&ovRead)), uintptr(unsafe.Pointer(&bytesCancelled)), 1)
					r.NamedPipeReadTimeoutHandled = true
					return false, ""
				}
			} else {
				return false, ""
			}
		}

		clientMsg := string(readBuf[:bytesRead])

		// ImpersonateNamedPipeClient
		rImp, _, errImp := procImpersonateNamedPipeClient.Call(hServerPipe)
		if rImp == 0 {
			r.Notes = append(r.Notes, fmt.Sprintf("ImpersonateNamedPipeClient failed: %v", errImp))
			resp := []byte("ONUSLY_AUTH_DENIED")
			var written uint32
			procWriteFile.Call(hServerPipe, uintptr(unsafe.Pointer(&resp[0])), uintptr(len(resp)), uintptr(unsafe.Pointer(&written)), 0)
			return false, clientMsg
		}

		// Guarantee RevertToSelf on return
		defer func() {
			rRev, _, _ := procRevertToSelf.Call()
			if rRev != 0 {
				r.NamedPipeRevertToSelfWorks = true
			}
		}()

		// Open thread token via GetCurrentThread pseudo-handle (HANDLE)-2
		hThread := getCurrentThreadHandle()
		var hThreadToken syscall.Handle
		rTok, _, errTok := procOpenThreadToken.Call(
			hThread,
			uintptr(0x0008), // TOKEN_QUERY
			1,               // OpenAsSelf = TRUE
			uintptr(unsafe.Pointer(&hThreadToken)),
		)
		if rTok == 0 {
			r.Notes = append(r.Notes, fmt.Sprintf("OpenThreadToken failed: %v", errTok))
			resp := []byte("ONUSLY_AUTH_DENIED")
			var written uint32
			procWriteFile.Call(hServerPipe, uintptr(unsafe.Pointer(&resp[0])), uintptr(len(resp)), uintptr(unsafe.Pointer(&written)), 0)
			return false, clientMsg
		}
		defer procCloseHandle.Call(uintptr(hThreadToken))

		// Query TokenImpersonationLevel (9)
		var impLevel uint32
		var retLen uint32
		procGetTokenInformation.Call(
			uintptr(hThreadToken),
			9,
			uintptr(unsafe.Pointer(&impLevel)),
			uintptr(unsafe.Sizeof(impLevel)),
			uintptr(unsafe.Pointer(&retLen)),
		)
		r.NamedPipeClientImpLevel = impLevel

		// Query TokenUser (1)
		sidValid := false
		binaryMatched := false
		retLen = 0
		procGetTokenInformation.Call(uintptr(hThreadToken), 1, 0, 0, uintptr(unsafe.Pointer(&retLen)))
		if retLen > 0 {
			uBuf := make([]byte, retLen)
			rU, _, _ := procGetTokenInformation.Call(
				uintptr(hThreadToken),
				1,
				uintptr(unsafe.Pointer(&uBuf[0])),
				uintptr(retLen),
				uintptr(unsafe.Pointer(&retLen)),
			)
			if rU != 0 {
				tu := (*TOKEN_USER)(unsafe.Pointer(&uBuf[0]))
				rValid, _, _ := procIsValidSid.Call(tu.User.Sid)
				if rValid != 0 {
					sidValid = true
					r.NamedPipeClientSIDValid = true

					var strSid *uint16
					procConvertSidToStringSidW.Call(tu.User.Sid, uintptr(unsafe.Pointer(&strSid)))
					if strSid != nil {
						r.NamedPipeClientTokenUserSID = ptrToUTF16String(strSid)
						procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
					}

					expectedSidPtr, _ := syscall.UTF16PtrFromString(r.NamedPipeClientTokenUserSID)
					var expectedBinarySid uintptr
					procConvertStringSidToSidW.Call(uintptr(unsafe.Pointer(expectedSidPtr)), uintptr(unsafe.Pointer(&expectedBinarySid)))
					if expectedBinarySid != 0 {
						rEq, _, _ := procEqualSid.Call(tu.User.Sid, expectedBinarySid)
						if rEq != 0 {
							binaryMatched = true
							r.NamedPipeBinarySIDMatched = true
						}
						procLocalFree.Call(expectedBinarySid)
					}
				}
			}
		}

		authSuccess := (impLevel == 1 && sidValid && binaryMatched)
		resp := []byte("ONUSLY_AUTH_DENIED")
		if authSuccess {
			resp = []byte("ONUSLY_AUTH_OK")
		}
		var written uint32
		procWriteFile.Call(hServerPipe, uintptr(unsafe.Pointer(&resp[0])), uintptr(len(resp)), uintptr(unsafe.Pointer(&written)), 0)

		return authSuccess, clientMsg
	}

	// 1. Process Client 1 (wait up to 10 seconds)
	_ = os.WriteFile(markerC1, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
	c1Success, c1Msg := handleClient(10000)
	_ = os.Remove(markerC1)
	if c1Success {
		r.NamedPipeAuthTestSuccess = true
		if c1Msg == "ONUSLY_ELEVATED_CLIENT_PING" {
			r.ElevatedClientAuthSuccess = true
		} else if c1Msg == "ONUSLY_UNELEVATED_CLIENT_PING" {
			r.UnelevatedClientAuthSuccess = true
		}
	}

	// 2. Process Client 2 if cancellation has not been signaled (wait up to 5 seconds)
	if r.ExecutionMode != "STANDALONE_CONSOLE" {
		select {
		case <-cancelChan:
			return
		default:
		}

		_ = os.WriteFile(markerC2, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
		c2Success, c2Msg := handleClient(5000)
		_ = os.Remove(markerC2)
		if c2Success {
			if c2Msg == "ONUSLY_UNELEVATED_CLIENT_PING" {
				r.UnelevatedClientAuthSuccess = true
			} else if c2Msg == "ONUSLY_ELEVATED_CLIENT_PING" {
				r.ElevatedClientAuthSuccess = true
			}
		} else if c2Msg == "" {
			r.Notes = append(r.Notes, "Second client did not connect within 5s window (optional dual-client test)")
		}
	}
}

func testServiceSIDPositiveAccess(r *ProbeResult) {
	if r.ServiceSIDFound == "" {
		return
	}

	dataDir := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe")
	_ = os.MkdirAll(dataDir, 0755)
	secureFile := filepath.Join(dataDir, "service_sid_protected.dat")

	// SDDL: Grant Full Control exclusively to probe's Service SID and Administrators (BA)
	// D:(A;;GA;;;<ServiceSID>)(A;;GA;;;BA)
	sddl := fmt.Sprintf("D:(A;;GA;;;%s)(A;;GA;;;BA)", r.ServiceSIDFound)
	sddlPtr, _ := syscall.UTF16PtrFromString(sddl)

	var pSD uintptr
	rConv, _, errConv := procConvertStringSDToSDW.Call(
		uintptr(unsafe.Pointer(sddlPtr)),
		SDDL_REVISION_1,
		uintptr(unsafe.Pointer(&pSD)),
		0,
	)
	if rConv == 0 || pSD == 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("ConvertStringSecurityDescriptorToSecurityDescriptorW failed: %v", errConv))
		return
	}
	defer procLocalFree.Call(pSD)

	filePathPtr, _ := syscall.UTF16PtrFromString(secureFile)

	// Write test file
	testData := []byte("PROTECTED_SERVICE_SID_DATA")
	if err := os.WriteFile(secureFile, testData, 0600); err != nil {
		r.Notes = append(r.Notes, fmt.Sprintf("Failed to write initial secure_test.dat: %v", err))
		return
	}

	// Apply DACL
	rSec, _, errSec := procSetFileSecurityW.Call(
		uintptr(unsafe.Pointer(filePathPtr)),
		DACL_SECURITY_INFORMATION,
		pSD,
	)
	if rSec == 0 {
		r.Notes = append(r.Notes, fmt.Sprintf("SetFileSecurityW failed: %v", errSec))
		return
	}

	// Positive Access Test: Read the file back from this process (which possesses the Service SID)
	data, errRead := os.ReadFile(secureFile)
	if errRead == nil && string(data) == string(testData) {
		r.ServiceSIDPositiveAccess = true
	} else {
		r.ServiceSIDPositiveAccess = false
		r.Notes = append(r.Notes, fmt.Sprintf("Service SID positive access test failed: %v", errRead))
	}
}

type ClientIdentityInfo struct {
	Timestamp                  string `json:"timestamp"`
	ClientMode                 string `json:"client_mode"`
	ProcessID                  uint32 `json:"process_id"`
	SessionID                  uint32 `json:"session_id"`
	ActiveConsoleSessionID     uint32 `json:"active_console_session_id"`
	IsPhysicalConsole          bool   `json:"is_physical_console"`
	TokenUserSID               string `json:"token_user_sid"`
	TokenUserName              string `json:"token_user_name,omitempty"`
	IsElevated                 bool   `json:"is_elevated"`
	IntegrityLevelName         string `json:"integrity_level_name"`
	IntegrityLevelRID          uint32 `json:"integrity_level_rid"`
	BelongsToPairedConsoleUser bool   `json:"belongs_to_paired_console_user"`
	PipeConnectSuccess         bool   `json:"pipe_connect_success"`
	PipeAttempts               int    `json:"pipe_attempts"`
	Win32ErrorCode             uint32 `json:"win32_error_code,omitempty"`
	ServerResponse             string `json:"server_response"`
}

// runDiagnosticClient connects as an interactive user to the named pipe using Win32 CreateFileW
// with SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION and records its exact token identity
func runDiagnosticClient(clientMode string, mismatchTest bool) {
	info := ClientIdentityInfo{
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		ClientMode: clientMode,
		ProcessID:  uint32(os.Getpid()),
	}

	// 1. Process Session ID
	var sessID uint32
	procProcessIdToSessionId.Call(uintptr(info.ProcessID), uintptr(unsafe.Pointer(&sessID)))
	info.SessionID = sessID

	rConsole, _, _ := procWTSGetActiveConsoleSessionId.Call()
	info.ActiveConsoleSessionID = uint32(rConsole)
	info.IsPhysicalConsole = (info.SessionID == info.ActiveConsoleSessionID && info.ActiveConsoleSessionID != 0xFFFFFFFF)

	// 2. Token User, Elevation, and Integrity Level
	var hToken syscall.Handle
	r1, _, _ := procOpenProcessToken.Call(
		getCurrentProcessHandle(),
		uintptr(0x0008), // TOKEN_QUERY
		uintptr(unsafe.Pointer(&hToken)),
	)
	if r1 != 0 {
		defer syscall.CloseHandle(hToken)

		// TokenUser (1)
		var reqLen uint32
		procGetTokenInformation.Call(uintptr(hToken), 1, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
		if reqLen > 0 {
			uBuf := make([]byte, reqLen)
			rU, _, _ := procGetTokenInformation.Call(uintptr(hToken), 1, uintptr(unsafe.Pointer(&uBuf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
			if rU != 0 {
				tu := (*TOKEN_USER)(unsafe.Pointer(&uBuf[0]))
				var strSid *uint16
				procConvertSidToStringSidW.Call(tu.User.Sid, uintptr(unsafe.Pointer(&strSid)))
				if strSid != nil {
					info.TokenUserSID = ptrToUTF16String(strSid)
					procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
				}
			}
		}

		// TokenElevation (20)
		var elev TOKEN_ELEVATION
		var elevLen uint32
		rE, _, _ := procGetTokenInformation.Call(
			uintptr(hToken),
			20, // TokenElevation
			uintptr(unsafe.Pointer(&elev)),
			uintptr(unsafe.Sizeof(elev)),
			uintptr(unsafe.Pointer(&elevLen)),
		)
		if rE != 0 {
			info.IsElevated = (elev.TokenIsElevated != 0)
		}

		// TokenIntegrityLevel (25)
		reqLen = 0
		procGetTokenInformation.Call(uintptr(hToken), 25, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
		if reqLen > 0 {
			iBuf := make([]byte, reqLen)
			rI, _, _ := procGetTokenInformation.Call(uintptr(hToken), 25, uintptr(unsafe.Pointer(&iBuf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
			if rI != 0 {
				tml := (*TOKEN_MANDATORY_LABEL)(unsafe.Pointer(&iBuf[0]))
				var strSid *uint16
				procConvertSidToStringSidW.Call(tml.Label.Sid, uintptr(unsafe.Pointer(&strSid)))
				if strSid != nil {
					sidStr := ptrToUTF16String(strSid)
					procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
					switch sidStr {
					case "S-1-16-4096":
						info.IntegrityLevelName = "Low (Untrusted / AppContainer)"
						info.IntegrityLevelRID = 4096
					case "S-1-16-8192":
						info.IntegrityLevelName = "Medium (Standard Unelevated User / Browser Host)"
						info.IntegrityLevelRID = 8192
					case "S-1-16-12288":
						info.IntegrityLevelName = "High (Elevated Administrator)"
						info.IntegrityLevelRID = 12288
					case "S-1-16-16384":
						info.IntegrityLevelName = "System"
						info.IntegrityLevelRID = 16384
					default:
						info.IntegrityLevelName = sidStr
					}
				}
			}
		}
	}

	// 3. Resolve active console session user and determine BelongsToPairedConsoleUser
	if info.ActiveConsoleSessionID != 0xFFFFFFFF {
		var pUser *uint16
		var pDomain *uint16
		var bytesReturned uint32
		rQ, _, _ := procWTSQuerySessionInformationW.Call(0, uintptr(info.ActiveConsoleSessionID), 5, uintptr(unsafe.Pointer(&pUser)), uintptr(unsafe.Pointer(&bytesReturned)))
		if rQ != 0 && pUser != nil {
			info.TokenUserName = ptrToUTF16String(pUser)
			procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pUser)))
		}
		rD, _, _ := procWTSQuerySessionInformationW.Call(0, uintptr(info.ActiveConsoleSessionID), 7, uintptr(unsafe.Pointer(&pDomain)), uintptr(unsafe.Pointer(&bytesReturned)))
		var domStr string
		if rD != 0 && pDomain != nil {
			domStr = ptrToUTF16String(pDomain)
			procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pDomain)))
		}
		if info.TokenUserName != "" {
			fullAcc := info.TokenUserName
			if domStr != "" {
				fullAcc = domStr + "\\" + info.TokenUserName
			}
			pAcc, _ := syscall.UTF16PtrFromString(fullAcc)
			var sidBuf [256]byte
			sidLen := uint32(len(sidBuf))
			var domBuf [256]uint16
			domLen := uint32(len(domBuf))
			var sidUse uint32
			rL, _, _ := procLookupAccountNameW.Call(0, uintptr(unsafe.Pointer(pAcc)), uintptr(unsafe.Pointer(&sidBuf[0])), uintptr(unsafe.Pointer(&sidLen)), uintptr(unsafe.Pointer(&domBuf[0])), uintptr(unsafe.Pointer(&domLen)), uintptr(unsafe.Pointer(&sidUse)))
			if rL != 0 {
				var strSid *uint16
				procConvertSidToStringSidW.Call(uintptr(unsafe.Pointer(&sidBuf[0])), uintptr(unsafe.Pointer(&strSid)))
				if strSid != nil {
					consoleSid := ptrToUTF16String(strSid)
					procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
					info.BelongsToPairedConsoleUser = (info.TokenUserSID != "" && info.TokenUserSID == consoleSid)
				}
			}
		}
	}

	// 4. Connect to named pipe with SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION
	pipeName := `\\.\pipe\OnuslyValidationProbePipe`
	pipeNamePtr, _ := syscall.UTF16PtrFromString(pipeName)

	flags := uint32(SECURITY_SQOS_PRESENT | SECURITY_IDENTIFICATION)
	var hClient uintptr
	var errOpen error
	var lastErrCode uint32
	deadline := time.Now().Add(5 * time.Second)
	pipeAttempts := 0

	for {
		pipeAttempts++
		hClient, _, errOpen = procCreateFileW.Call(
			uintptr(unsafe.Pointer(pipeNamePtr)),
			uintptr(GENERIC_READ|GENERIC_WRITE),
			0,
			0,
			uintptr(OPEN_EXISTING),
			uintptr(flags),
			0,
		)
		if hClient != uintptr(syscall.InvalidHandle) && hClient != 0 {
			lastErrCode = 0
			break
		}

		errno, ok := errOpen.(syscall.Errno)
		if ok {
			lastErrCode = uint32(errno)
		} else {
			lastErrCode = 2
		}

		// Win32 Error 2: ERROR_FILE_NOT_FOUND, Error 231: ERROR_PIPE_BUSY
		if (lastErrCode == 2 || lastErrCode == 231) && time.Now().Before(deadline) {
			procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(pipeNamePtr)), 100)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		break
	}

	info.PipeAttempts = pipeAttempts
	info.Win32ErrorCode = lastErrCode

	if hClient == uintptr(syscall.InvalidHandle) || hClient == 0 {
		info.PipeConnectSuccess = false
		fmt.Printf("[%s Error] CreateFileW failed (attempts: %d, win32_code: %d): %v\n", clientMode, pipeAttempts, lastErrCode, errOpen)
	} else {
		defer procCloseHandle.Call(hClient)
		info.PipeConnectSuccess = true

		msg := []byte("ONUSLY_ELEVATED_CLIENT_PING")
		if clientMode == "UNELEVATED_CONSOLE_USER" {
			msg = []byte("ONUSLY_UNELEVATED_CLIENT_PING")
		} else if mismatchTest {
			msg = []byte("ONUSLY_SIMULATE_MISMATCH_PING")
		}

		var written uint32
		procWriteFile.Call(
			hClient,
			uintptr(unsafe.Pointer(&msg[0])),
			uintptr(len(msg)),
			uintptr(unsafe.Pointer(&written)),
			0,
		)

		var respBuf [256]byte
		var readBytes uint32
		rRead, _, _ := procReadFile.Call(
			hClient,
			uintptr(unsafe.Pointer(&respBuf[0])),
			uintptr(len(respBuf)),
			uintptr(unsafe.Pointer(&readBytes)),
			0,
		)
		if rRead != 0 && readBytes > 0 {
			info.ServerResponse = string(respBuf[:readBytes])
		}
		fmt.Printf("[%s Success] Connected with SQOS Identification. Response: %s\n", clientMode, info.ServerResponse)
	}

	// Record client identity to disk
	dataDir := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe")
	_ = os.MkdirAll(dataDir, 0755)

	idBytes, _ := json.MarshalIndent(info, "", "  ")
	_ = os.WriteFile(filepath.Join(dataDir, "client_identity.json"), idBytes, 0644)

	if clientMode == "ELEVATED_ADMIN" {
		_ = os.WriteFile(filepath.Join(dataDir, "client_elevated.json"), idBytes, 0644)
	} else if clientMode == "UNELEVATED_CONSOLE_USER" {
		_ = os.WriteFile(filepath.Join(dataDir, "client_unelevated.json"), idBytes, 0644)
	}
}

// runNegativeAccessTest is executed by the secondary generic LocalService service
func runNegativeAccessTest() {
	secureFile := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe", "service_sid_protected.dat")
	outReport := filepath.Join(os.Getenv("ProgramData"), "OnuslyProbe", "negative_test.json")

	type NegativeResult struct {
		Timestamp                  string   `json:"timestamp"`
		TargetFilePath             string   `json:"target_file_path"`
		TargetFileExistsBeforeTest bool     `json:"target_file_exists_before_test"`
		ProcessID                  uint32   `json:"process_id"`
		TokenUserSID               string   `json:"token_user_sid"`
		IsLocalService             bool     `json:"is_local_service"`
		HasProbeServiceSID         bool     `json:"has_probe_service_sid"`
		GroupSIDsFound             []string `json:"group_sids_found"`
		GenericLocalServiceDenied  bool     `json:"generic_localservice_denied"`
		Win32ErrorCaptured         uint32   `json:"win32_error_captured"`
		Notes                      []string `json:"notes"`
	}

	res := NegativeResult{
		Timestamp:      time.Now().UTC().Format(time.RFC3339),
		TargetFilePath: secureFile,
		ProcessID:      uint32(os.Getpid()),
		GroupSIDsFound: []string{},
		Notes:          []string{},
	}

	// 1. Verify target file exists before test
	if _, err := os.Stat(secureFile); err == nil {
		res.TargetFileExistsBeforeTest = true
	} else {
		res.TargetFileExistsBeforeTest = false
		res.Notes = append(res.Notes, fmt.Sprintf("Target file missing before test: %v", err))
	}

	// 2. Capture caller token identity: assert TokenUser == S-1-5-19
	var hToken syscall.Handle
	r1, _, _ := procOpenProcessToken.Call(
		getCurrentProcessHandle(),
		uintptr(0x0008), // TOKEN_QUERY
		uintptr(unsafe.Pointer(&hToken)),
	)
	if r1 != 0 {
		defer syscall.CloseHandle(hToken)

		var reqLen uint32
		procGetTokenInformation.Call(uintptr(hToken), 1, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
		if reqLen > 0 {
			uBuf := make([]byte, reqLen)
			rU, _, _ := procGetTokenInformation.Call(uintptr(hToken), 1, uintptr(unsafe.Pointer(&uBuf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
			if rU != 0 {
				tu := (*TOKEN_USER)(unsafe.Pointer(&uBuf[0]))
				var strSid *uint16
				procConvertSidToStringSidW.Call(tu.User.Sid, uintptr(unsafe.Pointer(&strSid)))
				if strSid != nil {
					res.TokenUserSID = ptrToUTF16String(strSid)
					procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
				}
				res.IsLocalService = (res.TokenUserSID == "S-1-5-19")
			}
		}

		// 3. Capture caller token groups: assert probe Service SID (S-1-5-80-...) is ABSENT
		reqLen = 0
		procGetTokenInformation.Call(uintptr(hToken), 2, 0, 0, uintptr(unsafe.Pointer(&reqLen)))
		if reqLen > 0 {
			gBuf := make([]byte, reqLen)
			rG, _, _ := procGetTokenInformation.Call(uintptr(hToken), 2, uintptr(unsafe.Pointer(&gBuf[0])), uintptr(reqLen), uintptr(unsafe.Pointer(&reqLen)))
			if rG != 0 {
				count := *(*uint32)(unsafe.Pointer(&gBuf[0]))
				offset := uintptr(8)
				if uintptr(len(gBuf)) >= offset+uintptr(count)*16 {
					groups := unsafe.Slice((*SID_AND_ATTRIBUTES)(unsafe.Pointer(uintptr(unsafe.Pointer(&gBuf[0]))+offset)), count)
					for i := uint32(0); i < count && i < 1024; i++ {
						var strSid *uint16
						procConvertSidToStringSidW.Call(groups[i].Sid, uintptr(unsafe.Pointer(&strSid)))
						if strSid != nil {
							s := ptrToUTF16String(strSid)
							procLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
							res.GroupSIDsFound = append(res.GroupSIDsFound, s)
							if len(s) > 9 && s[:9] == "S-1-5-80-" {
								res.HasProbeServiceSID = true
							}
						}
					}
				}
			}
		}
	}

	// 4. Call CreateFileW and capture GetLastError immediately
	filePathPtr, _ := syscall.UTF16PtrFromString(secureFile)
	hFile, _, errOpen := procCreateFileW.Call(
		uintptr(unsafe.Pointer(filePathPtr)),
		uintptr(GENERIC_READ|GENERIC_WRITE),
		uintptr(FILE_SHARE_READ),
		0,
		uintptr(OPEN_EXISTING),
		0,
		0,
	)

	if hFile == uintptr(syscall.InvalidHandle) || hFile == 0 {
		res.GenericLocalServiceDenied = true
		if errno, ok := errOpen.(syscall.Errno); ok {
			res.Win32ErrorCaptured = uint32(errno)
		} else if lastErr, ok := syscall.GetLastError().(syscall.Errno); ok {
			res.Win32ErrorCaptured = uint32(lastErr)
		}
	} else {
		res.GenericLocalServiceDenied = false
		procCloseHandle.Call(hFile)
		res.Notes = append(res.Notes, "UNEXPECTED: Generic LocalService was granted access to Service-SID-protected file!")
	}

	data, _ := json.MarshalIndent(res, "", "  ")
	_ = os.WriteFile(outReport, data, 0644)
}
