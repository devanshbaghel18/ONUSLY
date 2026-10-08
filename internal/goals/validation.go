package goals

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var (
	// Android package name regex:
	// Starts with letter, segments separated by dot, each segment starts with letter followed by alphanumeric or underscore.
	appPackageRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z][A-Za-z0-9_]*)+$`)

	// DNS label regex:
	// Starts and ends with alphanumeric, hyphens allowed in between; or single alphanumeric character.
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

const (
	MaxAppsCount     = 20
	MaxDomainsCount  = 20
	MaxAppLength     = 255
	MaxDomainLength  = 253
	MaxDNSLabelLength = 63
)

// NormalizeAndValidateApps trims, preserves case, validates against the Android package
// regex (max 255 chars), deduplicates, and sorts ascending.
// Returns an error identifying the bad value if any item is invalid, or if exceeding 20 items.
func NormalizeAndValidateApps(rawApps []string) ([]string, error) {
	if len(rawApps) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]bool)
	var normalized []string

	for _, raw := range rawApps {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("invalid app package name %q: package name cannot be empty", raw)
		}
		if len(trimmed) > MaxAppLength {
			return nil, fmt.Errorf("invalid app package name %q: length exceeds %d characters", raw, MaxAppLength)
		}
		if !appPackageRegex.MatchString(trimmed) {
			return nil, fmt.Errorf("invalid app package name %q: must match format (e.g. com.example.app)", raw)
		}

		if !seen[trimmed] {
			seen[trimmed] = true
			normalized = append(normalized, trimmed)
		}
	}

	if len(normalized) > MaxAppsCount {
		return nil, fmt.Errorf("maximum %d apps allowed, got %d", MaxAppsCount, len(normalized))
	}

	sort.Strings(normalized)
	return normalized, nil
}

// NormalizeAndValidateDomains trims, lowercases, strips scheme/path/query/port/leading www./trailing dot,
// rejects IP literals, wildcards, non-ASCII, validates DNS labels (at least one dot, max 253 chars),
// deduplicates, and sorts ascending.
// Returns an error identifying the bad value if any item is invalid, or if exceeding 20 items.
func NormalizeAndValidateDomains(rawDomains []string) ([]string, error) {
	if len(rawDomains) == 0 {
		return []string{}, nil
	}

	seen := make(map[string]bool)
	var normalized []string

	for _, raw := range rawDomains {
		cleaned, err := cleanAndValidateSingleDomain(raw)
		if err != nil {
			return nil, err
		}
		if !seen[cleaned] {
			seen[cleaned] = true
			normalized = append(normalized, cleaned)
		}
	}

	if len(normalized) > MaxDomainsCount {
		return nil, fmt.Errorf("maximum %d domains allowed, got %d", MaxDomainsCount, len(normalized))
	}

	sort.Strings(normalized)
	return normalized, nil
}

func cleanAndValidateSingleDomain(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("invalid domain %q: domain cannot be empty", raw)
	}

	d := strings.ToLower(trimmed)

	// Check non-ASCII
	for i := 0; i < len(d); i++ {
		if d[i] > 127 || d[i] < 32 {
			return "", fmt.Errorf("invalid domain %q: non-ASCII characters are not allowed", raw)
		}
	}

	// Reject wildcards
	if strings.Contains(d, "*") {
		return "", fmt.Errorf("invalid domain %q: wildcards are not allowed", raw)
	}

	// Strip scheme (e.g. https:// or http:// or custom:// or //)
	if idx := strings.Index(d, "://"); idx != -1 {
		d = d[idx+3:]
	} else if strings.HasPrefix(d, "//") {
		d = strings.TrimPrefix(d, "//")
	}

	// Strip path, query, fragment
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(d, sep); idx != -1 {
			d = d[:idx]
		}
	}

	// Check IP literal before port stripping (e.g. [::1]:8080 or ::1 or 127.0.0.1:80)
	rawHostCandidate := strings.Trim(d, "[]")
	if net.ParseIP(rawHostCandidate) != nil {
		return "", fmt.Errorf("invalid domain %q: IP literals are not allowed", raw)
	}

	// Strip port (e.g. :8080)
	if strings.Contains(d, ":") {
		host, _, err := net.SplitHostPort(d)
		if err == nil {
			d = host
		} else {
			lastColon := strings.LastIndex(d, ":")
			portPart := d[lastColon+1:]
			if _, err := strconv.Atoi(portPart); err == nil {
				d = d[:lastColon]
			}
		}
	}

	// Recheck IP literal after port stripping
	d = strings.Trim(d, "[]")
	if net.ParseIP(d) != nil {
		return "", fmt.Errorf("invalid domain %q: IP literals are not allowed", raw)
	}

	// Strip leading www.
	if strings.HasPrefix(d, "www.") {
		d = strings.TrimPrefix(d, "www.")
	}

	// Strip trailing dot .
	d = strings.TrimSuffix(d, ".")

	if d == "" {
		return "", fmt.Errorf("invalid domain %q: resulting domain is empty", raw)
	}

	if len(d) > MaxDomainLength {
		return "", fmt.Errorf("invalid domain %q: length exceeds %d characters", raw, MaxDomainLength)
	}

	// Reject IP literals (IPv4 and IPv6)
	if net.ParseIP(d) != nil {
		return "", fmt.Errorf("invalid domain %q: IP literals are not allowed", raw)
	}

	// Require valid DNS labels with at least one dot
	labels := strings.Split(d, ".")
	if len(labels) < 2 {
		return "", fmt.Errorf("invalid domain %q: must contain at least one dot (valid domain name required)", raw)
	}

	// Check if all labels are numeric (e.g. 192.168.1.1 variants or IPv4-like)
	allNumeric := true
	for _, l := range labels {
		if _, err := strconv.Atoi(l); err != nil {
			allNumeric = false
			break
		}
	}
	if allNumeric {
		return "", fmt.Errorf("invalid domain %q: IP literals are not allowed", raw)
	}

	for _, label := range labels {
		if len(label) < 1 || len(label) > MaxDNSLabelLength {
			return "", fmt.Errorf("invalid domain %q: label %q must be between 1 and %d characters", raw, label, MaxDNSLabelLength)
		}
		if !dnsLabelRegex.MatchString(label) {
			return "", fmt.Errorf("invalid domain %q: label %q contains invalid characters or hyphens at ends", raw, label)
		}
	}

	return d, nil
}

// CheckTargetsSubset verifies that all items in existing are present in updated.
// Returns false if any item from existing is missing in updated.
func CheckTargetsSubset(existing, updated []string) bool {
	if len(existing) == 0 {
		return true
	}
	set := make(map[string]bool, len(updated))
	for _, item := range updated {
		set[item] = true
	}
	for _, item := range existing {
		if !set[item] {
			return false
		}
	}
	return true
}

// Errors for target and goal mutation locking
var (
	ErrGoalLocked           = errors.New("goal_locked")
	ErrTargetsLocked        = errors.New("targets_locked")
	ErrAccountabilityLocked = errors.New("cannot change accountability to none while targets are locked")
)
