package goals_test

import (
	"reflect"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

func TestCatalog_GetDomainsForApp(t *testing.T) {
	tests := []struct {
		app      string
		expected []string
	}{
		{"com.instagram.android", []string{"instagram.com"}},
		{"com.google.android.youtube", []string{"youtube.com", "youtu.be"}},
		{"com.google.android.youtube.shorts", []string{"shorts.youtube.com"}},
		{"com.twitter.android", []string{"x.com", "twitter.com"}},
		{"com.reddit.frontpage", []string{"reddit.com"}},
		{"com.netflix.mediaclient", []string{"netflix.com"}},
		{"com.discord", []string{"discord.com"}},
		{"tv.twitch.android.app", []string{"twitch.tv"}},
		{"com.zhiliaoapp.musically", []string{"tiktok.com"}},
		{"com.ss.android.ugc.trill", []string{"tiktok.com"}},
		{"com.facebook.katana", []string{"facebook.com"}},
		{"com.facebook.orca", []string{"facebook.com"}},
		{"com.snapchat.android", []string{"snapchat.com"}},
		{"com.linkedin.android", []string{"linkedin.com"}},
		{"com.pinterest", []string{"pinterest.com"}},
		{"com.unknown.app", nil},
		{"", nil},
	}

	for _, tc := range tests {
		actual := goals.GetDomainsForApp(tc.app)
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("GetDomainsForApp(%q) = %v, expected %v", tc.app, actual, tc.expected)
		}
	}
}

func TestCatalog_GetAppsForDomain(t *testing.T) {
	tests := []struct {
		domain   string
		expected []string
	}{
		{"instagram.com", []string{"com.instagram.android"}},
		{"www.instagram.com", []string{"com.instagram.android"}},
		{"youtube.com", []string{"com.google.android.youtube"}},
		{"youtu.be", []string{"com.google.android.youtube"}},
		{"shorts.youtube.com", []string{"com.google.android.youtube.shorts"}},
		{"x.com", []string{"com.twitter.android"}},
		{"twitter.com", []string{"com.twitter.android"}},
		{"reddit.com", []string{"com.reddit.frontpage"}},
		{"netflix.com", []string{"com.netflix.mediaclient"}},
		{"discord.com", []string{"com.discord"}},
		{"twitch.tv", []string{"tv.twitch.android.app"}},
		{"tiktok.com", []string{"com.zhiliaoapp.musically", "com.ss.android.ugc.trill"}},
		{"facebook.com", []string{"com.facebook.katana", "com.facebook.orca"}},
		{"snapchat.com", []string{"com.snapchat.android"}},
		{"linkedin.com", []string{"com.linkedin.android"}},
		{"pinterest.com", []string{"com.pinterest"}},
		{"customdomain.com", nil},
		{"unmapped-service.net", nil},
		{"", nil},
	}

	for _, tc := range tests {
		actual := goals.GetAppsForDomain(tc.domain)
		if !reflect.DeepEqual(actual, tc.expected) {
			t.Errorf("GetAppsForDomain(%q) = %v, expected %v", tc.domain, actual, tc.expected)
		}
	}
}

func TestCatalog_AppToDomainExpansion(t *testing.T) {
	tests := []struct {
		name            string
		inputApps       []string
		expectedApps    []string
		expectedDomains []string
	}{
		{
			name:            "Instagram app expands to domain",
			inputApps:       []string{"com.instagram.android"},
			expectedApps:    []string{"com.instagram.android"},
			expectedDomains: []string{"instagram.com"},
		},
		{
			name:            "YouTube app expands to both youtube.com and youtu.be",
			inputApps:       []string{"com.google.android.youtube"},
			expectedApps:    []string{"com.google.android.youtube"},
			expectedDomains: []string{"youtu.be", "youtube.com"},
		},
		{
			name:            "Reddit app expands to reddit.com",
			inputApps:       []string{"com.reddit.frontpage"},
			expectedApps:    []string{"com.reddit.frontpage"},
			expectedDomains: []string{"reddit.com"},
		},
		{
			name:            "Netflix app expands to netflix.com",
			inputApps:       []string{"com.netflix.mediaclient"},
			expectedApps:    []string{"com.netflix.mediaclient"},
			expectedDomains: []string{"netflix.com"},
		},
		{
			name:            "TikTok app expands to domain and alternate app package",
			inputApps:       []string{"com.zhiliaoapp.musically"},
			expectedApps:    []string{"com.ss.android.ugc.trill", "com.zhiliaoapp.musically"},
			expectedDomains: []string{"tiktok.com"},
		},
		{
			name:            "Multiple apps expand cleanly",
			inputApps:       []string{"com.instagram.android", "com.reddit.frontpage"},
			expectedApps:    []string{"com.instagram.android", "com.reddit.frontpage"},
			expectedDomains: []string{"instagram.com", "reddit.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apps, domains, err := goals.ExpandAssociatedTargets(tc.inputApps, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(apps, tc.expectedApps) {
				t.Errorf("apps: got %v, expected %v", apps, tc.expectedApps)
			}
			if !reflect.DeepEqual(domains, tc.expectedDomains) {
				t.Errorf("domains: got %v, expected %v", domains, tc.expectedDomains)
			}
		})
	}
}

