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

	// Canonical desktop application identifier regex:
	// Starts with alphanumeric, can contain alphanumeric, dots, underscores, hyphens. Max 255 chars.
	desktopAppRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

	// DNS label regex:
	// Starts and ends with alphanumeric, hyphens allowed in between; or single alphanumeric character.
	dnsLabelRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
)

// Protected operating system processes and ONUSLY components that must never be restricted.
var protectedAppTargets = map[string]bool{
	// Windows core system & shells
	"csrss":                    true,
	"smss":                     true,
	"wininit":                  true,
	"winlogon":                 true,
	"services":                 true,
	"lsass":                    true,
	"svchost":                  true,
	"dwm":                      true,
	"explorer":                 true,
	"taskmgr":                  true,
	"sihost":                   true,
	"fontdrvhost":             true,
	"searchhost":              true,
	"shellexperiencehost":     true,
	"startmenuexperiencehost":  true,
	"ctfmon":                  true,
	"conhost":                 true,
	"cmd":                     true,
	"powershell":              true,
	"pwsh":                    true,
	"wsmprovhost":             true,
	"dllhost":                 true,

	// Linux core system & daemons
	"systemd":                 true,
	"init":                    true,
	"kthreadd":                true,
	"dbus-daemon":             true,
	"systemd-journald":        true,
	"systemd-logind":          true,
	"systemd-resolved":        true,
	"systemd-udevd":           true,
	"xorg":                    true,
	"wayland":                 true,
	"gnome-shell":             true,
	"plasmashell":             true,
	"bash":                    true,
	"sh":                      true,
	"zsh":                     true,
	"login":                   true,
	"sshd":                    true,
	"cron":                    true,
	"polkitd":                 true,

	// ONUSLY Desktop Agent & Native Messaging Host
	"onusly-agent":            true,
	"onusly-native-host":      true,
	"onusly":                  true,
}

// IsProtectedAppTarget returns true if the target is an operating system component or ONUSLY binary.
func IsProtectedAppTarget(app string) bool {
	norm := strings.ToLower(strings.TrimSpace(app))
	norm = strings.TrimSuffix(norm, ".exe")
	norm = strings.TrimSuffix(norm, ".desktop")
	if protectedAppTargets[norm] {
		return true
	}
	compact := strings.ReplaceAll(strings.ReplaceAll(norm, "-", ""), "_", "")
	for p := range protectedAppTargets {
		pCompact := strings.ReplaceAll(strings.ReplaceAll(p, "-", ""), "_", "")
		if compact == pCompact {
			return true
		}
	}
	return false
}

const (
	MaxAppsCount      = 20
	MaxDomainsCount   = 20
	MaxAppLength      = 255
	MaxDomainLength   = 253
	MaxDNSLabelLength = 63
)

// NormalizeAndValidateApps trims, validates against Android packages or canonical
// desktop application identifiers (max 255 chars), rejects protected system processes,
// deduplicates, and sorts ascending.
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

		// Reject path traversal and directory paths
		if strings.Contains(trimmed, "/") || strings.Contains(trimmed, "\\") {
			return nil, fmt.Errorf("invalid app target %q: path traversal and directory paths are not allowed", raw)
		}

		// Reject shell metacharacters / injection
		for _, ch := range []string{"&", "|", ";", ">", "<", "$", "`", "*", "?", "~", "!", "(", ")", "{", "}", "[", "]", "'", "\"", "\n", "\r", "\t"} {
			if strings.Contains(trimmed, ch) {
				return nil, fmt.Errorf("invalid app target %q: illegal characters not allowed", raw)
			}
		}

		// Strip .exe or .desktop suffix if supplied
		cleanID := strings.TrimSuffix(trimmed, ".exe")
		cleanID = strings.TrimSuffix(cleanID, ".desktop")

		// Check protected processes
		if IsProtectedAppTarget(cleanID) {
			return nil, fmt.Errorf("invalid app target %q: system and ONUSLY processes are protected and cannot be restricted", raw)
		}

		// Must match Android package or canonical desktop identifier
		if !appPackageRegex.MatchString(cleanID) && !desktopAppRegex.MatchString(cleanID) {
			return nil, fmt.Errorf("invalid app target %q: must match format (e.g. com.example.app or discord)", raw)
		}

		if !seen[cleanID] {
			seen[cleanID] = true
			normalized = append(normalized, cleanID)
		}
	}

	if len(normalized) > MaxAppsCount {
		return nil, fmt.Errorf("maximum %d apps allowed, got %d", MaxAppsCount, len(normalized))
	}

	sort.Strings(normalized)
	return normalized, nil
}

