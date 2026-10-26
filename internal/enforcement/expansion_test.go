package enforcement_test

import (
	"context"
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

func TestEnforcement_LegacyGoalExpansion(t *testing.T) {
	// Historical/legacy goals in DynamoDB may have been saved before bidirectional catalog expansion:
	// 1. Goal with only apps, no domains.
	// 2. Goal with only domains, no apps.
	// 3. Goal with custom unmapped domain.
	// 4. Goal with YouTube Shorts (path isolation check).
	// 5. Goal with Twitter app-only (should expand to both twitter.com and x.com).

	legacyGoals := []goals.Goal{
		{
			ID:             "g-app-only",
			Title:          "Legacy App-Only Goal",
			Status:         "active",
			CreatedAt:      "2026-09-01T10:00:00Z",
			BlockedApps:    []string{"com.instagram.android"},
			BlockedDomains: []string{}, // legacy goal without domains
		},
		{
			ID:             "g-domain-only",
			Title:          "Legacy Domain-Only Goal",
			Status:         "active",
			CreatedAt:      "2026-09-01T11:00:00Z",
			BlockedApps:    nil, // nil apps in legacy data
			BlockedDomains: []string{"reddit.com"},
		},
		{
			ID:             "g-custom-domain",
			Title:          "Legacy Custom Domain Goal",
			Status:         "proof_submitted",
			CreatedAt:      "2026-09-01T12:00:00Z",
			BlockedApps:    []string{},
			BlockedDomains: []string{"customdomain.com"},
		},
		{
			ID:             "g-shorts",
			Title:          "Legacy YouTube Shorts Goal",
			Status:         "active",
			CreatedAt:      "2026-09-01T13:00:00Z",
			BlockedApps:    []string{"com.google.android.youtube.shorts"},
			BlockedDomains: []string{},
		},
		{
			ID:             "g-twitter",
			Title:          "Legacy Twitter Goal",
			Status:         "active",
			CreatedAt:      "2026-09-01T14:00:00Z",
			BlockedApps:    []string{"com.twitter.android"},
			BlockedDomains: nil,
		},
	}

	mockLister := &mockGoalLister{
		goalsByUser: map[string][]goals.Goal{
			"user-legacy": legacyGoals,
		},
	}

	service := enforcement.NewService(mockLister)
	res, err := service.GetBlocklist(context.Background(), "user-legacy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Targets) != 5 {
		t.Fatalf("expected 5 targets, got %d", len(res.Targets))
	}

	// Target 1: g-app-only (Instagram app-only legacy goal)
	t1 := res.Targets[0]
	if t1.GoalID != "g-app-only" {
		t.Errorf("expected target 1 to be g-app-only, got %q", t1.GoalID)
	}
	if len(t1.Apps) != 1 || t1.Apps[0] != "com.instagram.android" {
		t.Errorf("expected target 1 apps [com.instagram.android], got %v", t1.Apps)
	}
	if len(t1.Domains) != 1 || t1.Domains[0] != "instagram.com" {
		t.Errorf("expected target 1 domains [instagram.com] expanded from app, got %v", t1.Domains)
	}

	// Target 2: g-domain-only (Reddit domain-only legacy goal)
	t2 := res.Targets[1]
	if t2.GoalID != "g-domain-only" {
		t.Errorf("expected target 2 to be g-domain-only, got %q", t2.GoalID)
	}
	if len(t2.Apps) != 1 || t2.Apps[0] != "com.reddit.frontpage" {
		t.Errorf("expected target 2 apps [com.reddit.frontpage] expanded from domain, got %v", t2.Apps)
	}
	if len(t2.Domains) != 1 || t2.Domains[0] != "reddit.com" {
		t.Errorf("expected target 2 domains [reddit.com], got %v", t2.Domains)
	}

	// Target 3: g-custom-domain (Custom unmapped domain)
	t3 := res.Targets[2]
	if t3.GoalID != "g-custom-domain" {
		t.Errorf("expected target 3 to be g-custom-domain, got %q", t3.GoalID)
	}
	if len(t3.Apps) != 0 {
		t.Errorf("expected target 3 apps to be empty for custom domain, got %v", t3.Apps)
	}
	if len(t3.Domains) != 1 || t3.Domains[0] != "customdomain.com" {
		t.Errorf("expected target 3 domains [customdomain.com], got %v", t3.Domains)
	}

	// Target 4: g-shorts (YouTube Shorts legacy goal)
	t4 := res.Targets[3]
	if t4.GoalID != "g-shorts" {
		t.Errorf("expected target 4 to be g-shorts, got %q", t4.GoalID)
	}
	if len(t4.Apps) != 1 || t4.Apps[0] != "com.google.android.youtube.shorts" {
		t.Errorf("expected target 4 apps [com.google.android.youtube.shorts], got %v", t4.Apps)
	}
	if len(t4.Domains) != 1 || t4.Domains[0] != "shorts.youtube.com" {
		t.Errorf("expected target 4 domains [shorts.youtube.com], got %v", t4.Domains)
	}
	// Verify strict isolation from full YouTube
	for _, a := range t4.Apps {
		if a == "com.google.android.youtube" {
			t.Fatalf("target 4 leaked full YouTube app!")
		}
	}
	for _, d := range t4.Domains {
		if d == "youtube.com" || d == "youtu.be" {
			t.Fatalf("target 4 leaked full YouTube domain %q!", d)
		}
	}

	// Target 5: g-twitter (Twitter app-only legacy goal)
	t5 := res.Targets[4]
	if t5.GoalID != "g-twitter" {
		t.Errorf("expected target 5 to be g-twitter, got %q", t5.GoalID)
	}
	if len(t5.Apps) != 1 || t5.Apps[0] != "com.twitter.android" {
		t.Errorf("expected target 5 apps [com.twitter.android], got %v", t5.Apps)
	}
	// App-only with no domains expands to both twitter.com and x.com
	if len(t5.Domains) != 2 || t5.Domains[0] != "twitter.com" || t5.Domains[1] != "x.com" {
		t.Errorf("expected target 5 domains [twitter.com x.com], got %v", t5.Domains)
	}

	// Verify Version hash format
	if !strings.HasPrefix(res.Version, "v1-") || len(res.Version) != 19 {
		t.Errorf("expected version to be 'v1-' followed by 16 hex characters, got %q", res.Version)
	}

	// Verify idempotency and stability of version across multiple calls
	res2, err := service.GetBlocklist(context.Background(), "user-legacy")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if res.Version != res2.Version {
		t.Errorf("version is not stable across calls: %q vs %q", res.Version, res2.Version)
	}
}
