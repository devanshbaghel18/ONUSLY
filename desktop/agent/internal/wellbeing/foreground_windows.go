//go:build windows

package wellbeing

import (
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32DLL                    = windows.NewLazySystemDLL("user32.dll")
	procGetForegroundWindow      = user32DLL.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32DLL.NewProc("GetWindowThreadProcessId")
	procGetLastInputInfo         = user32DLL.NewProc("GetLastInputInfo")

	kernel32DLL        = windows.NewLazySystemDLL("kernel32.dll")
	procGetTickCount64 = kernel32DLL.NewProc("GetTickCount64")
)

type lastInputInfo struct {
	cbSize uint32
	dwTime uint32
}

// SampleForegroundApp returns the executable name (e.g. "chrome.exe") of the currently focused window
// and whether the user has been idle longer than [idleThresholdMs]. Privacy-safe: never reads window titles.
func SampleForegroundApp(idleThresholdMs uint64) (exeName string, isIdle bool) {
	idleMs := getUserIdleMillis()
	if idleThresholdMs > 0 && idleMs >= idleThresholdMs {
		return "", true
	}

	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return "", false
	}

	var pid uint32
	_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return "", false
	}

	hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil || hProc == 0 {
		return "", false
	}
	defer windows.CloseHandle(hProc)

	var buf [windows.MAX_PATH]uint16
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(hProc, 0, &buf[0], &size); err != nil {
		return "", false
	}

	fullPath := windows.UTF16ToString(buf[:size])
	return filepath.Base(fullPath), false
}

func getUserIdleMillis() uint64 {
	var lii lastInputInfo
	lii.cbSize = uint32(unsafe.Sizeof(lii))
	r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&lii)))
	if r == 0 {
		return 0
	}
	ticks, _, _ := procGetTickCount64.Call()
	now32 := uint32(ticks)
	return uint64(now32 - lii.dwTime)
}
