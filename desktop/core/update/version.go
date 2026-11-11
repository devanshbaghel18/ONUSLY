package update

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SemVer represents a parsed Semantic Version 2.0 structure.
type SemVer struct {
	Major      int
	Minor      int
	Patch      int
	PreRelease []string
	Raw        string
}

// ParseSemVer parses a SemVer 2.0 version string.
func ParseSemVer(v string) (SemVer, error) {
	raw := strings.TrimSpace(v)
	s := strings.TrimPrefix(raw, "v")

	// Strip build metadata (+build)
	if idx := strings.Index(s, "+"); idx != -1 {
		s = s[:idx]
	}

	var preParts []string
	if idx := strings.Index(s, "-"); idx != -1 {
		preStr := s[idx+1:]
		s = s[:idx]
		if preStr != "" {
			preParts = strings.Split(preStr, ".")
		}
	}

	coreParts := strings.Split(s, ".")
	if len(coreParts) != 3 {
		return SemVer{}, fmt.Errorf("invalid semantic version format %q (expected major.minor.patch)", raw)
	}

	major, err := strconv.Atoi(coreParts[0])
	if err != nil || major < 0 {
		return SemVer{}, fmt.Errorf("invalid major version %q in %q", coreParts[0], raw)
	}

	minor, err := strconv.Atoi(coreParts[1])
	if err != nil || minor < 0 {
		return SemVer{}, fmt.Errorf("invalid minor version %q in %q", coreParts[1], raw)
	}

	patch, err := strconv.Atoi(coreParts[2])
	if err != nil || patch < 0 {
		return SemVer{}, fmt.Errorf("invalid patch version %q in %q", coreParts[2], raw)
	}

	return SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		PreRelease: preParts,
		Raw:        raw,
	}, nil
}

// Compare returns:
//  -1 if s < other
//   0 if s == other
//   1 if s > other
func (s SemVer) Compare(other SemVer) int {
	if s.Major != other.Major {
		if s.Major > other.Major {
			return 1
		}
		return -1
	}

	if s.Minor != other.Minor {
		if s.Minor > other.Minor {
			return 1
		}
		return -1
	}

	if s.Patch != other.Patch {
		if s.Patch > other.Patch {
			return 1
		}
		return -1
	}

	// Normal release has higher precedence than a pre-release version:
	// e.g. 1.0.0 > 1.0.0-rc1
	if len(s.PreRelease) == 0 && len(other.PreRelease) > 0 {
		return 1
	}
	if len(s.PreRelease) > 0 && len(other.PreRelease) == 0 {
		return -1
	}
	if len(s.PreRelease) == 0 && len(other.PreRelease) == 0 {
		return 0
	}

	// Both have pre-release identifiers: compare element-by-element
	minLen := len(s.PreRelease)
	if len(other.PreRelease) < minLen {
		minLen = len(other.PreRelease)
	}

	for i := 0; i < minLen; i++ {
		p1 := s.PreRelease[i]
		p2 := other.PreRelease[i]

		n1, err1 := strconv.Atoi(p1)
		n2, err2 := strconv.Atoi(p2)

		if err1 == nil && err2 == nil {
			if n1 != n2 {
				if n1 > n2 {
					return 1
				}
				return -1
			}
			continue
		}

		// Numeric has lower precedence than string identifier (SemVer spec 11.4)
		if err1 == nil && err2 != nil {
			return -1
		}
		if err1 != nil && err2 == nil {
			return 1
		}

		// Both are string identifiers: compare lexically
		if p1 != p2 {
			if p1 > p2 {
				return 1
			}
			return -1
		}
	}

	if len(s.PreRelease) > len(other.PreRelease) {
		return 1
	}
	if len(s.PreRelease) < len(other.PreRelease) {
		return -1
	}

	return 0
}

