//go:build windows

package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDPAPICredentialStore_SaveAndGet(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-dpapi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	store := NewDPAPICredentialStore(tokenFile)

	testJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTEyMzQiLCJleHAiOjI1MzQwMjMwMDd9.fake-signature-bytes"

	// 1. Initially empty
	tok, err := store.GetToken()
	if err != nil {
		t.Fatalf("Expected nil error for non-existent token, got %v", err)
	}
	if tok != "" {
		t.Fatalf("Expected empty token, got %q", tok)
	}

	// 2. Save
	if err := store.SaveToken(testJWT); err != nil {
		t.Fatalf("SaveToken failed: %v", err)
	}

	// Verify ciphertext file is not plaintext
	rawBytes, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Fatalf("Failed to read token file: %v", err)
	}
	if string(rawBytes) == testJWT {
		t.Fatalf("Token was stored as plaintext instead of DPAPI ciphertext!")
	}

	// 3. Get back
	decrypted, err := store.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed: %v", err)
	}
	if decrypted != testJWT {
		t.Fatalf("Decrypted token mismatch: got %q, expected %q", decrypted, testJWT)
	}

	// 4. Overwrite (re-pair)
	testJWT2 := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLTU2NzgiLCJleHAiOjI1MzQwMjMwMDd9.second-signature-bytes"
	if err := store.SaveToken(testJWT2); err != nil {
		t.Fatalf("Overwriting token failed: %v", err)
	}
	decrypted2, err := store.GetToken()
	if err != nil {
		t.Fatalf("GetToken failed after overwrite: %v", err)
	}
	if decrypted2 != testJWT2 {
		t.Fatalf("Overwritten token mismatch: got %q, expected %q", decrypted2, testJWT2)
	}

	// 5. Clear
	if err := store.ClearToken(); err != nil {
		t.Fatalf("ClearToken failed: %v", err)
	}
	tokAfterClear, err := store.GetToken()
	if err != nil {
		t.Fatalf("GetToken error after clear: %v", err)
	}
	if tokAfterClear != "" {
		t.Fatalf("Expected empty token after clear, got %q", tokAfterClear)
	}
}

func TestDPAPICredentialStore_EmptyTokenRejection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-dpapi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewDPAPICredentialStore(filepath.Join(tmpDir, "token.dat"))
	if err := store.SaveToken(""); err == nil {
		t.Fatalf("Expected error when saving empty token, got nil")
	}
	if err := store.SaveToken("   "); err == nil {
		t.Fatalf("Expected error when saving whitespace token, got nil")
	}
}

func TestDPAPICredentialStore_CorruptDataRecovery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-dpapi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	store := NewDPAPICredentialStore(tokenFile)

	// Write garbage ciphertext
	if err := os.WriteFile(tokenFile, []byte("NOT_A_VALID_DPAPI_BLOB_CORRUPTED"), 0600); err != nil {
		t.Fatalf("Failed to write corrupt file: %v", err)
	}

	_, err = store.GetToken()
	if err == nil {
		t.Fatalf("Expected error on corrupted DPAPI ciphertext, got nil")
	}
}

func TestDPAPICredentialStore_BakRecovery(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-dpapi-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	store := NewDPAPICredentialStore(tokenFile)

	testJWT := "valid.jwt.token"
	if err := store.SaveToken(testJWT); err != nil {
		t.Fatalf("SaveToken failed: %v", err)
	}

	// Simulate crash: rename token.dat to token.dat.bak
	if err := os.Rename(tokenFile, tokenFile+".bak"); err != nil {
		t.Fatalf("Failed to rename to .bak: %v", err)
	}

	// GetToken should fall back to .bak
	recovered, err := store.GetToken()
	if err != nil {
		t.Fatalf("Failed to recover from .bak: %v", err)
	}
	if recovered != testJWT {
		t.Fatalf("Recovered token mismatch: got %q, expected %q", recovered, testJWT)
	}
}

