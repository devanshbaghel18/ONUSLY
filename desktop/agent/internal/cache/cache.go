package cache

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

var (
	ErrCacheCorrupt = errors.New("policy cache corrupted and quarantined; no valid policy loaded")
)

// CacheHealthStatus reflects the operational integrity of the local policy cache.
type CacheHealthStatus struct {
	Valid          bool   `json:"valid"`
	Degraded       bool   `json:"degraded"`
	DegradedReason string `json:"degradedReason,omitempty"`
	IsLKG          bool   `json:"isLkg"`
	Migrated       bool   `json:"migrated"`
}

type FileBlocklistCache struct {
	cachePath string
	mutex     sync.RWMutex
	health    CacheHealthStatus
}

func NewFileBlocklistCache(cachePath string) *FileBlocklistCache {
	return &FileBlocklistCache{
		cachePath: cachePath,
		health: CacheHealthStatus{
			Valid:    false,
			Degraded: false,
		},
	}
}

func (c *FileBlocklistCache) GetHealth() CacheHealthStatus {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.health
}

func (c *FileBlocklistCache) lkgPath() string {
	return c.cachePath + ".lkg"
}

func (c *FileBlocklistCache) alternatePath() string {
	dir := filepath.Dir(c.cachePath)
	base := filepath.Base(c.cachePath)
	if base == "policy_cache.json" {
		return filepath.Join(dir, "blocklist_cache.json")
	}
	if base == "blocklist_cache.json" {
		return filepath.Join(dir, "policy_cache.json")
	}
	return ""
}

// isNewer reports whether candidate policy data is newer than baseline.
func isNewer(candidate, baseline *core.BlocklistData) bool {
	if candidate == nil {
		return false
	}
	if baseline == nil {
		return true
	}

	// 1. Compare LastSyncAt
	tCandSync, errCandSync := time.Parse(time.RFC3339, candidate.LastSyncAt)
	tBaseSync, errBaseSync := time.Parse(time.RFC3339, baseline.LastSyncAt)
	if errCandSync == nil && errBaseSync == nil {
		if tCandSync.After(tBaseSync) {
			return true
		}
		if tCandSync.Before(tBaseSync) {
			return false
		}
	}

	// 2. Compare GeneratedAt
	tCandGen, errCandGen := time.Parse(time.RFC3339, candidate.GeneratedAt)
	tBaseGen, errBaseGen := time.Parse(time.RFC3339, baseline.GeneratedAt)
	if errCandGen == nil && errBaseGen == nil {
		if tCandGen.After(tBaseGen) {
			return true
		}
		if tCandGen.Before(tBaseGen) {
			return false
		}
	}

	// 3. Compare Version strings
	return candidate.Version > baseline.Version
}

