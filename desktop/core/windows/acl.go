//go:build windows

package windows

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	ProductionServiceSID   = "S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172"
	GenericLocalServiceSID = "S-1-5-19"
	SystemSID              = "S-1-5-18"
	AdminsSID              = "S-1-5-32-544"
)

var (
	ErrACLInheritanceNotProtected   = errors.New("secure directory inheritance is not protected")
	ErrACLMissingServiceSID         = errors.New("secure directory is missing Service SID grant")
	ErrACLMissingSystemOrAdmins     = errors.New("secure directory is missing SYSTEM or Administrators grant")
	ErrACLGenericLocalServiceLeaked = errors.New("CRITICAL SECURITY ERROR: generic LocalService has access to secure directory")
	ErrACLOrdinaryUsersLeaked       = errors.New("ordinary users or interactive principals have access to secure directory")
)

type ACLVerificationResult struct {
	InheritanceProtected      bool   `json:"inheritanceProtected"`
	HasSystemFullControl      bool   `json:"hasSystemFullControl"`
	HasAdminsFullControl      bool   `json:"hasAdminsFullControl"`
	HasServiceSIDFullControl  bool   `json:"hasServiceSIDFullControl"`
	ServiceSID                string `json:"serviceSid"`
	GenericLocalServiceDenied bool   `json:"genericLocalServiceDenied"`
	OrdinaryUsersDenied       bool   `json:"ordinaryUsersDenied"`
	SDDL                      string `json:"sddl"`
}

// BuildSecureDirSDDL returns the explicit protected SDDL string for sensitive directories.
func BuildSecureDirSDDL(serviceSID string) string {
	if serviceSID == "" {
		serviceSID = ProductionServiceSID
	}
	// D:P = Protected DACL (inheritance blocked)
	// (A;OICI;GA;;;SY) = SYSTEM Full Control (Object Inherit + Container Inherit)
	// (A;OICI;GA;;;BA) = Administrators Full Control (Object Inherit + Container Inherit)
	// (A;OICI;GA;;;serviceSID) = Service SID Full Control (Object Inherit + Container Inherit)
	return fmt.Sprintf("D:P(A;OICI;GA;;;SY)(A;OICI;GA;;;BA)(A;OICI;GA;;;%s)", serviceSID)
}

// ConfigureSecureDirectoryACL creates or updates directory permissions with strict ACL isolation.
func ConfigureSecureDirectoryACL(dirPath string, serviceSID string) error {
	if err := os.MkdirAll(dirPath, 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	sddl := BuildSecureDirSDDL(serviceSID)
	sddlPtr, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		return err
	}

	var pSD uintptr
	rConv, _, errConv := ProcConvertStringSDToSDW.Call(
		uintptr(unsafe.Pointer(sddlPtr)),
		SDDL_REVISION_1,
		uintptr(unsafe.Pointer(&pSD)),
		0,
	)
	if rConv == 0 {
		return fmt.Errorf("ConvertStringSecurityDescriptorToSecurityDescriptorW failed: %w", errConv)
	}
	defer ProcLocalFree.Call(pSD)

	var pDacl uintptr
	var daclPresent, daclDefaulted int32
	rDacl, _, errDacl := ProcGetSecurityDescriptorDacl.Call(
		pSD,
		uintptr(unsafe.Pointer(&daclPresent)),
		uintptr(unsafe.Pointer(&pDacl)),
		uintptr(unsafe.Pointer(&daclDefaulted)),
	)
	if rDacl == 0 {
		return fmt.Errorf("GetSecurityDescriptorDacl failed: %w", errDacl)
	}

	dirPtr, err := syscall.UTF16PtrFromString(dirPath)
	if err != nil {
		return err
	}

	rSet, _, errSet := ProcSetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		SE_FILE_OBJECT,
		uintptr(DACL_SECURITY_INFORMATION|PROTECTED_DACL_SECURITY_INFORMATION),
		0,
		0,
		pDacl,
		0,
	)
	if rSet != 0 {
		return fmt.Errorf("SetNamedSecurityInfoW failed (win32 err %d): %w", rSet, errSet)
	}

	return nil
}

