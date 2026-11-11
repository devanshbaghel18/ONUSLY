package sync_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type mockCredStore struct {
	token string
}

func (m *mockCredStore) SaveToken(t string) error  { m.token = t; return nil }
func (m *mockCredStore) GetToken() (string, error) { return m.token, nil }
func (m *mockCredStore) ClearToken() error         { m.token = ""; return nil }

func TestSyncer_LifecycleAndOfflinePreservation(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-syncer-test-*")
	defer os.RemoveAll(tmpDir)

	cacheFile := filepath.Join(tmpDir, "blocklist_cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	// Valid mock JWT for sub: user-desktop-123
	// Header: {"alg":"HS256","typ":"JWT"} -> eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9
	// Payload: {"sub":"user-desktop-123","exp":1999999999} -> eyJzdWIiOiJ1c2VyLWRlc2t0b3AtMTIzIiwiZXhwIjoxOTk5OTk5OTk5fQ
	mockJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLWRlc2t0b3AtMTIzIiwiZXhwIjoxOTk5OTk5OTk5fQ.signature"
	credStore := &mockCredStore{token: mockJWT}

	requestCount := 0
	etagSent := ""

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		etagSent = r.Header.Get("If-None-Match")

		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer "+mockJWT {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		if etagSent == "v1-active123" {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		w.Header().Set("ETag", `"v1-active123"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      "user-desktop-123",
			"version":     "v1-active123",
			"generatedAt": "2026-11-06T00:00:00Z",
			"targets": []map[string]any{
				{
					"goalId":    "goal-ig",
					"goalTitle": "No Instagram",
					"status":    "active",
					"domains":   []string{"instagram.com"},
				},
			},
		})
	}))
	defer server.Close()

	syncer := sync.NewSyncer(server.URL, credStore, cacheStore, server.Client())

	// 1. Initial Sync -> Fetches 200 OK and caches targets
	data, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("expected successful sync, got: %v", err)
	}
	if len(data.Targets) != 1 || data.Targets[0].Domains[0] != "instagram.com" {
		t.Fatalf("unexpected targets: %+v", data.Targets)
	}
	if data.Version != "v1-active123" {
		t.Errorf("expected version v1-active123, got %s", data.Version)
	}

	// 2. Second Sync -> Sends If-None-Match and receives 304 Not Modified
	data304, err := syncer.Sync(context.Background())
	if err != nil {
		t.Fatalf("expected 304 sync to succeed, got %v", err)
	}
	if etagSent != "v1-active123" {
		t.Errorf("expected client to send If-None-Match: v1-active123, sent: %s", etagSent)
	}
	if len(data304.Targets) != 1 {
		t.Errorf("targets should be preserved across 304, got %d", len(data304.Targets))
	}

	// 3. Offline / Server Outage -> Closes server and attempts sync
	server.Close()
	dataOffline, err := syncer.Sync(context.Background())
	if dataOffline == nil || len(dataOffline.Targets) != 1 {
		t.Fatalf("CRITICAL INVARIANT VIOLATION: offline sync must preserve cached restrictions, got: %+v (err: %v)", dataOffline, err)
	}
	if dataOffline.Targets[0].Domains[0] != "instagram.com" {
		t.Errorf("expected offline cache to maintain instagram.com restriction")
	}
}

func TestSyncer_DeviceRevokedAndCredentialExpired(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "onusly-syncer-d8-*")
	defer os.RemoveAll(tmpDir)

	cacheFile := filepath.Join(tmpDir, "blocklist_cache.json")
	cacheStore := cache.NewFileBlocklistCache(cacheFile)

	mockJWT := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ1c2VyLWRlc2t0b3AtNDU2IiwiZXhwIjoxOTk5OTk5OTk5fQ.signature"
	credStore := &mockCredStore{token: mockJWT}

	// 1. Initial healthy sync to seed cache
	initialServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("ETag", `"v1-seed"`)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"userId":      "user-desktop-456",
			"version":     "v1-seed",
			"generatedAt": "2026-11-11T00:00:00Z",
			"targets": []map[string]any{
				{
					"goalId":    "goal-discord",
					"goalTitle": "No Discord",
					"status":    "active",
					"domains":   []string{"discord.com"},
				},
			},
		})
	}))
	defer initialServer.Close()

	syncer := sync.NewSyncer(initialServer.URL, credStore, cacheStore, initialServer.Client())
	seeded, err := syncer.Sync(context.Background())
	if err != nil || seeded == nil || len(seeded.Targets) != 1 {
		t.Fatalf("Failed to seed initial blocklist: %v", err)
	}

	// 2. Test CREDENTIAL_EXPIRED: returns core.ErrCredentialExpired, offline cache retained
	expiredServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "CREDENTIAL_EXPIRED",
			"code":  "credential_expired",
		})
	}))
	defer expiredServer.Close()

	expiredSyncer := sync.NewSyncer(expiredServer.URL, credStore, cacheStore, expiredServer.Client())
	// Pre-load cached blocklist into memory
	_, _ = expiredSyncer.GetCachedBlocklist()

	expData, expErr := expiredSyncer.Sync(context.Background())
	if !errors.Is(expErr, core.ErrCredentialExpired) {
		t.Fatalf("Expected core.ErrCredentialExpired, got: %v", expErr)
	}
	if expData == nil || len(expData.Targets) != 1 {
		t.Fatalf("CREDENTIAL_EXPIRED must preserve offline cache for user grace period, got: %+v", expData)
	}

	// 3. Test DEVICE_REVOKED: returns core.ErrDeviceRevoked, cache strictly purged
	revokedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": "DEVICE_REVOKED",
			"code":  "device_revoked",
		})
	}))
	defer revokedServer.Close()

	revokedSyncer := sync.NewSyncer(revokedServer.URL, credStore, cacheStore, revokedServer.Client())
	// Pre-load cached blocklist into memory
	_, _ = revokedSyncer.GetCachedBlocklist()

	revData, revErr := revokedSyncer.Sync(context.Background())
	if !errors.Is(revErr, core.ErrDeviceRevoked) {
		t.Fatalf("Expected core.ErrDeviceRevoked, got: %v", revErr)
	}
	if revData != nil {
		t.Fatalf("DEVICE_REVOKED must return nil blocklist, got: %+v", revData)
	}

	// Verify memory cache is purged
	cachedMem, _ := revokedSyncer.GetCachedBlocklist()
	if cachedMem != nil {
		t.Fatalf("DEVICE_REVOKED must purge in-memory cache, got: %+v", cachedMem)
	}

	// Verify disk cache is purged
	cachedDisk, _ := cacheStore.Get("user-desktop-456")
	if cachedDisk != nil {
		t.Fatalf("DEVICE_REVOKED must purge disk cache, got: %+v", cachedDisk)
	}
}