// NormalizeAndValidateApplicationTargets validates and normalizes explicitly typed targets.
func NormalizeAndValidateApplicationTargets(rawTargets []ApplicationTarget) ([]ApplicationTarget, error) {
	if len(rawTargets) == 0 {
		return []ApplicationTarget{}, nil
	}

	seen := make(map[string]bool)
	var normalized []ApplicationTarget

	for _, t := range rawTargets {
		kind := strings.ToLower(strings.TrimSpace(t.Kind))
		id := strings.TrimSpace(t.Identifier)

		if kind != "desktop_app" && kind != "android_package" {
			return nil, fmt.Errorf("invalid application target kind %q: must be 'desktop_app' or 'android_package'", t.Kind)
		}

		if id == "" {
			return nil, fmt.Errorf("invalid application target identifier for %s: cannot be empty", kind)
		}
		if len(id) > MaxAppLength {
			return nil, fmt.Errorf("invalid application target identifier %q: length exceeds %d characters", id, MaxAppLength)
		}

		if kind == "desktop_app" {
			id = strings.TrimSuffix(id, ".exe")
			id = strings.TrimSuffix(id, ".desktop")

			if IsProtectedAppTarget(id) {
				return nil, fmt.Errorf("invalid application target %q: system and ONUSLY processes are protected and cannot be restricted", t.Identifier)
			}
			if strings.Contains(id, "/") || strings.Contains(id, "\\") {
				return nil, fmt.Errorf("invalid application target %q: path traversal and directory paths are not allowed", t.Identifier)
			}
			for _, ch := range []string{"&", "|", ";", ">", "<", "$", "`", "*", "?", "~", "!", "(", ")", "{", "}", "[", "]", "'", "\"", "\n", "\r", "\t"} {
				if strings.Contains(id, ch) {
					return nil, fmt.Errorf("invalid application target %q: illegal characters not allowed", t.Identifier)
				}
			}
			if !desktopAppRegex.MatchString(id) {
				return nil, fmt.Errorf("invalid desktop app identifier %q: must start with letter or number", t.Identifier)
			}
		} else {
			// android_package
			if !appPackageRegex.MatchString(id) {
				return nil, fmt.Errorf("invalid android package name %q: must match format (e.g. com.example.app)", t.Identifier)
			}
		}

		key := kind + ":" + strings.ToLower(id)
		if !seen[key] {
			seen[key] = true
			normalized = append(normalized, ApplicationTarget{
				Kind:       kind,
				Identifier: id,
			})
		}
	}

	if len(normalized) > MaxAppsCount {
		return nil, fmt.Errorf("maximum %d application targets allowed, got %d", MaxAppsCount, len(normalized))
	}

	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Kind == normalized[j].Kind {
			return normalized[i].Identifier < normalized[j].Identifier
		}
		return normalized[i].Kind < normalized[j].Kind
	})

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
	ErrGoalLocked             = errors.New("goal_locked")
	ErrTargetsLocked          = errors.New("targets_locked")
	ErrAccountabilityLocked   = errors.New("cannot change accountability to none while targets are locked")
	ErrAccountabilityRequired = errors.New("accountability_required")
	ErrGoalInReview           = errors.New("goal_in_review")
	ErrGoalConflict           = errors.New("goal_conflict: resource was modified concurrently")
)
