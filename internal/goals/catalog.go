package goals

import (
	"strings"
)

// CatalogEntry defines a canonical association between a service's native app package names and web domains.
type CatalogEntry struct {
	Name    string   `json:"name"`
	Apps    []string `json:"apps"`
	Domains []string `json:"domains"`
}

// CanonicalCatalog is the backend source of truth for canonical app-to-domain associations.
var CanonicalCatalog = []CatalogEntry{
	{
		Name:    "Instagram",
		Apps:    []string{"com.instagram.android"},
		Domains: []string{"instagram.com"},
	},
	{
		Name:    "YouTube",
		Apps:    []string{"com.google.android.youtube"},
		Domains: []string{"youtube.com", "youtu.be"},
	},
	{
		// CRITICAL: path-only rule! Must NOT associate with com.google.android.youtube or youtube.com.
		Name:    "YouTube Shorts",
		Apps:    []string{"com.google.android.youtube.shorts"},
		Domains: []string{"shorts.youtube.com"},
	},
	{
		Name:    "Twitter / X",
		Apps:    []string{"com.twitter.android"},
		Domains: []string{"x.com", "twitter.com"},
	},
	{
		Name:    "Reddit",
		Apps:    []string{"com.reddit.frontpage"},
		Domains: []string{"reddit.com"},
	},
	{
		Name:    "Netflix",
		Apps:    []string{"com.netflix.mediaclient"},
		Domains: []string{"netflix.com"},
	},
	{
		Name:    "Discord",
		Apps:    []string{"com.discord"},
		Domains: []string{"discord.com"},
	},
	{
		Name:    "Twitch",
		Apps:    []string{"tv.twitch.android.app"},
		Domains: []string{"twitch.tv"},
	},
	{
		Name:    "TikTok",
		Apps:    []string{"com.zhiliaoapp.musically", "com.ss.android.ugc.trill"},
		Domains: []string{"tiktok.com"},
	},
	{
		Name:    "Facebook",
		Apps:    []string{"com.facebook.katana", "com.facebook.orca"},
		Domains: []string{"facebook.com"},
	},
	{
		Name:    "Snapchat",
		Apps:    []string{"com.snapchat.android"},
		Domains: []string{"snapchat.com"},
	},
	{
		Name:    "LinkedIn",
		Apps:    []string{"com.linkedin.android"},
		Domains: []string{"linkedin.com"},
	},
	{
		Name:    "Pinterest",
		Apps:    []string{"com.pinterest"},
		Domains: []string{"pinterest.com"},
	},
}

var (
	appToDomainsMap = make(map[string][]string)
	domainToAppsMap = make(map[string][]string)
)

func init() {
	for _, entry := range CanonicalCatalog {
		for _, app := range entry.Apps {
			appToDomainsMap[app] = entry.Domains
		}
		for _, domain := range entry.Domains {
			domainToAppsMap[domain] = entry.Apps
		}
	}
}

// GetDomainsForApp returns the canonical domain names associated with an app package name, or nil if unmapped.
func GetDomainsForApp(appPackage string) []string {
	trimmed := strings.TrimSpace(appPackage)
	if trimmed == "" {
		return nil
	}
	domains, ok := appToDomainsMap[trimmed]
	if !ok || len(domains) == 0 {
		return nil
	}
	res := make([]string, len(domains))
	copy(res, domains)
	return res
}

// GetAppsForDomain returns the canonical app package names associated with a domain, or nil for unmapped/custom domains.
func GetAppsForDomain(domain string) []string {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return nil
	}
	cleaned, err := cleanAndValidateSingleDomain(trimmed)
	if err != nil {
		cleaned = strings.ToLower(trimmed)
		if strings.HasPrefix(cleaned, "www.") {
			cleaned = strings.TrimPrefix(cleaned, "www.")
		}
		cleaned = strings.TrimSuffix(cleaned, ".")
	}
	apps, ok := domainToAppsMap[cleaned]
	if !ok || len(apps) == 0 {
		return nil
	}
	res := make([]string, len(apps))
	copy(res, apps)
	return res
}

// ExpandAssociatedTargets safely and idempotently expands missing associated domains from apps
// and missing associated apps from domains according to the CanonicalCatalog.
// Normalizes and deduplicates results via NormalizeAndValidateApps and NormalizeAndValidateDomains.
// Custom unmapped domains remain domain-only with no app generated.
// YouTube Shorts is strictly isolated from full YouTube.
func ExpandAssociatedTargets(rawApps []string, rawDomains []string) ([]string, []string, error) {
	var normApps []string
	var normDomains []string
	var err error

	if len(rawApps) > 0 {
		normApps, err = NormalizeAndValidateApps(rawApps)
		if err != nil {
			return nil, nil, err
		}
	} else {
		normApps = []string{}
	}

	if len(rawDomains) > 0 {
		normDomains, err = NormalizeAndValidateDomains(rawDomains)
		if err != nil {
			return nil, nil, err
		}
	} else {
		normDomains = []string{}
	}

	if len(normApps) == 0 && len(normDomains) == 0 {
		return []string{}, []string{}, nil
	}

	hasTwitterInRawDomains := false
	hasXInRawDomains := false
	for _, d := range normDomains {
		if d == "twitter.com" {
			hasTwitterInRawDomains = true
		}
		if d == "x.com" {
			hasXInRawDomains = true
		}
	}

	resultApps := append([]string(nil), normApps...)
	resultDomains := append([]string(nil), normDomains...)

	// 1. Expand domains from apps
	for _, a := range normApps {
		associatedDomains := GetDomainsForApp(a)
		if len(associatedDomains) == 0 {
			continue
		}
		if a == "com.twitter.android" {
			// Compatibility rule:
			// If com.twitter.android is present and either x.com or twitter.com is ALREADY in rawDomains,
			// do not force both unless neither is present.
			if hasTwitterInRawDomains || hasXInRawDomains {
				// At least one is already in rawDomains; do not force the other.
				continue
			}
			resultDomains = append(resultDomains, associatedDomains...)
		} else {
			resultDomains = append(resultDomains, associatedDomains...)
		}
	}

	// 2. Expand apps from domains
	for _, d := range normDomains {
		associatedApps := GetAppsForDomain(d)
		if len(associatedApps) == 0 {
			// Custom / unmapped domain: remains domain-only, no app generated
			continue
		}
		resultApps = append(resultApps, associatedApps...)
	}

	// 3. For any cataloged entries that were added, ensure bidirectional consistency
	// (e.g. if facebook.com was added from com.facebook.katana, or tiktok.com from com.zhiliaoapp.musically)
	// while respecting the Twitter exception.
	for _, d := range resultDomains {
		if d == "twitter.com" || d == "x.com" {
			if !containsString(resultApps, "com.twitter.android") {
				resultApps = append(resultApps, "com.twitter.android")
			}
			continue
		}
		associatedApps := GetAppsForDomain(d)
		if len(associatedApps) > 0 {
			resultApps = append(resultApps, associatedApps...)
		}
	}

	finalApps, err := NormalizeAndValidateApps(resultApps)
	if err != nil {
		return nil, nil, err
	}
	finalDomains, err := NormalizeAndValidateDomains(resultDomains)
	if err != nil {
		return nil, nil, err
	}

	return finalApps, finalDomains, nil
}

func containsString(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}