func TestCatalog_DomainToAppExpansion(t *testing.T) {
	tests := []struct {
		name            string
		inputDomains    []string
		expectedApps    []string
		expectedDomains []string
	}{
		{
			name:            "Instagram domain expands to app",
			inputDomains:    []string{"instagram.com"},
			expectedApps:    []string{"com.instagram.android"},
			expectedDomains: []string{"instagram.com"},
		},
		{
			name:            "YouTube domain expands to app",
			inputDomains:    []string{"youtube.com"},
			expectedApps:    []string{"com.google.android.youtube"},
			expectedDomains: []string{"youtube.com"},
		},
		{
			name:            "youtu.be expands to YouTube app",
			inputDomains:    []string{"youtu.be"},
			expectedApps:    []string{"com.google.android.youtube"},
			expectedDomains: []string{"youtu.be"},
		},
		{
			name:            "Discord domain expands to app",
			inputDomains:    []string{"discord.com"},
			expectedApps:    []string{"com.discord"},
			expectedDomains: []string{"discord.com"},
		},
		{
			name:            "TikTok domain expands to both app variants",
			inputDomains:    []string{"tiktok.com"},
			expectedApps:    []string{"com.ss.android.ugc.trill", "com.zhiliaoapp.musically"},
			expectedDomains: []string{"tiktok.com"},
		},
		{
			name:            "Facebook domain expands to both katana and orca",
			inputDomains:    []string{"facebook.com"},
			expectedApps:    []string{"com.facebook.katana", "com.facebook.orca"},
			expectedDomains: []string{"facebook.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			apps, domains, err := goals.ExpandAssociatedTargets(nil, tc.inputDomains)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(apps, tc.expectedApps) {
				t.Errorf("apps: got %v, expected %v", apps, tc.expectedApps)
			}
			if !reflect.DeepEqual(domains, tc.expectedDomains) {
				t.Errorf("domains: got %v, expected %v", domains, tc.expectedDomains)
			}
		})
	}
}

