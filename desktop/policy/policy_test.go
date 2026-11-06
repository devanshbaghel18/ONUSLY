package policy_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"github.com/devanshbaghel18/ONUSLY/desktop/policy"
)

func TestPolicyManager_MultiBrowserApplicationAndDrift(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-policy-test-*")
	defer os.RemoveAll(tmpDir)

	fakeNativeBin := filepath.Join(tmpDir, "onusly-native-host")
	_ = os.WriteFile(fakeNativeBin, []byte("#!/bin/sh\nexit 0\n"), 0755)

	mgr := policy.NewLinuxPolicyManager(fakeNativeBin, filepath.Join(tmpDir, "backup"), true)

	// Create simulated mock browser targets
	chromePolicyDir := filepath.Join(tmpDir, "chrome-policies")
	chromeNativeDir := filepath.Join(tmpDir, "chrome-native")

	firefoxPolicyDir := filepath.Join(tmpDir, "firefox-policies")
	firefoxNativeDir := filepath.Join(tmpDir, "firefox-native")

	mockBrowsers := []core.BrowserInfo{
		{
			Type:              core.BrowserChrome,
			BinaryPath:        "/bin/true",
			PolicyDir:         chromePolicyDir,
			NativeHostDir:     chromeNativeDir,
			UserNativeHostDir: chromeNativeDir,
			Installed:         true,
		},
		{
			Type:              core.BrowserFirefox,
			BinaryPath:        "/bin/true",
			PolicyDir:         firefoxPolicyDir,
			NativeHostDir:     firefoxNativeDir,
			UserNativeHostDir: firefoxNativeDir,
			Installed:         true,
		},
	}

	// 1. Apply Policies
	if err := mgr.ApplyPolicies(mockBrowsers); err != nil {
		t.Fatalf("failed to apply policies: %v", err)
	}

	// 2. Verify Chromium Policy
	chromePolicyFile := filepath.Join(chromePolicyDir, "onusly.json")
	data, err := os.ReadFile(chromePolicyFile)
	if err != nil {
		t.Fatalf("failed to read Chrome policy: %v", err)
	}
	var chromePolicy map[string]any
	if err := json.Unmarshal(data, &chromePolicy); err != nil {
		t.Fatalf("invalid Chrome policy JSON: %v", err)
	}

	forceList, ok := chromePolicy["ExtensionInstallForcelist"].([]any)
	if !ok || len(forceList) == 0 {
		t.Fatalf("ExtensionInstallForcelist missing or empty in Chrome policy")
	}
	if !strings.HasPrefix(forceList[0].(string), policy.ChromiumExtensionID) {
		t.Errorf("expected force install entry with extension ID %s, got %v", policy.ChromiumExtensionID, forceList[0])
	}
	if chromePolicy["IncognitoModeAvailability"] != float64(1) {
		t.Errorf("expected IncognitoModeAvailability = 1, got %v", chromePolicy["IncognitoModeAvailability"])
	}

	chromeExtSettings, ok := chromePolicy["ExtensionSettings"].(map[string]any)
	if !ok {
		t.Fatalf("ExtensionSettings missing in Chrome policy")
	}
	ourExt, ok := chromeExtSettings[policy.ChromiumExtensionID].(map[string]any)
	if !ok || ourExt["installation_mode"] != "force_installed" {
		t.Errorf("expected ExtensionSettings force_installed for %s, got %+v", policy.ChromiumExtensionID, ourExt)
	}

	// 3. Verify Chromium Native Messaging Manifest (allowed_origins)
	chromeManifestFile := filepath.Join(chromeNativeDir, policy.NativeHostName+".json")
	data, err = os.ReadFile(chromeManifestFile)
	if err != nil {
		t.Fatalf("failed to read Chrome native manifest: %v", err)
	}
	var chromeManifest map[string]any
	_ = json.Unmarshal(data, &chromeManifest)
	origins, ok := chromeManifest["allowed_origins"].([]any)
	if !ok || len(origins) == 0 {
		t.Fatalf("allowed_origins missing in Chromium native manifest")
	}
	expectedOrigin := "chrome-extension://" + policy.ChromiumExtensionID + "/"
	if origins[0].(string) != expectedOrigin {
		t.Errorf("expected origin %s, got %s", expectedOrigin, origins[0])
	}

	// 4. Verify Firefox Policy (ExtensionSettings with private_browsing: true)
	firefoxPolicyFile := filepath.Join(firefoxPolicyDir, "policies.json")
	data, err = os.ReadFile(firefoxPolicyFile)
	if err != nil {
		t.Fatalf("failed to read Firefox policy: %v", err)
	}
	var firefoxWrap struct {
		Policies map[string]any `json:"policies"`
	}
	_ = json.Unmarshal(data, &firefoxWrap)
	extSettings, ok := firefoxWrap.Policies["ExtensionSettings"].(map[string]any)
	if !ok {
		t.Fatalf("ExtensionSettings missing in Firefox policies.json")
	}
	onuslyExt, ok := extSettings[policy.FirefoxExtensionID].(map[string]any)
	if !ok {
		t.Fatalf("FirefoxExtensionID %s missing in ExtensionSettings", policy.FirefoxExtensionID)
	}
	if firefoxWrap.Policies["DisablePrivateBrowsing"] != true {
		t.Errorf("expected DisablePrivateBrowsing: true for Firefox, got %v", firefoxWrap.Policies["DisablePrivateBrowsing"])
	}
	if onuslyExt["installation_mode"] != "force_installed" {
		t.Errorf("expected installation_mode: force_installed for Firefox, got %v", onuslyExt["installation_mode"])
	}
	if onuslyExt["private_browsing"] != true {
		t.Errorf("expected private_browsing: true for Firefox, got %v", onuslyExt["private_browsing"])
	}

	// 5. Verify Firefox Native Messaging Manifest (allowed_extensions)
	firefoxManifestFile := filepath.Join(firefoxNativeDir, policy.NativeHostName+".json")
	data, err = os.ReadFile(firefoxManifestFile)
	if err != nil {
		t.Fatalf("failed to read Firefox native manifest: %v", err)
	}
	var ffManifest map[string]any
	_ = json.Unmarshal(data, &ffManifest)
	exts, ok := ffManifest["allowed_extensions"].([]any)
	if !ok || len(exts) == 0 {
		t.Fatalf("allowed_extensions missing in Firefox native manifest")
	}
	if exts[0].(string) != policy.FirefoxExtensionID {
		t.Errorf("expected allowed extension %s, got %s", policy.FirefoxExtensionID, exts[0])
	}

	// 6. Verify Detection & Verification
	issues, err := mgr.VerifyPolicies(mockBrowsers)
	if err != nil || len(issues) != 0 {
		t.Fatalf("expected 0 issues on healthy configuration, got %d (err: %v)", len(issues), err)
	}

	// 7. Drift Detection: Delete one policy file
	_ = os.Remove(chromePolicyFile)
	driftIssues, _ := mgr.VerifyPolicies(mockBrowsers)
	if len(driftIssues) != 1 || driftIssues[0].Issue != "policy_missing" {
		t.Fatalf("expected drift detection for missing policy, got %+v", driftIssues)
	}

	// 8. Remove Policies / Uninstall
	if err := mgr.RemovePolicies(mockBrowsers); err != nil {
		t.Fatalf("failed to remove policies: %v", err)
	}
	if _, err := os.Stat(firefoxPolicyFile); !os.IsNotExist(err) {
		t.Errorf("expected Firefox policies.json to be removed")
	}
}

