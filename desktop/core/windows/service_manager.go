//go:build windows

package windows

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	ErrServiceNotFound = errors.New("windows service not found")
	ErrServiceTimeout  = errors.New("timed out waiting for service state transition")
	ErrInvalidPE       = errors.New("file is not a valid Windows PE AMD64 executable")
)

type ServiceConfig struct {
	ServiceName string `json:"serviceName"`
	DisplayName string `json:"displayName"`
	BinPath     string `json:"binPath"`
	Account     string `json:"account"`
	StartType   string `json:"startType"`
	SIDType     string `json:"sidType"`
	Privileges  string `json:"privileges"`
}

type WindowsServiceManager struct {
	serviceName string
	binPath     string
}

func NewWindowsServiceManager(serviceName, binPath string) *WindowsServiceManager {
	if serviceName == "" {
		serviceName = "OnuslyDesktopAgent"
	}
	return &WindowsServiceManager{
		serviceName: serviceName,
		binPath:     binPath,
	}
}

// BuildCreateArgs returns the exact sc.exe arguments for creating the service.
func (m *WindowsServiceManager) BuildCreateArgs() []string {
	binArg := fmt.Sprintf(`"%s" run`, m.binPath)
	return []string{
		"create", m.serviceName,
		"binPath=", binArg,
		"obj=", `NT AUTHORITY\LocalService`,
		"type=", "own",
		"start=", "auto",
		"DisplayName=", "ONUSLY Desktop Enforcement Service",
	}
}

// BuildUpdateConfigArgs returns the exact sc.exe arguments for updating existing service configuration.
func (m *WindowsServiceManager) BuildUpdateConfigArgs() []string {
	binArg := fmt.Sprintf(`"%s" run`, m.binPath)
	return []string{
		"config", m.serviceName,
		"binPath=", binArg,
		"obj=", `NT AUTHORITY\LocalService`,
		"type=", "own",
		"start=", "auto",
		"DisplayName=", "ONUSLY Desktop Enforcement Service",
	}
}

