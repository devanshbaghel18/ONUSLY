package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core/update"
)

func main() {
	var (
		dir             = flag.String("dir", "build/staging", "Directory containing built artifacts")
		version         = flag.String("version", "1.0.0-rc1", "Release candidate version")
		securityVersion = flag.Uint64("security-version", 1, "Monotonically increasing security anti-rollback version")
		channel         = flag.String("channel", "beta", "Release channel (beta, alpha, stable)")
		osName          = flag.String("os", "windows", "Target OS (windows, linux, macos)")
		arch            = flag.String("arch", "amd64", "Target architecture (amd64, arm64)")
		outFile         = flag.String("out", "", "Output JSON path for update manifest")
		keyPath         = flag.String("key", "", "Path to Ed25519 private key hex file (optional; generated if absent)")
	)
	flag.Parse()

	if *outFile == "" {
		*outFile = filepath.Join(*dir, fmt.Sprintf("UPDATE_MANIFEST_%s_%s.json", *osName, *arch))
	}

	var privKey ed25519.PrivateKey
	var pubKey ed25519.PublicKey

	if *keyPath != "" && fileExists(*keyPath) {
		data, err := os.ReadFile(*keyPath)
		if err != nil {
			log.Fatalf("Failed to read key file %s: %v", *keyPath, err)
		}
		rawHex := string(data)
		seed, err := hex.DecodeString(rawHex)
		if err != nil || len(seed) != ed25519.SeedSize {
			if len(seed) == ed25519.PrivateKeySize {
				privKey = ed25519.PrivateKey(seed)
				pubKey = privKey.Public().(ed25519.PublicKey)
			} else {
				log.Fatalf("Invalid Ed25519 key size in %s (expected %d or %d bytes, got %d)",
					*keyPath, ed25519.SeedSize, ed25519.PrivateKeySize, len(seed))
			}
		} else {
			privKey = ed25519.NewKeyFromSeed(seed)
			pubKey = privKey.Public().(ed25519.PublicKey)
		}
	} else {
		// Generate an ephemeral test signing keypair for staging
		var err error
		pubKey, privKey, err = ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatalf("Failed to generate test Ed25519 keypair: %v", err)
		}
		testKeyPath := filepath.Join(*dir, "staging_ed25519.priv")
		testPubPath := filepath.Join(*dir, "staging_ed25519.pub")
		_ = os.WriteFile(testKeyPath, []byte(hex.EncodeToString(privKey)), 0600)
		_ = os.WriteFile(testPubPath, []byte(hex.EncodeToString(pubKey)), 0644)
	}

	manifest, err := update.BuildAndSignManifest(
		*dir,
		*version,
		*securityVersion,
		*channel,
		*osName,
		*arch,
		30*24*time.Hour,
		privKey,
	)
	if err != nil {
		log.Fatalf("Failed to build and sign manifest: %v", err)
	}

	if err := manifest.VerifySignature(pubKey); err != nil {
		log.Fatalf("FATAL: Generated manifest signature verification failed: %v", err)
	}

	rawJSON, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		log.Fatalf("Failed to serialize manifest: %v", err)
	}

	if err := os.WriteFile(*outFile, rawJSON, 0644); err != nil {
		log.Fatalf("Failed to write manifest to %s: %v", *outFile, err)
	}

	fmt.Printf("Generated signed update manifest for %s/%s at %s\n", *osName, *arch, *outFile)
	fmt.Printf("Signature: %s\n", manifest.Signature)
	fmt.Printf("Trusted Public Key (hex): %s\n", hex.EncodeToString(pubKey))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
