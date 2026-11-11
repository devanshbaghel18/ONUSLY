package update

import (
	"crypto/ed25519"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// VerifierConfig specifies configuration options for the update verifier.
type VerifierConfig struct {
	TrustedPublicKey ed25519.PublicKey
	CurrentOS        string
	CurrentArch      string
	CurrentChannel   string // e.g. "stable", "beta", "staging"
	CurrentVersion   string // e.g. "1.0.0-rc1"
	RollbackMgr      *AntiRollbackManager
	MaxClockSkew     time.Duration
}

// UpdateVerifier provides strict 12-point verification of update manifests and artifacts.
type UpdateVerifier struct {
	config VerifierConfig
}

// NewUpdateVerifier constructs an UpdateVerifier with sensible security defaults.
func NewUpdateVerifier(cfg VerifierConfig) (*UpdateVerifier, error) {
	if len(cfg.TrustedPublicKey) != ed25519.PublicKeySize {
		return nil, errors.New("trusted ed25519 public key must be exactly 32 bytes")
	}
	if cfg.CurrentOS == "" {
		return nil, errors.New("current OS must be specified")
	}
	if cfg.CurrentArch == "" {
		return nil, errors.New("current architecture must be specified")
	}
	if cfg.CurrentChannel == "" {
		cfg.CurrentChannel = "stable"
	}
	if cfg.MaxClockSkew == 0 {
		cfg.MaxClockSkew = 5 * time.Minute
	}

	return &UpdateVerifier{config: cfg}, nil
}

// VerifyManifest executes the 12-point security check against an update manifest.
func (v *UpdateVerifier) VerifyManifest(manifest *Manifest, allowDowngrade bool) error {
	if manifest == nil {
		return errors.New("cannot verify nil manifest")
	}

	// 1. Syntax, structure, and component limits
	if err := manifest.ValidateSyntax(); err != nil {
		return err
	}

	// 2. Schema version
	if manifest.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("%w: expected %d, got %d", ErrUnsupportedSchema, CurrentSchemaVersion, manifest.SchemaVersion)
	}

	// 3. Product identity
	if manifest.ProductID != DefaultProductID {
		return fmt.Errorf("%w: expected %q, got %q", ErrProductMismatch, DefaultProductID, manifest.ProductID)
	}

	// 4. Platform check
	if strings.ToLower(manifest.OperatingSystem) != strings.ToLower(v.config.CurrentOS) {
		return fmt.Errorf("%w: manifest target %s != host %s",
			ErrPlatformMismatch, manifest.OperatingSystem, v.config.CurrentOS)
	}

	// 5. Architecture check
	if strings.ToLower(manifest.Architecture) != strings.ToLower(v.config.CurrentArch) {
		return fmt.Errorf("%w: manifest target %s != host %s",
			ErrArchitectureMismatch, manifest.Architecture, v.config.CurrentArch)
	}

	// 6. Release channel check
	if strings.ToLower(manifest.ReleaseChannel) != strings.ToLower(v.config.CurrentChannel) {
		return fmt.Errorf("%w: manifest channel %q != configured %q",
			ErrChannelMismatch, manifest.ReleaseChannel, v.config.CurrentChannel)
	}

	// 7. Cryptographic Ed25519 signature
	if err := manifest.VerifySignature(v.config.TrustedPublicKey); err != nil {
		return err
	}

	// 8. Timestamps and manifest freshness
	now := time.Now().UTC()
	createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAt)
	if err != nil {
		return fmt.Errorf("%w: invalid created_at timestamp: %v", ErrInvalidManifestSyntax, err)
	}

	expiresAt, err := time.Parse(time.RFC3339, manifest.ExpiresAt)
	if err != nil {
		return fmt.Errorf("%w: invalid expires_at timestamp: %v", ErrInvalidManifestSyntax, err)
	}

	if expiresAt.Before(createdAt) || expiresAt.Equal(createdAt) {
		return ErrInvalidExpiry
	}

	if createdAt.After(now.Add(v.config.MaxClockSkew)) {
		return ErrManifestFutureTimestamp
	}

	if now.After(expiresAt) {
		return ErrManifestExpired
	}

	// 9. Minimum supported version compatibility
	if manifest.MinSupportedVersion != "" && v.config.CurrentVersion != "" {
		cmp, err := CompareVersions(v.config.CurrentVersion, manifest.MinSupportedVersion)
		if err != nil {
			return fmt.Errorf("failed to compare min_supported_version: %w", err)
		}
		if cmp < 0 {
			return fmt.Errorf("%w: current version %s < minimum supported %s",
				ErrUpgradePathUnsupported, v.config.CurrentVersion, manifest.MinSupportedVersion)
		}
	}

	// 10. Anti-rollback check
	if v.config.RollbackMgr != nil {
		if err := v.config.RollbackMgr.CheckUpdate(manifest, allowDowngrade); err != nil {
			return err
		}
	}

	return nil
}

// VerifyArtifact verifies an artifact's file size and SHA-256 digest against the manifest specification.
func (v *UpdateVerifier) VerifyArtifact(component *ComponentItem, artifactPath string) error {
	if component == nil {
		return errors.New("cannot verify nil component specification")
	}

	fi, err := os.Stat(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to inspect artifact %s: %w", artifactPath, err)
	}

	if !fi.Mode().IsRegular() {
		return fmt.Errorf("artifact %s is not a regular file", artifactPath)
	}

	if fi.Size() != component.SizeBytes {
		return fmt.Errorf("%w: file %s size is %d bytes, manifest specifies %d bytes",
			ErrArtifactSizeMismatch, artifactPath, fi.Size(), component.SizeBytes)
	}

	actualSha, size, err := ComputeFileSHA256(artifactPath)
	if err != nil {
		return fmt.Errorf("failed to compute digest for %s: %w", artifactPath, err)
	}

	if size != component.SizeBytes {
		return fmt.Errorf("%w: read %d bytes during hash calculation, expected %d",
			ErrArtifactSizeMismatch, size, component.SizeBytes)
	}

	cleanExpected := strings.ToLower(strings.TrimSpace(component.SHA256))
	if actualSha != cleanExpected {
		return fmt.Errorf("%w: file %s computed %s, manifest specifies %s",
			ErrArtifactDigestMismatch, artifactPath, actualSha, cleanExpected)
	}

	return nil
}