// InstallServiceNative creates or updates the service in SCM using native Win32 APIs.
func (m *WindowsServiceManager) InstallServiceNative() error {
	if m.binPath == "" {
		return errors.New("cannot install service with empty binary path")
	}

	hSCM, _, errSCM := ProcOpenSCManagerW.Call(0, 0, uintptr(SC_MANAGER_ALL_ACCESS))
	if hSCM == 0 {
		return fmt.Errorf("OpenSCManagerW failed: %w", errSCM)
	}
	defer ProcCloseServiceHandle.Call(hSCM)

	serviceNamePtr, _ := syscall.UTF16PtrFromString(m.serviceName)
	displayNamePtr, _ := syscall.UTF16PtrFromString("ONUSLY Desktop Enforcement Service")
	binArg := fmt.Sprintf(`"%s" run`, m.binPath)
	binPathPtr, _ := syscall.UTF16PtrFromString(binArg)
	accountPtr, _ := syscall.UTF16PtrFromString(`NT AUTHORITY\LocalService`)

	var hService uintptr
	// 1. Check if service already exists
	hExisting, _, _ := ProcOpenServiceW.Call(hSCM, uintptr(unsafe.Pointer(serviceNamePtr)), uintptr(SERVICE_ALL_ACCESS))
	if hExisting != 0 {
		hService = hExisting
		defer ProcCloseServiceHandle.Call(hService)

		// Check existing binary path for conflict
		var bytesNeeded uint32
		ProcQueryServiceConfigW.Call(hService, 0, 0, uintptr(unsafe.Pointer(&bytesNeeded)))
		if bytesNeeded > 0 {
			buf := make([]byte, bytesNeeded)
			rQC, _, _ := ProcQueryServiceConfigW.Call(hService, uintptr(unsafe.Pointer(&buf[0])), uintptr(bytesNeeded), uintptr(unsafe.Pointer(&bytesNeeded)))
			if rQC != 0 {
				qsc := (*QUERY_SERVICE_CONFIG)(unsafe.Pointer(&buf[0]))
				existingBin := PtrToUTF16String(qsc.BinaryPathName)
				if existingBin != "" && !strings.Contains(strings.ToLower(existingBin), "onusly-agent.exe") {
					return fmt.Errorf("service conflict: service %q already exists with conflicting binary: %s", m.serviceName, existingBin)
				}
			}
		}

		// Update configuration safely via ChangeServiceConfigW
		rChange, _, errChange := ProcChangeServiceConfigW.Call(
			hService,
			uintptr(SERVICE_WIN32_OWN_PROCESS),
			uintptr(SERVICE_AUTO_START),
			uintptr(SERVICE_ERROR_NORMAL),
			uintptr(unsafe.Pointer(binPathPtr)),
			0, // lpLoadOrderGroup
			0, // lpdwTagId
			0, // lpDependencies
			uintptr(unsafe.Pointer(accountPtr)),
			0, // lpPassword
			uintptr(unsafe.Pointer(displayNamePtr)),
		)
		if rChange == 0 {
			return fmt.Errorf("ChangeServiceConfigW failed: %w", errChange)
		}
	} else {
		// Create new service via CreateServiceW
		rCreate, _, errCreate := ProcCreateServiceW.Call(
			hSCM,
			uintptr(unsafe.Pointer(serviceNamePtr)),
			uintptr(unsafe.Pointer(displayNamePtr)),
			uintptr(SERVICE_ALL_ACCESS),
			uintptr(SERVICE_WIN32_OWN_PROCESS),
			uintptr(SERVICE_AUTO_START),
			uintptr(SERVICE_ERROR_NORMAL),
			uintptr(unsafe.Pointer(binPathPtr)),
			0, // lpLoadOrderGroup
			0, // lpdwTagId
			0, // lpDependencies
			uintptr(unsafe.Pointer(accountPtr)),
			0, // lpPassword
		)
		if rCreate == 0 {
			return fmt.Errorf("CreateServiceW failed: %w", errCreate)
		}
		hService = rCreate
		defer ProcCloseServiceHandle.Call(hService)
	}

	// 2. Configure Service SID to UNRESTRICTED
	sidInfo := SERVICE_SID_INFO{ServiceSidType: SERVICE_SID_TYPE_UNRESTRICTED}
	rSID, _, errSID := ProcChangeServiceConfig2W.Call(
		hService,
		uintptr(SERVICE_CONFIG_SERVICE_SID_INFO),
		uintptr(unsafe.Pointer(&sidInfo)),
	)
	if rSID == 0 {
		return fmt.Errorf("ChangeServiceConfig2W(SERVICE_CONFIG_SERVICE_SID_INFO) failed: %w", errSID)
	}

	// 3. Configure Required Privileges: SeChangeNotifyPrivilege only
	privsUTF16, _ := syscall.UTF16FromString("SeChangeNotifyPrivilege")
	privsUTF16 = append(privsUTF16, 0) // double null terminator for multi-string
	privsInfo := SERVICE_REQUIRED_PRIVILEGES_INFO{PmszRequiredPrivileges: &privsUTF16[0]}
	rPrivs, _, errPrivs := ProcChangeServiceConfig2W.Call(
		hService,
		uintptr(SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO),
		uintptr(unsafe.Pointer(&privsInfo)),
	)
	if rPrivs == 0 {
		return fmt.Errorf("ChangeServiceConfig2W(SERVICE_CONFIG_REQUIRED_PRIVILEGES_INFO) failed: %w", errPrivs)
	}

	// 4. Configure automatic failure recovery actions (restart after 5s, 10s)
	actions := [2]SC_ACTION{
		{Type: SC_ACTION_RESTART, Delay: 5000},
		{Type: SC_ACTION_RESTART, Delay: 10000},
	}
	failActions := SERVICE_FAILURE_ACTIONS{
		DwResetPeriod: 86400,
		CActions:      uint32(len(actions)),
		LpsaActions:   uintptr(unsafe.Pointer(&actions[0])),
	}
	ProcChangeServiceConfig2W.Call(
		hService,
		uintptr(SERVICE_CONFIG_FAILURE_ACTIONS),
		uintptr(unsafe.Pointer(&failActions)),
	)

	// 5. Configure description
	descUTF16, _ := syscall.UTF16PtrFromString("Central OS-level enforcement daemon and canonical blocklist cache for ONUSLY accountability goals.")
	descInfo := SERVICE_DESCRIPTION{LpDescription: descUTF16}
	ProcChangeServiceConfig2W.Call(
		hService,
		uintptr(SERVICE_CONFIG_DESCRIPTION),
		uintptr(unsafe.Pointer(&descInfo)),
	)

	return nil
}

