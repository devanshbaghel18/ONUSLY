package main

import (
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"github.com/devanshbaghel18/ONUSLY/desktop/policy"
)

func TestPolicyCommands_MissingAction(t *testing.T) {
	err := runPolicyCommand([]string{}, t.TempDir())
	if err == nil {
		t.Fatalf("expected error on empty policy action, got nil")
	}
}

func TestPolicyCommands_UnknownAction(t *testing.T) {
	err := runPolicyCommand([]string{"unknown_action"}, t.TempDir())
	if err == nil {
		t.Fatalf("expected error on unknown policy action, got nil")
	}
}

func TestPolicyCommands_MockLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	mockReg := policy.NewMockRegistry()
	mgr := policy.NewWindowsPolicyManager(
		`C:\Program Files\Onusly\onusly-native-host.exe`,
		tmpDir+`\manifests`,
		tmpDir+`\backup_state.json`,
		"https://api.onusly.com/downloads/updates.xml",
		mockReg,
	)

	// 1. Status with mock manager
	if err := runPolicyStatus(mgr); err != nil {
		t.Fatalf("runPolicyStatus failed: %v", err)
	}

	// 2. Apply with mock manager
	if err := runPolicyApply(mgr); err != nil {
		t.Fatalf("runPolicyApply failed: %v", err)
	}

	// 3. Verify zero drift
	browsers, _ := mgr.DetectBrowsers()
	issues, err := mgr.VerifyPolicies(browsers)
	if err != nil {
		t.Fatalf("VerifyPolicies failed: %v", err)
	}
	var activeIssues []core.PolicyIssue
	for _, iss := range issues {
		activeIssues = append(activeIssues, iss)
	}

	// 4. Remove with mock manager
	if err := runPolicyRemove(mgr); err != nil {
		t.Fatalf("runPolicyRemove failed: %v", err)
	}
}