func (c *FileBlocklistCache) Get(expectedUserID string) (*core.BlocklistData, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 1. Check if primary cache exists
	data, err := os.ReadFile(c.cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			// Check for alternate/legacy cache for seamless upgrade migration
			alt := c.alternatePath()
			if alt != "" {
				if altData, errAlt := os.ReadFile(alt); errAlt == nil {
					var altCached core.BlocklistData
					if errUnmarshal := json.Unmarshal(bytes.TrimPrefix(altData, []byte("\xef\xbb\xbf")), &altCached); errUnmarshal == nil && altCached.Version != "" {
						// Migrate alternate cache to primary path and update LKG
						_ = c.saveInternal(&altCached)
						_ = os.Rename(alt, alt+".archived")
						c.health = CacheHealthStatus{
							Valid:    true,
							Degraded: false,
							Migrated: true,
						}
						if expectedUserID != "" && altCached.UserID != "" && altCached.UserID != expectedUserID {
							return nil, core.ErrUserMismatch
						}
						return &altCached, nil
					} else {
						// Legacy cache is corrupt: quarantine it
						_ = os.Rename(alt, alt+".corrupt")
					}
				}
			}

			// Check for Last-Known-Good fallback if primary is missing
			if lkgData, errLkg := os.ReadFile(c.lkgPath()); errLkg == nil {
				var lkgCached core.BlocklistData
				if errUnmarshal := json.Unmarshal(bytes.TrimPrefix(lkgData, []byte("\xef\xbb\xbf")), &lkgCached); errUnmarshal == nil && lkgCached.Version != "" {
					// Heal primary cache from LKG
					_ = os.WriteFile(c.cachePath, lkgData, 0600)
					c.health = CacheHealthStatus{
						Valid:          true,
						Degraded:       true,
						DegradedReason: "primary_cache_missing_restored_from_lkg",
						IsLKG:          true,
					}
					if expectedUserID != "" && lkgCached.UserID != "" && lkgCached.UserID != expectedUserID {
						return nil, core.ErrUserMismatch
					}
					return &lkgCached, nil
				} else {
					_ = os.Rename(c.lkgPath(), c.lkgPath()+".corrupt")
				}
			}

			// Clean initial state: missing cache
			c.health = CacheHealthStatus{
				Valid:          false,
				Degraded:       true,
				DegradedReason: "no_cache_file_found",
			}
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read blocklist cache: %w", err)
	}

	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))

	var cached core.BlocklistData
	if err := json.Unmarshal(data, &cached); err != nil {
		// Corruption detected: quarantine corrupted primary file
		corruptPath := c.cachePath + ".corrupt"
		_ = os.Rename(c.cachePath, corruptPath)

		// Attempt recovery from Last-Known-Good (LKG)
		if lkgData, errLkg := os.ReadFile(c.lkgPath()); errLkg == nil {
			var lkgCached core.BlocklistData
			if errUnmarshal := json.Unmarshal(bytes.TrimPrefix(lkgData, []byte("\xef\xbb\xbf")), &lkgCached); errUnmarshal == nil && lkgCached.Version != "" {
				// Heal primary cache by writing valid LKG data back
				_ = os.WriteFile(c.cachePath, lkgData, 0600)
				c.health = CacheHealthStatus{
					Valid:          true,
					Degraded:       true,
					DegradedReason: "primary_cache_corrupt_quarantined_restored_from_lkg",
					IsLKG:          true,
				}
				if expectedUserID != "" && lkgCached.UserID != "" && lkgCached.UserID != expectedUserID {
					return nil, core.ErrUserMismatch
				}
				return &lkgCached, nil
			} else {
				// LKG is also corrupt: quarantine it
				_ = os.Rename(c.lkgPath(), c.lkgPath()+".corrupt")
			}
		}

		// Also check legacy if LKG wasn't available
		alt := c.alternatePath()
		if alt != "" {
			if altData, errAlt := os.ReadFile(alt); errAlt == nil {
				var altCached core.BlocklistData
				if errUnmarshal := json.Unmarshal(bytes.TrimPrefix(altData, []byte("\xef\xbb\xbf")), &altCached); errUnmarshal == nil && altCached.Version != "" {
					_ = c.saveInternal(&altCached)
					_ = os.Rename(alt, alt+".archived")
					c.health = CacheHealthStatus{
						Valid:          true,
						Degraded:       true,
						DegradedReason: "primary_and_lkg_corrupt_restored_from_legacy",
						Migrated:       true,
					}
					if expectedUserID != "" && altCached.UserID != "" && altCached.UserID != expectedUserID {
						return nil, core.ErrUserMismatch
					}
					return &altCached, nil
				} else {
					_ = os.Rename(alt, alt+".corrupt")
				}
			}
		}

		// LKG and legacy not available or all corrupt
		c.health = CacheHealthStatus{
			Valid:          false,
			Degraded:       true,
			DegradedReason: "primary_cache_corrupt_quarantined_no_lkg",
		}
		return nil, ErrCacheCorrupt
	}

	// Primary is valid: verify and heal LKG if LKG is missing or corrupt
	lkgData, errLkg := os.ReadFile(c.lkgPath())
	if errLkg != nil {
		// Missing LKG: heal LKG by saving valid primary data
		_ = os.WriteFile(c.lkgPath(), data, 0600)
	} else {
		var lkgCheck core.BlocklistData
		if errUnmarshal := json.Unmarshal(bytes.TrimPrefix(lkgData, []byte("\xef\xbb\xbf")), &lkgCheck); errUnmarshal != nil || lkgCheck.Version == "" {
			// Corrupt LKG: heal LKG by saving valid primary data
			_ = os.WriteFile(c.lkgPath(), data, 0600)
		}
	}

	// Primary is valid: reconcile with legacy cache if legacy exists
	alt := c.alternatePath()
	if alt != "" {
		if altData, errAlt := os.ReadFile(alt); errAlt == nil {
			var altCached core.BlocklistData
			if errUnmarshal := json.Unmarshal(bytes.TrimPrefix(altData, []byte("\xef\xbb\xbf")), &altCached); errUnmarshal == nil && altCached.Version != "" {
				if isNewer(&altCached, &cached) {
					// Legacy is newer! Migrate legacy to primary
					cached = altCached
					_ = c.saveInternal(&altCached)
					c.health = CacheHealthStatus{
						Valid:    true,
						Degraded: false,
						Migrated: true,
					}
				}
				// Archive legacy file to avoid repeated reconciliation
				_ = os.Rename(alt, alt+".archived")
			} else {
				_ = os.Rename(alt, alt+".corrupt")
			}
		}
	}

	// Multi-user safety check: prevent cache leakage across accounts
	if expectedUserID != "" && cached.UserID != "" && cached.UserID != expectedUserID {
		return nil, core.ErrUserMismatch
	}

	if !c.health.Migrated {
		c.health = CacheHealthStatus{
			Valid:    true,
			Degraded: false,
		}
	}

	return &cached, nil
}

func (c *FileBlocklistCache) saveInternal(data *core.BlocklistData) error {
	if data == nil {
		return errors.New("cannot save nil blocklist data")
	}

	if err := os.MkdirAll(filepath.Dir(c.cachePath), 0700); err != nil {
		return fmt.Errorf("failed to create cache dir: %w", err)
	}

	bytesData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache data: %w", err)
	}

	// Atomic write using a temp file in the same directory with explicit fsync flush
	tmpPath := fmt.Sprintf("%s.tmp.%d.%d", c.cachePath, os.Getpid(), time.Now().UnixNano())
	f, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temp cache file: %w", err)
	}

	if _, err := f.Write(bytesData); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp cache: %w", err)
	}

	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to flush temp cache: %w", err)
	}

	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp cache: %w", err)
	}

	if err := os.Rename(tmpPath, c.cachePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to commit cache rename: %w", err)
	}

	// Also update Last-Known-Good backup
	lkgTmp := fmt.Sprintf("%s.tmp.%d.%d", c.lkgPath(), os.Getpid(), time.Now().UnixNano())
	if err := os.WriteFile(lkgTmp, bytesData, 0600); err == nil {
		_ = os.Rename(lkgTmp, c.lkgPath())
	}

	c.health = CacheHealthStatus{
		Valid:    true,
		Degraded: false,
	}

	return nil
}

func (c *FileBlocklistCache) Save(data *core.BlocklistData) error {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.saveInternal(data)
}

func (c *FileBlocklistCache) Clear() error {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	_ = os.Remove(c.lkgPath())
	_ = os.Remove(c.cachePath + ".corrupt")
	c.health = CacheHealthStatus{
		Valid:    false,
		Degraded: false,
	}

	err := os.Remove(c.cachePath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
