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

func TestWindowsPolicy_UpdateURLValidation(t *testing.T) {
	validURLs := []string{
		"https://api.onusly.com/downloads/updates.xml",
		"http://localhost:8080/downloads/updates.xml",
		"https://static.onusly.com/extension/onusly.crx",
		"https://addons.mozilla.org/firefox/downloads/file/onusly.xpi",
		"http://127.0.0.1:8080/updates.xml",
	}

	for _, u := range validURLs {
		if err := policy.ValidateUpdateURL(u); err != nil {
			t.Errorf("expected URL %q to be valid, got: %v", u, err)
		}
	}

	invalidURLs := []struct {
		url string
		err string
	}{
		{"", "cannot be empty"},
		{"not-a-url", "invalid extension update URL"},
		{"ftp://api.onusly.com/updates.xml", "must use http or https scheme"},
		{"https:///updates.xml", "must specify a valid host"},
		{"https://api.onusly.com/downloads/file.exe", "expected .xml, .crx"},
	}

	for _, tc := range invalidURLs {
		err := policy.ValidateUpdateURL(tc.url)
		if err == nil {
			t.Errorf("expected URL %q to fail validation, but it succeeded", tc.url)
		} else if !strings.Contains(err.Error(), tc.err) {
			t.Errorf("expected error containing %q, got: %v", tc.err, err)
		}
	}
}

func TestWindowsPolicy_ManifestGeneration(t *testing.T) {
	nativeBin := `C:\Program Files\Onusly\onusly-native-host.exe`

	// 1. Chromium Manifest
	chromeBytes, err := policy.GenerateChromiumManifest(nativeBin)
	if err != nil {
		t.Fatalf("GenerateChromiumManifest failed: %v", err)
	}

	var chromeMap map[string]any
	if err := json.Unmarshal(chromeBytes, &chromeMap); err != nil {
		t.Fatalf("Invalid JSON in Chromium manifest: %v", err)
	}

	if chromeMap["name"] != policy.NativeHostName {
		t.Errorf("expected name %s, got %v", policy.NativeHostName, chromeMap["name"])
	}
	if chromeMap["path"] != nativeBin {
		t.Errorf("expected path %s, got %v", nativeBin, chromeMap["path"])
	}
	if chromeMap["type"] != "stdio" {
		t.Errorf("expected type stdio, got %v", chromeMap["type"])
	}
	origins, ok := chromeMap["allowed_origins"].([]any)
	if !ok || len(origins) == 0 {
		t.Fatalf("allowed_origins missing or empty in Chromium manifest")
	}
	expectedOrigin := "chrome-extension://" + policy.ChromiumExtensionID + "/"
	foundOrigin := false
	for _, o := range origins {
		if o == expectedOrigin {
			foundOrigin = true
			break
		}
	}
	if !foundOrigin {
		t.Errorf("expected allowed origin %s, but not found in %v", expectedOrigin, origins)
	}

	// 2. Firefox Manifest
	firefoxBytes, err := policy.GenerateFirefoxManifest(nativeBin)
	if err != nil {
		t.Fatalf("GenerateFirefoxManifest failed: %v", err)
	}

	var firefoxMap map[string]any
	if err := json.Unmarshal(firefoxBytes, &firefoxMap); err != nil {
		t.Fatalf("Invalid JSON in Firefox manifest: %v", err)
	}

	if firefoxMap["name"] != policy.NativeHostName {
		t.Errorf("expected name %s, got %v", policy.NativeHostName, firefoxMap["name"])
	}
	exts, ok := firefoxMap["allowed_extensions"].([]any)
	if !ok || len(exts) == 0 {
		t.Fatalf("allowed_extensions missing or empty in Firefox manifest")
	}
	if exts[0] != policy.FirefoxExtensionID {
		t.Errorf("expected allowed extension %s, got %v", policy.FirefoxExtensionID, exts[0])
	}
}

