//go:build windows

package windows

import (
	"syscall"
	"unsafe"
)

var (
	modkernel32 = syscall.NewLazyDLL("kernel32.dll")
	modadvapi32 = syscall.NewLazyDLL("advapi32.dll")
	modwtsapi32 = syscall.NewLazyDLL("wtsapi32.dll")
	modcrypt32  = syscall.NewLazyDLL("crypt32.dll")

	// Kernel32 Process & Session
	ProcGetCurrentProcess            = modkernel32.NewProc("GetCurrentProcess")
	ProcGetCurrentProcessId          = modkernel32.NewProc("GetCurrentProcessId")
	ProcGetCurrentThread             = modkernel32.NewProc("GetCurrentThread")
	ProcProcessIdToSessionId         = modkernel32.NewProc("ProcessIdToSessionId")
	ProcWTSGetActiveConsoleSessionId = modkernel32.NewProc("WTSGetActiveConsoleSessionId")
	ProcGetNamedPipeClientSessionId  = modkernel32.NewProc("GetNamedPipeClientSessionId")

	// Kernel32 Named Pipe & File I/O
	ProcCreateNamedPipeW    = modkernel32.NewProc("CreateNamedPipeW")
	ProcWaitNamedPipeW      = modkernel32.NewProc("WaitNamedPipeW")
	ProcConnectNamedPipe    = modkernel32.NewProc("ConnectNamedPipe")
	ProcDisconnectNamedPipe = modkernel32.NewProc("DisconnectNamedPipe")
	ProcCreateFileW         = modkernel32.NewProc("CreateFileW")
	ProcReadFile            = modkernel32.NewProc("ReadFile")
	ProcWriteFile           = modkernel32.NewProc("WriteFile")
	ProcCloseHandle         = modkernel32.NewProc("CloseHandle")
	ProcFlushFileBuffers    = modkernel32.NewProc("FlushFileBuffers")
	ProcMoveFileExW         = modkernel32.NewProc("MoveFileExW")
	ProcReplaceFileW        = modkernel32.NewProc("ReplaceFileW")
	ProcLocalFree           = modkernel32.NewProc("LocalFree")

	// Kernel32 Overlapped & Events
	ProcCreateEventW           = modkernel32.NewProc("CreateEventW")
	ProcSetEvent               = modkernel32.NewProc("SetEvent")
	ProcResetEvent             = modkernel32.NewProc("ResetEvent")
	ProcWaitForSingleObject    = modkernel32.NewProc("WaitForSingleObject")
	ProcWaitForMultipleObjects = modkernel32.NewProc("WaitForMultipleObjects")
	ProcGetOverlappedResult    = modkernel32.NewProc("GetOverlappedResult")
	ProcCancelIoEx             = modkernel32.NewProc("CancelIoEx")

	// Advapi32 Service & SCM
	ProcStartServiceCtrlDispatcherW     = modadvapi32.NewProc("StartServiceCtrlDispatcherW")
	ProcRegisterServiceCtrlHandlerExW   = modadvapi32.NewProc("RegisterServiceCtrlHandlerExW")
	ProcSetServiceStatus                = modadvapi32.NewProc("SetServiceStatus")
	ProcOpenSCManagerW                  = modadvapi32.NewProc("OpenSCManagerW")
	ProcCreateServiceW                  = modadvapi32.NewProc("CreateServiceW")
	ProcOpenServiceW                    = modadvapi32.NewProc("OpenServiceW")
	ProcStartServiceW                   = modadvapi32.NewProc("StartServiceW")
	ProcControlService                  = modadvapi32.NewProc("ControlService")
	ProcDeleteService                   = modadvapi32.NewProc("DeleteService")
	ProcCloseServiceHandle              = modadvapi32.NewProc("CloseServiceHandle")
	ProcQueryServiceStatus              = modadvapi32.NewProc("QueryServiceStatus")
	ProcQueryServiceConfigW             = modadvapi32.NewProc("QueryServiceConfigW")
	ProcQueryServiceConfig2W            = modadvapi32.NewProc("QueryServiceConfig2W")
	ProcChangeServiceConfigW            = modadvapi32.NewProc("ChangeServiceConfigW")
	ProcChangeServiceConfig2W           = modadvapi32.NewProc("ChangeServiceConfig2W")
	ProcQueryServiceStatusEx            = modadvapi32.NewProc("QueryServiceStatusEx")

	// Advapi32 Tokens & Security
	ProcOpenProcessToken       = modadvapi32.NewProc("OpenProcessToken")
	ProcOpenThreadToken        = modadvapi32.NewProc("OpenThreadToken")
	ProcGetTokenInformation    = modadvapi32.NewProc("GetTokenInformation")
	ProcConvertSidToStringSidW = modadvapi32.NewProc("ConvertSidToStringSidW")
	ProcConvertStringSidToSidW = modadvapi32.NewProc("ConvertStringSidToSidW")
	ProcConvertStringSDToSDW   = modadvapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	ProcConvertSDToStringSDW   = modadvapi32.NewProc("ConvertSecurityDescriptorToStringSecurityDescriptorW")
	ProcGetNamedSecurityInfoW  = modadvapi32.NewProc("GetNamedSecurityInfoW")
	ProcSetNamedSecurityInfoW  = modadvapi32.NewProc("SetNamedSecurityInfoW")
	ProcGetSecurityDescriptorDacl = modadvapi32.NewProc("GetSecurityDescriptorDacl")
	ProcSetFileSecurityW       = modadvapi32.NewProc("SetFileSecurityW")
	ProcLookupAccountNameW     = modadvapi32.NewProc("LookupAccountNameW")
	ProcIsValidSid             = modadvapi32.NewProc("IsValidSid")
	ProcEqualSid               = modadvapi32.NewProc("EqualSid")
	ProcImpersonateNamedPipeClient = modadvapi32.NewProc("ImpersonateNamedPipeClient")
	ProcRevertToSelf           = modadvapi32.NewProc("RevertToSelf")

	// Wtsapi32 Sessions
	ProcWTSEnumerateSessionsW       = modwtsapi32.NewProc("WTSEnumerateSessionsW")
	ProcWTSQuerySessionInformationW = modwtsapi32.NewProc("WTSQuerySessionInformationW")
	ProcWTSFreeMemory               = modwtsapi32.NewProc("WTSFreeMemory")

	// Crypt32 DPAPI
	ProcCryptProtectData   = modcrypt32.NewProc("CryptProtectData")
	ProcCryptUnprotectData = modcrypt32.NewProc("CryptUnprotectData")
)

