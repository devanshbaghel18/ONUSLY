package apps_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
)

func TestImmunityProtection(t *testing.T) {
	enforcer := newTestEnforcer()

	target := apps.TargetApp{
		GoalID:     "goal-1",
		AppID:      "explorer",
		Status:     "active",
		Restricted: true,
	}

	// 1. Protected binary: explorer.exe
	procExplorer := apps.ProcessInfo{
		PID:       99990,
		BaseName:  "explorer.exe",
		ExePath:   `C:\Windows\explorer.exe`,
		StartTime: 1000,
	}
	enforced, err := enforcer.Enforce(context.Background(), procExplorer, target)
	if enforced || err == nil {
		t.Fatalf("expected error for protected explorer.exe, got enforced=%v, err=%v", enforced, err)
	}

	// 2. Protected binary: onusly-agent.exe
	procAgent := apps.ProcessInfo{
		PID:       99991,
		BaseName:  "onusly-agent.exe",
		ExePath:   `C:\Program Files\Onusly\onusly-agent.exe`,
		StartTime: 1000,
	}
	enforced, err = enforcer.Enforce(context.Background(), procAgent, target)
	if enforced || err == nil {
		t.Fatalf("expected error for protected onusly-agent.exe, got enforced=%v, err=%v", enforced, err)
	}

	// 3. Protected PID: current test process
	procSelf := apps.ProcessInfo{
		PID:       uint32(os.Getpid()),
		BaseName:  "fixture_test.exe",
		StartTime: 1000,
	}
	enforced, err = enforcer.Enforce(context.Background(), procSelf, target)
	if enforced || err == nil {
		t.Fatalf("expected error for current PID, got enforced=%v, err=%v", enforced, err)
	}

	// 4. Protected PID: system (PID <= 4)
	procSystem := apps.ProcessInfo{
		PID:       4,
		BaseName:  "System",
		StartTime: 1000,
	}
	enforced, err = enforcer.Enforce(context.Background(), procSystem, target)
	if enforced || err == nil {
		t.Fatalf("expected error for PID 4, got enforced=%v, err=%v", enforced, err)
	}
}

func TestLiveProcessEnforcementAndPIDReuse(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Compile a harmless dummy binary that sleeps for 30 seconds
	srcPath := filepath.Join(tempDir, "dummy_target.go")
	binName := "dummy_target"
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	binPath := filepath.Join(tempDir, binName)

	goSrc := `package main
import "time"
func main() {
	time.Sleep(30 * time.Second)
}
`
	if err := os.WriteFile(srcPath, []byte(goSrc), 0644); err != nil {
		t.Fatalf("failed to write dummy source: %v", err)
	}

	cmdBuild := exec.Command("go", "build", "-o", binPath, srcPath)
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("failed to build dummy target: %v: %s", err, string(out))
	}

	// 2. Launch live dummy target process
	cmdDummy := exec.Command(binPath)
	if err := cmdDummy.Start(); err != nil {
		t.Fatalf("failed to start dummy process: %v", err)
	}
	defer func() {
		if cmdDummy.Process != nil {
			_ = cmdDummy.Process.Kill()
		}
	}()

	pid := uint32(cmdDummy.Process.Pid)
	if pid == 0 {
		t.Fatalf("invalid dummy process PID")
	}

	// 3. Inspect process using ProcessMonitor
	monitor := newTestMonitor()
	enforcer := newTestEnforcer()

	procs, err := monitor.Snapshot(context.Background())
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}

	var found *apps.ProcessInfo
	for _, p := range procs {
		if p.PID == pid {
			cp := p
			found = &cp
			break
		}
	}

	if found == nil {
		t.Fatalf("process monitor did not observe dummy process PID %d", pid)
	}

	target := apps.TargetApp{
		GoalID:     "goal-dummy",
		AppID:      "dummy_target",
		Status:     "active",
		Restricted: true,
	}

	// 4. Test PID Reuse Race Protection:
	// If StartTime does not match the actual process creation time, enforcer must reject with error.
	staleProc := *found
	staleProc.StartTime = 12345 // mismatched start time
	enforced, err := enforcer.Enforce(context.Background(), staleProc, target)
	if runtime.GOOS == "windows" {
		if enforced || err == nil {
			t.Fatalf("expected PID reuse mismatch error, got enforced=%v, err=%v", enforced, err)
		}
	}

	// 5. Live Enforcement: Valid proc matching real StartTime
	enforced, err = enforcer.Enforce(context.Background(), *found, target)
	if err != nil {
		t.Fatalf("failed live enforcement against dummy process: %v", err)
	}
	if !enforced {
		t.Fatalf("expected dummy process to be enforced")
	}

	// Wait for process to terminate
	errWait := cmdDummy.Wait()
	if errWait == nil {
		t.Fatalf("expected dummy process to terminate with error/signal, but exited cleanly")
	}
	t.Logf("Dummy process %d successfully terminated by enforcer: %v", pid, errWait)
}
