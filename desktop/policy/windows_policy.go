package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

var (
	ErrInvalidUpdateURL       = errors.New("invalid extension update URL")
	ErrUpdateURLMissingScheme = errors.New("extension update URL must use http or https scheme")
	ErrUpdateURLMissingHost   = errors.New("extension update URL must specify a valid host")
	ErrUpdateURLInvalidExt    = errors.New("extension update URL must point to a valid manifest or package (.xml, .crx, .xpi)")
)

// ValidateUpdateURL verifies that the provided update manifest or package URL is syntactically valid and production-safe.
func ValidateUpdateURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("%w: URL cannot be empty", ErrInvalidUpdateURL)
	}

	parsed, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidUpdateURL, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ErrUpdateURLMissingScheme
	}

	if parsed.Host == "" {
		return ErrUpdateURLMissingHost
	}

	lowerPath := strings.ToLower(parsed.Path)
	ext := filepath.Ext(lowerPath)
	if ext != "" {
		if ext != ".xml" && ext != ".crx" && ext != ".xpi" {
			return fmt.Errorf("%w: invalid extension %s, expected .xml, .crx, or .xpi", ErrUpdateURLInvalidExt, ext)
		}
	} else if !strings.HasSuffix(lowerPath, "/") && !strings.Contains(lowerPath, "updates") {
		return fmt.Errorf("%w: expected .xml, .crx, or update endpoint, got %s", ErrUpdateURLInvalidExt, parsed.Path)
	}

	return nil
}

// GenerateChromiumManifest produces the Windows Native Messaging Host manifest for Chrome, Chromium, Brave, and Edge.
func GenerateChromiumManifest(nativeHostBinPath string) ([]byte, error) {
	if nativeHostBinPath == "" {
		return nil, errors.New("native host binary path cannot be empty")
	}

	absPath, err := filepath.Abs(nativeHostBinPath)
	if err != nil {
		absPath = nativeHostBinPath
	}

	manifest := map[string]any{
		"name":        NativeHostName,
		"description": "ONUSLY Desktop Enforcement Native Messaging Host",
		"path":        absPath,
		"type":        "stdio",
		"allowed_origins": []string{
			fmt.Sprintf("chrome-extension://%s/", ChromiumExtensionID),
			fmt.Sprintf("chrome-extension://%s/", LegacyChromiumExtensionID),
			"chrome-extension://admccjkmockfdflocgggjfgdacdodkdf/",
		},
	}

	return json.MarshalIndent(manifest, "", "  ")
}

// GenerateFirefoxManifest produces the Windows Native Messaging Host manifest for Mozilla Firefox.
func GenerateFirefoxManifest(nativeHostBinPath string) ([]byte, error) {
	if nativeHostBinPath == "" {
		return nil, errors.New("native host binary path cannot be empty")
	}

	absPath, err := filepath.Abs(nativeHostBinPath)
	if err != nil {
		absPath = nativeHostBinPath
	}

	manifest := map[string]any{
		"name":        NativeHostName,
		"description": "ONUSLY Desktop Enforcement Native Messaging Host",
		"path":        absPath,
		"type":        "stdio",
		"allowed_extensions": []string{
			FirefoxExtensionID,
		},
	}

	return json.MarshalIndent(manifest, "", "  ")
}

// PolicyBackupEntry records a single registry modification for idempotent rollback.
type PolicyBackupEntry struct {
	Browser     core.BrowserType `json:"browser"`
	KeyPath     string           `json:"keyPath"`
	ValueName   string           `json:"valueName"`
	PriorType   string           `json:"priorType,omitempty"`  // "string", "dword", "none"
	PriorValue  string           `json:"priorValue,omitempty"` // String representation of prior value
	CreatedByUs bool             `json:"createdByUs"`
	KeyCreated  bool             `json:"keyCreated"`
}

// PolicyBackupState records the full set of policy backups.
type PolicyBackupState struct {
	Timestamp time.Time           `json:"timestamp"`
	Entries   []PolicyBackupEntry `json:"entries"`
}

// RegistryAccessor provides an abstraction over Windows registry operations.
type RegistryAccessor interface {
	KeyExists(keyPath string) bool
	OpenKey(keyPath string, write bool) (RegistryKey, error)
	CreateKey(keyPath string) (RegistryKey, bool, error)
	DeleteKey(keyPath string) error
	DeleteValue(keyPath, valueName string) error
}

