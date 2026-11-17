//go:build windows

package enforcement

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
	"golang.org/x/sys/windows"
)

// EnsureDesktopAgentRunning checks if the desktop agent daemon is currently responsive.
// If it is unreachable or not running, it locates onusly-agent.exe and starts it
// detached in the background without any console window (CREATE_NO_WINDOW).
func EnsureDesktopAgentRunning() error {
	// 1. Check if already responding over named pipe
	resp, err := queryLocalAgent(core.IPCRequest{Action: "ping"}, 600*time.Millisecond)
	if err == nil && resp != nil && resp.Success {
		return nil // Agent is already running!
	}

	// 2. Discover local onusly-agent.exe
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData == "" {
		localAppData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Local")
	}

	candidates := []string{
		filepath.Join(localAppData, "Onusly", "bin", "onusly-agent.exe"),
		filepath.Join(os.Getenv("ProgramFiles"), "Onusly", "onusly-agent.exe"),
		filepath.Join(".", "desktop", "bin", "onusly-agent.exe"),
		filepath.Join("..", "desktop", "bin", "onusly-agent.exe"),
	}

	var agentExe string
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			agentExe = c
			break
		}
	}

	if agentExe == "" {
		return fmt.Errorf("onusly-agent.exe not found in standard paths")
	}

	baseDir := filepath.Dir(filepath.Dir(agentExe))
	configDir := filepath.Join(baseDir, "config")
	dataDir := filepath.Join(baseDir, "data")
	pipePath := `\\.\pipe\OnuslyDesktopPipe`
	apiURL := "http://localhost:8080"

	cmd := exec.Command(agentExe, "run",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
		"-api-url", apiURL,
	)

	// CREATE_NO_WINDOW (0x08000000) and DETACHED_PROCESS (0x00000008)
	// ensures no console window appears and closing any parent window does not terminate the daemon.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NO_WINDOW | windows.DETACHED_PROCESS,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to launch onusly-agent.exe: %w", err)
	}

	// Wait up to 3.5 seconds for pipe to be ready
	deadline := time.Now().Add(3500 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(300 * time.Millisecond)
		if r, e := queryLocalAgent(core.IPCRequest{Action: "ping"}, 400*time.Millisecond); e == nil && r != nil && r.Success {
			return nil
		}
	}

	return nil
}
