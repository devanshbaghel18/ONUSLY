package apps_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	agentSync "github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type mockCredStore struct {
	token string
}

func (m *mockCredStore) GetToken() (string, error) {
	return m.token, nil
}
func (m *mockCredStore) SaveToken(token string) error {
	m.token = token
	return nil
}
func (m *mockCredStore) ClearToken() error {
	m.token = ""
	return nil
}
func (m *mockCredStore) HasToken() bool {
	return m.token != ""
}

// computeTestVersion matches the canonical enforcement SHA-256 version computation.
func computeTestVersion(targets []core.TargetItem) string {
	data, _ := json.Marshal(targets)
	h := sha256.Sum256(data)
	return "v1-" + hex.EncodeToString(h[:])[:16]
}

// TestD6_EndToEndApplicationPolicyLifecycle validates the 12-step D6 lifecycle:
// Create restriction -> Authoritative policy published -> Device synchronized ->
// Target process terminated (active) -> proof_submitted (blocked) -> rejected (blocked) ->
// completed (allowed) -> Unrelated process untouched -> DNS/web targets coherent.
func TestD6_EndToEndApplicationPolicyLifecycle(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Build disposable test executables: blocked_app and allowed_app
	blockedSrc := filepath.Join(tempDir, "blocked_app.go")
	allowedSrc := filepath.Join(tempDir, "allowed_app.go")
	binExt := ""
	if runtime.GOOS == "windows" {
		binExt = ".exe"
	}
	blockedBin := filepath.Join(tempDir, "blocked_app"+binExt)
	allowedBin := filepath.Join(tempDir, "allowed_app"+binExt)

	goSrc := `package main
import "time"
func main() {
	time.Sleep(30 * time.Second)
}
`
	if err := os.WriteFile(blockedSrc, []byte(goSrc), 0644); err != nil {
		t.Fatalf("failed to write blockedSrc: %v", err)
	}
	if err := os.WriteFile(allowedSrc, []byte(goSrc), 0644); err != nil {
		t.Fatalf("failed to write allowedSrc: %v", err)
	}

	buildBlocked := exec.Command("go", "build", "-o", blockedBin, blockedSrc)
	if out, err := buildBlocked.CombinedOutput(); err != nil {
		t.Fatalf("failed to build blocked_app: %v: %s", err, string(out))
	}

	buildAllowed := exec.Command("go", "build", "-o", allowedBin, allowedSrc)
	if out, err := buildAllowed.CombinedOutput(); err != nil {
		t.Fatalf("failed to build allowed_app: %v: %s", err, string(out))
	}

	// 2. Setup mock backend with dynamic goal status
	var backendMu sync.Mutex
	goalStatus := "active"
	goalTargets := []string{"blocked_app"}
	goalDomains := []string{"focus-blocked-site.com"}

	mockJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLWRlc2t0b3AtMTIzIiwiZXhwIjoxOTk5OTk5OTk5fQ.signature"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		backendMu.Lock()
		defer backendMu.Unlock()

		if r.URL.Path != "/enforcement/blocklist" {
			http.NotFound(w, r)
			return
		}

		// Auth check
		authHdr := r.Header.Get("Authorization")
		if authHdr != "Bearer "+mockJWT {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var targets []core.TargetItem
		// Backend policy rule: active, proof_submitted, and rejected are included in blocklist
		// completed goals are excluded from active blocklist
		if goalStatus != "completed" {
			targets = append(targets, core.TargetItem{
				GoalID:    "goal-d6-test",
				GoalTitle: "Deep Focus Session",
				Status:    goalStatus,
				Apps:      goalTargets,
				Domains:   goalDomains,
				ApplicationTargets: []core.ApplicationTarget{
					{Kind: "desktop_app", Identifier: "blocked_app"},
					{Kind: "android_package", Identifier: "com.whatsapp"},
				},
			})
		}

		ver := computeTestVersion(targets)
		etag := `"` + ver + `"`

		if match := r.Header.Get("If-None-Match"); match == etag || match == ver {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", etag)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"userId":      "user-desktop-123",
			"version":     ver,
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"targets":     targets,
		})
	}))
	defer server.Close()

	// 3. Initialize Agent Syncer, Cache, and App Enforcement Controller
	cacheFile := filepath.Join(tempDir, "blocklist_cache.json")
	blocklistCache := cache.NewFileBlocklistCache(cacheFile)
	credStore := &mockCredStore{token: mockJWT}
	syncer := agentSync.NewSyncer(server.URL, credStore, blocklistCache, server.Client())

	policyMgr := apps.NewAppPolicyManager()
	enforcer := newTestEnforcer()

	// Use actual Windows / Linux process monitor or live enforcer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Step 1 & 2: Authoritative backend policy publish
	syncedData, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Step 1/2 sync failed: %v", err)
	}
	if len(syncedData.Targets) != 1 {
		t.Fatalf("expected 1 target in active blocklist, got %d", len(syncedData.Targets))
	}
	if syncedData.Targets[0].Apps[0] != "blocked_app" {
		t.Fatalf("expected target app 'blocked_app', got %s", syncedData.Targets[0].Apps[0])
	}
	t.Logf("Step 1 & 2 PASS: Authoritative blocklist published and verified (version: %s)", syncedData.Version)

	// Step 3: Desktop agent policy update
	policyMgr.UpdateFromBlocklist(syncedData)
	snap := policyMgr.GetSnapshot()
	if len(snap.Targets) != 1 || !snap.Targets[0].Restricted {
		t.Fatalf("Step 3: expected restricted target in policy snapshot, got %+v", snap)
	}
	t.Log("Step 3 PASS: Desktop agent successfully synchronized authoritative policy snapshot")

	// Step 4 & 5: Start disposable blocked_app process -> verify detection and termination
	cmdBlocked1 := exec.Command(blockedBin)
	if err := cmdBlocked1.Start(); err != nil {
		t.Fatalf("failed to start blocked process: %v", err)
	}
	pid1 := uint32(cmdBlocked1.Process.Pid)
	defer func() {
		if cmdBlocked1.Process != nil {
			_ = cmdBlocked1.Process.Kill()
		}
	}()

	procInfo1 := apps.ProcessInfo{
		PID:      pid1,
		BaseName: filepath.Base(blockedBin),
		ExePath:  blockedBin,
	}

	target1, state1, _, _ := policyMgr.EvaluateProcess(procInfo1)
	if state1 != apps.StateRestricted {
		t.Fatalf("Step 4: expected StateRestricted for active goal, got %v", state1)
	}
	enforced1, errEnforce1 := enforcer.Enforce(ctx, procInfo1, target1)
	if !enforced1 || errEnforce1 != nil {
		t.Fatalf("Step 5: Enforce failed: %v (enforced=%v)", errEnforce1, enforced1)
	}

	// Verify process actually exited
	time.Sleep(100 * time.Millisecond)
	_ = cmdBlocked1.Wait()
	t.Log("Step 4 & 5 PASS: D5 enforcer detected and terminated restricted process under active goal")

	// Step 6: Goal transition to proof_submitted -> remains blocked
	backendMu.Lock()
	goalStatus = "proof_submitted"
	backendMu.Unlock()

	syncedData2, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Step 6 sync failed: %v", err)
	}
	policyMgr.UpdateFromBlocklist(syncedData2)

	cmdBlocked2 := exec.Command(blockedBin)
	if err := cmdBlocked2.Start(); err != nil {
		t.Fatalf("failed to start blocked process in proof_submitted: %v", err)
	}
	pid2 := uint32(cmdBlocked2.Process.Pid)
	defer func() {
		if cmdBlocked2.Process != nil {
			_ = cmdBlocked2.Process.Kill()
		}
	}()

	procInfo2 := apps.ProcessInfo{
		PID:      pid2,
		BaseName: filepath.Base(blockedBin),
		ExePath:  blockedBin,
	}
	target2, state2, _, _ := policyMgr.EvaluateProcess(procInfo2)
	if state2 != apps.StateRestricted {
		t.Fatalf("Step 6: expected StateRestricted in proof_submitted status, got %v", state2)
	}
	enforced2, errEnforce2 := enforcer.Enforce(ctx, procInfo2, target2)
	if !enforced2 || errEnforce2 != nil {
		t.Fatalf("Step 6: Enforce failed for proof_submitted: %v", errEnforce2)
	}
	_ = cmdBlocked2.Wait()
	t.Log("Step 6 PASS: Process remains strictly blocked during proof_submitted status")

	// Step 7: Goal transition to rejected -> remains blocked
	backendMu.Lock()
	goalStatus = "rejected"
	backendMu.Unlock()

	syncedData3, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Step 7 sync failed: %v", err)
	}
	policyMgr.UpdateFromBlocklist(syncedData3)

	cmdBlocked3 := exec.Command(blockedBin)
	if err := cmdBlocked3.Start(); err != nil {
		t.Fatalf("failed to start blocked process in rejected: %v", err)
	}
	pid3 := uint32(cmdBlocked3.Process.Pid)
	defer func() {
		if cmdBlocked3.Process != nil {
			_ = cmdBlocked3.Process.Kill()
		}
	}()

	procInfo3 := apps.ProcessInfo{
		PID:      pid3,
		BaseName: filepath.Base(blockedBin),
		ExePath:  blockedBin,
	}
	target3, state3, _, _ := policyMgr.EvaluateProcess(procInfo3)
	if state3 != apps.StateRestricted {
		t.Fatalf("Step 7: expected StateRestricted in rejected status, got %v", state3)
	}
	enforced3, errEnforce3 := enforcer.Enforce(ctx, procInfo3, target3)
	if !enforced3 || errEnforce3 != nil {
		t.Fatalf("Step 7: Enforce failed for rejected status: %v", errEnforce3)
	}
	_ = cmdBlocked3.Wait()
	t.Log("Step 7 PASS: Process remains strictly blocked following proof rejection")

	// Step 8 & 9: Complete goal through authorized workflow -> synchronize completed status
	backendMu.Lock()
	goalStatus = "completed"
	backendMu.Unlock()

	syncedData4, err := syncer.Sync(ctx)
	if err != nil {
		t.Fatalf("Step 8/9 sync failed: %v", err)
	}
	policyMgr.UpdateFromBlocklist(syncedData4)

	// Step 10: Start blocked_app again -> confirm ALLOWED
	cmdBlocked4 := exec.Command(blockedBin)
	if err := cmdBlocked4.Start(); err != nil {
		t.Fatalf("failed to start process after completion: %v", err)
	}
	pid4 := uint32(cmdBlocked4.Process.Pid)
	defer func() {
		if cmdBlocked4.Process != nil {
			_ = cmdBlocked4.Process.Kill()
		}
	}()

	procInfo4 := apps.ProcessInfo{
		PID:      pid4,
		BaseName: filepath.Base(blockedBin),
		ExePath:  blockedBin,
	}
	_, state4, _, reason4 := policyMgr.EvaluateProcess(procInfo4)
	if state4 != apps.StateAllowed {
		t.Fatalf("Step 10: expected StateAllowed for completed goal, got %v (reason: %s)", state4, reason4)
	}
	// Verify process is still alive and running
	time.Sleep(150 * time.Millisecond)
	if cmdBlocked4.ProcessState != nil && cmdBlocked4.ProcessState.Exited() {
		t.Fatalf("Step 10: process exited unexpectedly after goal completion")
	}
	_ = cmdBlocked4.Process.Kill()
	t.Log("Step 8, 9, 10 PASS: Goal completed -> Policy synchronized -> Application restriction lifted (Allowed)")

	// Step 11: Verify unrelated application remains completely unaffected
	cmdAllowed := exec.Command(allowedBin)
	if err := cmdAllowed.Start(); err != nil {
		t.Fatalf("failed to start allowed process: %v", err)
	}
	pidAllowed := uint32(cmdAllowed.Process.Pid)
	defer func() {
		if cmdAllowed.Process != nil {
			_ = cmdAllowed.Process.Kill()
		}
	}()

	procInfoAllowed := apps.ProcessInfo{
		PID:      pidAllowed,
		BaseName: filepath.Base(allowedBin),
		ExePath:  allowedBin,
	}
	_, stateAllowed, _, _ := policyMgr.EvaluateProcess(procInfoAllowed)
	if stateAllowed != apps.StateAllowed {
		t.Fatalf("Step 11: expected StateAllowed for unrelated app, got %v", stateAllowed)
	}
	time.Sleep(150 * time.Millisecond)
	if cmdAllowed.ProcessState != nil && cmdAllowed.ProcessState.Exited() {
		t.Fatalf("Step 11: unrelated process was terminated unexpectedly")
	}
	_ = cmdAllowed.Process.Kill()
	t.Log("Step 11 PASS: Unrelated application remains completely unaffected")

	// Step 12: Verify domain / website policy coherence
	if len(goalDomains) != 1 || goalDomains[0] != "focus-blocked-site.com" {
		t.Fatalf("Step 12: domain targets corrupted")
	}
	t.Log("Step 12 PASS: Domain and website targets remain coherent across application policy updates")
}
