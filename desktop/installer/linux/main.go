package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"github.com/devanshbaghel18/ONUSLY/desktop/policy"
)

const systemdServiceTemplate = `[Unit]
Description=ONUSLY Desktop Unified Enforcement Agent
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/onusly-agent run --api-url=%s --config-dir=/etc/onusly --data-dir=/var/lib/onusly --socket=/run/onusly/agent.sock
Restart=always
RestartSec=5
LimitNOFILE=65536
CapabilityBoundingSet=
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
`

func main() {
	var (
		uninstall      = flag.Bool("uninstall", false, "Uninstall ONUSLY Desktop and restore browser policies")
		apiURL         = flag.String("api-url", "http://localhost:8080", "ONUSLY API base URL")
		updateURL      = flag.String("update-url", "", "Custom Chrome extension update manifest URL (default derived from api-url)")
		nonInteractive = flag.Bool("yes", false, "Accept consent disclosures automatically without interactive prompt")
		userMode       = flag.Bool("user", false, "Install in user mode without requiring root")
	)
	flag.Parse()

	isRoot := os.Geteuid() == 0
	useUserScope := *userMode || !isRoot

	nativeHostBin := "/usr/local/bin/onusly-native-host"
	agentBin := "/usr/local/bin/onusly-agent"
	if useUserScope {
		home, _ := os.UserHomeDir()
		nativeHostBin = filepath.Join(home, ".local", "bin", "onusly-native-host")
		agentBin = filepath.Join(home, ".local", "bin", "onusly-agent")
	}

	policyMgr := policy.NewLinuxPolicyManager(nativeHostBin, "", useUserScope)
	actualUpdateURL := *updateURL
	if actualUpdateURL == "" {
		actualUpdateURL = fmt.Sprintf("%s/downloads/updates.xml", strings.TrimRight(*apiURL, "/"))
	}
	policyMgr.SetUpdateURL(actualUpdateURL)
	browsers, err := policyMgr.DetectBrowsers()
	if err != nil {
		fmt.Printf("Error detecting browsers: %v\n", err)
		os.Exit(1)
	}

	if *uninstall {
		runUninstall(policyMgr, browsers, isRoot)
		return
	}

	runInstall(*apiURL, policyMgr, browsers, nativeHostBin, agentBin, *nonInteractive, isRoot, useUserScope)
}

