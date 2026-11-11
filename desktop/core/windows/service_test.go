//go:build windows

package windows

import (
	"errors"
	"testing"
)

func TestWindowsService_ConsoleSessionDetection(t *testing.T) {
	info, err := GetActiveConsoleSession()
	if err != nil {
		if errors.Is(err, ErrConsoleDetached) {
			t.Logf("Console session detached: %+v", info)
			return
		}
		t.Fatalf("GetActiveConsoleSession failed: %v", err)
	}

	if !info.IsAttached {
		t.Fatalf("Expected attached console, got detached")
	}
	if info.UserSID == "" {
		t.Fatalf("Expected resolved user SID, got empty string")
	}
	t.Logf("Detected Active Console Session: ID=%d, User=%s\\%s, SID=%s",
		info.SessionID, info.DomainName, info.UserName, info.UserSID)
}

func TestWindowsService_ConsoleSessionMonitor(t *testing.T) {
	monitor := NewConsoleSessionMonitor()
	info := monitor.GetCurrent()
	if info == nil {
		t.Fatalf("Expected non-nil console session info")
	}

	if info.IsAttached {
		// Valid session must pass validation
		if err := monitor.ValidateClientSession(info.SessionID); err != nil {
			t.Fatalf("Valid session failed validation: %v", err)
		}

		// Wrong session ID must fail validation
		wrongSessionID := info.SessionID + 999
		if err := monitor.ValidateClientSession(wrongSessionID); err == nil {
			t.Fatalf("Expected error for mismatched session %d, got nil", wrongSessionID)
		}
	}
}

func TestWindowsService_NonServiceLaunchDetection(t *testing.T) {
	svc := NewWindowsService("OnuslyTestService", nil, nil)
	err := svc.Run()
	if !errors.Is(err, ErrNotRunningAsService) {
		t.Fatalf("Expected ErrNotRunningAsService (error 1063) when run from console, got: %v", err)
	}
}

func TestWindowsService_ControlHandling(t *testing.T) {
	monitor := NewConsoleSessionMonitor()
	svc := NewWindowsService("OnuslyTestService", monitor, nil)

	// Interrogate control
	rInterrogate := svc.handleControl(SERVICE_CONTROL_INTERROGATE, 0, 0)
	if rInterrogate != 0 {
		t.Fatalf("Expected 0 for INTERROGATE, got %d", rInterrogate)
	}

	// Session change control
	rSession := svc.handleControl(SERVICE_CONTROL_SESSIONCHANGE, WTS_CONSOLE_CONNECT, 0)
	if rSession != 0 {
		t.Fatalf("Expected 0 for SESSIONCHANGE, got %d", rSession)
	}

	// Power event control
	rPower := svc.handleControl(SERVICE_CONTROL_POWEREVENT, PBT_APMRESUMEAUTOMATIC, 0)
	if rPower != 0 {
		t.Fatalf("Expected 0 for POWEREVENT, got %d", rPower)
	}

	// Unsupported control
	rBad := svc.handleControl(0x9999, 0, 0)
	if rBad != 0x00000078 { // ERROR_CALL_NOT_IMPLEMENTED
		t.Fatalf("Expected 0x78 for unknown control, got %d", rBad)
	}
}

