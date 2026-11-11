package dns

import (
	"net"
	"strings"

	"golang.org/x/net/idna"
)

// NormalizeDomain cleans and canonicalizes a domain name:
// trims whitespace, converts to lowercase, converts IDN/Punycode to ASCII,
// removes trailing dots, and strips URI schemes or ports if present.
func NormalizeDomain(domain string) string {
	d := strings.TrimSpace(domain)
	if d == "" {
		return ""
	}

	// Strip URL schemes if inadvertently included
	if idx := strings.Index(d, "://"); idx != -1 {
		d = d[idx+3:]
	}

	// Strip path, query, fragment
	for _, sep := range []string{"/", "?", "#"} {
		if idx := strings.Index(d, sep); idx != -1 {
			d = d[:idx]
		}
	}

	// Strip port if present (distinguishing IPv6 literals)
	if strings.Contains(d, ":") && !strings.HasPrefix(d, "[") {
		if host, _, err := net.SplitHostPort(d); err == nil {
			d = host
		} else {
			// Simple split if SplitHostPort failed
			parts := strings.Split(d, ":")
			d = parts[0]
		}
	}

	// Remove leading and trailing dots
	d = strings.Trim(d, ".")
	d = strings.ToLower(d)

	// Punycode / IDNA normalization (ASCII format for DNS wire matching)
	if ascii, err := idna.Lookup.ToASCII(d); err == nil && ascii != "" {
		d = ascii
	}

	return d
}

// IsDomainMatch checks if a candidate query hostname matches a target blocked domain.
// Supports:
// 1. Exact match: "example.com" == "example.com"
// 2. Subdomain match: "sub.example.com" matches "example.com"
// 3. Normalized "www" parity: "www.example.com" matches "example.com" and vice versa.
// Strictly prevents unsafe substring matching:
// - "notexample.com" does NOT match "example.com"
// - "example.com.evil.test" does NOT match "example.com"
// - "evil-example.com" does NOT match "example.com"
func IsDomainMatch(queryHostname, targetDomain string) bool {
	q := NormalizeDomain(queryHostname)
	t := NormalizeDomain(targetDomain)

	if q == "" || t == "" {
		return false
	}

	// 1. Direct equality
	if q == t {
		return true
	}

	// 2. Subdomain match on target
	if strings.HasSuffix(q, "."+t) {
		return true
	}

	// 3. Stripped 'www.' normalization
	cleanQ := strings.TrimPrefix(q, "www.")
	cleanT := strings.TrimPrefix(t, "www.")

	if cleanQ == cleanT {
		return true
	}
	if strings.HasSuffix(cleanQ, "."+cleanT) {
		return true
	}

	return false
}

// IsProtectedDomain returns true if the domain must NEVER be blocked by ONUSLY
// (e.g. localhost, local LAN infrastructure, ONUSLY cloud dashboard/API endpoints).
func IsProtectedDomain(domain string) bool {
	d := NormalizeDomain(domain)
	if d == "" {
		return true
	}

	// Localhost and loopback addresses
	if d == "localhost" || strings.HasSuffix(d, ".localhost") {
		return true
	}
	if d == "127.0.0.1" || d == "::1" || d == "0.0.0.0" {
		return true
	}

	// Local LAN, multicast, and home network standard suffixes
	if strings.HasSuffix(d, ".local") || strings.HasSuffix(d, ".internal") ||
		strings.HasSuffix(d, ".home.arpa") || strings.HasSuffix(d, ".lan") {
		return true
	}

	// Reverse DNS lookups for RFC1918 private IP ranges
	if strings.HasSuffix(d, ".in-addr.arpa") || strings.HasSuffix(d, ".ip6.arpa") {
		return true
	}

	// ONUSLY production, staging, and API domains
	if d == "onusly.com" || strings.HasSuffix(d, ".onusly.com") {
		return true
	}

	return false
}