// RegistryKey represents an open registry key.
type RegistryKey interface {
	GetStringValue(name string) (string, error)
	GetDWordValue(name string) (uint32, error)
	GetValueNames() ([]string, error)
	SetStringValue(name, value string) error
	SetDWordValue(name string, value uint32) error
	DeleteValue(name string) error
	Close() error
}

// BrowserRegistryPaths encapsulates the registry paths for a specific browser.
type BrowserRegistryPaths struct {
	BrowserType   core.BrowserType
	PolicyKey     string
	NativeHostKey string
	BinaryPaths   []string
}

// WindowsPolicyManager manages browser enterprise policies and native host registrations on Windows.
type WindowsPolicyManager struct {
	nativeHostBinPath string
	manifestDir       string
	backupStatePath   string
	updateURL         string
	reg               RegistryAccessor
}

func NewWindowsPolicyManager(
	nativeHostBinPath string,
	manifestDir string,
	backupStatePath string,
	updateURL string,
	reg RegistryAccessor,
) *WindowsPolicyManager {
	if manifestDir == "" {
		programFiles := os.Getenv("ProgramFiles")
		if programFiles == "" {
			programFiles = `C:\Program Files`
		}
		manifestDir = filepath.Join(programFiles, "Onusly", "manifests")
	}
	if backupStatePath == "" {
		programData := os.Getenv("ProgramData")
		if programData == "" {
			programData = `C:\ProgramData`
		}
		backupStatePath = filepath.Join(programData, "Onusly", "policy-backups", "backup_state.json")
	}
	return &WindowsPolicyManager{
		nativeHostBinPath: nativeHostBinPath,
		manifestDir:       manifestDir,
		backupStatePath:   backupStatePath,
		updateURL:         updateURL,
		reg:               reg,
	}
}

func (m *WindowsPolicyManager) SetUpdateURL(url string) {
	m.updateURL = url
}

