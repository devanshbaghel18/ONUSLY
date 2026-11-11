package update_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)

func generateTestKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 test keypair: %v", err)
	}
	return pub, priv
}

func createTestArtifact(t *testing.T, dir, filename, content string) (string, int64, string) {
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test artifact: %v", err)
	}
	sha, size, err := update.ComputeFileSHA256(path)
	if err != nil {
		t.Fatalf("failed to compute test artifact sha: %v", err)
	}
	return path, size, sha
}

func TestUpdateVerifier_Comprehensive12PointChecks(t *testing.T) {
	pubKey, privKey := generateTestKeyPair(t)
	tmpDir, err := os.MkdirTemp("", "onusly-verifier-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	artDir := filepath.Join(tmpDir, "artifacts")
	_ = os.MkdirAll(artDir, 0700)

	artPath, artSize, artSha := createTestArtifact(t, artDir, "onusly-agent-windows-amd64.exe", "binary payload v1.1.0")

	antiRollbackPath := filepath.Join(tmpDir, "anti_rollback.json")
	rollbackMgr := update.NewAntiRollbackManager(antiRollbackPath)
	// Seed current version 1.0.0, security version 1
	_ = rollbackMgr.RecordAccepted(&update.Manifest{
		ReleaseVersion:  "1.0.0",
		SecurityVersion: 1,
	})

	cfg := update.VerifierConfig{
		TrustedPublicKey: pubKey,
		CurrentOS:        "windows",
		CurrentArch:      "amd64",
		CurrentChannel:   "stable",
		CurrentVersion:   "1.0.0",
		RollbackMgr:      rollbackMgr,
	}

	verifier, err := update.NewUpdateVerifier(cfg)
	if err != nil {
		t.Fatalf("failed to create verifier: %v", err)
	}

	now := time.Now().UTC()
	baseManifest := update.Manifest{
		SchemaVersion:   1,
		ProductID:       "onusly",
		ReleaseVersion:  "1.1.0",
		SecurityVersion: 2,
		ReleaseChannel:  "stable",
		OperatingSystem: "windows",
		Architecture:    "amd64",
		CreatedAt:       now.Format(time.RFC3339),
		ExpiresAt:       now.Add(24 * time.Hour).Format(time.RFC3339),
		Components: []update.ComponentItem{
			{
				ComponentID: "agent",
				Filename:    "onusly-agent-windows-amd64.exe",
				SizeBytes:   artSize,
				SHA256:      artSha,
			},
		},
	}

	// 1. Valid signed manifest and valid artifact passes
	validM := baseManifest
	if err := validM.Sign(privKey); err != nil {
		t.Fatalf("failed to sign manifest: %v", err)
	}

	if err := verifier.VerifyManifest(&validM, false); err != nil {
		t.Fatalf("expected valid manifest to pass, got: %v", err)
	}

	if err := verifier.VerifyArtifact(&validM.Components[0], artPath); err != nil {
		t.Fatalf("expected valid artifact to pass, got: %v", err)
	}

	// 2. Unsigned manifest rejected
	unsignedM := validM
	unsignedM.Signature = ""
	if err := verifier.VerifyManifest(&unsignedM, false); err != update.ErrMissingSignature {
		t.Fatalf("expected ErrMissingSignature, got: %v", err)
	}

	// 3. Tampered signature rejected
	tamperedSigM := validM
	tamperedSigM.Signature = "AAAA" + tamperedSigM.Signature[4:]
	if err := verifier.VerifyManifest(&tamperedSigM, false); err == nil {
		t.Fatalf("expected error on tampered signature, got nil")
	}

	// 4. Tampered manifest field after signing rejected
	tamperedFieldM := validM
	tamperedFieldM.ReleaseVersion = "1.2.0" // changed without re-signing!
	if err := verifier.VerifyManifest(&tamperedFieldM, false); err == nil {
		t.Fatalf("expected signature verification failure after payload tampering, got nil")
	}

	// 5. Wrong operating system rejected
	wrongOSM := baseManifest
	wrongOSM.OperatingSystem = "linux"
	_ = wrongOSM.Sign(privKey)
	if err := verifier.VerifyManifest(&wrongOSM, false); err == nil {
		t.Fatalf("expected error for OS mismatch, got nil")
	}

	// 6. Wrong architecture rejected
	wrongArchM := baseManifest
	wrongArchM.Architecture = "arm64"
	_ = wrongArchM.Sign(privKey)
	if err := verifier.VerifyManifest(&wrongArchM, false); err == nil {
		t.Fatalf("expected error for architecture mismatch, got nil")
	}

	// 7. Wrong release channel rejected
	wrongChanM := baseManifest
	wrongChanM.ReleaseChannel = "beta"
	_ = wrongChanM.Sign(privKey)
	if err := verifier.VerifyManifest(&wrongChanM, false); err == nil {
		t.Fatalf("expected error for channel mismatch, got nil")
	}

	// 8. Expired manifest rejected
	expiredM := baseManifest
	expiredM.CreatedAt = now.Add(-48 * time.Hour).Format(time.RFC3339)
	expiredM.ExpiresAt = now.Add(-24 * time.Hour).Format(time.RFC3339)
	_ = expiredM.Sign(privKey)
	if err := verifier.VerifyManifest(&expiredM, false); !errors.Is(err, update.ErrManifestExpired) {
		t.Fatalf("expected ErrManifestExpired, got: %v", err)
	}

	// 9. Future timestamp rejected
	futureM := baseManifest
	futureM.CreatedAt = now.Add(2 * time.Hour).Format(time.RFC3339)
	futureM.ExpiresAt = now.Add(26 * time.Hour).Format(time.RFC3339)
	_ = futureM.Sign(privKey)
	if err := verifier.VerifyManifest(&futureM, false); !errors.Is(err, update.ErrManifestFutureTimestamp) {
		t.Fatalf("expected ErrManifestFutureTimestamp, got: %v", err)
	}

	// 10. Path traversal in component filename rejected
	traversalM := baseManifest
	traversalM.Components = []update.ComponentItem{
		{
			ComponentID: "agent",
			Filename:    "../../evil.exe",
			SizeBytes:   artSize,
			SHA256:      artSha,
		},
	}
	_ = traversalM.Sign(privKey)
	if err := verifier.VerifyManifest(&traversalM, false); !errors.Is(err, update.ErrPathTraversalDetected) {
		t.Fatalf("expected ErrPathTraversalDetected, got: %v", err)
	}

	// 11. Artifact digest mismatch rejected
	tamperedArtPath := filepath.Join(artDir, "tampered.exe")
	_ = os.WriteFile(tamperedArtPath, []byte("compromised payload"), 0644)
	compSpec := validM.Components[0]
	compSpec.SizeBytes = int64(len("compromised payload"))
	if err := verifier.VerifyArtifact(&compSpec, tamperedArtPath); err == nil {
		t.Fatalf("expected digest mismatch error, got nil")
	}

	// 12. Artifact size mismatch rejected
	compSpecWrongSize := validM.Components[0]
	compSpecWrongSize.SizeBytes = 999999
	if err := verifier.VerifyArtifact(&compSpecWrongSize, artPath); err == nil {
		t.Fatalf("expected size mismatch error, got nil")
	}
}

func TestStagedUpdateDownloader_MockServerAndFailureModes(t *testing.T) {
	pubKey, privKey := generateTestKeyPair(t)
	tmpDir, err := os.MkdirTemp("", "onusly-downloader-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	stagingDir := filepath.Join(tmpDir, "staging")
	antiRollbackPath := filepath.Join(tmpDir, "anti_rollback.json")
	rollbackMgr := update.NewAntiRollbackManager(antiRollbackPath)

	cfg := update.VerifierConfig{
		TrustedPublicKey: pubKey,
		CurrentOS:        "windows",
		CurrentArch:      "amd64",
		CurrentChannel:   "stable",
		CurrentVersion:   "1.0.0",
		RollbackMgr:      rollbackMgr,
	}
	verifier, _ := update.NewUpdateVerifier(cfg)

	now := time.Now().UTC()
	validContent := "valid agent executable content"
	sum := sha256.Sum256([]byte(validContent))
	artSha := hex.EncodeToString(sum[:])
	artSize := int64(len(validContent))

	manifest := update.Manifest{
		SchemaVersion:   1,
		ProductID:       "onusly",
		ReleaseVersion:  "1.1.0",
		SecurityVersion: 2,
		ReleaseChannel:  "stable",
		OperatingSystem: "windows",
		Architecture:    "amd64",
		CreatedAt:       now.Format(time.RFC3339),
		ExpiresAt:       now.Add(24 * time.Hour).Format(time.RFC3339),
		Components: []update.ComponentItem{
			{
				ComponentID: "agent",
				Filename:    "onusly-agent-windows-amd64.exe",
				SizeBytes:   artSize,
				SHA256:      artSha,
			},
		},
	}
	_ = manifest.Sign(privKey)

	// Mock server
	simulateTamper := false
	simulateTruncate := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest.json" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(mustJSON(manifest)))
			return
		}
		if r.URL.Path == "/onusly-agent-windows-amd64.exe" {
			if simulateTamper {
				_, _ = w.Write([]byte("TAMPERED DATA THAT ALTERS HASH"))
				return
			}
			if simulateTruncate {
				_, _ = w.Write([]byte(validContent[:5])) // short write!
				return
			}
			_, _ = w.Write([]byte(validContent))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// 1. Fetch manifest
	fetchedM, err := update.FetchManifest(context.Background(), server.Client(), server.URL+"/manifest.json")
	if err != nil {
		t.Fatalf("FetchManifest failed: %v", err)
	}
	if fetchedM.ReleaseVersion != "1.1.0" {
		t.Fatalf("expected version 1.1.0, got: %s", fetchedM.ReleaseVersion)
	}

	// 2. Stage components cleanly
	staged, err := update.StageComponents(context.Background(), server.Client(), server.URL, fetchedM, stagingDir, verifier, false)
	if err != nil {
		t.Fatalf("StageComponents failed: %v", err)
	}
	agentStagedPath, ok := staged["agent"]
	if !ok || !fileExists(agentStagedPath) {
		t.Fatalf("agent component was not staged properly: %+v", staged)
	}

	// 3. Simulated tampered download is caught and rejected
	simulateTamper = true
	tamperStagingDir := filepath.Join(tmpDir, "staging_tamper")
	_, errTamper := update.StageComponents(context.Background(), server.Client(), server.URL, fetchedM, tamperStagingDir, verifier, false)
	if errTamper == nil {
		t.Fatalf("expected StageComponents to reject tampered download, got nil")
	}

	// 4. Simulated truncated download is caught and rejected
	simulateTamper = false
	simulateTruncate = true
	truncStagingDir := filepath.Join(tmpDir, "staging_trunc")
	_, errTrunc := update.StageComponents(context.Background(), server.Client(), server.URL, fetchedM, truncStagingDir, verifier, false)
	if errTrunc == nil {
		t.Fatalf("expected StageComponents to reject truncated download, got nil")
	}
}

func mustJSON(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
