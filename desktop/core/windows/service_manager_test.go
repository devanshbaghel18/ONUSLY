//go:build windows

package windows_test

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

func TestServiceManager_ConfigurationGeneration(t *testing.T) {
	serviceName := "OnuslyDesktopAgent"
	binPath := `C:\Program Files\Onusly\onusly-agent.exe`

	mgr := win.NewWindowsServiceManager(serviceName, binPath)
	if mgr == nil {
		t.Fatalf("failed to create WindowsServiceManager")
	}

	// Verify constant Service SID matches expected deterministic Service SID
	if win.ProductionServiceSID != "S-1-5-80-3463423020-2351545225-1748562058-3133284821-4013307172" {
		t.Errorf("unexpected ProductionServiceSID: %s", win.ProductionServiceSID)
	}

	// Verify SDDL generation contains required principals and blocks generic LocalService
	sddl := win.BuildSecureDirSDDL(win.ProductionServiceSID)
	if !strings.HasPrefix(sddl, "D:P") {
		t.Errorf("SDDL must protect inheritance (D:P), got: %s", sddl)
	}
	if !strings.Contains(sddl, ";;;SY)") {
		t.Errorf("SDDL must grant SYSTEM full control, got: %s", sddl)
	}
	if !strings.Contains(sddl, ";;;BA)") {
		t.Errorf("SDDL must grant Administrators full control, got: %s", sddl)
	}
	if !strings.Contains(sddl, win.ProductionServiceSID) {
		t.Errorf("SDDL must grant unique Service SID, got: %s", sddl)
	}
	if strings.Contains(sddl, ";;;LS)") || strings.Contains(sddl, ";;;S-1-5-19)") {
		t.Errorf("CRITICAL SECURITY DEFECT: SDDL must NOT grant generic LocalService, got: %s", sddl)
	}
}

func TestServiceManager_NonExistentServiceQuery(t *testing.T) {
	mgr := win.NewWindowsServiceManager("NonExistentBogusService9999", `C:\bogus.exe`)
	running, err := mgr.IsRunning()
	if running {
		t.Errorf("non-existent service should not be reported as running")
	}
	if err == nil {
		t.Errorf("expected error querying non-existent service, got nil")
	}
}

func TestServiceManager_ArgumentConstruction(t *testing.T) {
	serviceName := "OnuslyDesktopAgent"
	binPathWithSpaces := `C:\Program Files\Onusly\onusly-agent.exe`

	mgr := win.NewWindowsServiceManager(serviceName, binPathWithSpaces)

	// 1. Verify Create arguments
	createArgs := mgr.BuildCreateArgs()
	if len(createArgs) != 12 {
		t.Fatalf("expected 12 arguments for sc.exe create, got %d: %v", len(createArgs), createArgs)
	}

	if createArgs[0] != "create" || createArgs[1] != serviceName {
		t.Errorf("unexpected command or service name: %v", createArgs[:2])
	}

	expectedBinArg := `"` + binPathWithSpaces + `" run`
	foundBin := false
	foundObj := false
	foundDisplay := false
	for i := 0; i < len(createArgs)-1; i++ {
		if createArgs[i] == "binPath=" && createArgs[i+1] == expectedBinArg {
			foundBin = true
		}
		if createArgs[i] == "obj=" && createArgs[i+1] == `NT AUTHORITY\LocalService` {
			foundObj = true
		}
		if createArgs[i] == "DisplayName=" && createArgs[i+1] == "ONUSLY Desktop Enforcement Service" {
			foundDisplay = true
		}
	}

	if !foundBin {
		t.Errorf("expected binPath= %q in createArgs, got: %v", expectedBinArg, createArgs)
	}
	if !foundObj {
		t.Errorf("expected obj= NT AUTHORITY\\LocalService in createArgs, got: %v", createArgs)
	}
	if !foundDisplay {
		t.Errorf("expected DisplayName= ONUSLY Desktop Enforcement Service in createArgs, got: %v", createArgs)
	}

	// 2. Verify Config arguments
	configArgs := mgr.BuildUpdateConfigArgs()
	if configArgs[0] != "config" || configArgs[1] != serviceName {
		t.Errorf("unexpected command or service name for config: %v", configArgs[:2])
	}
}