// Win32 Constants
const (
	// SCM
	SERVICE_WIN32_OWN_PROCESS       = 0x00000010
	SERVICE_START_PENDING           = 0x00000002
	SERVICE_RUNNING                 = 0x00000004
	SERVICE_STOP_PENDING            = 0x00000003
	SERVICE_STOPPED                 = 0x00000001
	SERVICE_ACCEPT_STOP             = 0x00000001
	SERVICE_ACCEPT_SHUTDOWN         = 0x00000004
	SERVICE_ACCEPT_SESSIONCHANGE    = 0x00000080
	SERVICE_ACCEPT_POWEREVENT       = 0x00000040
	SERVICE_CONTROL_STOP            = 0x00000001
	SERVICE_CONTROL_SHUTDOWN        = 0x00000005
	SERVICE_CONTROL_SESSIONCHANGE   = 0x0000000B
	SERVICE_CONTROL_POWEREVENT      = 0x0000000D
	SERVICE_CONTROL_INTERROGATE     = 0x00000004

	// WTS Session Change Notifications
	WTS_CONSOLE_CONNECT    = 0x1
	WTS_CONSOLE_DISCONNECT = 0x2
	WTS_SESSION_LOGON      = 0x5
	WTS_SESSION_LOGOFF     = 0x6
	WTS_SESSION_LOCK       = 0x7
	WTS_SESSION_UNLOCK     = 0x8

	// Power Events
	PBT_APMSUSPEND         = 0x0004
	PBT_APMRESUMEAUTOMATIC = 0x0012
	PBT_APMRESUMESUSPEND   = 0x0007

	// Named Pipe & File
	PIPE_ACCESS_DUPLEX          = 0x00000003
	FILE_FLAG_FIRST_PIPE_INSTANCE = 0x00080000
	FILE_FLAG_OVERLAPPED        = 0x40000000
	PIPE_TYPE_MESSAGE           = 0x00000004
	PIPE_READMODE_MESSAGE       = 0x00000002
	PIPE_WAIT                   = 0x00000000

	GENERIC_READ  = 0x80000000
	GENERIC_WRITE = 0x40000000
	OPEN_EXISTING = 3

	// Security Quality of Service (SQOS)
	SECURITY_SQOS_PRESENT   = 0x00100000
	SECURITY_IDENTIFICATION = 0x00010000 // Impersonation Level 1

	// File Replacement & Move
	MOVEFILE_REPLACE_EXISTING   = 0x00000001
	MOVEFILE_WRITE_THROUGH      = 0x00000008
	REPLACEFILE_WRITE_THROUGH   = 0x00000001

	// Security & SDDL
	SDDL_REVISION_1            = 1
	DACL_SECURITY_INFORMATION  = 0x00000004
	PROTECTED_DACL_SECURITY_INFORMATION = 0x80000000
	UNPROTECTED_DACL_SECURITY_INFORMATION = 0x20000000
	SE_FILE_OBJECT             = 1

	// SCM Access & Config
	SC_MANAGER_CONNECT                     = 0x0001
	SC_MANAGER_ALL_ACCESS                  = 0xF003F
	SERVICE_QUERY_STATUS                   = 0x0004
	SERVICE_START                          = 0x0010
	SERVICE_STOP                           = 0x0020
	SERVICE_ALL_ACCESS                     = 0xF01FF
	SERVICE_AUTO_START                     = 0x00000002
	SERVICE_DEMAND_START                   = 0x00000003
	SERVICE_ERROR_NORMAL                   = 0x00000001
	SERVICE_CONFIG_DESCRIPTION              = 1
	SERVICE_CONFIG_FAILURE_ACTIONS          = 2
	SERVICE_CONFIG_SERVICE_SID_INFO         = 5
	SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO = 6
	SERVICE_SID_TYPE_UNRESTRICTED          = 1
	SC_ACTION_RESTART                       = 1
	SERVICE_NO_CHANGE                       = 0xFFFFFFFF

	// DPAPI
	CRYPTPROTECT_LOCAL_MACHINE = 0x00000004
	CRYPTPROTECT_UI_FORBIDDEN  = 0x00000001

	// Overlapped Wait
	WAIT_OBJECT_0 = 0x00000000
	WAIT_TIMEOUT  = 0x00000102
)