func (m *WindowsPolicyManager) GetBrowserRegistryPaths() []BrowserRegistryPaths {
	return []BrowserRegistryPaths{
		{
			BrowserType:   core.BrowserChrome,
			PolicyKey:     `SOFTWARE\Policies\Google\Chrome`,
			NativeHostKey: `SOFTWARE\Google\Chrome\NativeMessagingHosts\` + NativeHostName,
			BinaryPaths: []string{
				`C:\Program Files\Google\Chrome\Application\chrome.exe`,
				`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			},
		},
		{
			BrowserType:   core.BrowserEdge,
			PolicyKey:     `SOFTWARE\Policies\Microsoft\Edge`,
			NativeHostKey: `SOFTWARE\Microsoft\Edge\NativeMessagingHosts\` + NativeHostName,
			BinaryPaths: []string{
				`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
				`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			},
		},
		{
			BrowserType:   core.BrowserBrave,
			PolicyKey:     `SOFTWARE\Policies\BraveSoftware\Brave`,
			NativeHostKey: `SOFTWARE\BraveSoftware\Brave\NativeMessagingHosts\` + NativeHostName,
			BinaryPaths: []string{
				`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
				`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
			},
		},
		{
			BrowserType:   core.BrowserChromium,
			PolicyKey:     `SOFTWARE\Policies\Chromium`,
			NativeHostKey: `SOFTWARE\Chromium\NativeMessagingHosts\` + NativeHostName,
			BinaryPaths: []string{
				`C:\Program Files\Chromium\Application\chrome.exe`,
				`C:\Program Files (x86)\Chromium\Application\chrome.exe`,
			},
		},
		{
			BrowserType:   core.BrowserFirefox,
			PolicyKey:     `SOFTWARE\Policies\Mozilla\Firefox`,
			NativeHostKey: `SOFTWARE\Mozilla\NativeMessagingHosts\` + NativeHostName,
			BinaryPaths: []string{
				`C:\Program Files\Mozilla Firefox\firefox.exe`,
				`C:\Program Files (x86)\Mozilla Firefox\firefox.exe`,
			},
		},
	}
}

// DetectBrowsers inspects the filesystem and registry to find installed browsers on Windows.
func (m *WindowsPolicyManager) DetectBrowsers() ([]core.BrowserInfo, error) {
	var results []core.BrowserInfo
	paths := m.GetBrowserRegistryPaths()

	for _, p := range paths {
		var foundBin string
		for _, bin := range p.BinaryPaths {
			if _, err := os.Stat(bin); err == nil {
				foundBin = bin
				break
			}
		}

		installed := foundBin != ""
		if !installed && m.reg != nil {
			// Check if browser installation registry key exists
			var regDetectKey string
			switch p.BrowserType {
			case core.BrowserChrome:
				regDetectKey = `SOFTWARE\Google\Chrome`
			case core.BrowserEdge:
				regDetectKey = `SOFTWARE\Microsoft\Edge`
			case core.BrowserBrave:
				regDetectKey = `SOFTWARE\BraveSoftware\Brave`
			case core.BrowserFirefox:
				regDetectKey = `SOFTWARE\Mozilla\Mozilla Firefox`
			}
			if regDetectKey != "" && m.reg.KeyExists(regDetectKey) {
				installed = true
			}
		}

		info := core.BrowserInfo{
			Type:                  p.BrowserType,
			BinaryPath:            foundBin,
			PolicyDir:             p.PolicyKey,
			NativeHostDir:         p.NativeHostKey,
			SupportsManagedPolicy: true,
			Installed:             installed,
		}
		results = append(results, info)
	}

	return results, nil
}

// ApplyPolicies registers the native host manifests and configures enterprise force-install policies.
func (m *WindowsPolicyManager) ApplyPolicies(browsers []core.BrowserInfo) error {
	if err := ValidateUpdateURL(m.updateURL); err != nil {
		return fmt.Errorf("preflight policy configuration failed: %w", err)
	}

	if err := os.MkdirAll(m.manifestDir, 0755); err != nil {
		return fmt.Errorf("failed to create manifest directory: %w", err)
	}

	// 1. Write Chromium Manifest
	chromeManifestPath := filepath.Join(m.manifestDir, NativeHostName+"_chrome.json")
	chromeManifestBytes, err := GenerateChromiumManifest(m.nativeHostBinPath)
	if err != nil {
		return fmt.Errorf("failed to generate Chromium manifest: %w", err)
	}
	if err := os.WriteFile(chromeManifestPath, chromeManifestBytes, 0644); err != nil {
		return fmt.Errorf("failed to write Chromium manifest: %w", err)
	}

	// 2. Write Firefox Manifest
	firefoxManifestPath := filepath.Join(m.manifestDir, NativeHostName+"_firefox.json")
	firefoxManifestBytes, err := GenerateFirefoxManifest(m.nativeHostBinPath)
	if err != nil {
		return fmt.Errorf("failed to generate Firefox manifest: %w", err)
	}
	if err := os.WriteFile(firefoxManifestPath, firefoxManifestBytes, 0644); err != nil {
		return fmt.Errorf("failed to write Firefox manifest: %w", err)
	}

	// Load existing backup state or initialize
	backupState := m.loadBackupState()

	for _, b := range browsers {
		if !b.Installed {
			continue
		}

		var manifestPath string
		if b.Type == core.BrowserFirefox {
			manifestPath = firefoxManifestPath
		} else {
			manifestPath = chromeManifestPath
		}

		// Register Native Messaging Host in Registry
		if err := m.registerNativeHost(b.Type, b.NativeHostDir, manifestPath, &backupState); err != nil {
			return fmt.Errorf("failed to register native host for %s: %w", b.Type, err)
		}

		// Apply Enterprise Force-Install Policy
		if b.Type == core.BrowserFirefox {
			if err := m.applyFirefoxPolicy(b.PolicyDir, &backupState); err != nil {
				return fmt.Errorf("failed to configure Firefox policy: %w", err)
			}
		} else {
			if err := m.applyChromiumPolicy(b.Type, b.PolicyDir, &backupState); err != nil {
				return fmt.Errorf("failed to configure %s policy: %w", b.Type, err)
			}
		}
	}

	// Persist backup state for surgical rollback
	return m.saveBackupState(backupState)
}

func (m *WindowsPolicyManager) registerNativeHost(
	browserType core.BrowserType,
	hostKeyPath string,
	manifestPath string,
	backupState *PolicyBackupState,
) error {
	k, openedExisting, err := m.reg.CreateKey(hostKeyPath)
	if err != nil {
		return err
	}
	defer k.Close()

	// Default value in registry is named ""
	priorVal, priorErr := k.GetStringValue("")
	entry := PolicyBackupEntry{
		Browser:     browserType,
		KeyPath:     hostKeyPath,
		ValueName:   "",
		CreatedByUs: true,
		KeyCreated:  !openedExisting,
	}
	if priorErr == nil {
		entry.PriorType = "string"
		entry.PriorValue = priorVal
		entry.CreatedByUs = false
	} else {
		entry.PriorType = "none"
	}
	backupState.Entries = append(backupState.Entries, entry)

	return k.SetStringValue("", manifestPath)
}

func (m *WindowsPolicyManager) applyChromiumPolicy(
	browserType core.BrowserType,
	policyKeyPath string,
	backupState *PolicyBackupState,
) error {
	forceKeyPath := policyKeyPath + `\ExtensionInstallForcelist`
	k, openedExisting, err := m.reg.CreateKey(forceKeyPath)
	if err != nil {
		return err
	}
	defer k.Close()

	valueNames, _ := k.GetValueNames()
	forceEntry := fmt.Sprintf("%s;%s", ChromiumExtensionID, m.updateURL)

	// Check if an existing entry already covers this extension
	var existingName string
	maxIndex := 0
	for _, name := range valueNames {
		idx, parseErr := strconv.Atoi(name)
		if parseErr == nil && idx > maxIndex {
			maxIndex = idx
		}

		val, errVal := k.GetStringValue(name)
		if errVal == nil {
			if strings.HasPrefix(val, ChromiumExtensionID+";") || strings.HasPrefix(val, LegacyChromiumExtensionID+";") {
				existingName = name
				break
			}
		}
	}

	if existingName != "" {
		// Update existing value
		priorVal, _ := k.GetStringValue(existingName)
		backupState.Entries = append(backupState.Entries, PolicyBackupEntry{
			Browser:     browserType,
			KeyPath:     forceKeyPath,
			ValueName:   existingName,
			PriorType:   "string",
			PriorValue:  priorVal,
			CreatedByUs: false,
			KeyCreated:  !openedExisting,
		})
		return k.SetStringValue(existingName, forceEntry)
	}

	// Add new numeric entry without overwriting other extensions
	newName := strconv.Itoa(maxIndex + 1)
	backupState.Entries = append(backupState.Entries, PolicyBackupEntry{
		Browser:     browserType,
		KeyPath:     forceKeyPath,
		ValueName:   newName,
		PriorType:   "none",
		CreatedByUs: true,
		KeyCreated:  !openedExisting,
	})

	return k.SetStringValue(newName, forceEntry)
}

func (m *WindowsPolicyManager) applyFirefoxPolicy(
	policyKeyPath string,
	backupState *PolicyBackupState,
) error {
	// 1. DisablePrivateBrowsing to prevent accountability bypass
	rootK, openedExisting, err := m.reg.CreateKey(policyKeyPath)
	if err != nil {
		return err
	}
	defer rootK.Close()

	priorDword, errDword := rootK.GetDWordValue("DisablePrivateBrowsing")
	entryPriv := PolicyBackupEntry{
		Browser:     core.BrowserFirefox,
		KeyPath:     policyKeyPath,
		ValueName:   "DisablePrivateBrowsing",
		CreatedByUs: errDword != nil,
		KeyCreated:  !openedExisting,
	}
	if errDword == nil {
		entryPriv.PriorType = "dword"
		entryPriv.PriorValue = strconv.FormatUint(uint64(priorDword), 10)
	} else {
		entryPriv.PriorType = "none"
	}
	backupState.Entries = append(backupState.Entries, entryPriv)
	_ = rootK.SetDWordValue("DisablePrivateBrowsing", 1)

	// 2. ExtensionSettings\extension@onusly.com
	extKeyPath := policyKeyPath + `\ExtensionSettings\` + FirefoxExtensionID
	extK, extOpenedExisting, errExt := m.reg.CreateKey(extKeyPath)
	if errExt != nil {
		return errExt
	}
	defer extK.Close()

	priorInstallMode, errMode := extK.GetStringValue("installation_mode")
	entryMode := PolicyBackupEntry{
		Browser:     core.BrowserFirefox,
		KeyPath:     extKeyPath,
		ValueName:   "installation_mode",
		CreatedByUs: errMode != nil,
		KeyCreated:  !extOpenedExisting,
	}
	if errMode == nil {
		entryMode.PriorType = "string"
		entryMode.PriorValue = priorInstallMode
	} else {
		entryMode.PriorType = "none"
	}
	backupState.Entries = append(backupState.Entries, entryMode)
	_ = extK.SetStringValue("installation_mode", "force_installed")

	priorInstallURL, errURL := extK.GetStringValue("install_url")
	entryURL := PolicyBackupEntry{
		Browser:     core.BrowserFirefox,
		KeyPath:     extKeyPath,
		ValueName:   "install_url",
		CreatedByUs: errURL != nil,
		KeyCreated:  !extOpenedExisting,
	}
	if errURL == nil {
		entryURL.PriorType = "string"
		entryURL.PriorValue = priorInstallURL
	} else {
		entryURL.PriorType = "none"
	}
	backupState.Entries = append(backupState.Entries, entryURL)

	return extK.SetStringValue("install_url", m.updateURL)
}

// VerifyPolicies checks registry settings and manifest files for consistency and policy drift.
func (m *WindowsPolicyManager) VerifyPolicies(browsers []core.BrowserInfo) ([]core.PolicyIssue, error) {
	var issues []core.PolicyIssue

	for _, b := range browsers {
		if !b.Installed {
			continue
		}

		// 1. Verify Native Messaging Host Registry Key
		if !m.reg.KeyExists(b.NativeHostDir) {
			issues = append(issues, core.PolicyIssue{
				Browser: b.Type,
				Issue:   "native_host_registry_missing",
				Details: fmt.Sprintf("Registry key missing: %s", b.NativeHostDir),
			})
		} else {
			k, err := m.reg.OpenKey(b.NativeHostDir, false)
			if err == nil {
				manifestPath, errVal := k.GetStringValue("")
				k.Close()
				if errVal != nil || manifestPath == "" {
					issues = append(issues, core.PolicyIssue{
						Browser: b.Type,
						Issue:   "native_host_manifest_empty",
						Details: "Default manifest value in registry is missing or empty",
					})
				} else if _, errStat := os.Stat(manifestPath); os.IsNotExist(errStat) {
					issues = append(issues, core.PolicyIssue{
						Browser: b.Type,
						Issue:   "native_host_manifest_file_missing",
						Details: fmt.Sprintf("Manifest file on disk not found: %s", manifestPath),
					})
				}
			}
		}

		// 2. Verify Enterprise Force-Install Policy
		if b.Type == core.BrowserFirefox {
			extKeyPath := b.PolicyDir + `\ExtensionSettings\` + FirefoxExtensionID
			if !m.reg.KeyExists(extKeyPath) {
				issues = append(issues, core.PolicyIssue{
					Browser: b.Type,
					Issue:   "firefox_policy_missing",
					Details: fmt.Sprintf("ExtensionSettings key missing: %s", extKeyPath),
				})
			} else {
				k, err := m.reg.OpenKey(extKeyPath, false)
				if err == nil {
					mode, _ := k.GetStringValue("installation_mode")
					k.Close()
					if mode != "force_installed" {
						issues = append(issues, core.PolicyIssue{
							Browser: b.Type,
							Issue:   "firefox_policy_invalid",
							Details: fmt.Sprintf("Expected installation_mode=force_installed, got %s", mode),
						})
					}
				}
			}
		} else {
			forceKeyPath := b.PolicyDir + `\ExtensionInstallForcelist`
			if !m.reg.KeyExists(forceKeyPath) {
				issues = append(issues, core.PolicyIssue{
					Browser: b.Type,
					Issue:   "chromium_force_policy_missing",
					Details: fmt.Sprintf("ExtensionInstallForcelist key missing: %s", forceKeyPath),
				})
			} else {
				k, err := m.reg.OpenKey(forceKeyPath, false)
				if err == nil {
					valueNames, _ := k.GetValueNames()
					found := false
					for _, name := range valueNames {
						val, _ := k.GetStringValue(name)
						if strings.HasPrefix(val, ChromiumExtensionID+";") {
							found = true
							break
						}
					}
					k.Close()
					if !found {
						issues = append(issues, core.PolicyIssue{
							Browser: b.Type,
							Issue:   "chromium_extension_entry_missing",
							Details: fmt.Sprintf("Extension ID %s not found in %s", ChromiumExtensionID, forceKeyPath),
						})
					}
				}
			}
		}
	}

	return issues, nil
}

// RemovePolicies surgically rolls back only the entries created by ONUSLY, preserving unrelated enterprise policies.
func (m *WindowsPolicyManager) RemovePolicies(browsers []core.BrowserInfo) error {
	backupState := m.loadBackupState()
	if len(backupState.Entries) == 0 {
		// Nothing recorded in backup, perform best-effort cleanup of ONUSLY entries
		for _, b := range browsers {
			_ = m.reg.DeleteKey(b.NativeHostDir)
			if b.Type == core.BrowserFirefox {
				_ = m.reg.DeleteKey(b.PolicyDir + `\ExtensionSettings\` + FirefoxExtensionID)
			} else {
				forceKeyPath := b.PolicyDir + `\ExtensionInstallForcelist`
				if k, err := m.reg.OpenKey(forceKeyPath, true); err == nil {
					valueNames, _ := k.GetValueNames()
					for _, name := range valueNames {
						val, _ := k.GetStringValue(name)
						if strings.HasPrefix(val, ChromiumExtensionID+";") || strings.HasPrefix(val, LegacyChromiumExtensionID+";") {
							_ = k.DeleteValue(name)
						}
					}
					k.Close()
				}
			}
		}
		_ = os.Remove(filepath.Join(m.manifestDir, NativeHostName+"_chrome.json"))
		_ = os.Remove(filepath.Join(m.manifestDir, NativeHostName+"_firefox.json"))
		return nil
	}

	// Roll back entries in reverse order
	for i := len(backupState.Entries) - 1; i >= 0; i-- {
		entry := backupState.Entries[i]

		if entry.CreatedByUs {
			// Newly created value: delete it
			_ = m.reg.DeleteValue(entry.KeyPath, entry.ValueName)

			// If the entire key was created by us and has no remaining values, remove the subkey
			if entry.KeyCreated {
				if k, err := m.reg.OpenKey(entry.KeyPath, false); err == nil {
					names, _ := k.GetValueNames()
					k.Close()
					if len(names) == 0 {
						_ = m.reg.DeleteKey(entry.KeyPath)
					}
				}
			}
		} else if entry.PriorType == "string" {
			// Prior string existed: restore it
			if k, err := m.reg.OpenKey(entry.KeyPath, true); err == nil {
				_ = k.SetStringValue(entry.ValueName, entry.PriorValue)
				k.Close()
			}
		} else if entry.PriorType == "dword" {
			// Prior dword existed: restore it
			if dw, errParse := strconv.ParseUint(entry.PriorValue, 10, 32); errParse == nil {
				if k, err := m.reg.OpenKey(entry.KeyPath, true); err == nil {
					_ = k.SetDWordValue(entry.ValueName, uint32(dw))
					k.Close()
				}
			}
		}
	}

	// Remove manifest files
	_ = os.Remove(filepath.Join(m.manifestDir, NativeHostName+"_chrome.json"))
	_ = os.Remove(filepath.Join(m.manifestDir, NativeHostName+"_firefox.json"))

	// Remove backup state file
	_ = os.Remove(m.backupStatePath)

	return nil
}

func (m *WindowsPolicyManager) loadBackupState() PolicyBackupState {
	var state PolicyBackupState
	data, err := os.ReadFile(m.backupStatePath)
	if err == nil {
		_ = json.Unmarshal(data, &state)
	}
	if state.Entries == nil {
		state.Entries = []PolicyBackupEntry{}
	}
	return state
}

func (m *WindowsPolicyManager) saveBackupState(state PolicyBackupState) error {
	state.Timestamp = time.Now()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	_ = os.MkdirAll(filepath.Dir(m.backupStatePath), 0700)
	return os.WriteFile(m.backupStatePath, data, 0600)
}
