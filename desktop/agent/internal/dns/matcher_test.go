package dns

import (
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestNormalizeDomain(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"EXAMPLE.COM", "example.com"},
		{"  example.com.  ", "example.com"},
		{"http://example.com/some/path", "example.com"},
		{"https://EXAMPLE.COM:8080/query?a=1", "example.com"},
		{"münchen.de", "xn--mnchen-3ya.de"},
		{"xn--mnchen-3ya.de", "xn--mnchen-3ya.de"},
		{"...sub.example.com...", "sub.example.com"},
		{"", ""},
		{"   ", ""},
	}

	for _, tt := range tests {
		actual := NormalizeDomain(tt.input)
		if actual != tt.expected {
			t.Errorf("NormalizeDomain(%q) = %q; expected %q", tt.input, actual, tt.expected)
		}
	}
}

func TestIsDomainMatch(t *testing.T) {
	tests := []struct {
		query    string
		target   string
		expected bool
	}{
		// Exact matches
		{"example.com", "example.com", true},
		{"EXAMPLE.COM", "example.com", true},
		{"example.com.", "example.com", true},

		// Subdomains
		{"www.example.com", "example.com", true},
		{"sub.example.com", "example.com", true},
		{"a.b.c.example.com", "example.com", true},
		{"m.youtube.com", "youtube.com", true},

		// Normalized www parity
		{"example.com", "www.example.com", true},
		{"sub.example.com", "www.example.com", true},

		// Strict boundary checks: unsafe substrings must NOT match
		{"notexample.com", "example.com", false},
		{"example.com.evil.test", "example.com", false},
		{"evil-example.com", "example.com", false},
		{"myexample.com", "example.com", false},
		{"example.community", "example.com", false},

		// Internationalized Domain Names (IDN)
		{"sub.münchen.de", "münchen.de", true},
		{"sub.xn--mnchen-3ya.de", "münchen.de", true},

		// Empty/Malformed inputs
		{"", "example.com", false},
		{"example.com", "", false},
		{"", "", false},
	}

	for _, tt := range tests {
		actual := IsDomainMatch(tt.query, tt.target)
		if actual != tt.expected {
			t.Errorf("IsDomainMatch(%q, %q) = %v; expected %v", tt.query, tt.target, actual, tt.expected)
		}
	}
}

func TestIsProtectedDomain(t *testing.T) {
	protected := []string{
		"localhost",
		"sub.localhost",
		"127.0.0.1",
		"::1",
		"mydevice.local",
		"printer.internal",
		"router.home.arpa",
		"1.0.0.127.in-addr.arpa",
		"onusly.com",
		"api.onusly.com",
		"dashboard.onusly.com",
		"onusly.app",
		"onusly.duckdns.org",
	}

	for _, p := range protected {
		if !IsProtectedDomain(p) {
			t.Errorf("IsProtectedDomain(%q) = false; expected true", p)
		}
	}

	unprotected := []string{
		"reddit.com",
		"youtube.com",
		"facebook.com",
		"twitter.com",
		"notonusly.com",
		"onusly.com.evil.com",
	}

	for _, u := range unprotected {
		if IsProtectedDomain(u) {
			t.Errorf("IsProtectedDomain(%q) = true; expected false", u)
		}
	}
}

func TestIsPathScopedShortsRule_DoesNotBlockFullYouTube(t *testing.T) {
	if !IsPathScopedShortsRule("youtube.com/shorts") || !IsPathScopedShortsRule("https://www.youtube.com/shorts/abc") {
		t.Fatalf("expected youtube.com/shorts URLs to be recognized as path-scoped Shorts rules")
	}
	if IsPathScopedShortsRule("youtube.com") || IsPathScopedShortsRule("shorts.youtube.com") {
		t.Fatalf("did not expect plain hostnames to be classified as path-scoped Shorts rules")
	}

	pm := NewPolicyManager(NewMemoryDNSCache(100))
	_, err := pm.UpdateFromBlocklist(&core.BlocklistData{
		Version: "v1-shorts-only",
		Targets: []core.TargetItem{
			{
				GoalID:  "g-shorts",
				Status:  "active",
				Domains: []string{"youtube.com/shorts", "m.youtube.com/shorts", "shorts.youtube.com"},
			},
		},
	})
	if err != nil {
		t.Fatalf("UpdateFromBlocklist failed: %v", err)
	}
	if blocked, _ := pm.CheckBlocked("www.youtube.com"); blocked {
		t.Fatalf("path-scoped youtube.com/shorts rule must NOT block www.youtube.com in DNS")
	}
	if blocked, _ := pm.CheckBlocked("shorts.youtube.com"); !blocked {
		t.Fatalf("expected explicit shorts.youtube.com host to remain blocked")
	}
}
