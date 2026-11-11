//go:build windows

package ipc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func TestWindowsFailure_InterruptedCommitRecovery(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-recovery-test-*")
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	credStore := auth.NewDPAPICredentialStore(tokenFile)

	pairingFile := filepath.Join(tmpDir, "pairing_state.json")
	pairingMgr := auth.NewPairingStateManager(pairingFile)

	cacheFile := filepath.Join(tmpDir, "cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	user := "recovery-user"
	validJWT := makeTestJWT(user, 3600)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      user,
			"version":     "v-recovered",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"targets":     []core.TargetItem{},
		})
	}))
	defer backend.Close()

	syncer := sync.NewSyncer(backend.URL, credStore, cacheStore, backend.Client())

	// Case 1: Interrupted Commit with Token Present -> Promote to PAIRED
	_ = pairingMgr.SaveState(&auth.PairingState{
		Status:     auth.StatusPairingCommitPending,
		OwnerSID:   "S-1-5-21-mock-owner",
		Generation: 2,
	})
	_ = credStore.SaveToken(validJWT)

	recState, err := pairingMgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if recState.Status != auth.StatusPaired {
		t.Fatalf("Expected status PAIRED after interrupted commit recovery with valid token, got %s", recState.Status)
	}

	// Case 2: Interrupted Commit with Token Missing -> Transition to REPAIR_REQUIRED
	_ = credStore.ClearToken()
	_ = pairingMgr.SaveState(&auth.PairingState{
		Status:     auth.StatusPairingCommitPending,
		OwnerSID:   "S-1-5-21-mock-owner",
		Generation: 3,
	})

	recState2, err := pairingMgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if recState2.Status != auth.StatusRepairRequired {
		t.Fatalf("Expected status REPAIR_REQUIRED after interrupted commit without token, got %s", recState2.Status)
	}

	// Case 3: Paired State with Corrupt Token -> Transition to REPAIR_REQUIRED
	_ = os.WriteFile(tokenFile, []byte("NOT_A_VALID_DPAPI_BLOB"), 0600)
	_ = pairingMgr.SaveState(&auth.PairingState{
		Status:     auth.StatusPaired,
		OwnerSID:   "S-1-5-21-mock-owner",
		Generation: 4,
	})

	recState3, err := pairingMgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if recState3.Status != auth.StatusRepairRequired {
		t.Fatalf("Expected status REPAIR_REQUIRED when token is corrupt, got %s", recState3.Status)
	}
}

func TestWindowsFailure_OfflineResilience(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-offline-test-*")
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	credStore := auth.NewDPAPICredentialStore(tokenFile)

	cacheFile := filepath.Join(tmpDir, "cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	user := "offline-user"
	validJWT := makeTestJWT(user, 3600)
	_ = credStore.SaveToken(validJWT)

	// Pre-seed cache
	initialBlocklist := &core.BlocklistData{
		UserID:      user,
		Version:     "v1-offline-seed",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		LastSyncAt:  time.Now().UTC().Format(time.RFC3339),
		Targets: []core.TargetItem{
			{
				GoalID:    "g-offline",
				GoalTitle: "Offline Protection",
				Status:    "active",
				Domains:   []string{"distraction.com"},
			},
		},
	}
	if err := cacheStore.Save(initialBlocklist); err != nil {
		t.Fatalf("Failed to save pre-seed cache: %v", err)
	}

	// Syncer points to an unreachable endpoint (simulating network down / HTTP 521)
	unreachableURL := "http://127.0.0.1:59999"
	syncer := sync.NewSyncer(unreachableURL, credStore, cacheStore, &http.Client{Timeout: 100 * time.Millisecond})

	// CRITICAL INVARIANT TEST: Network failure must NEVER wipe valid cached restrictions!
	data, err := syncer.Sync(context.Background())
	if err != nil {
		t.Logf("Notice of expected network failure: %v", err)
	}
	if data == nil {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: syncer wiped cached blocklist on network failure!")
	}
	if len(data.Targets) != 1 || data.Targets[0].Domains[0] != "distraction.com" {
		t.Fatalf("Expected 1 target preserved during network outage, got %+v", data.Targets)
	}

	// Verify GetCachedBlocklist also preserves data
	cachedData, errCached := syncer.GetCachedBlocklist()
	if errCached != nil || cachedData == nil || len(cachedData.Targets) != 1 {
		t.Fatalf("Cached blocklist unavailable during offline state: %v", errCached)
	}
}

func TestWindowsFailure_ETag304NotModified(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-etag-test-*")
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	credStore := auth.NewDPAPICredentialStore(tokenFile)

	cacheFile := filepath.Join(tmpDir, "cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	user := "etag-user"
	validJWT := makeTestJWT(user, 3600)
	_ = credStore.SaveToken(validJWT)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Header.Get("If-None-Match") == "v-etag-100" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v-etag-100"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      user,
			"version":     "v-etag-100",
			"generatedAt": time.Now().UTC().Format(time.RFC3339),
			"targets": []core.TargetItem{
				{
					GoalID:    "g-etag",
					GoalTitle: "ETag Goal",
					Status:    "active",
					Domains:   []string{"example.com"},
				},
			},
		})
	}))
	defer server.Close()

	syncer := sync.NewSyncer(server.URL, credStore, cacheStore, server.Client())

	// 1st request -> 200 OK
	data1, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("First sync failed: %v", err)
	}
	if data1.Version != "v-etag-100" {
		t.Errorf("Expected version v-etag-100, got %s", data1.Version)
	}

	// 2nd request -> 304 Not Modified
	data2, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("Second sync (conditional) failed: %v", err)
	}
	if data2 == nil || len(data2.Targets) != 1 {
		t.Fatalf("Expected cached targets on 304 Not Modified, got %+v", data2)
	}
	if requestCount != 2 {
		t.Errorf("Expected 2 HTTP requests, got %d", requestCount)
	}
}

func TestWindowsFailure_TokenExpirationHandling(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-token-exp-test-*")
	defer os.RemoveAll(tmpDir)

	tokenFile := filepath.Join(tmpDir, "token.dat")
	credStore := auth.NewDPAPICredentialStore(tokenFile)

	pairingFile := filepath.Join(tmpDir, "pairing_state.json")
	pairingMgr := auth.NewPairingStateManager(pairingFile)

	cacheFile := filepath.Join(tmpDir, "cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	user := "expired-user"
	// Create token that expired 10 seconds ago
	expiredJWT := makeTestJWT(user, -10)
	_ = credStore.SaveToken(expiredJWT)

	_ = pairingMgr.SaveState(&auth.PairingState{
		Status:     auth.StatusPaired,
		OwnerSID:   "S-1-5-21-mock-owner",
		Generation: 1,
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	syncer := sync.NewSyncer(server.URL, credStore, cacheStore, server.Client())

	// Recover should recognize expired token and transition to AUTHENTICATION_REQUIRED
	recState, err := pairingMgr.Recover(context.Background(), credStore, syncer)
	if err != nil {
		t.Fatalf("Recover failed: %v", err)
	}
	if recState.Status != auth.StatusAuthRequired {
		t.Fatalf("Expected status AUTHENTICATION_REQUIRED for expired token, got %s", recState.Status)
	}
}