// UninstallServiceNative stops and removes the service from SCM using native Win32 APIs.
func (m *WindowsServiceManager) UninstallServiceNative() error {
	_ = m.StopServiceNative()

	hSCM, _, errSCM := ProcOpenSCManagerW.Call(0, 0, uintptr(SC_MANAGER_ALL_ACCESS))
	if hSCM == 0 {
		return fmt.Errorf("OpenSCManagerW failed: %w", errSCM)
	}
	defer ProcCloseServiceHandle.Call(hSCM)

	serviceNamePtr, _ := syscall.UTF16PtrFromString(m.serviceName)
	hService, _, _ := ProcOpenServiceW.Call(hSCM, uintptr(unsafe.Pointer(serviceNamePtr)), uintptr(SERVICE_ALL_ACCESS))
	if hService == 0 {
		return nil // Service does not exist
	}
	defer ProcCloseServiceHandle.Call(hService)

	rDel, _, errDel := ProcDeleteService.Call(hService)
	if rDel == 0 {
		return fmt.Errorf("DeleteService failed: %w", errDel)
	}
	return nil
}

// StartServiceNative requests SCM to start the service using StartServiceW and waits for SERVICE_RUNNING.
func (m *WindowsServiceManager) StartServiceNative() error {
	hSCM, _, errSCM := ProcOpenSCManagerW.Call(0, 0, uintptr(SC_MANAGER_CONNECT))
	if hSCM == 0 {
		return fmt.Errorf("OpenSCManagerW failed: %w", errSCM)
	}
	defer ProcCloseServiceHandle.Call(hSCM)

	serviceNamePtr, _ := syscall.UTF16PtrFromString(m.serviceName)
	hService, _, errOpen := ProcOpenServiceW.Call(hSCM, uintptr(unsafe.Pointer(serviceNamePtr)), uintptr(SERVICE_START|SERVICE_QUERY_STATUS))
	if hService == 0 {
		return fmt.Errorf("OpenServiceW failed: %w", errOpen)
	}
	defer ProcCloseServiceHandle.Call(hService)

	rStart, _, errStart := ProcStartServiceW.Call(hService, 0, 0)
	if rStart == 0 {
		errno, ok := errStart.(syscall.Errno)
		if !ok || errno != 1056 { // 1056 = ERROR_SERVICE_ALREADY_RUNNING
			return fmt.Errorf("StartServiceW failed: %w", errStart)
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		running, err := m.IsRunningNative()
		if err == nil && running {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("%w: failed to reach SERVICE_RUNNING within 10s", ErrServiceTimeout)
}

// StopServiceWithTimeout requests SCM to stop the service and waits up to timeout for SERVICE_STOPPED.
func (m *WindowsServiceManager) StopServiceWithTimeout(timeout time.Duration) error {
	hSCM, _, errSCM := ProcOpenSCManagerW.Call(0, 0, uintptr(SC_MANAGER_CONNECT))
	if hSCM == 0 {
		return fmt.Errorf("OpenSCManagerW failed: %w", errSCM)
	}
	defer ProcCloseServiceHandle.Call(hSCM)

	serviceNamePtr, _ := syscall.UTF16PtrFromString(m.serviceName)
	hService, _, errOpen := ProcOpenServiceW.Call(hSCM, uintptr(unsafe.Pointer(serviceNamePtr)), uintptr(SERVICE_STOP|SERVICE_QUERY_STATUS))
	if hService == 0 {
		errno, ok := errOpen.(syscall.Errno)
		if ok && errno == 1060 { // ERROR_SERVICE_DOES_NOT_EXIST
			return nil // Service does not exist, so it is already stopped
		}
		return fmt.Errorf("OpenServiceW failed: %w", errOpen)
	}
	defer ProcCloseServiceHandle.Call(hService)

	var status SERVICE_STATUS
	ProcControlService.Call(hService, uintptr(SERVICE_CONTROL_STOP), uintptr(unsafe.Pointer(&status)))

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		running, err := m.IsRunningNative()
		if err == nil && !running {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("%w: failed to reach SERVICE_STOPPED within %v", ErrServiceTimeout, timeout)
}

// StopServiceNative requests SCM to stop the service using ControlService and waits for SERVICE_STOPPED.
func (m *WindowsServiceManager) StopServiceNative() error {
	return m.StopServiceWithTimeout(8 * time.Second)
}

// UpgradeService gracefully upgrades an existing service installation.
// If the service is running, it stops it within stopTimeout, updates its SCM configuration,
// and restarts it if it was running.
func (m *WindowsServiceManager) UpgradeService(newBinPath string, stopTimeout time.Duration) (wasRunning bool, err error) {
	if newBinPath != "" {
		m.binPath = newBinPath
	}

	running, errCheck := m.IsRunningNative()
	if errCheck == nil && running {
		wasRunning = true
		if err := m.StopServiceWithTimeout(stopTimeout); err != nil {
			return true, fmt.Errorf("failed to stop service for upgrade: %w", err)
		}
	}

	if err := m.InstallServiceNative(); err != nil {
		return wasRunning, fmt.Errorf("failed to update service configuration: %w", err)
	}

	if wasRunning {
		if err := m.StartServiceNative(); err != nil {
			return true, fmt.Errorf("failed to restart service after upgrade: %w", err)
		}
	}

	return wasRunning, nil
}

// ValidatePE checks whether a given file path is a valid Windows PE AMD64 executable.
func ValidatePE(filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var dosHeader [64]byte
	if _, err := f.ReadAt(dosHeader[:], 0); err != nil {
		return fmt.Errorf("%w: unable to read DOS header: %v", ErrInvalidPE, err)
	}

	if dosHeader[0] != 'M' || dosHeader[1] != 'Z' {
		return fmt.Errorf("%w: missing MZ signature", ErrInvalidPE)
	}

	peOffset := binary.LittleEndian.Uint32(dosHeader[0x3C:0x40])
	if peOffset < 64 || peOffset > 10*1024*1024 {
		return fmt.Errorf("%w: invalid PE header offset 0x%x", ErrInvalidPE, peOffset)
	}

	var peHeader [24]byte
	if _, err := f.ReadAt(peHeader[:], int64(peOffset)); err != nil {
		return fmt.Errorf("%w: unable to read PE header: %v", ErrInvalidPE, err)
	}

	if peHeader[0] != 'P' || peHeader[1] != 'E' || peHeader[2] != 0 || peHeader[3] != 0 {
		return fmt.Errorf("%w: missing PE signature", ErrInvalidPE)
	}

	machine := binary.LittleEndian.Uint16(peHeader[4:6])
	if machine != 0x8664 {
		return fmt.Errorf("%w: unsupported machine architecture 0x%04x (expected 0x8664 AMD64)", ErrInvalidPE, machine)
	}

	return nil
}

// IsRunningNative queries SCM to determine if the service is currently running.
func (m *WindowsServiceManager) IsRunningNative() (bool, error) {
	hSCM, _, _ := ProcOpenSCManagerW.Call(0, 0, uintptr(0x0001)) // SC_MANAGER_CONNECT
	if hSCM == 0 {
		return m.IsRunning()
	}
	defer ProcCloseServiceHandle.Call(hSCM)

	serviceNamePtr, _ := syscall.UTF16PtrFromString(m.serviceName)
	hService, _, errOpen := ProcOpenServiceW.Call(hSCM, uintptr(unsafe.Pointer(serviceNamePtr)), uintptr(0x0004)) // SERVICE_QUERY_STATUS
	if hService == 0 {
		errno, ok := errOpen.(syscall.Errno)
		if ok && errno == 1060 {
			return false, ErrServiceNotFound
		}
		return false, fmt.Errorf("OpenServiceW failed: %w", errOpen)
	}
	defer ProcCloseServiceHandle.Call(hService)

	var status SERVICE_STATUS
	r, _, errStatus := ProcQueryServiceStatus.Call(hService, uintptr(unsafe.Pointer(&status)))
	if r == 0 {
		return false, fmt.Errorf("QueryServiceStatus failed: %w", errStatus)
	}

	return status.CurrentState == SERVICE_RUNNING, nil
}

// InstallService registers OnuslyDesktopAgent in SCM with LocalService, unrestricted SID, and stripped privileges.
func (m *WindowsServiceManager) InstallService() error {
	if m.binPath == "" {
		return errors.New("cannot install service with empty binary path")
	}

	// Prefer native Win32 SCM API (direct advapi32.dll calls with zero quoting issues)
	if errNative := m.InstallServiceNative(); errNative == nil {
		return nil
	}

	// Fallback to sc.exe create / config if native API is blocked
	cmdCreate := exec.Command("sc.exe", m.BuildCreateArgs()...)
	if out, err := cmdCreate.CombinedOutput(); err != nil {
		if strings.Contains(string(out), "1073") { // ERROR_SERVICE_EXISTS = 1073
			cmdConfig := exec.Command("sc.exe", m.BuildUpdateConfigArgs()...)
			if outConfig, errConfig := cmdConfig.CombinedOutput(); errConfig != nil {
				return fmt.Errorf("sc config failed: %s: %w", string(outConfig), errConfig)
			}
		} else {
			return fmt.Errorf("sc create failed: %s: %w", string(out), err)
		}
	}

	// 2. Configure Service SID to unrestricted
	cmdSID := exec.Command("sc.exe", "sidtype", m.serviceName, "unrestricted")
	if out, err := cmdSID.CombinedOutput(); err != nil {
		return fmt.Errorf("sc sidtype failed: %s: %w", string(out), err)
	}

	// 3. Configure privileges to retain only SeChangeNotifyPrivilege
	cmdPrivs := exec.Command("sc.exe", "privs", m.serviceName, "SeChangeNotifyPrivilege")
	if out, err := cmdPrivs.CombinedOutput(); err != nil {
		return fmt.Errorf("sc privs failed: %s: %w", string(out), err)
	}

	// 4. Configure automatic failure recovery actions (restart after 5s, 10s)
	cmdFail := exec.Command("sc.exe", "failure", m.serviceName,
		"reset=", "86400",
		"actions=", "restart/5000/restart/10000//0",
	)
	_ = cmdFail.Run()

	// 5. Configure description
	cmdDesc := exec.Command("sc.exe", "description", m.serviceName,
		"Central OS-level enforcement daemon and canonical blocklist cache for ONUSLY accountability goals.",
	)
	_ = cmdDesc.Run()

	return nil
}

// UninstallService stops and removes the service from SCM.
func (m *WindowsServiceManager) UninstallService() error {
	if errNative := m.UninstallServiceNative(); errNative == nil {
		return nil
	}

	_ = m.StopService()

	cmdDelete := exec.Command("sc.exe", "delete", m.serviceName)
	if out, err := cmdDelete.CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "1060") { // ERROR_SERVICE_DOES_NOT_EXIST = 1060
			return fmt.Errorf("sc delete failed: %s: %w", string(out), err)
		}
	}

	return nil
}

// StartService requests SCM to start the service and waits until SERVICE_RUNNING.
func (m *WindowsServiceManager) StartService() error {
	if errNative := m.StartServiceNative(); errNative == nil {
		return nil
	}

	cmdStart := exec.Command("sc.exe", "start", m.serviceName)
	if out, err := cmdStart.CombinedOutput(); err != nil {
		if !strings.Contains(string(out), "1056") { // ERROR_SERVICE_ALREADY_RUNNING = 1056
			return fmt.Errorf("sc start failed: %s: %w", string(out), err)
		}
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		running, err := m.IsRunning()
		if err == nil && running {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("%w: failed to reach SERVICE_RUNNING within 10s", ErrServiceTimeout)
}

// StopService requests SCM to stop the service and waits until SERVICE_STOPPED.
func (m *WindowsServiceManager) StopService() error {
	if errNative := m.StopServiceNative(); errNative == nil {
		return nil
	}

	cmdStop := exec.Command("sc.exe", "stop", m.serviceName)
	_ = cmdStop.Run()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		running, err := m.IsRunning()
		if err == nil && !running {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("%w: failed to reach SERVICE_STOPPED within 8s", ErrServiceTimeout)
}

// IsRunning queries SCM to determine if the service is currently running.
func (m *WindowsServiceManager) IsRunning() (bool, error) {
	if running, err := m.IsRunningNative(); err == nil {
		return running, nil
	}

	cmdQuery := exec.Command("sc.exe", "query", m.serviceName)
	out, err := cmdQuery.CombinedOutput()
	if err != nil {
		if strings.Contains(string(out), "1060") {
			return false, ErrServiceNotFound
		}
		return false, err
	}

	return strings.Contains(string(out), "STATE") && strings.Contains(string(out), "RUNNING"), nil
}

// QueryConfig inspects SCM configuration for the service.
func (m *WindowsServiceManager) QueryConfig() (*ServiceConfig, error) {
	cmdQC := exec.Command("sc.exe", "qc", m.serviceName)
	outQC, errQC := cmdQC.CombinedOutput()
	if errQC != nil {
		return nil, fmt.Errorf("sc qc failed: %s: %w", string(outQC), errQC)
	}

	cmdSID := exec.Command("sc.exe", "qsidtype", m.serviceName)
	outSID, _ := cmdSID.CombinedOutput()

	cmdPrivs := exec.Command("sc.exe", "qprivs", m.serviceName)
	outPrivs, _ := cmdPrivs.CombinedOutput()

	cfg := &ServiceConfig{
		ServiceName: m.serviceName,
		SIDType:     strings.TrimSpace(string(outSID)),
		Privileges:  strings.TrimSpace(string(outPrivs)),
	}

	lines := strings.Split(string(outQC), "\n")
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "BINARY_PATH_NAME") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				cfg.BinPath = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(l, "SERVICE_START_NAME") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				cfg.Account = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(l, "START_TYPE") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				cfg.StartType = strings.TrimSpace(parts[1])
			}
		} else if strings.HasPrefix(l, "DISPLAY_NAME") {
			parts := strings.SplitN(l, ":", 2)
			if len(parts) == 2 {
				cfg.DisplayName = strings.TrimSpace(parts[1])
			}
		}
	}

	return cfg, nil
}