// VerifySecureDirectoryACL inspects the effective DACL on disk and confirms security invariants.
func VerifySecureDirectoryACL(dirPath string, expectedServiceSID string) (*ACLVerificationResult, error) {
	if expectedServiceSID == "" {
		expectedServiceSID = ProductionServiceSID
	}

	dirPtr, err := syscall.UTF16PtrFromString(dirPath)
	if err != nil {
		return nil, err
	}

	var pDacl uintptr
	var pSD uintptr
	rGet, _, errGet := ProcGetNamedSecurityInfoW.Call(
		uintptr(unsafe.Pointer(dirPtr)),
		SE_FILE_OBJECT,
		uintptr(DACL_SECURITY_INFORMATION),
		0,
		0,
		uintptr(unsafe.Pointer(&pDacl)),
		0,
		uintptr(unsafe.Pointer(&pSD)),
	)
	if rGet != 0 {
		return nil, fmt.Errorf("GetNamedSecurityInfoW failed (win32 err %d): %w", rGet, errGet)
	}
	if pSD != 0 {
		defer ProcLocalFree.Call(pSD)
	}

	var pSDDL *uint16
	var sddlLen uint32
	rConv, _, errConv := ProcConvertSDToStringSDW.Call(
		pSD,
		SDDL_REVISION_1,
		uintptr(DACL_SECURITY_INFORMATION),
		uintptr(unsafe.Pointer(&pSDDL)),
		uintptr(unsafe.Pointer(&sddlLen)),
	)
	if rConv == 0 {
		return nil, fmt.Errorf("ConvertSecurityDescriptorToStringSecurityDescriptorW failed: %w", errConv)
	}
	defer ProcLocalFree.Call(uintptr(unsafe.Pointer(pSDDL)))

	sddl := syscall.UTF16ToString((*[4096]uint16)(unsafe.Pointer(pSDDL))[:])

	res := &ACLVerificationResult{
		SDDL:       sddl,
		ServiceSID: expectedServiceSID,
	}

	// 1. Inheritance check: D:P or D:PAI indicates protected DACL (inheritance blocked)
	if strings.Contains(sddl, "D:P") || strings.Contains(sddl, "D:PAI") {
		res.InheritanceProtected = true
	}

	// 2. SYSTEM Full Control check
	if strings.Contains(sddl, ";;;SY)") || strings.Contains(sddl, ";;;S-1-5-18)") {
		res.HasSystemFullControl = true
	}

	// 3. Administrators Full Control check
	if strings.Contains(sddl, ";;;BA)") || strings.Contains(sddl, ";;;S-1-5-32-544)") {
		res.HasAdminsFullControl = true
	}

	// 4. Unique Service SID check
	if strings.Contains(sddl, expectedServiceSID) {
		res.HasServiceSIDFullControl = true
	}

	// 5. Generic LocalService denial check: must NOT have any grant
	if !strings.Contains(sddl, ";;;LS)") && !strings.Contains(sddl, ";;;S-1-5-19)") {
		res.GenericLocalServiceDenied = true
	}

	// 6. Ordinary users denial check: BU (Users), AU (Authenticated Users), IU (Interactive Users), WD (Everyone)
	hasUserLeak := strings.Contains(sddl, ";;;BU)") ||
		strings.Contains(sddl, ";;;AU)") ||
		strings.Contains(sddl, ";;;IU)") ||
		strings.Contains(sddl, ";;;WD)") ||
		strings.Contains(sddl, ";;;S-1-5-32-545)")

	if !hasUserLeak {
		res.OrdinaryUsersDenied = true
	}

	// Validate critical invariants
	if !res.InheritanceProtected {
		return res, fmt.Errorf("%w: SDDL=%s", ErrACLInheritanceNotProtected, sddl)
	}
	if !res.HasSystemFullControl || !res.HasAdminsFullControl {
		return res, fmt.Errorf("%w: SDDL=%s", ErrACLMissingSystemOrAdmins, sddl)
	}
	if !res.HasServiceSIDFullControl {
		return res, fmt.Errorf("%w: expected %s, SDDL=%s", ErrACLMissingServiceSID, expectedServiceSID, sddl)
	}
	if !res.GenericLocalServiceDenied {
		return res, fmt.Errorf("%w: SDDL=%s", ErrACLGenericLocalServiceLeaked, sddl)
	}
	if !res.OrdinaryUsersDenied {
		return res, fmt.Errorf("%w: SDDL=%s", ErrACLOrdinaryUsersLeaked, sddl)
	}

	return res, nil
}

