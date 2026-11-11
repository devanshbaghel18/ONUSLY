//go:build windows

package ipc

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/identity"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

// makeTestJWT generates a valid 3-part base64-encoded JWT with the specified subject and expiry.
func makeTestJWT(sub string, expSecsFromNow int64) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	exp := time.Now().Unix() + expSecsFromNow
	payloadJSON := fmt.Sprintf(`{"sub":%q,"exp":%d}`, sub, exp)
	payload := base64.RawURLEncoding.EncodeToString([]byte(payloadJSON))
	sig := base64.RawURLEncoding.EncodeToString([]byte("test-mock-signature-bytes"))
	return fmt.Sprintf("%s.%s.%s", header, payload, sig)
}

func setupMockBackend(t *testing.T, expectedUser, expectedCode, expectedToken string) *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /enforcement/pair/request", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":      expectedCode,
			"expiresAt": time.Now().Add(10 * time.Minute).Unix(),
		})
	})

	mux.HandleFunc("POST /enforcement/pair/exchange", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Code != expectedCode {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid pairing code"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"token": expectedToken})
	})

	mux.HandleFunc("GET /enforcement/blocklist", func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		sub, err := auth.ParseTokenUserID(token)
		if err != nil || sub != expectedUser {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if r.Header.Get("If-None-Match") == "v-test-etag" {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("ETag", `"v-test-etag"`)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      expectedUser,
			"version":     "v-test-etag",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"targets": []core.TargetItem{
				{
					GoalID:    "goal-block-distractions",
					GoalTitle: "Block Distractions",
					Status:    "active",
					Domains:   []string{"distraction.example.com", "procrastinate.com"},
				},
			},
		})
	})

	return httptest.NewServer(mux)
}

