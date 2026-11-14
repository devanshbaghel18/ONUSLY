//go:build windows

package ipc

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type mockBackendStore struct {
	mu           sync.Mutex
	pairCode     string
	pairExpires  int64
	confirmed    bool
	tokenClaimed bool
	heartbeats   []map[string]any
}

func startAcceptanceBackend(t *testing.T, store *mockBackendStore, user, token string) *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /enforcement/pair/request", func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()

		code := "T9K4M2"
		store.pairCode = code
		store.pairExpires = time.Now().Add(10 * time.Minute).Unix()
		store.confirmed = false
		store.tokenClaimed = false

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":      code,
			"expiresAt": store.pairExpires,
		})
	})

	mux.HandleFunc("POST /enforcement/pair/confirm", func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()

		var body struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body.Code != store.pairCode || time.Now().Unix() > store.pairExpires {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid or expired code"})
			return
		}

		store.confirmed = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "confirmed"})
	})

	mux.HandleFunc("POST /enforcement/pair/exchange", func(w http.ResponseWriter, r *http.Request) {
		store.mu.Lock()
		defer store.mu.Unlock()

		var body struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)

		if body.Code != store.pairCode || !store.confirmed || store.tokenClaimed {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "pending or already claimed"})
			return
		}

		store.tokenClaimed = true
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	})

	mux.HandleFunc("GET /enforcement/blocklist", func(w http.ResponseWriter, r *http.Request) {
		authH := r.Header.Get("Authorization")
		if !strings.HasPrefix(authH, "Bearer ") || strings.TrimPrefix(authH, "Bearer ") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Header.Get("If-None-Match") == "v-acc-42" {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v-acc-42"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      user,
			"version":     "v-acc-42",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"targets": []core.TargetItem{
				{
					GoalID:    "g-active-1",
					GoalTitle: "Block Social Media",
					Status:    "active",
					Domains:   []string{"facebook.com", "instagram.com"},
				},
				{
					GoalID:    "g-proof-sub-2",
					GoalTitle: "Block Gaming",
					Status:    "proof_submitted",
					Domains:   []string{"steamcommunity.com"},
				},
				{
					GoalID:    "g-rejected-3",
					GoalTitle: "Block News",
					Status:    "rejected",
					Domains:   []string{"distractingnews.com"},
				},
			},
		})
	})

	mux.HandleFunc("POST /enforcement/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		authH := r.Header.Get("Authorization")
		if !strings.HasPrefix(authH, "Bearer ") || strings.TrimPrefix(authH, "Bearer ") != token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		var hb map[string]any
		_ = json.NewDecoder(r.Body).Decode(&hb)

		store.mu.Lock()
		store.heartbeats = append(store.heartbeats, hb)
		store.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
	})

	return httptest.NewServer(mux)
}