func TestWindowsPolicy_MultiBrowserApplicationAndMerge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-win-policy-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	manifestDir := filepath.Join(tmpDir, "manifests")
	backupStatePath := filepath.Join(tmpDir, "backup_state.json")
	fakeNativeBin := filepath.Join(tmpDir, "onusly-native-host.exe")
	_ = os.WriteFile(fakeNativeBin, []byte("fake-binary"), 0755)

	mockReg := policy.NewMockRegistry()

	// Pre-seed an unrelated enterprise extension in Chrome policy
	chromePolicyKey := `SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist`
	k, _, _ := mockReg.CreateKey(chromePolicyKey)
	_ = k.SetStringValue("1", "other_enterprise_ext_id;https://example.com/update.xml")
	k.Close()

	mgr := policy.NewWindowsPolicyManager(
		fakeNativeBin,
		manifestDir,
		backupStatePath,
		"https://api.onusly.com/downloads/updates.xml",
		mockReg,
	)

	mockBrowsers := []core.BrowserInfo{
		{
			Type:          core.BrowserChrome,
			PolicyDir:     `SOFTWARE\Policies\Google\Chrome`,
			NativeHostDir: `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + policy.NativeHostName,
			Installed:     true,
		},
		{
			Type:          core.BrowserFirefox,
			PolicyDir:     `SOFTWARE\Policies\Mozilla\Firefox`,
			NativeHostDir: `SOFTWARE\Mozilla\NativeMessagingHosts\` + policy.NativeHostName,
			Installed:     true,
		},
		{
			Type:          core.BrowserBrave,
			PolicyDir:     `SOFTWARE\Policies\BraveSoftware\Brave`,
			NativeHostDir: `SOFTWARE\BraveSoftware\Brave\NativeMessagingHosts\` + policy.NativeHostName,
			Installed:     false, // Test that uninstalled browser is skipped
		},
	}

	// 1. Apply Policies
	if err := mgr.ApplyPolicies(mockBrowsers); err != nil {
		t.Fatalf("ApplyPolicies failed: %v", err)
	}

	// 2. Verify Chrome Native Host Registration
	chromeNativeKey := `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + policy.NativeHostName
	if !mockReg.KeyExists(chromeNativeKey) {
		t.Fatalf("Chrome native host key was not created in registry")
	}
	kNative, err := mockReg.OpenKey(chromeNativeKey, false)
	if err != nil {
		t.Fatalf("failed to open Chrome native host key: %v", err)
	}
	val, _ := kNative.GetStringValue("")
	kNative.Close()
	expectedManifestPath := filepath.Join(manifestDir, policy.NativeHostName+"_chrome.json")
	if val != expectedManifestPath {
		t.Errorf("expected native host manifest path %s, got %s", expectedManifestPath, val)
	}

	// 3. Verify Chrome Policy Merge: "1" must be preserved, ONUSLY must be "2"
	kChrome, err := mockReg.OpenKey(chromePolicyKey, false)
	if err != nil {
		t.Fatalf("failed to open Chrome force-install key: %v", err)
	}
	val1, _ := kChrome.GetStringValue("1")
	val2, _ := kChrome.GetStringValue("2")
	kChrome.Close()

	if val1 != "other_enterprise_ext_id;https://example.com/update.xml" {
		t.Errorf("CRITICAL: Unrelated extension at '1' was corrupted! got %s", val1)
	}
	expectedOnuslyEntry := policy.ChromiumExtensionID + ";https://api.onusly.com/downloads/updates.xml"
	if val2 != expectedOnuslyEntry {
		t.Errorf("expected ONUSLY extension at '2' (%s), got %s", expectedOnuslyEntry, val2)
	}

	// Verify Chrome DnsOverHttpsMode = "off"
	kChromeRoot, errCRoot := mockReg.OpenKey(`SOFTWARE\Policies\Google\Chrome`, false)
	if errCRoot != nil {
		t.Fatalf("failed to open Chrome root policy key: %v", errCRoot)
	}
	dohMode, _ := kChromeRoot.GetStringValue("DnsOverHttpsMode")
	kChromeRoot.Close()
	if dohMode != "off" {
		t.Errorf("expected Chrome DnsOverHttpsMode=off, got %s", dohMode)
	}

	// 4. Verify Firefox Policy
	firefoxPolicyKey := `SOFTWARE\Policies\Mozilla\Firefox`
	kFF, err := mockReg.OpenKey(firefoxPolicyKey, false)
	if err != nil {
		t.Fatalf("failed to open Firefox policy key: %v", err)
	}
	disablePrivate, _ := kFF.GetDWordValue("DisablePrivateBrowsing")
	kFF.Close()
	if disablePrivate != 1 {
		t.Errorf("expected DisablePrivateBrowsing=1, got %d", disablePrivate)
	}

	ffExtKey := `SOFTWARE\Policies\Mozilla\Firefox\ExtensionSettings\` + policy.FirefoxExtensionID
	kFFExt, err := mockReg.OpenKey(ffExtKey, false)
	if err != nil {
		t.Fatalf("failed to open Firefox extension settings key: %v", err)
	}
	mode, _ := kFFExt.GetStringValue("installation_mode")
	installURL, _ := kFFExt.GetStringValue("install_url")
	kFFExt.Close()
	if mode != "force_installed" {
		t.Errorf("expected installation_mode=force_installed, got %s", mode)
	}
	if installURL != "https://api.onusly.com/downloads/updates.xml" {
		t.Errorf("expected install_url, got %s", installURL)
	}

	// 5. Verify Uninstalled browser (Brave) was untouched
	bravePolicyKey := `SOFTWARE\Policies\BraveSoftware\Brave\ExtensionInstallForcelist`
	if mockReg.KeyExists(bravePolicyKey) {
		t.Errorf("uninstalled Brave browser should not have had policy applied")
	}

	// 6. Verify Verification Report
	issues, err := mgr.VerifyPolicies(mockBrowsers)
	if err != nil {
		t.Fatalf("VerifyPolicies failed: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 policy issues after clean installation, got %+v", issues)
	}

	// 7. Test Surgical Rollback / Uninstall
	if err := mgr.RemovePolicies(mockBrowsers); err != nil {
		t.Fatalf("RemovePolicies failed: %v", err)
	}

	// Check Chrome: ONUSLY entry "2" must be deleted, but unrelated "1" MUST STILL EXIST!
	kChromeAfter, err := mockReg.OpenKey(chromePolicyKey, false)
	if err != nil {
		t.Fatalf("Chrome policy key should still exist because '1' was present: %v", err)
	}
	val1After, errVal1 := kChromeAfter.GetStringValue("1")
	_, errVal2 := kChromeAfter.GetStringValue("2")
	kChromeAfter.Close()

	if errVal1 != nil || val1After != "other_enterprise_ext_id;https://example.com/update.xml" {
		t.Errorf("CRITICAL REGRESSION: Unrelated extension '1' was lost during uninstall! got %s, err %v", val1After, errVal1)
	}
	if errVal2 == nil {
		t.Errorf("SECURITY INVARIANT FAILED: ONUSLY extension '2' was not removed during uninstall!")
	}

	// Check Chrome Native Host: Key must be deleted
	if mockReg.KeyExists(chromeNativeKey) {
		t.Errorf("Chrome native host key was not removed during uninstall")
	}

	// Check Firefox Extension: Key must be deleted
	if mockReg.KeyExists(ffExtKey) {
		t.Errorf("Firefox ONUSLY extension settings key was not removed during uninstall")
	}
}

func TestWindowsPolicy_IdempotentInstallationAndRemoval(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-idempotent-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	manifestDir := filepath.Join(tmpDir, "manifests")
	backupStatePath := filepath.Join(tmpDir, "backup_state.json")
	fakeNativeBin := filepath.Join(tmpDir, "onusly-native-host.exe")
	_ = os.WriteFile(fakeNativeBin, []byte("fake-binary"), 0755)

	mockReg := policy.NewMockRegistry()
	mgr := policy.NewWindowsPolicyManager(
		fakeNativeBin,
		manifestDir,
		backupStatePath,
		"https://api.onusly.com/downloads/updates.xml",
		mockReg,
	)

	mockBrowsers := []core.BrowserInfo{
		{
			Type:          core.BrowserChrome,
			PolicyDir:     `SOFTWARE\Policies\Google\Chrome`,
			NativeHostDir: `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + policy.NativeHostName,
			Installed:     true,
		},
	}

	// 1. First installation
	if err := mgr.ApplyPolicies(mockBrowsers); err != nil {
		t.Fatalf("first ApplyPolicies failed: %v", err)
	}

	// 2. Second installation (must be strictly idempotent)
	if err := mgr.ApplyPolicies(mockBrowsers); err != nil {
		t.Fatalf("second ApplyPolicies failed: %v", err)
	}

	// Verify only 1 entry was created in ExtensionInstallForcelist, not duplicated!
	chromePolicyKey := `SOFTWARE\Policies\Google\Chrome\ExtensionInstallForcelist`
	kChrome, err := mockReg.OpenKey(chromePolicyKey, false)
	if err != nil {
		t.Fatalf("failed to open Chrome force-install key: %v", err)
	}
	names, _ := kChrome.GetValueNames()
	kChrome.Close()

	if len(names) != 1 {
		t.Errorf("expected exactly 1 entry after idempotent repeat install, got %d: %v", len(names), names)
	}

	// 3. First removal
	if err := mgr.RemovePolicies(mockBrowsers); err != nil {
		t.Fatalf("first RemovePolicies failed: %v", err)
	}

	// 4. Second removal (must succeed cleanly without error)
	if err := mgr.RemovePolicies(mockBrowsers); err != nil {
		t.Fatalf("second RemovePolicies failed: %v", err)
	}
}