func TestCatalog_CustomUnmappedDomains(t *testing.T) {
	// Custom unmapped domains must remain domain-only with NO app generated
	customDomains := []string{"customdomain.com", "news.ycombinator.com", "sub.internal-app.org"}
	apps, domains, err := goals.ExpandAssociatedTargets(nil, customDomains)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(apps) != 0 {
		t.Errorf("expected 0 apps for unmapped domains, got %v", apps)
	}
	expectedDomains := []string{"customdomain.com", "news.ycombinator.com", "sub.internal-app.org"}
	if !reflect.DeepEqual(domains, expectedDomains) {
		t.Errorf("expected domains %v, got %v", expectedDomains, domains)
	}

	// Mixed custom domain and catalog app
	apps, domains, err = goals.ExpandAssociatedTargets([]string{"com.instagram.android"}, []string{"customdomain.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expectedApps := []string{"com.instagram.android"}
	expectedDomains = []string{"customdomain.com", "instagram.com"}
	if !reflect.DeepEqual(apps, expectedApps) {
		t.Errorf("expected apps %v, got %v", expectedApps, apps)
	}
	if !reflect.DeepEqual(domains, expectedDomains) {
		t.Errorf("expected domains %v, got %v", expectedDomains, domains)
	}
}

func TestCatalog_YouTubeShortsPathIsolation(t *testing.T) {
	// CRITICAL REQUIREMENT: YouTube Shorts path-only rule!
	// Must NOT associate with com.google.android.youtube or youtube.com / youtu.be

	// 1. App-to-domain expansion: YouTube Shorts app only expands to shorts.youtube.com
	shortsApp := []string{"com.google.android.youtube.shorts"}
	apps, domains, err := goals.ExpandAssociatedTargets(shortsApp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) != 1 || apps[0] != "com.google.android.youtube.shorts" {
		t.Errorf("expected only shorts app, got %v", apps)
	}
	if len(domains) != 1 || domains[0] != "shorts.youtube.com" {
		t.Errorf("expected only shorts.youtube.com, got %v", domains)
	}

	// Verify absence of full YouTube
	for _, a := range apps {
		if a == "com.google.android.youtube" {
			t.Fatalf("YouTube Shorts app leaked full YouTube app!")
		}
	}
	for _, d := range domains {
		if d == "youtube.com" || d == "youtu.be" {
			t.Fatalf("YouTube Shorts app leaked full YouTube domain %q!", d)
		}
	}

	// 2. Domain-to-app expansion: shorts.youtube.com only expands to shorts app
	shortsDomain := []string{"shorts.youtube.com"}
	apps, domains, err = goals.ExpandAssociatedTargets(nil, shortsDomain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(apps) != 1 || apps[0] != "com.google.android.youtube.shorts" {
		t.Errorf("expected only shorts app, got %v", apps)
	}
	if len(domains) != 1 || domains[0] != "shorts.youtube.com" {
		t.Errorf("expected only shorts.youtube.com, got %v", domains)
	}

	// 3. Full YouTube app does NOT expand to shorts
	ytApp := []string{"com.google.android.youtube"}
	apps, domains, err = goals.ExpandAssociatedTargets(ytApp, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range apps {
		if a == "com.google.android.youtube.shorts" {
			t.Fatalf("Full YouTube app leaked shorts app!")
		}
	}
	for _, d := range domains {
		if d == "shorts.youtube.com" {
			t.Fatalf("Full YouTube app leaked shorts domain!")
		}
	}

	// 4. Full YouTube domain does NOT expand to shorts
	ytDomain := []string{"youtube.com"}
	apps, domains, err = goals.ExpandAssociatedTargets(nil, ytDomain)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, a := range apps {
		if a == "com.google.android.youtube.shorts" {
			t.Fatalf("Full YouTube domain leaked shorts app!")
		}
	}
	for _, d := range domains {
		if d == "shorts.youtube.com" {
			t.Fatalf("Full YouTube domain leaked shorts domain!")
		}
	}
}

func TestCatalog_TwitterCompatibilityRule(t *testing.T) {
	// Rule: If com.twitter.android is present and either x.com or twitter.com is ALREADY in rawDomains,
	// do not force both unless neither is present.

	// Case 1: com.twitter.android with NO domains -> expands to BOTH x.com and twitter.com
	{
		apps, domains, err := goals.ExpandAssociatedTargets([]string{"com.twitter.android"}, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDomains := []string{"twitter.com", "x.com"}
		if !reflect.DeepEqual(domains, expectedDomains) {
			t.Errorf("expected both domains %v, got %v", expectedDomains, domains)
		}
		if len(apps) != 1 || apps[0] != "com.twitter.android" {
			t.Errorf("expected apps [com.twitter.android], got %v", apps)
		}
	}

	// Case 2: com.twitter.android with x.com ALREADY present -> retains ONLY x.com, does NOT force twitter.com
	{
		apps, domains, err := goals.ExpandAssociatedTargets(
			[]string{"com.twitter.android", "com.instagram.android"},
			[]string{"x.com", "instagram.com"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDomains := []string{"instagram.com", "x.com"}
		if !reflect.DeepEqual(domains, expectedDomains) {
			t.Errorf("expected domains %v, got %v", expectedDomains, domains)
		}
		expectedApps := []string{"com.instagram.android", "com.twitter.android"}
		if !reflect.DeepEqual(apps, expectedApps) {
			t.Errorf("expected apps %v, got %v", expectedApps, apps)
		}
	}

	// Case 3: com.twitter.android with twitter.com ALREADY present -> retains ONLY twitter.com, does NOT force x.com
	{
		apps, domains, err := goals.ExpandAssociatedTargets(
			[]string{"com.twitter.android"},
			[]string{"twitter.com"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDomains := []string{"twitter.com"}
		if !reflect.DeepEqual(domains, expectedDomains) {
			t.Errorf("expected domains %v, got %v", expectedDomains, domains)
		}
		if len(apps) != 1 || apps[0] != "com.twitter.android" {
			t.Errorf("expected apps [com.twitter.android], got %v", apps)
		}
	}

	// Case 4: x.com domain-only input -> expands to com.twitter.android and retains ONLY x.com
	{
		apps, domains, err := goals.ExpandAssociatedTargets(nil, []string{"x.com"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(apps) != 1 || apps[0] != "com.twitter.android" {
			t.Errorf("expected app com.twitter.android, got %v", apps)
		}
		if len(domains) != 1 || domains[0] != "x.com" {
			t.Errorf("expected domains [x.com], got %v", domains)
		}
	}

	// Case 5: twitter.com domain-only input -> expands to com.twitter.android and retains ONLY twitter.com
	{
		apps, domains, err := goals.ExpandAssociatedTargets(nil, []string{"twitter.com"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(apps) != 1 || apps[0] != "com.twitter.android" {
			t.Errorf("expected app com.twitter.android, got %v", apps)
		}
		if len(domains) != 1 || domains[0] != "twitter.com" {
			t.Errorf("expected domains [twitter.com], got %v", domains)
		}
	}

	// Case 6: Both x.com and twitter.com provided -> both preserved
	{
		apps, domains, err := goals.ExpandAssociatedTargets([]string{"com.twitter.android"}, []string{"twitter.com", "x.com"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expectedDomains := []string{"twitter.com", "x.com"}
		if !reflect.DeepEqual(domains, expectedDomains) {
			t.Errorf("expected both domains %v, got %v", expectedDomains, domains)
		}
		if len(apps) != 1 || apps[0] != "com.twitter.android" {
			t.Errorf("expected apps [com.twitter.android], got %v", apps)
		}
	}
}

func TestCatalog_Idempotency(t *testing.T) {
	testCases := []struct {
		name    string
		apps    []string
		domains []string
	}{
		{
			name:    "Empty inputs",
			apps:    []string{},
			domains: []string{},
		},
		{
			name:    "Single app without domains",
			apps:    []string{"com.instagram.android"},
			domains: nil,
		},
		{
			name:    "Single domain without apps",
			apps:    nil,
			domains: []string{"reddit.com"},
		},
		{
			name:    "Twitter with x.com",
			apps:    []string{"com.twitter.android"},
			domains: []string{"x.com"},
		},
		{
			name:    "Twitter with twitter.com",
			apps:    []string{"com.twitter.android"},
			domains: []string{"twitter.com"},
		},
		{
			name:    "Twitter app-only (expands to both)",
			apps:    []string{"com.twitter.android"},
			domains: nil,
		},
		{
			name:    "YouTube Shorts",
			apps:    []string{"com.google.android.youtube.shorts"},
			domains: nil,
		},
		{
			name:    "Full YouTube",
			apps:    []string{"com.google.android.youtube"},
			domains: nil,
		},
		{
			name:    "Facebook multi-app",
			apps:    []string{"com.facebook.katana"},
			domains: nil,
		},
		{
			name:    "TikTok multi-app",
			apps:    []string{"com.zhiliaoapp.musically"},
			domains: nil,
		},
		{
			name:    "Custom unmapped domain",
			apps:    nil,
			domains: []string{"customdomain.com"},
		},
		{
			name:    "Multiple mixed services",
			apps:    []string{"com.instagram.android", "com.reddit.frontpage", "tv.twitch.android.app"},
			domains: []string{"netflix.com", "discord.com"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// First expansion
			apps1, domains1, err := goals.ExpandAssociatedTargets(tc.apps, tc.domains)
			if err != nil {
				t.Fatalf("first expansion failed: %v", err)
			}

			// Second expansion with the results of the first
			apps2, domains2, err := goals.ExpandAssociatedTargets(apps1, domains1)
			if err != nil {
				t.Fatalf("second expansion failed: %v", err)
			}

			if !reflect.DeepEqual(apps1, apps2) {
				t.Errorf("IDEMPOTENCY VIOLATION on apps:\nfirst:  %v\nsecond: %v", apps1, apps2)
			}
			if !reflect.DeepEqual(domains1, domains2) {
				t.Errorf("IDEMPOTENCY VIOLATION on domains:\nfirst:  %v\nsecond: %v", domains1, domains2)
			}
		})
	}
}
