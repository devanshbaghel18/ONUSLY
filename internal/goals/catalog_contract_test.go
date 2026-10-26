package goals_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

// TestCatalogContract_CrossPlatformConsistency asserts that the client target catalogs
// in Android (Kotlin TargetCatalog.kt), Extension (JavaScript APP_DOMAINS_MAP), and
// Frontend (JavaScript APP_CATALOG in blockedApps.js) exactly reflect the Go CanonicalCatalog.
func TestCatalogContract_CrossPlatformConsistency(t *testing.T) {
	// Locate repository root
	repoRoot := filepath.Join("..", "..")

	kotlinPath := filepath.Join(repoRoot, "android", "app", "src", "main", "java", "com", "onusly", "app", "data", "model", "TargetCatalog.kt")
	extensionPath := filepath.Join(repoRoot, "extension", "src", "blockerEngine.js")
	frontendPath := filepath.Join(repoRoot, "frontend", "src", "lib", "blockedApps.js")

	kotlinBytes, err := os.ReadFile(kotlinPath)
	if err != nil {
		t.Fatalf("Failed to read Kotlin TargetCatalog at %s: %v", kotlinPath, err)
	}
	kotlinContent := string(kotlinBytes)

	extensionBytes, err := os.ReadFile(extensionPath)
	if err != nil {
		t.Fatalf("Failed to read Extension blockerEngine.js at %s: %v", extensionPath, err)
	}
	extensionContent := string(extensionBytes)

	frontendBytes, err := os.ReadFile(frontendPath)
	if err != nil {
		t.Fatalf("Failed to read Frontend blockedApps.js at %s: %v", frontendPath, err)
	}
	frontendContent := string(frontendBytes)

	// 1. Every entry in Go CanonicalCatalog must exist in all client platforms
	for _, entry := range goals.CanonicalCatalog {
		t.Run("Service_"+entry.Name, func(t *testing.T) {
			// Check apps in all catalogs
			for _, app := range entry.Apps {
				if !strings.Contains(kotlinContent, `"`+app+`"`) {
					t.Errorf("Kotlin TargetCatalog.kt is missing canonical app package: %q (service: %s)", app, entry.Name)
				}
				if !strings.Contains(extensionContent, `"`+app+`"`) {
					t.Errorf("Extension APP_DOMAINS_MAP is missing canonical app package: %q (service: %s)", app, entry.Name)
				}
				if !strings.Contains(frontendContent, `"`+app+`"`) {
					t.Errorf("Frontend APP_CATALOG is missing canonical app package: %q (service: %s)", app, entry.Name)
				}
			}

			// Check domains in all catalogs
			for _, domain := range entry.Domains {
				if !strings.Contains(kotlinContent, `"`+domain+`"`) {
					t.Errorf("Kotlin TargetCatalog.kt is missing canonical domain: %q (service: %s)", domain, entry.Name)
				}
				if !strings.Contains(extensionContent, `"`+domain+`"`) {
					t.Errorf("Extension APP_DOMAINS_MAP is missing canonical domain: %q (service: %s)", domain, entry.Name)
				}
				if !strings.Contains(frontendContent, `"`+domain+`"`) {
					t.Errorf("Frontend APP_CATALOG is missing canonical domain: %q (service: %s)", domain, entry.Name)
				}
			}
		})
	}

	// 2. Verify Package Aliases (multi-package apps)
	t.Run("PackageAliases", func(t *testing.T) {
		// TikTok must have both musically and trill
		tiktokPkgs := []string{"com.zhiliaoapp.musically", "com.ss.android.ugc.trill"}
		for _, pkg := range tiktokPkgs {
			if !strings.Contains(kotlinContent, `"`+pkg+`"`) {
				t.Errorf("Kotlin missing TikTok package alias: %s", pkg)
			}
			if !strings.Contains(extensionContent, `"`+pkg+`"`) {
				t.Errorf("Extension missing TikTok package alias: %s", pkg)
			}
			if !strings.Contains(frontendContent, `"`+pkg+`"`) {
				t.Errorf("Frontend missing TikTok package alias: %s", pkg)
			}
		}

		// Facebook must have both katana and orca
		fbPkgs := []string{"com.facebook.katana", "com.facebook.orca"}
		for _, pkg := range fbPkgs {
			if !strings.Contains(kotlinContent, `"`+pkg+`"`) {
				t.Errorf("Kotlin missing Facebook package alias: %s", pkg)
			}
			if !strings.Contains(extensionContent, `"`+pkg+`"`) {
				t.Errorf("Extension missing Facebook package alias: %s", pkg)
			}
			if !strings.Contains(frontendContent, `"`+pkg+`"`) {
				t.Errorf("Frontend missing Facebook package alias: %s", pkg)
			}
		}
	})

	// 3. Verify Domain Aliases (multi-domain services)
	t.Run("DomainAliases", func(t *testing.T) {
		// Twitter / X must have both x.com and twitter.com
		twitterDomains := []string{"x.com", "twitter.com"}
		for _, dom := range twitterDomains {
			if !strings.Contains(kotlinContent, `"`+dom+`"`) {
				t.Errorf("Kotlin missing Twitter domain alias: %s", dom)
			}
			if !strings.Contains(extensionContent, `"`+dom+`"`) {
				t.Errorf("Extension missing Twitter domain alias: %s", dom)
			}
			if !strings.Contains(frontendContent, `"`+dom+`"`) {
				t.Errorf("Frontend missing Twitter domain alias: %s", dom)
			}
		}

		// YouTube must have both youtube.com and youtu.be
		ytDomains := []string{"youtube.com", "youtu.be"}
		for _, dom := range ytDomains {
			if !strings.Contains(kotlinContent, `"`+dom+`"`) {
				t.Errorf("Kotlin missing YouTube domain alias: %s", dom)
			}
			if !strings.Contains(extensionContent, `"`+dom+`"`) {
				t.Errorf("Extension missing YouTube domain alias: %s", dom)
			}
			if !strings.Contains(frontendContent, `"`+dom+`"`) {
				t.Errorf("Frontend missing YouTube domain alias: %s", dom)
			}
		}
	})

	// 4. Verify Strict YouTube Shorts Isolation
	t.Run("YouTubeShortsStrictIsolation", func(t *testing.T) {
		// YouTube Shorts must have shorts.youtube.com and com.google.android.youtube.shorts
		if !strings.Contains(kotlinContent, `"shorts.youtube.com"`) || !strings.Contains(kotlinContent, `"com.google.android.youtube.shorts"`) {
			t.Errorf("Kotlin missing YouTube Shorts canonical definitions")
		}
		if !strings.Contains(extensionContent, `"shorts.youtube.com"`) || !strings.Contains(extensionContent, `"com.google.android.youtube.shorts"`) {
			t.Errorf("Extension missing YouTube Shorts canonical definitions")
		}

		// Check Go catalog maps
		shortsDomains := goals.GetDomainsForApp("com.google.android.youtube.shorts")
		if len(shortsDomains) != 1 || shortsDomains[0] != "shorts.youtube.com" {
			t.Errorf("Go catalog: expected only shorts.youtube.com for Shorts app, got %v", shortsDomains)
		}

		shortsApps := goals.GetAppsForDomain("shorts.youtube.com")
		if len(shortsApps) != 1 || shortsApps[0] != "com.google.android.youtube.shorts" {
			t.Errorf("Go catalog: expected only com.google.android.youtube.shorts for shorts.youtube.com, got %v", shortsApps)
		}

		// Full YouTube app must NEVER return shorts.youtube.com
		ytDomains := goals.GetDomainsForApp("com.google.android.youtube")
		for _, d := range ytDomains {
			if d == "shorts.youtube.com" {
				t.Fatalf("CONTRACT BREACH: Full YouTube app mapped to shorts.youtube.com!")
			}
		}

		// Full YouTube domain must NEVER return shorts pseudo-package
		ytApps := goals.GetAppsForDomain("youtube.com")
		for _, a := range ytApps {
			if a == "com.google.android.youtube.shorts" {
				t.Fatalf("CONTRACT BREACH: Full YouTube domain mapped to com.google.android.youtube.shorts!")
			}
		}
	})
}