func TestWindowsPolicy_PartialInstallationRollback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-partial-rollback-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	manifestDir := filepath.Join(tmpDir, "manifests")
	backupStatePath := filepath.Join(tmpDir, "backup_state.json")
	fakeNativeBin := filepath.Join(tmpDir, "onusly-native-host.exe")
	_ = os.WriteFile(fakeNativeBin, []byte("fake-binary"), 0755)

	mockReg := policy.NewMockRegistry()
	mgr := policy.NewWindowsPolicyManager(
		fakeNativeBin,
		manifestDir,
		backupStatePath,
		"https://api.onusly.com/downloads/updates.xml",
		mockReg,
	)

	// Single browser installed
	mockBrowsers := []core.BrowserInfo{
		{
			Type:          core.BrowserChrome,
			PolicyDir:     `SOFTWARE\Policies\Google\Chrome`,
			NativeHostDir: `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + policy.NativeHostName,
			Installed:     true,
		},
	}

	if err := mgr.ApplyPolicies(mockBrowsers); err != nil {
		t.Fatalf("ApplyPolicies failed: %v", err)
	}

	// Ensure manifest and registry exist
	chromeNativeKey := `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + policy.NativeHostName
	if !mockReg.KeyExists(chromeNativeKey) {
		t.Fatalf("native host key not found after install")
	}

	// Rollback
	if err := mgr.RemovePolicies(mockBrowsers); err != nil {
		t.Fatalf("RemovePolicies failed: %v", err)
	}

	// Verify complete cleanup
	if mockReg.KeyExists(chromeNativeKey) {
		t.Errorf("native host key still exists after rollback")
	}
}