// Win32 Structures
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

type SERVICE_SID_INFO struct {
	ServiceSidType uint32
}

type SERVICE_REQUIRED_PRIVILEGES_INFO struct {
	PmszRequiredPrivileges *uint16
}

type SC_ACTION struct {
	Type  uint32 // SC_ACTION_RESTART = 1
	Delay uint32 // ms
}

type SERVICE_FAILURE_ACTIONS struct {
	DwResetPeriod uint32
	LpRebootMsg   *uint16
	LpCommand     *uint16
	CActions      uint32
	_             uint32 // 64-bit alignment padding
	LpsaActions   uintptr // *SC_ACTION
}

type SERVICE_DESCRIPTION struct {
	LpDescription *uint16
}

type QUERY_SERVICE_CONFIG struct {
	ServiceType      uint32
	StartType        uint32
	ErrorControl     uint32
	_                uint32 // 64-bit padding
	BinaryPathName   *uint16
	LoadOrderGroup   *uint16
	TagId            uint32
	_                uint32 // 64-bit padding
	Dependencies     *uint16
	ServiceStartName *uint16
	DisplayName      *uint16
}

type WTSSESSION_NOTIFICATION struct {
	Size      uint32
	SessionID uint32
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
	_                    uint32 // 64-bit alignment padding
	LpSecurityDescriptor uintptr
	BInheritHandle       int32
	_                    uint32 // Struct size padding to 24 bytes
}

type DATA_BLOB struct {
	CbData uint32
	_      uint32 // 64-bit alignment padding
	PbData *byte
}

type SID_AND_ATTRIBUTES struct {
	Sid        uintptr
	Attributes uint32
	_          uint32 // 64-bit alignment padding
}

type TOKEN_USER struct {
	User SID_AND_ATTRIBUTES
}

type TOKEN_GROUPS struct {
	GroupCount uint32
	_          uint32 // 64-bit alignment padding so Groups starts at offset 8
	Groups     [1]SID_AND_ATTRIBUTES
}

// ParseTokenGroups parses a raw byte buffer returned by GetTokenInformation(TokenGroups).
func ParseTokenGroups(buf []byte) []SID_AND_ATTRIBUTES {
	if len(buf) < int(unsafe.Offsetof(TOKEN_GROUPS{}.Groups)) {
		return nil
	}
	tg := (*TOKEN_GROUPS)(unsafe.Pointer(&buf[0]))
	if tg.GroupCount == 0 {
		return nil
	}
	reqSize := int(unsafe.Offsetof(TOKEN_GROUPS{}.Groups)) + int(tg.GroupCount)*int(unsafe.Sizeof(SID_AND_ATTRIBUTES{}))
	if len(buf) < reqSize {
		return nil
	}
	return unsafe.Slice(&tg.Groups[0], tg.GroupCount)
}

// Utility Helpers
func PtrToUTF16String(p *uint16) string {
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

func StringToSid(sidStr string) (uintptr, error) {
	sidPtr, err := syscall.UTF16PtrFromString(sidStr)
	if err != nil {
		return 0, err
	}
	var pSid uintptr
	r, _, errCall := ProcConvertStringSidToSidW.Call(
		uintptr(unsafe.Pointer(sidPtr)),
		uintptr(unsafe.Pointer(&pSid)),
	)
	if r == 0 || pSid == 0 {
		return 0, errCall
	}
	return pSid, nil
}

func SidToString(pSid uintptr) (string, error) {
	var strSid *uint16
	r, _, errCall := ProcConvertSidToStringSidW.Call(pSid, uintptr(unsafe.Pointer(&strSid)))
	if r == 0 || strSid == nil {
		return "", errCall
	}
	defer ProcLocalFree.Call(uintptr(unsafe.Pointer(strSid)))
	return PtrToUTF16String(strSid), nil
}

func EqualSid(sid1, sid2 uintptr) bool {
	r, _, _ := ProcEqualSid.Call(sid1, sid2)
	return r != 0
}
