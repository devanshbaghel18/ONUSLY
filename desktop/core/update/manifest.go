package update

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	CurrentSchemaVersion = 1
	DefaultProductID     = "onusly"
	MaxArtifactSize      = 150 * 1024 * 1024 // 150MB safety limit
)

var (
	ErrInvalidManifestSyntax   = errors.New("invalid update manifest syntax")
	ErrUnsupportedSchema       = errors.New("unsupported update manifest schema version")
	ErrProductMismatch         = errors.New("update manifest product ID mismatch")
	ErrPlatformMismatch        = errors.New("update manifest target platform mismatch")
	ErrArchitectureMismatch    = errors.New("update manifest target architecture mismatch")
	ErrChannelMismatch         = errors.New("update manifest release channel mismatch")
	ErrMissingSignature        = errors.New("update manifest is missing cryptographic signature")
	ErrInvalidSignature        = errors.New("update manifest cryptographic signature verification failed")
	ErrManifestExpired         = errors.New("update manifest has expired")
	ErrManifestFutureTimestamp = errors.New("update manifest created timestamp is in the future")
	ErrInvalidExpiry           = errors.New("update manifest expires_at must be after created_at")
	ErrEmptyComponents         = errors.New("update manifest contains no components")
	ErrInvalidComponentID      = errors.New("unknown or invalid component identifier")
	ErrDuplicateComponent      = errors.New("duplicate component identifier in manifest")
	ErrPathTraversalDetected   = errors.New("path traversal or unsafe filename detected in component")
	ErrInvalidSHA256           = errors.New("invalid SHA-256 digest format in manifest")
	ErrArtifactSizeInvalid     = errors.New("artifact size is zero or exceeds maximum permitted size")
	ErrArtifactSizeMismatch    = errors.New("artifact file size does not match manifest specification")
	ErrArtifactDigestMismatch  = errors.New("artifact SHA-256 digest does not match manifest specification")
	ErrIncompleteDownload      = errors.New("artifact download terminated prematurely")
	ErrDownloadExceededSize    = errors.New("artifact download exceeded expected size")
	ErrVersionDowngrade        = errors.New("update release version is lower than highest accepted version (rollback prevented)")
	ErrSecurityDowngrade       = errors.New("update security version is lower than highest accepted version (rollback prevented)")
	ErrUpgradePathUnsupported  = errors.New("current installed version is below the minimum supported version for this update")
)

var safeFilenamePattern = regexp.MustCompile(`^[a-zA-Z0-9_\-\.]+$`)

// ComponentItem describes an individual verifiable software artifact in a release.
type ComponentItem struct {
	ComponentID string `json:"component_id"` // "agent", "native_host"
	Filename    string `json:"filename"`     // e.g. "onusly-agent-windows-amd64.exe"
	SizeBytes   int64  `json:"size_bytes"`
	SHA256      string `json:"sha256"` // 64-char lowercase hex
}

// Manifest represents the authoritative signed software update manifest.
type Manifest struct {
	SchemaVersion       int             `json:"schema_version"`
	ProductID           string          `json:"product_id"`
	ReleaseVersion      string          `json:"release_version"`
	SecurityVersion     uint64          `json:"security_version"`
	ReleaseChannel      string          `json:"release_channel"` // "stable", "beta", "staging"
	OperatingSystem     string          `json:"operating_system"` // "windows", "linux"
	Architecture        string          `json:"architecture"`     // "amd64"
	MinSupportedVersion string          `json:"min_supported_version,omitempty"`
	MinPolicyVersion    uint32          `json:"min_policy_version,omitempty"`
	MinProtocolVersion  uint32          `json:"min_protocol_version,omitempty"`
	CreatedAt           string          `json:"created_at"` // RFC3339
	ExpiresAt           string          `json:"expires_at"` // RFC3339
	Components          []ComponentItem `json:"components"`
	Signature           string          `json:"signature,omitempty"` // Base64 encoded Ed25519 signature
}

// manifestPayload defines the canonical structure for cryptographic signing/verification.
type manifestPayload struct {
	SchemaVersion       int             `json:"schema_version"`
	ProductID           string          `json:"product_id"`
	ReleaseVersion      string          `json:"release_version"`
	SecurityVersion     uint64          `json:"security_version"`
	ReleaseChannel      string          `json:"release_channel"`
	OperatingSystem     string          `json:"operating_system"`
	Architecture        string          `json:"architecture"`
	MinSupportedVersion string          `json:"min_supported_version,omitempty"`
	MinPolicyVersion    uint32          `json:"min_policy_version,omitempty"`
	MinProtocolVersion  uint32          `json:"min_protocol_version,omitempty"`
	CreatedAt           string          `json:"created_at"`
	ExpiresAt           string          `json:"expires_at"`
	Components          []ComponentItem `json:"components"`
}