func TestWindowsNamedPipe_PairingFlow(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "onusly-pairing-ipc-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	pipePath := fmt.Sprintf(`\\.\pipe\OnuslyTestPairingIPC_%d`, os.Getpid())
	testUser := "user-ipc-test-42"
	testCode := "KX7M92"
	testJWT := makeTestJWT(testUser, 3600)

	backend := setupMockBackend(t, testUser, testCode, testJWT)
	defer backend.Close()

	// Configure components
	configDir := filepath.Join(tmpDir, "config")
	dataDir := filepath.Join(tmpDir, "data")
	secureDir := filepath.Join(tmpDir, "secure")
	_ = os.MkdirAll(configDir, 0700)
	_ = os.MkdirAll(dataDir, 0700)
	_ = os.MkdirAll(secureDir, 0700)

	idFile := filepath.Join(configDir, "device_id")
	_ = os.WriteFile(idFile, []byte("11111111-2222-3333-4444-555555555555"), 0600)
	idStore := identity.NewFileDeviceIdentityStore(idFile)

	tokenFile := filepath.Join(secureDir, "token.dat")
	credStore := auth.NewDPAPICredentialStore(tokenFile)

	pairingStateFile := filepath.Join(configDir, "pairing_state.json")
	pairingMgr := auth.NewPairingStateManager(pairingStateFile)

	cacheFile := filepath.Join(dataDir, "blocklist_cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	syncer := sync.NewSyncer(backend.URL, credStore, cacheStore, backend.Client())

	server := NewServer(pipePath, syncer, credStore, idStore)
	server.SetPairingManager(pairingMgr)
	server.SetAPIBaseURL(backend.URL)
	server.SetPairingClient(auth.NewPairingClient(backend.Client()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start IPC server: %v", err)
	}
	defer server.Stop()

	time.Sleep(50 * time.Millisecond)

	// Step 1: Initial status query before pairing
	statusResp, err := Query(pipePath, core.IPCRequest{Action: "get-status"}, 2*time.Second)
	if err != nil {
		t.Fatalf("Query(get-status) failed: %v", err)
	}
	if !statusResp.Success || statusResp.Status == nil {
		t.Fatalf("Expected successful status, got %+v", statusResp)
	}
	if statusResp.Status.PairingState != string(auth.StatusInstalledUnpaired) && statusResp.Status.PairingState != string(auth.StatusUninstalled) {
		t.Errorf("Expected unpaired state, got %s", statusResp.Status.PairingState)
	}
	if statusResp.Status.Connected {
		t.Errorf("Expected connected=false before pairing")
	}
	if statusResp.Status.Enforcing {
		t.Errorf("Expected enforcing=false before pairing")
	}

	// Step 2: Request pairing code over IPC
	reqResp, err := Query(pipePath, core.IPCRequest{Action: "pair-request"}, 5*time.Second)
	if err != nil {
		t.Fatalf("Query(pair-request) failed: %v", err)
	}
	if !reqResp.Success {
		t.Fatalf("Expected pair-request success, got error: %s", reqResp.Error)
	}
	if reqResp.PairCode != testCode {
		t.Fatalf("Expected pairing code %s, got %s", testCode, reqResp.PairCode)
	}
	if reqResp.ExpiresAt <= time.Now().Unix() {
		t.Errorf("Expected expiresAt in the future, got %d", reqResp.ExpiresAt)
	}

	// Verify state transitioned to PAIRING_PENDING
	st, err := pairingMgr.GetState()
	if err != nil || st.Status != auth.StatusPairingPending {
		t.Fatalf("Expected status PAIRING_PENDING, got %+v (err %v)", st, err)
	}

	// Step 3: Complete pairing via pair-poll
	pollResp, err := Query(pipePath, core.IPCRequest{Action: "pair-poll", Code: testCode}, 5*time.Second)
	if err != nil {
		t.Fatalf("Query(pair-poll) failed: %v", err)
	}
	if !pollResp.Success {
		t.Fatalf("Expected pair-poll success, got error: %s", pollResp.Error)
	}
	if pollResp.Status == nil {
		t.Fatalf("Expected status in pair-poll response, got nil")
	}

	// SECURITY INVARIANT CHECK: Zero credentials exposed in IPC response
	respJSON, _ := json.Marshal(pollResp)
	if strings.Contains(string(respJSON), testJWT) {
		t.Fatalf("CRITICAL SECURITY VIOLATION: raw JWT leaked in IPC response: %s", string(respJSON))
	}

	// Verify post-pairing status
	if pollResp.Status.PairingState != string(auth.StatusPaired) {
		t.Errorf("Expected status PAIRED, got %s", pollResp.Status.PairingState)
	}
	if !pollResp.Status.Connected {
		t.Errorf("Expected connected=true after pairing")
	}
	if pollResp.Status.UserID != testUser {
		t.Errorf("Expected user %s, got %s", testUser, pollResp.Status.UserID)
	}
	if pollResp.Status.TargetsCount != 1 {
		t.Errorf("Expected 1 target locked, got %d", pollResp.Status.TargetsCount)
	}
	if !pollResp.Status.Enforcing {
		t.Errorf("Expected enforcing=true when targets > 0")
	}

	// Step 4: Verify credential was securely saved in DPAPI store
	savedTok, err := credStore.GetToken()
	if err != nil || savedTok != testJWT {
		t.Fatalf("Expected saved token %q, got %q (err %v)", testJWT, savedTok, err)
	}

	// Step 5: Query diagnostics over IPC
	diagResp, err := Query(pipePath, core.IPCRequest{Action: "diagnostics"}, 2*time.Second)
	if err != nil {
		t.Fatalf("Query(diagnostics) failed: %v", err)
	}
	if !diagResp.Success || diagResp.Diagnostics == nil {
		t.Fatalf("Expected diagnostics, got %+v", diagResp)
	}
	if diagResp.Diagnostics["has_credentials"] != "true" {
		t.Errorf("Expected has_credentials: true in diagnostics")
	}
	if diagResp.Diagnostics["user_id"] != testUser {
		t.Errorf("Expected user_id %s, got %s", testUser, diagResp.Diagnostics["user_id"])
	}

	// Step 6: Query get-blocklist over IPC
	blockListResp, err := Query(pipePath, core.IPCRequest{Action: "get-blocklist"}, 2*time.Second)
	if err != nil {
		t.Fatalf("Query(get-blocklist) failed: %v", err)
	}
	if !blockListResp.Success || blockListResp.Blocklist == nil {
		t.Fatalf("Expected blocklist, got %+v", blockListResp)
	}
	if len(blockListResp.Blocklist.Targets) != 1 || blockListResp.Blocklist.Targets[0].Domains[0] != "distraction.example.com" {
		t.Errorf("Unexpected targets in blocklist: %+v", blockListResp.Blocklist.Targets)
	}

	// Step 7: Attempt duplicate pairing when already paired
	dupResp, err := Query(pipePath, core.IPCRequest{Action: "pair-request"}, 2*time.Second)
	if err != nil {
		t.Fatalf("Query(duplicate pair-request) error: %v", err)
	}
	if dupResp.Success {
		t.Fatalf("Expected duplicate pairing to be rejected, but got success!")
	}
}

func TestWindowsNamedPipe_PairCancel(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-pair-cancel-test-*")
	defer os.RemoveAll(tmpDir)

	pipePath := fmt.Sprintf(`\\.\pipe\OnuslyTestPairCancelIPC_%d`, os.Getpid())
	backend := setupMockBackend(t, "user1", "CN9999", makeTestJWT("user1", 3600))
	defer backend.Close()

	configDir := filepath.Join(tmpDir, "config")
	_ = os.MkdirAll(configDir, 0700)
	idStore := identity.NewFileDeviceIdentityStore(filepath.Join(configDir, "device_id"))
	credStore := auth.NewDPAPICredentialStore(filepath.Join(tmpDir, "token.dat"))
	pairingMgr := auth.NewPairingStateManager(filepath.Join(configDir, "pairing_state.json"))
	syncer := sync.NewSyncer(backend.URL, credStore, cache.NewFileBlocklistCache(filepath.Join(tmpDir, "cache.json")), nil)

	server := NewServer(pipePath, syncer, credStore, idStore)
	server.SetPairingManager(pairingMgr)
	server.SetAPIBaseURL(backend.URL)
	server.SetPairingClient(auth.NewPairingClient(backend.Client()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer server.Stop()
	time.Sleep(30 * time.Millisecond)

	// 1. Request pairing
	reqResp, err := Query(pipePath, core.IPCRequest{Action: "pair-request"}, 2*time.Second)
	if err != nil || !reqResp.Success {
		t.Fatalf("pair-request failed: %v", err)
	}

	st, _ := pairingMgr.GetState()
	if st.Status != auth.StatusPairingPending {
		t.Fatalf("Expected PAIRING_PENDING, got %s", st.Status)
	}

	// 2. Cancel pairing
	cancelResp, err := Query(pipePath, core.IPCRequest{Action: "pair-cancel"}, 2*time.Second)
	if err != nil || !cancelResp.Success {
		t.Fatalf("pair-cancel failed: %v", err)
	}

	stAfter, _ := pairingMgr.GetState()
	if stAfter.Status != auth.StatusInstalledUnpaired {
		t.Fatalf("Expected INSTALLED_UNPAIRED after cancel, got %s", stAfter.Status)
	}
}