func TestWindows_EndToEndAcceptance(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-acc-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	configDir := filepath.Join(tmpDir, "config")
	dataDir := filepath.Join(tmpDir, "data")
	_ = os.MkdirAll(configDir, 0700)
	_ = os.MkdirAll(dataDir, 0700)

	deviceID := "22222222-3333-4444-5555-666666666666"
	_ = os.WriteFile(filepath.Join(configDir, "device_id"), []byte(deviceID), 0600)

	pipePath := fmt.Sprintf(`\\.\pipe\OnuslyAccPipe_%d`, os.Getpid())
	testUser := "user-desktop-acceptance"
	testToken := makeTestJWT(testUser, 7200)

	store := &mockBackendStore{}
	backend := startAcceptanceBackend(t, store, testUser, testToken)
	defer backend.Close()

	agentExe, _ := filepath.Abs(filepath.Join(".", "..", "..", "..", "build", "bin", "onusly-agent.exe"))
	nativeHostExe, _ := filepath.Abs(filepath.Join(".", "..", "..", "..", "build", "bin", "onusly-native-host.exe"))

	if _, err := os.Stat(agentExe); os.IsNotExist(err) {
		agentExe, _ = filepath.Abs(filepath.Join(".", "..", "..", "..", "..", "build", "bin", "onusly-agent.exe"))
		nativeHostExe, _ = filepath.Abs(filepath.Join(".", "..", "..", "..", "..", "build", "bin", "onusly-native-host.exe"))
	}

	if _, err := os.Stat(agentExe); os.IsNotExist(err) {
		t.Skipf("Agent binary not found at %s. Skipping acceptance test.", agentExe)
	}
	if _, err := os.Stat(nativeHostExe); os.IsNotExist(err) {
		t.Skipf("Native host binary not found at %s. Skipping acceptance test.", nativeHostExe)
	}

	// 1. Launch standalone onusly-agent daemon
	daemonCtx, daemonCancel := context.WithCancel(context.Background())
	defer daemonCancel()

	cmdDaemon := exec.CommandContext(daemonCtx, agentExe,
		"run",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
		"-api-url", backend.URL,
	)
	cmdDaemon.Stdout = os.Stdout
	cmdDaemon.Stderr = os.Stderr

	if err := cmdDaemon.Start(); err != nil {
		t.Fatalf("Failed to start agent daemon: %v", err)
	}
	defer func() {
		daemonCancel()
		_ = cmdDaemon.Wait()
	}()

	// Wait up to 3s for pipe
	pipeReady := false
	for i := 0; i < 30; i++ {
		pResp, err := Query(pipePath, core.IPCRequest{Action: "ping"}, 200*time.Millisecond)
		if err == nil && pResp.Success {
			pipeReady = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !pipeReady {
		t.Fatalf("Agent named pipe %s did not become ready", pipePath)
	}

	// 2. Query initial status
	stResp, err := Query(pipePath, core.IPCRequest{Action: "get-status"}, 2*time.Second)
	if err != nil || !stResp.Success || stResp.Status == nil {
		t.Fatalf("Initial get-status failed: %v", err)
	}
	if stResp.Status.Connected {
		t.Errorf("Expected connected=false initially")
	}
	if stResp.Status.PairingState != string(auth.StatusInstalledUnpaired) && stResp.Status.PairingState != string(auth.StatusUninstalled) {
		t.Errorf("Expected initial state UNPAIRED, got %s", stResp.Status.PairingState)
	}

	// 3. Initiate pairing from CLI
	cmdPair := exec.Command(agentExe,
		"pair",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
		"-api-url", backend.URL,
	)

	var pairStdout, pairStderr bytes.Buffer
	cmdPair.Stdout = &pairStdout
	cmdPair.Stderr = &pairStderr

	// Run pairing in background because it waits for dashboard confirmation
	if err := cmdPair.Start(); err != nil {
		t.Fatalf("Failed to launch onusly-agent pair: %v", err)
	}

	// Wait for code to be generated in store
	codeReady := false
	for i := 0; i < 30; i++ {
		store.mu.Lock()
		c := store.pairCode
		store.mu.Unlock()
		if c != "" {
			codeReady = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !codeReady {
		t.Fatalf("Pairing code was not generated within 3s")
	}

	// 4. Simulate user dashboard confirming the pairing code
	confirmReq, _ := http.NewRequest("POST", backend.URL+"/enforcement/pair/confirm", strings.NewReader(fmt.Sprintf(`{"code":%q}`, store.pairCode)))
	confirmReq.Header.Set("Content-Type", "application/json")
	confirmResp, err := backend.Client().Do(confirmReq)
	if err != nil || confirmResp.StatusCode != http.StatusOK {
		t.Fatalf("Dashboard confirmation failed: %v (code %v)", err, confirmResp)
	}
	confirmResp.Body.Close()

	// Wait for onusly-agent pair to exit successfully
	pairDone := make(chan error, 1)
	go func() {
		pairDone <- cmdPair.Wait()
	}()

	select {
	case err := <-pairDone:
		if err != nil {
			t.Fatalf("onusly-agent pair exited with error: %v (stdout: %s, stderr: %s)", err, pairStdout.String(), pairStderr.String())
		}
	case <-time.After(10 * time.Second):
		_ = cmdPair.Process.Kill()
		t.Fatalf("onusly-agent pair timed out waiting for completion")
	}

	// SECURITY INVARIANT CHECK: Zero raw JWT in pair output
	if strings.Contains(pairStdout.String(), testToken) || strings.Contains(pairStderr.String(), testToken) {
		t.Fatalf("CRITICAL SECURITY VIOLATION: Raw token printed in CLI stdout/stderr!")
	}

	// 5. Query status post-pairing
	postStResp, err := Query(pipePath, core.IPCRequest{Action: "get-status"}, 2*time.Second)
	if err != nil || !postStResp.Success || postStResp.Status == nil {
		t.Fatalf("Post-pairing get-status failed: %v", err)
	}
	st := postStResp.Status
	if !st.Connected {
		t.Errorf("Expected connected=true after pairing")
	}
	if st.PairingState != string(auth.StatusPaired) {
		t.Errorf("Expected pairingState=PAIRED, got %s", st.PairingState)
	}
	if st.UserID != testUser {
		t.Errorf("Expected user %s, got %s", testUser, st.UserID)
	}
	if st.TargetsCount != 3 {
		t.Errorf("Expected 3 targets locked, got %d", st.TargetsCount)
	}
	if !st.Enforcing {
		t.Errorf("Expected enforcing=true")
	}

	// 6. Test native-messaging host over Stdin/Stdout
	cmdHost := exec.Command(nativeHostExe)
	cmdHost.Env = append(os.Environ(), "ONUSLY_IPC_PATH="+pipePath)

	hostStdin, err := cmdHost.StdinPipe()
	if err != nil {
		t.Fatalf("Failed to open host stdin: %v", err)
	}
	hostStdout, err := cmdHost.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to open host stdout: %v", err)
	}

	if err := cmdHost.Start(); err != nil {
		t.Fatalf("Failed to start native host: %v", err)
	}
	defer func() {
		_ = cmdHost.Process.Kill()
	}()

	// Send native framed message: {"action":"get-blocklist"}
	reqMsg := []byte(`{"action":"get-blocklist"}`)
	msgLen := uint32(len(reqMsg))
	_ = binary.Write(hostStdin, binary.LittleEndian, msgLen)
	_, _ = hostStdin.Write(reqMsg)

	// Read native framed response: 4 bytes len + JSON
	var respLen uint32
	if err := binary.Read(hostStdout, binary.LittleEndian, &respLen); err != nil {
		t.Fatalf("Failed to read native response length: %v", err)
	}
	respBuf := make([]byte, respLen)
	_, err = hostStdout.Read(respBuf)
	if err != nil {
		t.Fatalf("Failed to read native response body: %v", err)
	}

	var hostResp core.IPCResponse
	if err := json.Unmarshal(respBuf, &hostResp); err != nil {
		t.Fatalf("Failed to parse native response JSON: %v", err)
	}
	if !hostResp.Success || hostResp.Blocklist == nil {
		t.Fatalf("Expected valid blocklist from native host, got %+v", hostResp)
	}
	if len(hostResp.Blocklist.Targets) != 3 {
		t.Errorf("Expected 3 targets from native host, got %d", len(hostResp.Blocklist.Targets))
	}
	// Zero secrets check in native host output
	if strings.Contains(string(respBuf), testToken) {
		t.Fatalf("CRITICAL SECURITY VIOLATION: raw token leaked to native host!")
	}

	// 7. Test CLI status and diagnostics commands
	cmdStatus := exec.Command(agentExe,
		"status",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
	)
	statusOut, err := cmdStatus.CombinedOutput()
	if err != nil {
		t.Fatalf("onusly-agent status failed: %v", err)
	}
	if !strings.Contains(string(statusOut), "CONNECTED") || !strings.Contains(string(statusOut), testUser) {
		t.Errorf("Unexpected status output: %s", string(statusOut))
	}

	cmdDiag := exec.Command(agentExe,
		"diagnostics",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
	)
	diagOut, err := cmdDiag.CombinedOutput()
	if err != nil {
		t.Fatalf("onusly-agent diagnostics failed: %v", err)
	}
	if !strings.Contains(string(diagOut), "has_credentials: true") {
		t.Errorf("Unexpected diagnostics output: %s", string(diagOut))
	}

	// 8. Test restart resilience: stop daemon and restart
	daemonCancel()
	_ = cmdDaemon.Wait()

	// Verify files on disk before restart
	cacheData, err := os.ReadFile(filepath.Join(dataDir, "blocklist_cache.json"))
	if err != nil || !strings.Contains(string(cacheData), "facebook.com") {
		t.Fatalf("Cache data corrupted or missing on disk: %v", err)
	}

	// Restart daemon
	daemonCtx2, daemonCancel2 := context.WithCancel(context.Background())
	defer daemonCancel2()

	cmdDaemon2 := exec.CommandContext(daemonCtx2, agentExe,
		"run",
		"-config-dir", configDir,
		"-data-dir", dataDir,
		"-socket", pipePath,
		"-api-url", backend.URL,
	)
	if err := cmdDaemon2.Start(); err != nil {
		t.Fatalf("Failed to restart agent daemon: %v", err)
	}
	defer func() {
		daemonCancel2()
		_ = cmdDaemon2.Wait()
	}()

	time.Sleep(300 * time.Millisecond)

	// Query status after restart
	restartStResp, err := Query(pipePath, core.IPCRequest{Action: "get-status"}, 2*time.Second)
	if err != nil || !restartStResp.Success || restartStResp.Status == nil {
		t.Fatalf("Post-restart get-status failed: %v", err)
	}
	if !restartStResp.Status.Connected || restartStResp.Status.PairingState != string(auth.StatusPaired) {
		t.Errorf("Agent did not recover PAIRED state after restart: %+v", restartStResp.Status)
	}
	if restartStResp.Status.TargetsCount != 3 {
		t.Errorf("Expected 3 cached targets preserved after restart, got %d", restartStResp.Status.TargetsCount)
	}
}
