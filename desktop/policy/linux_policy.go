package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

const (
	ChromiumExtensionID       = "jifigaejbinmdhnljobkodcgeaplojmg"
	LegacyChromiumExtensionID = "gkichkblphmdbojdjnonljlegnmjcnhg"
	FirefoxExtensionID        = "extension@onusly.com"
	NativeHostName            = "com.onusly.desktop_agent"
)

type LinuxPolicyManager struct {
	nativeHostBinaryPath string
	backupDir            string
	updateURL            string
	useUserScope         bool
}

func NewLinuxPolicyManager(nativeHostBinaryPath string, backupDir string, useUserScope bool) *LinuxPolicyManager {
	if nativeHostBinaryPath == "" {
		nativeHostBinaryPath = "/usr/local/bin/onusly-native-host"
	}
	if backupDir == "" {
		if !useUserScope && os.Geteuid() == 0 {
			backupDir = "/var/lib/onusly/policy-backups"
		} else {
			home, _ := os.UserHomeDir()
			backupDir = filepath.Join(home, ".config", "onusly", "policy-backups")
		}
	}
	return &LinuxPolicyManager{
		nativeHostBinaryPath: nativeHostBinaryPath,
		backupDir:            backupDir,
		updateURL:            "http://127.0.0.1:8080/downloads/updates.xml",
		useUserScope:         useUserScope,
	}
}

// SetUpdateURL overrides the update manifest URL for self-hosted CRX installation.
func (m *LinuxPolicyManager) SetUpdateURL(url string) {
	m.updateURL = url
}

func (m *LinuxPolicyManager) DetectBrowsers() ([]core.BrowserInfo, error) {
	home, _ := os.UserHomeDir()
	candidates := []struct {
		typ           core.BrowserType
		bins          []string
		sysPolicyDir  string
		sysNativeDir  string
		userNativeDir string
	}{
		{
			typ:           core.BrowserChrome,
			bins:          []string{"google-chrome", "google-chrome-stable"},
			sysPolicyDir:  "/etc/opt/chrome/policies/managed",
			sysNativeDir:  "/etc/opt/chrome/native-messaging-hosts",
			userNativeDir: filepath.Join(home, ".config", "google-chrome", "NativeMessagingHosts"),
		},
		{
			typ:           core.BrowserChromium,
			bins:          []string{"chromium", "chromium-browser"},
			sysPolicyDir:  "/etc/chromium/policies/managed",
			sysNativeDir:  "/etc/chromium/native-messaging-hosts",
			userNativeDir: filepath.Join(home, ".config", "chromium", "NativeMessagingHosts"),
		},
		{
			typ:           core.BrowserBrave,
			bins:          []string{"brave-browser", "brave-browser-stable", "brave"},
			sysPolicyDir:  "/etc/brave/policies/managed",
			sysNativeDir:  "/etc/brave/native-messaging-hosts",
			userNativeDir: filepath.Join(home, ".config", "BraveSoftware", "Brave-Browser", "NativeMessagingHosts"),
		},
		{
			typ:           core.BrowserEdge,
			bins:          []string{"microsoft-edge", "microsoft-edge-stable"},
			sysPolicyDir:  "/etc/opt/edge/policies/managed",
			sysNativeDir:  "/etc/opt/edge/native-messaging-hosts",
			userNativeDir: filepath.Join(home, ".config", "microsoft-edge", "NativeMessagingHosts"),
		},
		{
			typ:           core.BrowserFirefox,
			bins:          []string{"firefox"},
			sysPolicyDir:  "/etc/firefox/policies",
			sysNativeDir:  "/usr/lib64/mozilla/native-messaging-hosts",
			userNativeDir: filepath.Join(home, ".mozilla", "native-messaging-hosts"),
		},
	}

	var results []core.BrowserInfo
	for _, c := range candidates {
		var foundBin string
		for _, b := range c.bins {
			if path, err := exec.LookPath(b); err == nil {
				foundBin = path
				break
			}
		}

		info := core.BrowserInfo{
			Type:                  c.typ,
			BinaryPath:            foundBin,
			PolicyDir:             c.sysPolicyDir,
			NativeHostDir:         c.sysNativeDir,
			UserNativeHostDir:     c.userNativeDir,
			SupportsManagedPolicy: true,
			Installed:             foundBin != "",
		}
		results = append(results, info)
	}

	return results, nil
}

