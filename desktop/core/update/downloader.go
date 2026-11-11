package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	MaxManifestDownloadSize = 1024 * 1024 // 1MB max manifest size
)

// FetchManifest retrieves and decodes an update manifest from a remote URL.
func FetchManifest(ctx context.Context, httpClient *http.Client, manifestURL string) (*Manifest, error) {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create manifest request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ONUSLY-UpdateClient/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch update manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update server returned status %d for manifest", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, MaxManifestDownloadSize)
	var manifest Manifest
	if err := json.NewDecoder(limitedReader).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidManifestSyntax, err)
	}

	return &manifest, nil
}

// StageComponents safely downloads or copies all artifacts into stagingDir, verifying hashes and sizes.
func StageComponents(
	ctx context.Context,
	httpClient *http.Client,
	baseURL string,
	manifest *Manifest,
	stagingDir string,
	verifier *UpdateVerifier,
	allowDowngrade bool,
) (map[string]string, error) {
	if manifest == nil {
		return nil, errors.New("cannot stage nil manifest")
	}
	if verifier == nil {
		return nil, errors.New("cannot stage without verifier")
	}

	// 1. Verify manifest before downloading any artifacts
	if err := verifier.VerifyManifest(manifest, allowDowngrade); err != nil {
		return nil, fmt.Errorf("manifest verification failed: %w", err)
	}

	if err := os.MkdirAll(stagingDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create staging directory %s: %w", stagingDir, err)
	}

	stagedPaths := make(map[string]string)
	isRemote := strings.HasPrefix(baseURL, "http://") || strings.HasPrefix(baseURL, "https://")

	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}

	for _, comp := range manifest.Components {
		// Strict filename safety check
		if !safeFilenamePattern.MatchString(comp.Filename) || filepath.Base(comp.Filename) != comp.Filename {
			return nil, fmt.Errorf("%w: %q", ErrPathTraversalDetected, comp.Filename)
		}

		destPath := filepath.Join(stagingDir, comp.Filename)
		tmpPath := fmt.Sprintf("%s.tmp.%d", destPath, os.Getpid())

		var stageErr error
		if isRemote {
			stageErr = downloadSingleComponent(ctx, httpClient, baseURL, &comp, tmpPath)
		} else {
			stageErr = copyLocalComponent(baseURL, &comp, tmpPath)
		}

		if stageErr != nil {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("failed to stage component %s: %w", comp.ComponentID, stageErr)
		}

		// Atomically rename temporary download to destination
		if err := os.Rename(tmpPath, destPath); err != nil {
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("failed to commit staged component %s: %w", comp.ComponentID, err)
		}

		// Post-stage verification of the committed file
		if err := verifier.VerifyArtifact(&comp, destPath); err != nil {
			_ = os.Remove(destPath)
			return nil, fmt.Errorf("staged artifact validation failed for %s: %w", comp.ComponentID, err)
		}

		stagedPaths[comp.ComponentID] = destPath
	}

	return stagedPaths, nil
}

func downloadSingleComponent(ctx context.Context, client *http.Client, baseURL string, comp *ComponentItem, tmpPath string) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/" + comp.Filename

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "ONUSLY-UpdateClient/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d for %s", resp.StatusCode, comp.Filename)
	}

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	hasher := sha256.New()
	limitReader := io.LimitReader(resp.Body, comp.SizeBytes+1)
	writer := io.MultiWriter(f, hasher)

	written, err := io.Copy(writer, limitReader)
	if err != nil {
		return fmt.Errorf("copy error during download: %w", err)
	}

	if written > comp.SizeBytes {
		return fmt.Errorf("%w: read %d, expected %d", ErrDownloadExceededSize, written, comp.SizeBytes)
	}
	if written < comp.SizeBytes {
		return fmt.Errorf("%w: read %d, expected %d", ErrIncompleteDownload, written, comp.SizeBytes)
	}

	calcSha := hex.EncodeToString(hasher.Sum(nil))
	expectedSha := strings.ToLower(strings.TrimSpace(comp.SHA256))
	if calcSha != expectedSha {
		return fmt.Errorf("%w: downloaded %s, expected %s", ErrArtifactDigestMismatch, calcSha, expectedSha)
	}

	return nil
}

func copyLocalComponent(baseDir string, comp *ComponentItem, tmpPath string) error {
	srcPath := filepath.Join(baseDir, comp.Filename)
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source artifact %s: %w", srcPath, err)
	}
	defer src.Close()

	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dst.Close()

	hasher := sha256.New()
	writer := io.MultiWriter(dst, hasher)

	copied, err := io.Copy(writer, src)
	if err != nil {
		return err
	}

	if copied != comp.SizeBytes {
		return fmt.Errorf("%w: copied %d, expected %d", ErrArtifactSizeMismatch, copied, comp.SizeBytes)
	}

	calcSha := hex.EncodeToString(hasher.Sum(nil))
	expectedSha := strings.ToLower(strings.TrimSpace(comp.SHA256))
	if calcSha != expectedSha {
		return fmt.Errorf("%w: local file has digest %s, expected %s", ErrArtifactDigestMismatch, calcSha, expectedSha)
	}

	return nil
}

