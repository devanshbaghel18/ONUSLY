//go:build windows

package auth

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

// DPAPICredentialStore persists the account JWT encrypted at machine scope using Windows DPAPI.
//
// SECURITY BOUNDARY INVARIANT:
// DPAPI machine scope (CRYPTPROTECT_LOCAL_MACHINE) allows any code running under SYSTEM,
// Administrators, or LocalService to decrypt the data. It does NOT provide per-user cryptographic
// isolation. The actual access boundary is enforced by NTFS ACLs on %ProgramData%\Onusly\secure,
// granting access strictly to LocalService (the service account), Administrators, and SYSTEM.
type DPAPICredentialStore struct {
	tokenPath string
	mu        sync.RWMutex
}

func NewDPAPICredentialStore(tokenPath string) *DPAPICredentialStore {
	return &DPAPICredentialStore{
		tokenPath: tokenPath,
	}
}

// SaveToken encrypts the plaintext JWT using DPAPI machine scope and atomically persists it.
func (s *DPAPICredentialStore) SaveToken(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return errors.New("cannot save empty token")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Dir(s.tokenPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create secure directory %s: %w", dir, err)
	}

	// 1. DPAPI CryptProtectData
	plainBytes := []byte(token)
	if len(plainBytes) > 65536 {
		return errors.New("token exceeds maximum allowed size (64KB)")
	}
	defer func() {
		for i := range plainBytes {
			plainBytes[i] = 0
		}
	}()
	var inBlob win.DATA_BLOB
	inBlob.CbData = uint32(len(plainBytes))
	inBlob.PbData = &plainBytes[0]

	var outBlob win.DATA_BLOB
	r, _, errCall := win.ProcCryptProtectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0, // szDataDescr
		0, // pOptionalEntropy
		0, // pvReserved
		0, // pPromptStruct
		uintptr(win.CRYPTPROTECT_LOCAL_MACHINE|win.CRYPTPROTECT_UI_FORBIDDEN),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 || outBlob.PbData == nil {
		return fmt.Errorf("CryptProtectData failed: %w", errCall)
	}
	defer win.ProcLocalFree.Call(uintptr(unsafe.Pointer(outBlob.PbData)))

	cipherBytes := unsafe.Slice(outBlob.PbData, outBlob.CbData)

	// 2. Atomic write via .tmp file with FlushFileBuffers
	tmpPath := fmt.Sprintf("%s.tmp.%d", s.tokenPath, os.Getpid())
	tmpPtr, err := syscall.UTF16PtrFromString(tmpPath)
	if err != nil {
		return err
	}

	hTmp, _, errCreate := win.ProcCreateFileW.Call(
		uintptr(unsafe.Pointer(tmpPtr)),
		uintptr(win.GENERIC_READ|win.GENERIC_WRITE),
		0,
		0,
		uintptr(win.OPEN_EXISTING),
		0,
		0,
	)
	// If file doesn't exist, create it (CREATE_ALWAYS = 2)
	if hTmp == uintptr(syscall.InvalidHandle) || hTmp == 0 {
		hTmp, _, errCreate = win.ProcCreateFileW.Call(
			uintptr(unsafe.Pointer(tmpPtr)),
			uintptr(win.GENERIC_READ|win.GENERIC_WRITE),
			0,
			0,
			2, // CREATE_ALWAYS
			0x00000080, // FILE_ATTRIBUTE_NORMAL
			0,
		)
		if hTmp == uintptr(syscall.InvalidHandle) || hTmp == 0 {
			return fmt.Errorf("failed to create temporary token file %s: %w", tmpPath, errCreate)
		}
	}

	var bytesWritten uint32
	rWrite, _, errWrite := win.ProcWriteFile.Call(
		hTmp,
		uintptr(unsafe.Pointer(&cipherBytes[0])),
		uintptr(len(cipherBytes)),
		uintptr(unsafe.Pointer(&bytesWritten)),
		0,
	)
	if rWrite == 0 || bytesWritten != uint32(len(cipherBytes)) {
		win.ProcCloseHandle.Call(hTmp)
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write token ciphertext: %w", errWrite)
	}

	// Guarantee durability before atomic replace
	win.ProcFlushFileBuffers.Call(hTmp)
	win.ProcCloseHandle.Call(hTmp)

	// 3. Separate First-Install (MoveFileEx) vs Re-Pair (ReplaceFile)
	dstPtr, err := syscall.UTF16PtrFromString(s.tokenPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	if _, errStat := os.Stat(s.tokenPath); os.IsNotExist(errStat) {
		// Destination does not exist: use MoveFileExW with MOVEFILE_WRITE_THROUGH
		rMove, _, errMove := win.ProcMoveFileExW.Call(
			uintptr(unsafe.Pointer(tmpPtr)),
			uintptr(unsafe.Pointer(dstPtr)),
			uintptr(win.MOVEFILE_REPLACE_EXISTING|win.MOVEFILE_WRITE_THROUGH),
		)
		if rMove == 0 {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("MoveFileExW failed to commit token: %w", errMove)
		}
	} else {
		// Destination exists: use ReplaceFileW with backup
		bakPath := s.tokenPath + ".bak"
		bakPtr, _ := syscall.UTF16PtrFromString(bakPath)
		rRep, _, errRep := win.ProcReplaceFileW.Call(
			uintptr(unsafe.Pointer(dstPtr)),
			uintptr(unsafe.Pointer(tmpPtr)),
			uintptr(unsafe.Pointer(bakPtr)),
			uintptr(win.REPLACEFILE_WRITE_THROUGH),
			0,
			0,
		)
		if rRep == 0 {
			// Fallback to MoveFileExW if ReplaceFileW fails
			rMove, _, errMove := win.ProcMoveFileExW.Call(
				uintptr(unsafe.Pointer(tmpPtr)),
				uintptr(unsafe.Pointer(dstPtr)),
				uintptr(win.MOVEFILE_REPLACE_EXISTING|win.MOVEFILE_WRITE_THROUGH),
			)
			if rMove == 0 {
				_ = os.Remove(tmpPath)
				return fmt.Errorf("ReplaceFileW and MoveFileExW failed: %v / %w", errRep, errMove)
			}
		}
		// Clean up backup once replacement succeeded
		_ = os.Remove(bakPath)
	}

	return nil
}

// GetToken decrypts and returns the persisted JWT. Returns ("", nil) if the file does not exist.
func (s *DPAPICredentialStore) GetToken() (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cipherBytes, err := os.ReadFile(s.tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Check if .bak exists for interrupted replacement recovery
			bakBytes, errBak := os.ReadFile(s.tokenPath + ".bak")
			if errBak == nil && len(bakBytes) > 0 {
				cipherBytes = bakBytes
			} else {
				return "", nil
			}
		} else {
			return "", err
		}
	}

	if len(cipherBytes) == 0 {
		return "", errors.New("token file is empty")
	}
	if len(cipherBytes) > 65536 {
		return "", errors.New("token file exceeds maximum allowed size (64KB)")
	}

	var inBlob win.DATA_BLOB
	inBlob.CbData = uint32(len(cipherBytes))
	inBlob.PbData = &cipherBytes[0]

	var outBlob win.DATA_BLOB
	r, _, errCall := win.ProcCryptUnprotectData.Call(
		uintptr(unsafe.Pointer(&inBlob)),
		0,
		0,
		0,
		0,
		uintptr(win.CRYPTPROTECT_UI_FORBIDDEN),
		uintptr(unsafe.Pointer(&outBlob)),
	)
	if r == 0 || outBlob.PbData == nil {
		return "", fmt.Errorf("CryptUnprotectData failed (corrupted or wrong machine scope): %w", errCall)
	}
	defer win.ProcLocalFree.Call(uintptr(unsafe.Pointer(outBlob.PbData)))

	plainBytes := unsafe.Slice(outBlob.PbData, outBlob.CbData)
	res := strings.TrimSpace(string(plainBytes))
	for i := range plainBytes {
		plainBytes[i] = 0
	}
	return res, nil
}

// ClearToken securely removes the token file and any lingering backups.
func (s *DPAPICredentialStore) ClearToken() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_ = os.Remove(s.tokenPath + ".bak")
	err := os.Remove(s.tokenPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