func runInstall(
	apiURL string,
	mgr *policy.LinuxPolicyManager,
	browsers []core.BrowserInfo,
	nativeHostBin, agentBin string,
	autoAccept bool,
	isRoot bool,
	useUserScope bool,
) {
	fmt.Println("=================================================================")
	fmt.Println("             ONUSLY DESKTOP — UNIFIED INSTALLER                  ")
	fmt.Println("        One Installation → One Account → All Browsers            ")
	fmt.Println("=================================================================")
	fmt.Println()

	var detected []core.BrowserInfo
	for _, b := range browsers {
		if b.Installed {
			detected = append(detected, b)
		}
	}

	fmt.Println("🔍 DETECTED BROWSERS ON THIS SYSTEM:")
	if len(detected) == 0 {
		fmt.Println("  (No supported browsers detected. Supported: Chrome, Chromium, Brave, Edge, Firefox)")
	} else {
		for _, b := range detected {
			fmt.Printf("  • %-10s -> %s\n", strings.ToUpper(string(b.Type)), b.BinaryPath)
		}
	}
	fmt.Println()

	// Consent & Privacy Disclosures (Section 7.3)
	fmt.Println("📋 INSTALLATION & PRIVACY DISCLOSURES:")
	fmt.Println("  1. Managed Policies:")
	fmt.Println("     - Registers ONUSLY extension force-installation for detected Chromium browsers.")
	fmt.Println("     - Registers Enterprise ExtensionSettings for Mozilla Firefox.")
	fmt.Println("     - Disables incognito/private mode bypassing during active goal locks.")
	fmt.Println("  2. Native Messaging Hosts:")
	fmt.Printf("     - Registers %s native host manifest pointing to %s.\n", policy.NativeHostName, nativeHostBin)
	fmt.Println("  3. Central Background Agent Service:")
	fmt.Println("     - Automatically keeps all browsers synchronized to the authoritative blocklist.")
	fmt.Println("     - Recovers enforcement seamlessly across reboots and network reconnects.")
	fmt.Println("  4. Privacy Guarantee:")
	fmt.Println("     - NO browsing history, NO screen contents, NO tab URLs are sent to ONUSLY.")
	fmt.Println("     - Telemetry only sends device health, blocklist version, and active issues.")
	fmt.Println("  5. Safe Recovery & Uninstallation:")
	fmt.Println("     - Existing browser policies are backed up. Run with --uninstall anytime.")
	fmt.Println()

	if !autoAccept {
		fmt.Print("Do you accept these configuration changes and wish to proceed? [y/N]: ")
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		ans = strings.ToLower(strings.TrimSpace(ans))
		if ans != "y" && ans != "yes" {
			fmt.Println("Installation aborted by user.")
			os.Exit(0)
		}
	}

	fmt.Println("\n⚙️  Applying configuration...")

	// 0. In system mode (root), guarantee dedicated 'onusly' group exists and configure paired user access
	if isRoot {
		if err := ensureSystemGroupAndACL(); err != nil {
			fmt.Printf("❌ Failed security configuration: %v\n", err)
			os.Exit(1)
		}
	}

	// 1. Copy binaries if present in local build tree
	_ = os.MkdirAll(filepath.Dir(nativeHostBin), 0755)
	_ = copyFileIfExists("onusly-native-host", nativeHostBin, 0755)
	_ = copyFileIfExists("onusly-agent", agentBin, 0755)

	// 2. Apply browser enterprise policies & native messaging manifests
	if err := mgr.ApplyPolicies(browsers); err != nil {
		fmt.Printf("❌ Failed to apply browser policies: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Enterprise browser policies and native messaging hosts registered.")

	// 3. Install systemd service if root
	if isRoot {
		serviceContent := fmt.Sprintf(systemdServiceTemplate, apiURL)
		servicePath := "/etc/systemd/system/onusly-desktop.service"
		if err := os.WriteFile(servicePath, []byte(serviceContent), 0644); err == nil {
			_ = exec.Command("systemctl", "daemon-reload").Run()
			_ = exec.Command("systemctl", "enable", "--now", "onusly-desktop").Run()
			fmt.Println("✅ Systemd service installed and started (onusly-desktop.service).")
		}
	} else {
		fmt.Printf("ℹ️  User-mode install: Start the agent manually or with a user service: %s run\n", agentBin)
	}

	// 4. Verify applied policies
	issues, err := mgr.VerifyPolicies(browsers)
	if err != nil || len(issues) > 0 {
		fmt.Printf("⚠️  Verification notice: %d configuration issue(s) detected.\n", len(issues))
		for _, iss := range issues {
			fmt.Printf("   • [%s] %s: %s\n", iss.Browser, iss.Issue, iss.Details)
		}
	} else {
		fmt.Println("✅ Verification passed: all detected browsers configured successfully.")
	}

	fmt.Println("\n🎉 ONUSLY Desktop installation complete!")
	fmt.Println("Next step: Pair with your ONUSLY account by running:")
	fmt.Printf("  %s pair\n", agentBin)
}

func runUninstall(mgr *policy.LinuxPolicyManager, browsers []core.BrowserInfo, isRoot bool) {
	fmt.Println("=================================================================")
	fmt.Println("             ONUSLY DESKTOP — UNINSTALLER                        ")
	fmt.Println("=================================================================")
	fmt.Println("Stopping service and restoring browser configurations...")

	if isRoot {
		_ = exec.Command("systemctl", "stop", "onusly-desktop").Run()
		_ = exec.Command("systemctl", "disable", "onusly-desktop").Run()
		_ = os.Remove("/etc/systemd/system/onusly-desktop.service")
		_ = exec.Command("systemctl", "daemon-reload").Run()
		fmt.Println("✅ Systemd service removed.")
	}

	if err := mgr.RemovePolicies(browsers); err != nil {
		fmt.Printf("Notice during policy removal: %v\n", err)
	} else {
		fmt.Println("✅ ONUSLY browser policies and native messaging manifests removed.")
	}

	fmt.Println("✅ Uninstallation complete. Normal browsing behavior restored.")
}

func ensureSystemGroupAndACL() error {
	// 1. Ensure 'onusly' system group exists
	if _, err := user.LookupGroup("onusly"); err != nil {
		fmt.Println("Creating dedicated system group 'onusly'...")
		cmd := exec.Command("groupadd", "-r", "onusly")
		if out, err := cmd.CombinedOutput(); err != nil {
			// Check if already created concurrently
			if _, chkErr := user.LookupGroup("onusly"); chkErr != nil {
				return fmt.Errorf("failed to create 'onusly' system group: %s (%w)", string(out), err)
			}
		}
		fmt.Println("✅ System group 'onusly' created.")
	} else {
		fmt.Println("✅ Dedicated system group 'onusly' already exists.")
	}

	grp, err := user.LookupGroup("onusly")
	if err != nil {
		return fmt.Errorf("failed to lookup 'onusly' group after creation: %w", err)
	}
	gid, err := strconv.Atoi(grp.Gid)
	if err != nil {
		return fmt.Errorf("invalid GID for 'onusly' group: %w", err)
	}

	// 2. Prepare /run/onusly socket directory (mode 0750, root:onusly)
	sockDir := "/run/onusly"
	if err := os.MkdirAll(sockDir, 0750); err != nil {
		return fmt.Errorf("failed to create socket directory %s: %w", sockDir, err)
	}
	if err := os.Chown(sockDir, 0, gid); err != nil {
		return fmt.Errorf("failed to chown %s to root:onusly: %w", sockDir, err)
	}
	if err := os.Chmod(sockDir, 0750); err != nil {
		return fmt.Errorf("failed to chmod %s to 0750: %w", sockDir, err)
	}

	// 3. Detect and grant access to the paired desktop user
	pairedUser := os.Getenv("SUDO_USER")
	if pairedUser == "" {
		if out, err := exec.Command("logname").Output(); err == nil {
			pairedUser = strings.TrimSpace(string(out))
		}
	}

	if pairedUser != "" && pairedUser != "root" {
		// Add user to 'onusly' group
		if err := exec.Command("usermod", "-aG", "onusly", pairedUser).Run(); err != nil {
			return fmt.Errorf("failed to add user %s to 'onusly' group: %w", pairedUser, err)
		}
		fmt.Printf("✅ Added paired desktop user %q to 'onusly' group.\n", pairedUser)

		// Grant immediate ACL on /run/onusly so current desktop session can connect without re-login
		if setfaclPath, err := exec.LookPath("setfacl"); err == nil {
			_ = exec.Command(setfaclPath, "-m", fmt.Sprintf("u:%s:r-x", pairedUser), sockDir).Run()
			_ = exec.Command(setfaclPath, "-d", "-m", fmt.Sprintf("u:%s:rw", pairedUser), sockDir).Run()
			_ = exec.Command(setfaclPath, "-d", "-m", "g:onusly:rw", sockDir).Run()
			fmt.Printf("✅ Configured immediate socket directory ACLs for user %q.\n", pairedUser)
		}
	}
	return nil
}

func copyFileIfExists(src, dst string, mode os.FileMode) error {
	execDir, _ := os.Executable()
	execDir = filepath.Dir(execDir)

	candidates := []string{
		src,
		filepath.Join("desktop", src),
		filepath.Join(execDir, src),
	}

	var in *os.File
	var err error
	for _, c := range candidates {
		in, err = os.Open(c)
		if err == nil {
			break
		}
	}
	if in == nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