// CompareVersions parses and compares two version strings.
func CompareVersions(v1, v2 string) (int, error) {
	s1, err := ParseSemVer(v1)
	if err != nil {
		return 0, err
	}
	s2, err := ParseSemVer(v2)
	if err != nil {
		return 0, err
	}
	return s1.Compare(s2), nil
}

// AntiRollbackState records the highest accepted software and security versions.
type AntiRollbackState struct {
	HighestReleaseVersion  string `json:"highest_release_version"`
	HighestSecurityVersion uint64 `json:"highest_security_version"`
	LastAcceptedAt         string `json:"last_accepted_at"`
}

// AntiRollbackManager persists and validates anti-rollback invariants.
type AntiRollbackManager struct {
	filePath string
	mu       sync.RWMutex
}

// NewAntiRollbackManager creates or opens an anti-rollback persistence manager.
func NewAntiRollbackManager(filePath string) *AntiRollbackManager {
	return &AntiRollbackManager{
		filePath: filePath,
	}
}

// GetState returns the persisted anti-rollback state or initialized defaults.
func (m *AntiRollbackManager) GetState() (AntiRollbackState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return AntiRollbackState{}, nil
		}
		return AntiRollbackState{}, err
	}

	var st AntiRollbackState
	if err := json.Unmarshal(data, &st); err != nil {
		return AntiRollbackState{}, fmt.Errorf("corrupt anti-rollback state: %w", err)
	}

	return st, nil
}

// CheckUpdate validates that an incoming update manifest does not violate anti-rollback rules.
func (m *AntiRollbackManager) CheckUpdate(manifest *Manifest, allowDowngrade bool) error {
	if manifest == nil {
		return errors.New("cannot check nil manifest")
	}

	st, err := m.GetState()
	if err != nil {
		return err
	}

	// 1. Monotonic security version check
	if st.HighestSecurityVersion > 0 && manifest.SecurityVersion < st.HighestSecurityVersion {
		if !allowDowngrade {
			return fmt.Errorf("%w: manifest security version %d < highest accepted %d",
				ErrSecurityDowngrade, manifest.SecurityVersion, st.HighestSecurityVersion)
		}
	}

	// 2. SemVer release version check
	if st.HighestReleaseVersion != "" {
		cmp, err := CompareVersions(manifest.ReleaseVersion, st.HighestReleaseVersion)
		if err != nil {
			return fmt.Errorf("failed to compare versions: %w", err)
		}
		if cmp < 0 {
			if !allowDowngrade {
				return fmt.Errorf("%w: manifest version %s < highest accepted %s",
					ErrVersionDowngrade, manifest.ReleaseVersion, st.HighestReleaseVersion)
			}
		}
	}

	return nil
}

// RecordAccepted records an accepted update, monotonically advancing version counters.
func (m *AntiRollbackManager) RecordAccepted(manifest *Manifest) error {
	if manifest == nil {
		return errors.New("cannot record nil manifest")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	st := AntiRollbackState{}
	data, err := os.ReadFile(m.filePath)
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}

	// Advance highest release version if manifest is higher
	if st.HighestReleaseVersion == "" {
		st.HighestReleaseVersion = manifest.ReleaseVersion
	} else {
		cmp, err := CompareVersions(manifest.ReleaseVersion, st.HighestReleaseVersion)
		if err == nil && cmp > 0 {
			st.HighestReleaseVersion = manifest.ReleaseVersion
		}
	}

	// Advance highest security version if manifest is higher
	if manifest.SecurityVersion > st.HighestSecurityVersion {
		st.HighestSecurityVersion = manifest.SecurityVersion
	}

	st.LastAcceptedAt = time.Now().UTC().Format(time.RFC3339)

	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create anti-rollback directory: %w", err)
	}

	payload, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", m.filePath, os.Getpid())
	if err := os.WriteFile(tmpFile, payload, 0600); err != nil {
		return fmt.Errorf("failed to write temp anti-rollback state: %w", err)
	}

	if err := os.Rename(tmpFile, m.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to commit anti-rollback state: %w", err)
	}

	return nil
}

