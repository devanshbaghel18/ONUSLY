//go:build windows

package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed assets/*
var assetsFS embed.FS

const (
	pipePath    = `\\.\pipe\OnuslyDesktopPipe`
	apiURL      = "http://localhost:8080"
	extensionID = "jifigaejbinmdhnljobkodcgeaplojmg"
)

type ipcRequest struct {
	Action string `json:"action"`
}

type ipcResponse struct {
	Success   bool   `json:"success"`
	PairCode  string `json:"pairCode,omitempty"`
	ExpiresAt int64  `json:"expiresAt,omitempty"`
	Error     string `json:"error,omitempty"`
}

func queryIPC(action string, timeout time.Duration) (*ipcResponse, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", pipePath)
		if err != nil {
			// On Windows named pipes, dial via standard Windows named pipe path
			conn, err = dialNamedPipe(pipePath, 500*time.Millisecond)
		}
		if err == nil {
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(timeout))

			req := ipcRequest{Action: action}
			data, _ := json.Marshal(req)
			if _, err := conn.Write(append(data, '\n')); err != nil {
				return nil, err
			}

			var resp ipcResponse
			if err := json.NewDecoder(conn).Decode(&resp); err != nil {
				return nil, err
			}
			return &resp, nil
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
			0,
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
		Name:           "com.onusly.desktop_agent",
		Description:    "ONUSLY Desktop Protection Native Host",
		Path:           nativeHostBinPath,
		Type:           "stdio",
		AllowedOrigins: []string{fmt.Sprintf("chrome-extension://%s/", extensionID)},
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return err
	}

	// Register in HKCU for Chrome and Edge
	hives := []string{
		`Software\Google\Chrome\NativeMessagingHosts\com.onusly.desktop_agent`,
		`Software\Microsoft\Edge\NativeMessagingHosts\com.onusly.desktop_agent`,
	}

	for _, hive := range hives {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, hive, registry.SET_VALUE)
		if err == nil {
			_ = k.SetStringValue("", manifestPath)
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

	cmd := fmt.Sprintf(`"%s" run -config-dir "%s" -data-dir "%s" -socket "%s" -api-url "%s"`,
		binPath, configDir, dataDir, pipePath, apiURL)

	return k.SetStringValue("OnuslyDesktopAgent", cmd)
}

func startDetachedDaemon(binPath, configDir, dataDir string) error {
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

func main() {
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

	// 1. Extract embedded binaries
	if err := extractAsset("onusly-agent.exe", agentBin); err != nil {
		showMessageBox("ONUSLY Setup Error", "Failed to extract core desktop agent: "+err.Error(), 0x10)
		os.Exit(1)
	}
	if err := extractAsset("onusly-native-host.exe", hostBin); err != nil {
		showMessageBox("ONUSLY Setup Error", "Failed to extract native messaging host: "+err.Error(), 0x10)
		os.Exit(1)
	}

	// 2. Register Native Messaging Host
	_ = registerNativeMessagingManifest(manifestPath, hostBin)

	// 3. Register auto-start on boot
	_ = registerAutoStart(agentBin, configDir, dataDir)

	// 4. Start daemon in background if not already responding
	pingResp, pingErr := queryIPC("ping", 1*time.Second)
	if pingErr != nil || pingResp == nil || !pingResp.Success {
		if err := startDetachedDaemon(agentBin, configDir, dataDir); err != nil {
			showMessageBox("ONUSLY Setup Error", "Failed to start background agent: "+err.Error(), 0x10)
			os.Exit(1)
		}
	}

	// 5. Query pairing code
	pairResp, pairErr := queryIPC("pair-request", 5*time.Second)
	if pairErr != nil || pairResp == nil || !pairResp.Success {
		// Open dashboard anyway so user can see their connected devices
		openBrowser("http://localhost:5173/dashboard")
		showMessageBox("ONUSLY Desktop Protection Installed",
			"ONUSLY Desktop Protection has been successfully installed and is running in the background!\n\nOpen your dashboard to link your device.",
			0x40, // MB_ICONINFORMATION
		)
		return
	}

	code := pairResp.PairCode
	dashboardURL := fmt.Sprintf("http://localhost:5173/dashboard?pairCode=%s", code)

	// 6. Automatically open Chrome/browser with pairing code pre-filled
	openBrowser(dashboardURL)

	// 7. Show sleek confirmation message with the code
	msg := fmt.Sprintf(
		"✅ ONUSLY Desktop Protection is now ACTIVE and protecting this PC!\n\n"+
			"🔑 Your 6-Character Pairing Code:\n\n         [ %s ]\n\n"+
			"Your ONUSLY dashboard has been opened in your browser to complete pairing with one click.\n\n"+
			"You can now close this window.",
		code,
	)
	showMessageBox("ONUSLY Desktop Protection Setup", msg, 0x40)
}
