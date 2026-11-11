package update

import (
	"crypto/ed25519"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BuildManifestFromDirectory inspects releaseDir and constructs an unsigned Manifest for osName and arch.
func BuildManifestFromDirectory(
	releaseDir string,
	releaseVersion string,
	securityVersion uint64,
	channel string,
	osName string,
	arch string,
	validityDuration time.Duration,
) (*Manifest, error) {
	if validityDuration <= 0 {
		validityDuration = 30 * 24 * time.Hour // default 30 days
	}

	var agentFilename, hostFilename string
	if osName == "windows" {
		agentFilename = fmt.Sprintf("onusly-agent-%s-%s.exe", osName, arch)
		hostFilename = fmt.Sprintf("onusly-native-host-%s-%s.exe", osName, arch)
	} else {
		agentFilename = fmt.Sprintf("onusly-agent-%s-%s", osName, arch)
		hostFilename = fmt.Sprintf("onusly-native-host-%s-%s", osName, arch)
	}

	agentPath := filepath.Join(releaseDir, agentFilename)
	agentSha, agentSize, err := ComputeFileSHA256(agentPath)
	if err != nil {
		// Fallback to non-platform named files if direct names aren't found
		fallbackAgent := filepath.Join(releaseDir, "onusly-agent.exe")
		if osName != "windows" {
			fallbackAgent = filepath.Join(releaseDir, "onusly-agent")
		}
		if _, statErr := os.Stat(fallbackAgent); statErr == nil {
			agentPath = fallbackAgent
			agentFilename = filepath.Base(fallbackAgent)
			agentSha, agentSize, err = ComputeFileSHA256(agentPath)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to locate agent binary in %s: %w", releaseDir, err)
		}
	}

	hostPath := filepath.Join(releaseDir, hostFilename)
	hostSha, hostSize, err := ComputeFileSHA256(hostPath)
	if err != nil {
		fallbackHost := filepath.Join(releaseDir, "onusly-native-host.exe")
		if osName != "windows" {
			fallbackHost = filepath.Join(releaseDir, "onusly-native-host")
		}
		if _, statErr := os.Stat(fallbackHost); statErr == nil {
			hostPath = fallbackHost
			hostFilename = filepath.Base(fallbackHost)
			hostSha, hostSize, err = ComputeFileSHA256(hostPath)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to locate native host binary in %s: %w", releaseDir, err)
		}
	}

	now := time.Now().UTC()
	manifest := &Manifest{
		SchemaVersion:       CurrentSchemaVersion,
		ProductID:           DefaultProductID,
		ReleaseVersion:      releaseVersion,
		SecurityVersion:     securityVersion,
		ReleaseChannel:      channel,
		OperatingSystem:     osName,
		Architecture:        arch,
		MinSupportedVersion: "1.0.0-rc1",
		MinPolicyVersion:    1,
		MinProtocolVersion:  1,
		CreatedAt:           now.Format(time.RFC3339),
		ExpiresAt:           now.Add(validityDuration).Format(time.RFC3339),
		Components: []ComponentItem{
			{
				ComponentID: "agent",
				Filename:    agentFilename,
				SizeBytes:   agentSize,
				SHA256:      agentSha,
			},
			{
				ComponentID: "native_host",
				Filename:    hostFilename,
				SizeBytes:   hostSize,
				SHA256:      hostSha,
			},
		},
	}

	return manifest, nil
}

// BuildAndSignManifest builds a manifest from releaseDir and signs it with privKey.
func BuildAndSignManifest(
	releaseDir string,
	releaseVersion string,
	securityVersion uint64,
	channel string,
	osName string,
	arch string,
	validityDuration time.Duration,
	privKey ed25519.PrivateKey,
) (*Manifest, error) {
	manifest, err := BuildManifestFromDirectory(releaseDir, releaseVersion, securityVersion, channel, osName, arch, validityDuration)
	if err != nil {
		return nil, err
	}

	if err := manifest.Sign(privKey); err != nil {
		return nil, fmt.Errorf("failed to sign update manifest: %w", err)
	}

	return manifest, nil
}