func TestServiceManager_ScSyntaxValidation(t *testing.T) {
	serviceName := "OnuslyTestSyntaxService"
	binPath := `C:\Program Files\Onusly\onusly-agent.exe`
	mgr := win.NewWindowsServiceManager(serviceName, binPath)

	createArgs := mgr.BuildCreateArgs()
	cmd := exec.Command("sc.exe", createArgs...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	// Critical check: sc.exe MUST NOT reject the arguments as invalid syntax / usage error.
	if strings.Contains(outStr, "USAGE:") || strings.Contains(outStr, "DESCRIPTION:") {
		t.Fatalf("sc.exe rejected argument syntax as invalid USAGE! Output: %s", outStr)
	}

	// In this non-elevated test context, we expect Access Denied (error 5) from OpenSCManager,
	// which confirms all arguments were syntactically valid and parsed by sc.exe.
	if err == nil {
		// If running elevated, service was created; clean up
		_ = exec.Command("sc.exe", "delete", serviceName).Run()
	} else if !strings.Contains(outStr, "5:") && !strings.Contains(outStr, "Access is denied") && !strings.Contains(outStr, "1073") {
		t.Logf("sc.exe returned non-syntax output (expected): %s", outStr)
	}
}

func TestServiceManager_PartialInstallRecovery(t *testing.T) {
	// Querying a non-existent or partially configured service should return an error
	mgr := win.NewWindowsServiceManager("OnuslyPartialBogusService", `C:\bogus\path.exe`)
	cfg, err := mgr.QueryConfig()
	if err == nil {
		t.Errorf("expected error querying non-existent service, got config: %+v", cfg)
	}

	// Empty binary path must be rejected before touching SCM
	emptyMgr := win.NewWindowsServiceManager("OnuslyEmptyBin", "")
	if err := emptyMgr.InstallService(); err == nil {
		t.Errorf("expected error when installing with empty binary path")
	}
}

func TestTokenGroups_MemoryLayoutAndParsing(t *testing.T) {
	// Verify exact 64-bit alignment offsets
	groupsOffset := unsafe.Offsetof(win.TOKEN_GROUPS{}.Groups)
	if groupsOffset != 8 {
		t.Fatalf("expected TOKEN_GROUPS.Groups offset 8 on 64-bit Windows, got %d", groupsOffset)
	}

	saSize := unsafe.Sizeof(win.SID_AND_ATTRIBUTES{})
	if saSize != 16 {
		t.Fatalf("expected SID_AND_ATTRIBUTES size 16 on 64-bit Windows, got %d", saSize)
	}

	// Construct a synthetic 2-group buffer with known pointers
	dummySid1 := uintptr(0x1000)
	dummySid2 := uintptr(0x2000)
	buf := make([]byte, 8+2*16)
	*(*uint32)(unsafe.Pointer(&buf[0])) = 2 // GroupCount = 2

	// Group 0 at offset 8
	*(*uintptr)(unsafe.Pointer(&buf[8])) = dummySid1
	*(*uint32)(unsafe.Pointer(&buf[16])) = 0x00000007 // Attributes

	// Group 1 at offset 24
	*(*uintptr)(unsafe.Pointer(&buf[24])) = dummySid2
	*(*uint32)(unsafe.Pointer(&buf[32])) = 0x0000000e // Attributes

	parsed := win.ParseTokenGroups(buf)
	if len(parsed) != 2 {
		t.Fatalf("expected 2 parsed groups, got %d", len(parsed))
	}

	if parsed[0].Sid != dummySid1 || parsed[0].Attributes != 0x00000007 {
		t.Errorf("unexpected Group 0: Sid=0x%x Attr=0x%x", parsed[0].Sid, parsed[0].Attributes)
	}
	if parsed[1].Sid != dummySid2 || parsed[1].Attributes != 0x0000000e {
		t.Errorf("unexpected Group 1: Sid=0x%x Attr=0x%x", parsed[1].Sid, parsed[1].Attributes)
	}
}

func TestTokenGroups_ServiceSIDBinaryAndStringMatching(t *testing.T) {
	// Allocate real Windows SIDs
	pTargetSid, err1 := win.StringToSid(win.ProductionServiceSID)
	if err1 != nil || pTargetSid == 0 {
		t.Fatalf("failed to convert ProductionServiceSID to binary SID: %v", err1)
	}
	defer win.ProcLocalFree.Call(pTargetSid)

	pOtherSid, err2 := win.StringToSid(win.GenericLocalServiceSID)
	if err2 != nil || pOtherSid == 0 {
		t.Fatalf("failed to convert GenericLocalServiceSID to binary SID: %v", err2)
	}
	defer win.ProcLocalFree.Call(pOtherSid)

	// Build synthetic TOKEN_GROUPS buffer with 2 real SIDs
	buf := make([]byte, 8+2*16)
	*(*uint32)(unsafe.Pointer(&buf[0])) = 2 // GroupCount = 2

	// Group 0: GenericLocalServiceSID
	*(*uintptr)(unsafe.Pointer(&buf[8])) = pOtherSid
	*(*uint32)(unsafe.Pointer(&buf[16])) = 0x00000007

	// Group 1: ProductionServiceSID
	*(*uintptr)(unsafe.Pointer(&buf[24])) = pTargetSid
	*(*uint32)(unsafe.Pointer(&buf[32])) = 0x0000000e

	groups := win.ParseTokenGroups(buf)
	if len(groups) != 2 {
		t.Fatalf("expected 2 parsed groups, got %d", len(groups))
	}

	foundBinary := false
	foundString := false

	for _, g := range groups {
		if win.EqualSid(g.Sid, pTargetSid) {
			foundBinary = true
		}
		str, _ := win.SidToString(g.Sid)
		if strings.EqualFold(str, win.ProductionServiceSID) {
			foundString = true
		}
	}

	if !foundBinary {
		t.Errorf("EqualSid failed to match binary Service SID in parsed token groups")
	}
	if !foundString {
		t.Errorf("SidToString failed to match string Service SID in parsed token groups")
	}
}

func TestServiceManager_ValidatePE_ValidExecutable(t *testing.T) {
	self, err := exec.LookPath(t.TempDir()) // fallback placeholder
	if exe, errExe := os.Executable(); errExe == nil {
		self = exe
	}
	if err != nil && self == "" {
		t.Skip("unable to find executable for test")
	}

	if err := win.ValidatePE(self); err != nil {
		t.Fatalf("ValidatePE failed on current test binary %s: %v", self, err)
	}
}

func TestServiceManager_ValidatePE_InvalidCases(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Non-existent file
	if err := win.ValidatePE(tempDir + `\nonexistent.exe`); err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}

	// 2. Truncated file (< 64 bytes)
	truncPath := tempDir + `\trunc.exe`
	if err := os.WriteFile(truncPath, []byte("MZshort"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := win.ValidatePE(truncPath); err == nil || !errors.Is(err, win.ErrInvalidPE) {
		t.Errorf("expected ErrInvalidPE for truncated file, got: %v", err)
	}

	// 3. Invalid DOS magic
	badMZPath := tempDir + `\badmz.exe`
	badMZ := make([]byte, 128)
	copy(badMZ, []byte("NOT_AN_MZ_BINARY"))
	if err := os.WriteFile(badMZPath, badMZ, 0644); err != nil {
		t.Fatal(err)
	}
	if err := win.ValidatePE(badMZPath); err == nil || !strings.Contains(err.Error(), "missing MZ signature") {
		t.Errorf("expected missing MZ signature error, got: %v", err)
	}

	// 4. Invalid PE offset (< 64)
	badOffsetPath := tempDir + `\badoffset.exe`
	badOffsetData := make([]byte, 128)
	badOffsetData[0] = 'M'
	badOffsetData[1] = 'Z'
	badOffsetData[0x3C] = 16 // peOffset = 16 < 64
	if err := os.WriteFile(badOffsetPath, badOffsetData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := win.ValidatePE(badOffsetPath); err == nil || !strings.Contains(err.Error(), "invalid PE header offset") {
		t.Errorf("expected invalid PE offset error, got: %v", err)
	}

	// 5. Invalid PE signature (not PE\0\0)
	badSigPath := tempDir + `\badsig.exe`
	badSigData := make([]byte, 256)
	badSigData[0] = 'M'
	badSigData[1] = 'Z'
	badSigData[0x3C] = 128 // peOffset = 128
	copy(badSigData[128:], []byte("NE\x00\x00"))
	if err := os.WriteFile(badSigPath, badSigData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := win.ValidatePE(badSigPath); err == nil || !strings.Contains(err.Error(), "missing PE signature") {
		t.Errorf("expected missing PE signature error, got: %v", err)
	}

	// 6. 32-bit i386 machine (0x014c) instead of 0x8664 AMD64
	x86Path := tempDir + `\x86.exe`
	x86Data := make([]byte, 256)
	x86Data[0] = 'M'
	x86Data[1] = 'Z'
	x86Data[0x3C] = 128 // peOffset = 128
	copy(x86Data[128:], []byte("PE\x00\x00"))
	x86Data[132] = 0x4C // 0x014c i386
	x86Data[133] = 0x01
	if err := os.WriteFile(x86Path, x86Data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := win.ValidatePE(x86Path); err == nil || !strings.Contains(err.Error(), "unsupported machine architecture") {
		t.Errorf("expected unsupported machine architecture error, got: %v", err)
	}

	// 7. Synthetic valid AMD64 PE executable
	validData := make([]byte, 256)
	validData[0] = 'M'
	validData[1] = 'Z'
	validData[0x3C] = 128
	copy(validData[128:], []byte("PE\x00\x00"))
	validData[132] = 0x64 // 0x8664 AMD64
	validData[133] = 0x86
	validPath := tempDir + `\valid_synthetic.exe`
	if err := os.WriteFile(validPath, validData, 0644); err != nil {
		t.Fatal(err)
	}
	if err := win.ValidatePE(validPath); err != nil {
		t.Errorf("expected valid synthetic AMD64 PE to pass, got: %v", err)
	}
}

func TestServiceManager_StopServiceWithTimeout_NonExistent(t *testing.T) {
	mgr := win.NewWindowsServiceManager("NonExistentBogusServiceForStop", `C:\bogus.exe`)
	// Stopping a non-existent service should return nil without blocking
	err := mgr.StopServiceWithTimeout(500 * time.Millisecond)
	if err != nil {
		t.Errorf("expected nil error stopping non-existent service, got: %v", err)
	}
}


