package enforcement_test

import (
	"context"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/internal/enforcement"
	"github.com/devanshbaghel18/ONUSLY/internal/goals"
)

// TestE2ESmoke_CrossPlatformSynchronization exercises the end-to-end synchronization flow:
// 1. Android Instagram app selection -> backend persistence -> extension delivery.
// 2. Dashboard Instagram domain selection -> backend persistence -> Android delivery.
// 3. Goal unblocking upon completion.
// 4. Multi-user session isolation and cache separation.
func TestE2ESmoke_CrossPlatformSynchronization(t *testing.T) {
	ctx := context.Background()

	// --- 1. Flow 1: Android Instagram app selection ---
	// Android sends blockedApps: ["com.instagram.android"], blockedDomains: [] (or paired ["instagram.com"])
	rawApps := []string{"com.instagram.android"}
	rawDomains := []string{}

	expandedApps, expandedDomains, err := goals.ExpandAssociatedTargets(rawApps, rawDomains)
	if err != nil {
		t.Fatalf("Failed to expand Android target: %v", err)
	}

	// Verify backend persistence data
	if len(expandedApps) != 1 || expandedApps[0] != "com.instagram.android" {
		t.Errorf("Backend did not retain com.instagram.android: %v", expandedApps)
	}
	if len(expandedDomains) != 1 || expandedDomains[0] != "instagram.com" {
		t.Errorf("Backend did not associate instagram.com with Instagram app: %v", expandedDomains)
	}

	// Simulate Goal persisted in database
	userA := "user_alice_phone"
	activeGoalA := goals.Goal{
		ID:             "goal_alice_ig",
		OwnerID:        userA,
		Title:          "Limit Instagram",
		Status:         "active",
		CreatedAt:      "2026-10-26T12:00:00Z",
		BlockedApps:    expandedApps,
		BlockedDomains: expandedDomains,
	}

	mockLister := &mockGoalLister{
		goalsByUser: map[string][]goals.Goal{
			userA: {activeGoalA},
		},
	}

	enforcementService := enforcement.NewService(mockLister)

	// Extension fetches blocklist
	blocklistA, err := enforcementService.GetBlocklist(ctx, userA)
	if err != nil {
		t.Fatalf("Extension failed to fetch blocklist: %v", err)
	}
	if len(blocklistA.Targets) != 1 {
		t.Fatalf("Expected 1 target for Alice, got %d", len(blocklistA.Targets))
	}
	if len(blocklistA.Targets[0].Domains) != 1 || blocklistA.Targets[0].Domains[0] != "instagram.com" {
		t.Errorf("Extension did not receive instagram.com domain: %v", blocklistA.Targets[0].Domains)
	}

	// --- 2. Flow 2: Dashboard Instagram domain selection ---
	// Web dashboard sends blockedDomains: ["instagram.com"], blockedApps: []
	userB := "user_bob_web"
	webRawApps := []string{}
	webRawDomains := []string{"instagram.com"}

	webExpandedApps, webExpandedDomains, err := goals.ExpandAssociatedTargets(webRawApps, webRawDomains)
	if err != nil {
		t.Fatalf("Failed to expand Web target: %v", err)
	}

	// Verify backend persistence data
	if len(webExpandedApps) != 1 || webExpandedApps[0] != "com.instagram.android" {
		t.Errorf("Backend did not associate com.instagram.android with instagram.com: %v", webExpandedApps)
	}
	if len(webExpandedDomains) != 1 || webExpandedDomains[0] != "instagram.com" {
		t.Errorf("Backend did not retain instagram.com: %v", webExpandedDomains)
	}

	activeGoalB := goals.Goal{
		ID:             "goal_bob_ig",
		OwnerID:        userB,
		Title:          "Stop Instagram Web",
		Status:         "active",
		CreatedAt:      "2026-10-26T12:05:00Z",
		BlockedApps:    webExpandedApps,
		BlockedDomains: webExpandedDomains,
	}

	mockLister.goalsByUser[userB] = []goals.Goal{activeGoalB}

	// Android phone syncs for Bob
	blocklistB, err := enforcementService.GetBlocklist(ctx, userB)
	if err != nil {
		t.Fatalf("Android failed to fetch blocklist for Bob: %v", err)
	}
	if len(blocklistB.Targets) != 1 {
		t.Fatalf("Expected 1 target for Bob, got %d", len(blocklistB.Targets))
	}
	if len(blocklistB.Targets[0].Apps) != 1 || blocklistB.Targets[0].Apps[0] != "com.instagram.android" {
		t.Errorf("Android did not receive com.instagram.android app: %v", blocklistB.Targets[0].Apps)
	}

	// --- 3. Flow 3: Unblocking when goal is completed ---
	completedGoalA := activeGoalA
	completedGoalA.Status = "completed"
	mockLister.goalsByUser[userA] = []goals.Goal{completedGoalA}

	unblockedBlocklistA, err := enforcementService.GetBlocklist(ctx, userA)
	if err != nil {
		t.Fatalf("Failed to fetch blocklist after completion: %v", err)
	}
	if len(unblockedBlocklistA.Targets) != 0 {
		t.Errorf("Completed goal must NOT be included in active blocklist targets, got: %v", unblockedBlocklistA.Targets)
	}

	// --- 4. Flow 4: Multi-user session isolation & account switching ---
	// An unrelated user (User C) with no active goals must receive empty blocklist
	userC := "user_charlie"
	blocklistC, err := enforcementService.GetBlocklist(ctx, userC)
	if err != nil {
		t.Fatalf("Failed to fetch blocklist for Charlie: %v", err)
	}
	if len(blocklistC.Targets) != 0 {
		t.Errorf("User C should receive empty blocklist, got: %v", blocklistC.Targets)
	}
}