// CanonicalBytes returns the deterministic JSON serialization of the manifest for signing/verification.
// The signature field is strictly excluded, and components are sorted by ComponentID for determinism.
func (m *Manifest) CanonicalBytes() ([]byte, error) {
	sortedComponents := make([]ComponentItem, len(m.Components))
	copy(sortedComponents, m.Components)
	sort.Slice(sortedComponents, func(i, j int) bool {
		return sortedComponents[i].ComponentID < sortedComponents[j].ComponentID
	})

	payload := manifestPayload{
		SchemaVersion:       m.SchemaVersion,
		ProductID:           m.ProductID,
		ReleaseVersion:      m.ReleaseVersion,
		SecurityVersion:     m.SecurityVersion,
		ReleaseChannel:      m.ReleaseChannel,
		OperatingSystem:     m.OperatingSystem,
		Architecture:        m.Architecture,
		MinSupportedVersion: m.MinSupportedVersion,
		MinPolicyVersion:    m.MinPolicyVersion,
		MinProtocolVersion:  m.MinProtocolVersion,
		CreatedAt:           m.CreatedAt,
		ExpiresAt:           m.ExpiresAt,
		Components:          sortedComponents,
	}

	return json.Marshal(payload)
}

// Sign signs the canonical bytes using an Ed25519 private key and stores the Base64 signature.
func (m *Manifest) Sign(privKey ed25519.PrivateKey) error {
	if len(privKey) != ed25519.PrivateKeySize {
		return errors.New("invalid ed25519 private key size")
	}

	data, err := m.CanonicalBytes()
	if err != nil {
		return fmt.Errorf("failed to generate canonical manifest bytes: %w", err)
	}

	sig := ed25519.Sign(privKey, data)
	m.Signature = base64.StdEncoding.EncodeToString(sig)
	return nil
}

// VerifySignature verifies the Ed25519 signature against the canonical manifest bytes.
func (m *Manifest) VerifySignature(pubKey ed25519.PublicKey) error {
	if len(pubKey) != ed25519.PublicKeySize {
		return errors.New("invalid ed25519 public key size")
	}

	if m.Signature == "" {
		return ErrMissingSignature
	}

	sigBytes, err := base64.StdEncoding.DecodeString(m.Signature)
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return fmt.Errorf("%w: invalid signature format", ErrInvalidSignature)
	}

	data, err := m.CanonicalBytes()
	if err != nil {
		return fmt.Errorf("failed to generate canonical manifest bytes: %w", err)
	}

	if !ed25519.Verify(pubKey, data, sigBytes) {
		return ErrInvalidSignature
	}

	return nil
}

// ValidateSyntax performs strict static validation on the manifest structure and component rules.
func (m *Manifest) ValidateSyntax() error {
	if m.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf("%w: expected %d, got %d", ErrUnsupportedSchema, CurrentSchemaVersion, m.SchemaVersion)
	}

	if strings.TrimSpace(m.ProductID) != DefaultProductID {
		return fmt.Errorf("%w: expected %q, got %q", ErrProductMismatch, DefaultProductID, m.ProductID)
	}

	if strings.TrimSpace(m.ReleaseVersion) == "" {
		return fmt.Errorf("%w: missing release_version", ErrInvalidManifestSyntax)
	}

	if m.SecurityVersion == 0 {
		return fmt.Errorf("%w: security_version must be a positive monotonic integer", ErrInvalidManifestSyntax)
	}

	if len(m.Components) == 0 {
		return ErrEmptyComponents
	}

	seenComponents := make(map[string]bool)
	for _, c := range m.Components {
		cid := strings.TrimSpace(c.ComponentID)
		if cid != "agent" && cid != "native_host" {
			return fmt.Errorf("%w: %q (allowed: agent, native_host)", ErrInvalidComponentID, cid)
		}
		if seenComponents[cid] {
			return fmt.Errorf("%w: %q", ErrDuplicateComponent, cid)
		}
		seenComponents[cid] = true

		if !safeFilenamePattern.MatchString(c.Filename) || filepath.Base(c.Filename) != c.Filename {
			return fmt.Errorf("%w: %q", ErrPathTraversalDetected, c.Filename)
		}

		if c.SizeBytes <= 0 || c.SizeBytes > MaxArtifactSize {
			return fmt.Errorf("%w: component %s has invalid size %d", ErrArtifactSizeInvalid, cid, c.SizeBytes)
		}

		cleanSha := strings.ToLower(strings.TrimSpace(c.SHA256))
		if len(cleanSha) != 64 {
			return fmt.Errorf("%w: component %s sha256 must be 64 characters", ErrInvalidSHA256, cid)
		}
		if _, err := hex.DecodeString(cleanSha); err != nil {
			return fmt.Errorf("%w: component %s sha256 is not valid hex: %v", ErrInvalidSHA256, cid, err)
		}
	}

	return nil
}

// ComputeFileSHA256 computes the lowercase hex SHA-256 digest of a local file.
func ComputeFileSHA256(filePath string) (string, int64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}

	return hex.EncodeToString(h.Sum(nil)), size, nil
}