func TestPolicyManager_PreExistingPolicyPreservationAndRollback(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-policy-preservation-*")
	defer os.RemoveAll(tmpDir)

	fakeNativeBin := filepath.Join(tmpDir, "onusly-native-host")
	_ = os.WriteFile(fakeNativeBin, []byte("#!/bin/sh\nexit 0\n"), 0755)

	mgr := policy.NewLinuxPolicyManager(fakeNativeBin, filepath.Join(tmpDir, "backup"), true)

	chromePolicyDir := filepath.Join(tmpDir, "chrome-policies")
	chromeNativeDir := filepath.Join(tmpDir, "chrome-native")
	_ = os.MkdirAll(chromePolicyDir, 0755)
	_ = os.MkdirAll(chromeNativeDir, 0755)

	// Seed pre-existing corporate Chrome policy
	originalChromePolicy := `{
  "HomepageLocation": "https://company.internal",
  "ExtensionInstallForcelist": [
    "corp-extension-id;https://clients2.google.com/service/update2/crx"
  ]
}`
	_ = os.WriteFile(filepath.Join(chromePolicyDir, "onusly.json"), []byte(originalChromePolicy), 0644)

	mockBrowsers := []core.BrowserInfo{
		{
			Type:              core.BrowserChrome,
			BinaryPath:        "/bin/true",
			PolicyDir:         chromePolicyDir,
			NativeHostDir:     chromeNativeDir,
			UserNativeHostDir: chromeNativeDir,
			Installed:         true,
		},
	}

	// 1. Apply ONUSLY policies
	if err := mgr.ApplyPolicies(mockBrowsers); err != nil {
		t.Fatalf("failed to apply policies: %v", err)
	}

	// Verify both corporate policy and ONUSLY policy coexist
	data, _ := os.ReadFile(filepath.Join(chromePolicyDir, "onusly.json"))
	var applied map[string]any
	_ = json.Unmarshal(data, &applied)

	if applied["HomepageLocation"] != "https://company.internal" {
		t.Errorf("expected HomepageLocation preserved, got %v", applied["HomepageLocation"])
	}
	list := applied["ExtensionInstallForcelist"].([]any)
	if len(list) != 2 {
		t.Fatalf("expected 2 extensions in force list (corp + onusly), got %d", len(list))
	}

	// 2. Remove policies (Uninstall)
	if err := mgr.RemovePolicies(mockBrowsers); err != nil {
		t.Fatalf("failed to remove policies: %v", err)
	}

	// Verify original pre-existing policy was restored from backup
	restoredData, err := os.ReadFile(filepath.Join(chromePolicyDir, "onusly.json"))
	if err != nil {
		t.Fatalf("expected pre-existing policy restored, file missing: %v", err)
	}
	var restored map[string]any
	_ = json.Unmarshal(restoredData, &restored)

	if restored["HomepageLocation"] != "https://company.internal" {
		t.Errorf("expected restored HomepageLocation, got %v", restored["HomepageLocation"])
	}
	restoredList := restored["ExtensionInstallForcelist"].([]any)
	if len(restoredList) != 1 || restoredList[0].(string) != "corp-extension-id;https://clients2.google.com/service/update2/crx" {
		t.Errorf("expected original 1 corporate extension restored, got %v", restoredList)
	}

	// Verify native messaging manifest was removed
	manifestPath := filepath.Join(chromeNativeDir, policy.NativeHostName+".json")
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Errorf("expected native messaging manifest removed after rollback")
	}
}
