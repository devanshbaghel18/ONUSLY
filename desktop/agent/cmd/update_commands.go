package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)

// PinnedStagingPublicKey is the canonical development/staging public key used for offline and test verification.
// Production deployments override this via environment or secure configuration.
const PinnedStagingPublicKeyHex = "4b84dd004818fa8db3463a5647565d774847e9282367cb6eb8838234bb4fbc0d"

func getTrustedPublicKey() (ed25519.PublicKey, error) {
	keyStr := strings.TrimSpace(os.Getenv("ONUSLY_UPDATE_PUBLIC_KEY"))
	if keyStr == "" {
		keyStr = PinnedStagingPublicKeyHex
	}

	// Try hex
	if len(keyStr) == 64 {
		b, err := hex.DecodeString(keyStr)
		if err == nil && len(b) == ed25519.PublicKeySize {
			return ed25519.PublicKey(b), nil
		}
	}

	// Try base64
	b, err := base64.StdEncoding.DecodeString(keyStr)
	if err == nil && len(b) == ed25519.PublicKeySize {
		return ed25519.PublicKey(b), nil
	}

	return nil, fmt.Errorf("invalid trusted public key format %q (expected 32-byte hex or base64)", keyStr)
}

func runUpdateCommand(args []string, configDir, dataDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing update subcommand. Usage: onusly-agent update [check|verify|stage]")
	}

	action := args[0]
	subArgs := args[1:]

	pubKey, err := getTrustedPublicKey()
	if err != nil {
		return fmt.Errorf("failed to load trusted public key: %w", err)
	}

	antiRollbackPath := filepath.Join(configDir, "anti_rollback.json")
	rollbackMgr := update.NewAntiRollbackManager(antiRollbackPath)

	channel := os.Getenv("ONUSLY_UPDATE_CHANNEL")
	if channel == "" {
		channel = "stable"
	}

	verifierCfg := update.VerifierConfig{
		TrustedPublicKey: pubKey,
		CurrentOS:        runtime.GOOS,
		CurrentArch:      runtime.GOARCH,
		CurrentChannel:   channel,
		CurrentVersion:   core.ReleaseVersion,
		RollbackMgr:      rollbackMgr,
	}

	verifier, err := update.NewUpdateVerifier(verifierCfg)
	if err != nil {
		return fmt.Errorf("failed to initialize verifier: %w", err)
	}

	switch action {
	case "check":
		fs := flag.NewFlagSet("update check", flag.ContinueOnError)
		feedURL := fs.String("feed-url", "http://localhost:8080/downloads/update_manifest.json", "Update feed manifest URL")
		allowDowngrade := fs.Bool("allow-downgrade", false, "Permit authorized downgrade (recovery mode only)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}

		fmt.Printf("Checking for updates from: %s\n", *feedURL)
		fmt.Printf("Current Release:         %s (Platform: %s/%s, Channel: %s)\n",
			core.ReleaseVersion, runtime.GOOS, runtime.GOARCH, channel)

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		client := &http.Client{Timeout: 15 * time.Second}
		manifest, err := update.FetchManifest(ctx, client, *feedURL)
		if err != nil {
			return fmt.Errorf("failed to fetch update manifest: %w", err)
		}

		if err := verifier.VerifyManifest(manifest, *allowDowngrade); err != nil {
			return fmt.Errorf("manifest verification failed: %w", err)
		}

		fmt.Println("=======================================================")
		fmt.Println("         AUTHENTIC UPDATE MANIFEST VERIFIED            ")
		fmt.Println("=======================================================")
		fmt.Printf("Release Version:   %s\n", manifest.ReleaseVersion)
		fmt.Printf("Security Version:  %d\n", manifest.SecurityVersion)
		fmt.Printf("Release Channel:   %s\n", manifest.ReleaseChannel)
		fmt.Printf("Target Platform:   %s/%s\n", manifest.OperatingSystem, manifest.Architecture)
		fmt.Printf("Created At:        %s\n", manifest.CreatedAt)
		fmt.Printf("Expires At:        %s\n", manifest.ExpiresAt)
		fmt.Printf("Components (%d):   \n", len(manifest.Components))
		for _, c := range manifest.Components {
			fmt.Printf("  • [%s] %s (%d bytes, sha256: %s...)\n",
				c.ComponentID, c.Filename, c.SizeBytes, c.SHA256[:16])
		}

		cmp, _ := update.CompareVersions(manifest.ReleaseVersion, core.ReleaseVersion)
		if cmp > 0 {
			fmt.Printf("\n🚀 Update Available: %s -> %s (Run 'onusly-agent update stage' to prepare)\n",
				core.ReleaseVersion, manifest.ReleaseVersion)
		} else {
			fmt.Printf("\n✅ System is up to date (Current version %s matches or exceeds manifest %s).\n",
				core.ReleaseVersion, manifest.ReleaseVersion)
		}
		return nil

	case "verify":
		if len(subArgs) == 0 {
			return fmt.Errorf("usage: onusly-agent update verify <manifest-path> [artifacts-dir] [--allow-downgrade]")
		}
		manifestPath := subArgs[0]
		artifactsDir := ""
		allowDowngrade := false
		for _, a := range subArgs[1:] {
			if a == "--allow-downgrade" {
				allowDowngrade = true
			} else if !strings.HasPrefix(a, "-") && artifactsDir == "" {
				artifactsDir = a
			}
		}

		data, err := os.ReadFile(manifestPath)
		if err != nil {
			return fmt.Errorf("failed to read manifest file: %w", err)
		}

		var manifest update.Manifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("invalid manifest JSON: %w", err)
		}

		if err := verifier.VerifyManifest(&manifest, allowDowngrade); err != nil {
			return fmt.Errorf("manifest verification failed: %w", err)
		}

		fmt.Println("=======================================================")
		fmt.Println("         MANIFEST CRYPTOGRAPHICALLY VERIFIED           ")
		fmt.Println("=======================================================")
		fmt.Printf("Product:         %s\n", manifest.ProductID)
		fmt.Printf("Release Version: %s\n", manifest.ReleaseVersion)
		fmt.Printf("Security Vers:   %d\n", manifest.SecurityVersion)
		fmt.Printf("Platform:        %s/%s\n", manifest.OperatingSystem, manifest.Architecture)
		fmt.Printf("Signature:       VERIFIED (Ed25519)\n")

		if artifactsDir != "" {
			fmt.Println("\nVerifying Artifact Digests:")
			for _, comp := range manifest.Components {
				artPath := filepath.Join(artifactsDir, comp.Filename)
				if err := verifier.VerifyArtifact(&comp, artPath); err != nil {
					return fmt.Errorf("artifact verification failed for %s (%s): %w", comp.ComponentID, comp.Filename, err)
				}
				fmt.Printf("  [PASS] %s (%s): SHA-256 and size verified\n", comp.ComponentID, comp.Filename)
			}
			fmt.Println("✅ All artifacts strictly match signed manifest digests.")
		}

		return nil

	case "stage":
		fs := flag.NewFlagSet("update stage", flag.ContinueOnError)
		feedURL := fs.String("feed-url", "http://localhost:8080/downloads/update_manifest.json", "Update feed manifest URL")
		artifactsBase := fs.String("artifacts-url", "", "Base URL or directory for artifacts (default derived from feed-url)")
		allowDowngrade := fs.Bool("allow-downgrade", false, "Permit authorized downgrade (recovery mode only)")
		if err := fs.Parse(subArgs); err != nil {
			return err
		}

		stagingDir := filepath.Join(dataDir, "staging")
		fmt.Printf("Staging update into: %s\n", stagingDir)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		client := &http.Client{Timeout: 60 * time.Second}
		manifest, err := update.FetchManifest(ctx, client, *feedURL)
		if err != nil {
			return fmt.Errorf("failed to fetch update manifest: %w", err)
		}

		baseArtifactsURL := *artifactsBase
		if baseArtifactsURL == "" {
			idx := strings.LastIndex(*feedURL, "/")
			if idx != -1 {
				baseArtifactsURL = (*feedURL)[:idx]
			} else {
				baseArtifactsURL = *feedURL
			}
		}

		staged, err := update.StageComponents(ctx, client, baseArtifactsURL, manifest, stagingDir, verifier, *allowDowngrade)
		if err != nil {
			return fmt.Errorf("failed to stage components: %w", err)
		}

		manifestCopyPath := filepath.Join(stagingDir, "manifest.json")
		mBytes, _ := json.MarshalIndent(manifest, "", "  ")
		_ = os.WriteFile(manifestCopyPath, mBytes, 0600)

		fmt.Println("=======================================================")
		fmt.Println("         UPDATE SUCCESSFULLY STAGED & VERIFIED         ")
		fmt.Println("=======================================================")
		fmt.Printf("Release Version: %s\n", manifest.ReleaseVersion)
		fmt.Printf("Staged Directory: %s\n", stagingDir)
		for cid, path := range staged {
			fmt.Printf("  • [%s] %s\n", cid, filepath.Base(path))
		}
		fmt.Println("\nTo apply this update, invoke the elevated installer:")
		if runtime.GOOS == "windows" {
			fmt.Printf("  powershell -ExecutionPolicy Bypass -File .\\install.ps1 -Upgrade -StagedDir %s\n", stagingDir)
		} else {
			fmt.Printf("  sudo onusly-installer -upgrade -staged-dir=%s\n", stagingDir)
		}
		return nil

	default:
		return errors.New("unknown update subcommand. Supported: check, verify, stage")
	}
}

