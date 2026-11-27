//go:build windows

package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed assets/*
var assetsFS embed.FS

const (
	pipePath         = `\\.\pipe\OnuslyAgentIPC`
	fallbackPipePath = `\\.\pipe\OnuslyDesktopPipe`
	extensionID      = "jifigaejbinmdhnljobkodcgeaplojmg"
	firefoxExtID     = "extension@onusly.com"
)

var (
	apiURL           = "https://onusly.duckdns.org"
	dashboardBaseURL = "https://onusly.duckdns.org"
)

type ipcRequest struct {
	Action string `json:"action"`
	Code   string `json:"code,omitempty"`
}

type ipcResponse struct {
	Success   bool   `json:"success"`
	PairCode  string `json:"pairCode,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

func queryIPC(action string, timeout time.Duration) (*ipcResponse, error) {
	return queryIPCWithCode(action, "", timeout)
}

func queryIPCWithCode(action, code string, timeout time.Duration) (*ipcResponse, error) {
	candidates := []string{pipePath, fallbackPipePath}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, p := range candidates {
			conn, err := dialNamedPipe(p, 400*time.Millisecond)
			if err == nil {
				timer := time.AfterFunc(timeout, func() { _ = conn.Close() })
				req := ipcRequest{Action: action, Code: code}
				data, _ := json.Marshal(req)
				if _, wErr := conn.Write(append(data, '\n')); wErr != nil {
					timer.Stop()
					_ = conn.Close()
					return nil, wErr
				}

				var resp ipcResponse
				decErr := json.NewDecoder(conn).Decode(&resp)
				timer.Stop()
				_ = conn.Close()
				if decErr != nil {
					return nil, decErr
				}
				return &resp, nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil, fmt.Errorf("timeout waiting for desktop agent IPC")
}

func dialNamedPipe(path string, timeout time.Duration) (net.Conn, error) {
	pathUTF16, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		h, err := windows.CreateFile(
			&pathUTF16[0],
			windows.GENERIC_READ|windows.GENERIC_WRITE,
			0,
			nil,
			windows.OPEN_EXISTING,
			uint32(win.SECURITY_SQOS_PRESENT|win.SECURITY_IDENTIFICATION),
			0,
		)
		if err == nil {
			f := os.NewFile(uintptr(h), path)
			return &pipeConn{File: f}, nil
		}
		if err == windows.ERROR_PIPE_BUSY {
			kernel32 := windows.NewLazySystemDLL("kernel32.dll")
			proc := kernel32.NewProc("WaitNamedPipeW")
			proc.Call(uintptr(unsafe.Pointer(&pathUTF16[0])), 200)
			continue
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, fmt.Errorf("pipe %s not available", path)
}

type pipeConn struct {
	*os.File
}

func (c *pipeConn) LocalAddr() net.Addr                { return &pipeAddr{c.Name()} }
func (c *pipeConn) RemoteAddr() net.Addr               { return &pipeAddr{c.Name()} }
func (c *pipeConn) SetDeadline(t time.Time) error      { return nil }
func (c *pipeConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *pipeConn) SetWriteDeadline(t time.Time) error { return nil }

type pipeAddr struct{ name string }

func (a *pipeAddr) Network() string { return "pipe" }
func (a *pipeAddr) String() string  { return a.name }

func showMessageBox(title, msg string, icon uint32) {
	user32 := windows.NewLazySystemDLL("user32.dll")
	proc := user32.NewProc("MessageBoxW")
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(msg)
	proc.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), uintptr(icon))
}

func openBrowser(targetURL string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
}

func extractAsset(assetName, destPath string) error {
	in, err := assetsFS.Open("assets/" + assetName)
	if err != nil {
		// Fallback: check if binary exists in sibling search paths
		altPaths := []string{
			assetName,
			filepath.Join("bin", assetName),
			filepath.Join("assets", assetName),
			filepath.Join("..", "..", "..", "bin", assetName),
			filepath.Join(filepath.Dir(destPath), assetName),
		}
		for _, alt := range altPaths {
			if data, readErr := os.ReadFile(alt); readErr == nil && len(data) > 0 {
				_ = os.Remove(destPath)
				return os.WriteFile(destPath, data, 0755)
			}
		}
		return fmt.Errorf("open embedded asset %s: %w", assetName, err)
	}
	defer in.Close()

	_ = os.Remove(destPath) // Remove old binary if exists
	out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("create destination %s: %w", destPath, err)
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func registerNativeMessagingManifest(manifestPath, nativeHostBinPath string) error {
	type manifest struct {
		Name           string   `json:"name"`
		Description    string   `json:"description"`
		Path           string   `json:"path"`
		Type           string   `json:"type"`
		AllowedOrigins []string `json:"allowed_origins"`
	}

	m := manifest{
		Name:        "com.onusly.desktop_agent",
		Description: "ONUSLY Desktop Protection Native Host",
		Path:        nativeHostBinPath,
		Type:        "stdio",
		AllowedOrigins: []string{
			fmt.Sprintf("chrome-extension://%s/", extensionID),
			"chrome-extension://gkichkblphmdbojdjnonljlegnmjcnhg/",
			"chrome-extension://admccjkmockfdflocgggjfgdacdodkdf/",
		},
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return err
	}

	// Register in HKCU for Chrome, Edge, Brave, and Chromium
	hives := []string{
		`Software\Google\Chrome\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Microsoft\Edge\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\BraveSoftware\Brave\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Chromium\NativeMessagingHosts\com.onusly.desktop_agent`,
	}

	for _, hive := range hives {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, hive, registry.SET_VALUE)
		if err == nil {
			_ = k.SetStringValue("", manifestPath)
			k.Close()
		}
	}

	// Best-effort disable Browser DoH when installer runs elevated so local DNS blocking works across all browsers
	for _, polKey := range []string{
		`SOFTWARE\Policies\Google\Chrome`,
		`SOFTWARE\Policies\Microsoft\Edge`,
		`SOFTWARE\Policies\BraveSoftware\Brave`,
		`SOFTWARE\Policies\Chromium`,
	} {
		if pk, _, err := registry.CreateKey(registry.LOCAL_MACHINE, polKey, registry.SET_VALUE); err == nil {
			_ = pk.SetStringValue("DnsOverHttpsMode", "off")
			pk.Close()
		}
	}
	if fpk, _, err := registry.CreateKey(registry.LOCAL_MACHINE, `SOFTWARE\Policies\Mozilla\Firefox\DNSOverHTTPS`, registry.SET_VALUE); err == nil {
		_ = fpk.SetDWordValue("Enabled", 0)
		fpk.Close()
	}

	// Also register Mozilla/Firefox native messaging manifest (uses allowed_extensions)
	type firefoxManifest struct {
		Name              string   `json:"name"`
		Description       string   `json:"description"`
		Path              string   `json:"path"`
		Type              string   `json:"type"`
		AllowedExtensions []string `json:"allowed_extensions"`
	}
	ffManifestPath := filepath.Join(filepath.Dir(manifestPath), "com.onusly.desktop_agent.firefox.json")
	ffData, err := json.MarshalIndent(firefoxManifest{
		Name:              "com.onusly.desktop_agent",
		Description:       "ONUSLY Desktop Protection Native Host",
		Path:              nativeHostBinPath,
		Type:              "stdio",
		AllowedExtensions: []string{firefoxExtID},
	}, "", "  ")
	if err == nil && os.WriteFile(ffManifestPath, ffData, 0644) == nil {
		k, _, err := registry.CreateKey(
			registry.CURRENT_USER,
			`Software\Mozilla\NativeMessagingHosts\com.onusly.desktop_agent`,
			registry.SET_VALUE,
		)
		if err == nil {
			_ = k.SetStringValue("", ffManifestPath)
			k.Close()
		}
	}

	return nil
}

func registerAutoStart(binPath, configDir, dataDir string) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()

	// Write a silent VBS launcher so Windows boot runs it completely without a console window
	vbsPath := filepath.Join(filepath.Dir(binPath), "onusly-agent-silent.vbs")
	vbsContent := fmt.Sprintf(
		"Set WshShell = CreateObject(\"WScript.Shell\")\r\n"+
			"WshShell.Run \"\"\"%s\"\" run -config-dir \"\"%s\"\" -data-dir \"\"%s\"\" -socket \"\"%s\"\" -api-url \"\"%s\"\"\", 0, False\r\n",
		binPath, configDir, dataDir, pipePath, apiURL,
	)
	_ = os.WriteFile(vbsPath, []byte(vbsContent), 0644)

	cmd := fmt.Sprintf(`wscript.exe "%s"`, vbsPath)
	return k.SetStringValue("OnuslyDesktopAgent", cmd)
}

func stopRunningProcesses() {
	_ = exec.Command("taskkill", "/F", "/IM", "onusly-agent.exe", "/T").Run()
	_ = exec.Command("taskkill", "/F", "/IM", "onusly-native-host.exe", "/T").Run()
	time.Sleep(500 * time.Millisecond)
}

func extractAssetWithRetry(name, dest string) error {
	var err error
	for i := 0; i < 4; i++ {
		err = extractAsset(name, dest)
		if err == nil {
			return nil
		}
		stopRunningProcesses()
		time.Sleep(300 * time.Millisecond)
	}
	return err
}

func startDetachedDaemon(binPath, configDir, dataDir string) error {
	vbsPath := filepath.Join(filepath.Dir(binPath), "onusly-agent-silent.vbs")
	if fi, err := os.Stat(vbsPath); err == nil && !fi.IsDir() {
		cmd := exec.Command("wscript.exe", vbsPath)
		return cmd.Start()
	}

	cmd := exec.Command(binPath, "run",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
		"-api-url", apiURL,
	)

	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.DETACHED_PROCESS,
	}

	return cmd.Start()
}

func registerUninstallerEntry(installerExePath, installDir string) error {
	k, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\Onusly`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer k.Close()

	uninstallExe := filepath.Join(installDir, "OnuslySetup.exe")
	if installerExePath != "" {
		if data, readErr := os.ReadFile(installerExePath); readErr == nil && len(data) > 0 {
			_ = os.WriteFile(uninstallExe, data, 0755)
		}
	}
	if _, err := os.Stat(uninstallExe); err != nil {
		uninstallExe = installerExePath
	}
	if uninstallExe == "" {
		return nil
	}

	_ = k.SetStringValue("DisplayName", "ONUSLY Desktop Protection")
	_ = k.SetStringValue("Publisher", "ONUSLY")
	_ = k.SetStringValue("InstallLocation", installDir)
	_ = k.SetStringValue("UninstallString", fmt.Sprintf(`"%s" -uninstall`, uninstallExe))
	_ = k.SetDWordValue("NoModify", 1)
	_ = k.SetDWordValue("NoRepair", 1)
	return nil
}

func uninstallApplication() {
	stopRunningProcesses()
	_ = exec.Command("sc.exe", "stop", "OnuslyDesktopAgent").Run()
	_ = exec.Command("sc.exe", "delete", "OnuslyDesktopAgent").Run()

	if k, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Run`, registry.SET_VALUE); err == nil {
		_ = k.DeleteValue("OnuslyDesktopAgent")
		k.Close()
	}
	for _, subkey := range []string{
		`Software\Google\Chrome\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Microsoft\Edge\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\BraveSoftware\Brave-Browser\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\BraveSoftware\Brave\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Chromium\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Mozilla\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Microsoft\Windows\CurrentVersion\Uninstall\Onusly`,
	} {
		_ = registry.DeleteKey(registry.CURRENT_USER, subkey)
	}

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}
	baseDir := filepath.Join(localAppData, "Onusly")
	_ = os.RemoveAll(filepath.Join(baseDir, "bin"))
	_ = os.RemoveAll(filepath.Join(baseDir, "config"))
	_ = os.RemoveAll(filepath.Join(baseDir, "data"))

	showMessageBox("ONUSLY Uninstaller", "ONUSLY Desktop Protection components and startup entries have been removed.", 0x40)
}

func main() {
	apiFlag := flag.String("api-url", "", "Override ONUSLY API URL")
	dashFlag := flag.String("dashboard-url", "", "Override ONUSLY Dashboard URL")
	uninstallFlag := flag.Bool("uninstall", false, "Uninstall ONUSLY Desktop Protection")
	flag.Parse()

	if *uninstallFlag {
		uninstallApplication()
		return
	}

	if envAPI := strings.TrimSpace(os.Getenv("ONUSLY_API_URL")); envAPI != "" {
		apiURL = envAPI
	}
	if strings.TrimSpace(*apiFlag) != "" {
		apiURL = strings.TrimSpace(*apiFlag)
	}
	apiURL = strings.TrimRight(apiURL, "/")

	if envDash := strings.TrimSpace(os.Getenv("ONUSLY_DASHBOARD_URL")); envDash != "" {
		dashboardBaseURL = envDash
	}
	if strings.TrimSpace(*dashFlag) != "" {
		dashboardBaseURL = strings.TrimSpace(*dashFlag)
	}
	dashboardBaseURL = strings.TrimRight(dashboardBaseURL, "/")

	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}

	baseDir := filepath.Join(localAppData, "Onusly")
	binDir := filepath.Join(baseDir, "bin")
	configDir := filepath.Join(baseDir, "config")
	dataDir := filepath.Join(baseDir, "data")

	_ = os.MkdirAll(binDir, 0755)
	_ = os.MkdirAll(configDir, 0755)
	_ = os.MkdirAll(dataDir, 0755)

	agentBin := filepath.Join(binDir, "onusly-agent.exe")
	hostBin := filepath.Join(binDir, "onusly-native-host.exe")
	manifestPath := filepath.Join(configDir, "com.onusly.desktop_agent.json")

	// 1. Terminate any currently running daemon instances so binaries can be overwritten cleanly
	stopRunningProcesses()

	// 2. Extract embedded binaries with retry
	if err := extractAssetWithRetry("onusly-agent.exe", agentBin); err != nil {
		showMessageBox("ONUSLY Setup Error", "Failed to extract core desktop agent: "+err.Error(), 0x10)
		os.Exit(1)
	}
	if err := extractAssetWithRetry("onusly-native-host.exe", hostBin); err != nil {
		showMessageBox("ONUSLY Setup Error", "Failed to extract native messaging host: "+err.Error(), 0x10)
		os.Exit(1)
	}

	// 3. Register Native Messaging Host
	_ = registerNativeMessagingManifest(manifestPath, hostBin)

	// 4. Register auto-start on boot and Add/Remove Programs uninstaller entry
	_ = registerAutoStart(agentBin, configDir, dataDir)
	exeSelf, _ := os.Executable()
	_ = registerUninstallerEntry(exeSelf, baseDir)

	// 5. Start daemon in background if not already responding
	pingResp, pingErr := queryIPC("ping", 1*time.Second)
	if pingErr != nil || pingResp == nil || !pingResp.Success {
		if err := startDetachedDaemon(agentBin, configDir, dataDir); err != nil {
			showMessageBox("ONUSLY Setup Error", "Failed to start background agent: "+err.Error(), 0x10)
			os.Exit(1)
		}
	}

	// 6. Query pairing code
	pairResp, pairErr := queryIPC("pair-request", 5*time.Second)
	if pairErr != nil || pairResp == nil || !pairResp.Success {
		// Open dashboard anyway so user can see their connected devices
		openBrowser(dashboardBaseURL + "/dashboard")
		showMessageBox("ONUSLY Desktop Protection Installed",
			"ONUSLY Desktop Protection has been successfully installed and is running in the background!\n\nOpen your dashboard to link your device.",
			0x40, // MB_ICONINFORMATION
		)
		return
	}

	code := pairResp.PairCode
	dashboardURL := fmt.Sprintf("%s/dashboard?pairCode=%s", dashboardBaseURL, code)

	// 7. Automatically open browser with pairing code pre-filled and poll until paired
	openBrowser(dashboardURL)

	pollResp, pollErr := queryIPCWithCode("pair-poll", code, 3*time.Minute)
	if pollErr == nil && pollResp != nil && pollResp.Success {
		showMessageBox("ONUSLY Desktop Protection Setup",
			"✅ ONUSLY Desktop Protection is now ACTIVE, PAIRED, and protecting this PC!\n\nYou can now close this window.",
			0x40,
		)
		return
	}

	// Fallback confirmation with pairing code if user hasn't finished browser sign-in yet
	msg := fmt.Sprintf(
		"ONUSLY Desktop Protection is installed and running.\n\n"+
			"🔑 Your 6-Character Pairing Code:\n\n         [ %s ]\n\n"+
			"Complete pairing in your ONUSLY dashboard (%s/dashboard) using this code.\n\n"+
			"You can now close this window.",
		code, dashboardBaseURL,
	)
	showMessageBox("ONUSLY Desktop Protection Setup", msg, 0x40)
}