func (m *LinuxPolicyManager) ApplyPolicies(browsers []core.BrowserInfo) error {
	for _, b := range browsers {
		if !b.Installed {
			continue
		}

		// 1. Write Native Messaging Host Manifest
		targetNativeDir := b.NativeHostDir
		if m.useUserScope || os.Geteuid() != 0 {
			targetNativeDir = b.UserNativeHostDir
		}

		if err := m.writeNativeManifest(b.Type, targetNativeDir); err != nil {
			return fmt.Errorf("failed to register native host for %s: %w", b.Type, err)
		}

		// 2. Write Enterprise Policies
		var policyErr error
		if b.Type == core.BrowserFirefox {
			policyErr = m.writeFirefoxPolicy(b.PolicyDir)
		} else {
			policyErr = m.writeChromiumPolicy(b.Type, b.PolicyDir)
		}

		if policyErr != nil {
			if m.useUserScope || os.Geteuid() != 0 {
				// In user scope without root, writing to /etc is restricted.
				// Native messaging is successfully registered in user config.
				// Enterprise policy is optional/root-only.
				continue
			}
			return fmt.Errorf("failed to configure %s policy: %w", b.Type, policyErr)
		}
	}
	return nil
}

func (m *LinuxPolicyManager) writeNativeManifest(browserType core.BrowserType, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	manifestPath := filepath.Join(dir, NativeHostName+".json")

	var manifest map[string]any
	if browserType == core.BrowserFirefox {
		manifest = map[string]any{
			"name":        NativeHostName,
			"description": "ONUSLY Desktop Enforcement Native Messaging Host",
			"path":        m.nativeHostBinaryPath,
			"type":        "stdio",
			"allowed_extensions": []string{
				FirefoxExtensionID,
			},
		}
	} else {
		manifest = map[string]any{
			"name":        NativeHostName,
			"description": "ONUSLY Desktop Enforcement Native Messaging Host",
			"path":        m.nativeHostBinaryPath,
			"type":        "stdio",
			"allowed_origins": []string{
				fmt.Sprintf("chrome-extension://%s/", ChromiumExtensionID),
				fmt.Sprintf("chrome-extension://%s/", LegacyChromiumExtensionID),
				"chrome-extension://admccjkmockfdflocgggjfgdacdodkdf/",
			},
		}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(manifestPath, data, 0644)
}

func (m *LinuxPolicyManager) writeChromiumPolicy(browserType core.BrowserType, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	policyPath := filepath.Join(dir, "onusly.json")

	// Read existing policy if present (Preserve unrelated settings)
	existing := make(map[string]any)
	if raw, err := os.ReadFile(policyPath); err == nil {
		_ = json.Unmarshal(raw, &existing)
		_ = m.backupFile(browserType, policyPath)
	}

	updateURL := m.updateURL
	if updateURL == "" {
		updateURL = "http://127.0.0.1:8080/downloads/updates.xml"
	}

	// 1. Force-install extension via self-hosted enterprise ExtensionSettings
	extSettings, ok := existing["ExtensionSettings"].(map[string]any)
	if !ok {
		extSettings = make(map[string]any)
	}
	extSettings[ChromiumExtensionID] = map[string]any{
		"installation_mode":   "force_installed",
		"update_url":          updateURL,
		"override_update_url": true,
	}
	existing["ExtensionSettings"] = extSettings

	// 2. Also configure ExtensionInstallForcelist for broad browser compatibility
	forceEntry := fmt.Sprintf("%s;%s", ChromiumExtensionID, updateURL)

	var forceList []string
	if rawList, ok := existing["ExtensionInstallForcelist"].([]any); ok {
		for _, v := range rawList {
			if s, ok := v.(string); ok && !strings.HasPrefix(s, ChromiumExtensionID) && !strings.HasPrefix(s, LegacyChromiumExtensionID) {
				forceList = append(forceList, s)
			}
		}
	}
	forceList = append(forceList, forceEntry)

	existing["ExtensionInstallForcelist"] = forceList
	existing["IncognitoModeAvailability"] = 1  // Enforced or restricted incognito
	existing["DeveloperToolsAvailability"] = 1 // Allow DevTools inspection on managed extensions
	existing["DnsOverHttpsMode"] = "off"        // Prevent browser DoH bypass of local DNS enforcement

	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(policyPath, data, 0644)
}

func (m *LinuxPolicyManager) writeFirefoxPolicy(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	policyPath := filepath.Join(dir, "policies.json")
	type policiesWrapper struct {
		Policies map[string]any `json:"policies"`
	}

	wrapper := policiesWrapper{Policies: make(map[string]any)}
	if raw, err := os.ReadFile(policyPath); err == nil {
		_ = json.Unmarshal(raw, &wrapper)
		_ = m.backupFile(core.BrowserFirefox, policyPath)
	}
	if wrapper.Policies == nil {
		wrapper.Policies = make(map[string]any)
	}

	// Disable private browsing in Firefox enterprise policy to prevent evasion
	wrapper.Policies["DisablePrivateBrowsing"] = true

	// Firefox Enterprise ExtensionSettings
	extSettings, ok := wrapper.Policies["ExtensionSettings"].(map[string]any)
	if !ok {
		extSettings = make(map[string]any)
	}

	extSettings[FirefoxExtensionID] = map[string]any{
		"installation_mode": "force_installed",
		"private_browsing":  true, // Enforce in private browsing mode
	}

	wrapper.Policies["ExtensionSettings"] = extSettings

	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(policyPath, data, 0644)
}

func (m *LinuxPolicyManager) backupFile(browserType core.BrowserType, src string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	_ = os.MkdirAll(m.backupDir, 0700)
	dest := filepath.Join(m.backupDir, fmt.Sprintf("%s-%s.bak", browserType, filepath.Base(src)))
	return os.WriteFile(dest, data, 0600)
}

func (m *LinuxPolicyManager) VerifyPolicies(browsers []core.BrowserInfo) ([]core.PolicyIssue, error) {
	var issues []core.PolicyIssue

	for _, b := range browsers {
		if !b.Installed {
			continue
		}

		// Verify native manifest
		targetNativeDir := b.NativeHostDir
		if m.useUserScope || os.Geteuid() != 0 {
			targetNativeDir = b.UserNativeHostDir
		}
		manifestPath := filepath.Join(targetNativeDir, NativeHostName+".json")
		if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
			issues = append(issues, core.PolicyIssue{
				Browser: b.Type,
				Issue:   "native_host_manifest_missing",
				Details: fmt.Sprintf("Manifest not found at %s", manifestPath),
			})
		}

		// Verify enterprise policy file
		if b.Type == core.BrowserFirefox {
			policyPath := filepath.Join(b.PolicyDir, "policies.json")
			if _, err := os.Stat(policyPath); os.IsNotExist(err) {
				issues = append(issues, core.PolicyIssue{
					Browser: b.Type,
					Issue:   "policy_missing",
					Details: fmt.Sprintf("Firefox policies.json missing at %s", policyPath),
				})
			}
		} else {
			policyPath := filepath.Join(b.PolicyDir, "onusly.json")
			if _, err := os.Stat(policyPath); os.IsNotExist(err) {
				issues = append(issues, core.PolicyIssue{
					Browser: b.Type,
					Issue:   "policy_missing",
					Details: fmt.Sprintf("Chromium policy missing at %s", policyPath),
				})
			}
		}
	}

	return issues, nil
}

func (m *LinuxPolicyManager) RemovePolicies(browsers []core.BrowserInfo) error {
	for _, b := range browsers {
		// Remove native messaging host manifest
		targetNativeDir := b.NativeHostDir
		if m.useUserScope || os.Geteuid() != 0 {
			targetNativeDir = b.UserNativeHostDir
		}
		_ = os.Remove(filepath.Join(targetNativeDir, NativeHostName+".json"))

		// Restore original policy from backup if present; otherwise delete ONUSLY policy file
		var policyPath string
		if b.Type == core.BrowserFirefox {
			policyPath = filepath.Join(b.PolicyDir, "policies.json")
		} else {
			policyPath = filepath.Join(b.PolicyDir, "onusly.json")
		}

		backupFile := filepath.Join(m.backupDir, fmt.Sprintf("%s-%s.bak", b.Type, filepath.Base(policyPath)))
		if backupData, err := os.ReadFile(backupFile); err == nil {
			_ = os.WriteFile(policyPath, backupData, 0644)
			_ = os.Remove(backupFile)
		} else {
			_ = os.Remove(policyPath)
		}
	}
	return nil
}
