//go:build windows

package windows_test

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

func TestSecureDirectoryACL_ConfigurationAndVerification(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-acl-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	securePath := filepath.Join(tmpDir, "secure")
	testServiceSID := win.ProductionServiceSID

	// 1. Configure the secure directory ACL
	if err := win.ConfigureSecureDirectoryACL(securePath, testServiceSID); err != nil {
		t.Fatalf("ConfigureSecureDirectoryACL failed: %v", err)
	}

	// 2. Verify the effective DACL on disk
	res, err := win.VerifySecureDirectoryACL(securePath, testServiceSID)
	if err != nil {
		t.Fatalf("VerifySecureDirectoryACL failed: %v (SDDL: %s)", err, res.SDDL)
	}

	if !res.InheritanceProtected {
		t.Errorf("expected inheritance to be protected")
	}
	if !res.HasSystemFullControl {
		t.Errorf("expected SYSTEM to have full control")
	}
	if !res.HasAdminsFullControl {
		t.Errorf("expected BUILTIN\\Administrators to have full control")
	}
	if !res.HasServiceSIDFullControl {
		t.Errorf("expected Service SID %s to have full control", testServiceSID)
	}
	if !res.GenericLocalServiceDenied {
		t.Errorf("CRITICAL SECURITY VIOLATION: Generic LocalService was not denied!")
	}
	if !res.OrdinaryUsersDenied {
		t.Errorf("expected ordinary users to be denied")
	}
}

func TestSecureDirectoryACL_GenericLocalServiceLeakDetection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-acl-leak-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	securePath := filepath.Join(tmpDir, "leaky_secure")
	_ = os.MkdirAll(securePath, 0700)

	// Intentionally construct an insecure SDDL that grants generic LocalService (LS)
	badSDDL := "D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GA;;;LS)(A;OICI;GA;;;" + win.ProductionServiceSID + ")"
	_ = setRawSDDL(securePath, badSDDL)

	_, errVerify := win.VerifySecureDirectoryACL(securePath, win.ProductionServiceSID)
	if errVerify == nil {
		t.Fatalf("SECURITY FAILURE: VerifySecureDirectoryACL must fail when generic LocalService is granted access!")
	}
	if !errors.Is(errVerify, win.ErrACLGenericLocalServiceLeaked) {
		t.Errorf("expected ErrACLGenericLocalServiceLeaked, got: %v", errVerify)
	}
}

func setRawSDDL(path string, sddl string) error {
	sddlPtr, _ := syscall.UTF16PtrFromString(sddl)
	var pSD uintptr
	rConv, _, _ := win.ProcConvertStringSDToSDW.Call(
		uintptr(unsafe.Pointer(sddlPtr)),
		win.SDDL_REVISION_1,
		uintptr(unsafe.Pointer(&pSD)),
		0,
	)
	if rConv == 0 {
		return errors.New("convert failed")
	}
	defer win.ProcLocalFree.Call(pSD)

	var pDacl uintptr
	var daclPresent, daclDefaulted int32
	win.ProcGetSecurityDescriptorDacl.Call(
		pSD,
		uintptr(unsafe.Pointer(&daclPresent)),
		uintptr(unsafe.Pointer(&pDacl)),
		uintptr(unsafe.Pointer(&daclDefaulted)),
	)

	dirPtr, _ := syscall.UTF16PtrFromString(path)
	win.ProcSetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		win.SE_FILE_OBJECT,
		uintptr(win.DACL_SECURITY_INFORMATION|win.PROTECTED_DACL_SECURITY_INFORMATION),
		0,
		0,
		pDacl,
		0,
	)
	return nil
}
