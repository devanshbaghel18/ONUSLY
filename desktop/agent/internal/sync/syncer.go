package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type Syncer struct {
	apiBaseURL      string
	credStore       core.CredentialStore
	cache           core.BlocklistCache
	httpClient      *http.Client
	mu              sync.Mutex
	lastKnownETag   string
	cachedBlocklist *core.BlocklistData
}

func NewSyncer(apiBaseURL string, credStore core.CredentialStore, cache core.BlocklistCache, httpClient *http.Client) *Syncer {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Syncer{
		apiBaseURL: apiBaseURL,
		credStore:  credStore,
		cache:      cache,
		httpClient: httpClient,
	}
}

func (s *Syncer) Sync(ctx context.Context) (*core.BlocklistData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	token, err := s.credStore.GetToken()
	if err != nil || token == "" {
		return nil, core.ErrUnauthenticated
	}

	userID, err := auth.ParseTokenUserID(token)
	if err != nil && !strings.Contains(err.Error(), "expired") {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	// Read existing local cache for conditional request
	cached, _ := s.cache.Get(userID)
	if cached != nil {
		s.cachedBlocklist = cached
		if cached.Version != "" {
			s.lastKnownETag = cached.Version
		}
	}

	url := fmt.Sprintf("%s/enforcement/blocklist", strings.TrimRight(s.apiBaseURL, "/"))
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return s.cachedBlocklist, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if s.lastKnownETag != "" {
		req.Header.Set("If-None-Match", s.lastKnownETag)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		// Network offline: CRITICAL INVARIANT: NEVER clear valid restrictions merely because a network request failed!
		if s.cachedBlocklist != nil {
			return s.cachedBlocklist, nil
		}
		return nil, fmt.Errorf("%w: %v", core.ErrServiceOffline, err)
	}
	defer resp.Body.Close()

	nowStr := time.Now().UTC().Format(time.RFC3339)

	// 304 Not Modified: Cache is fresh
	if resp.StatusCode == http.StatusNotModified {
		if s.cachedBlocklist != nil {
			s.cachedBlocklist.LastSyncAt = nowStr
			_ = s.cache.Save(s.cachedBlocklist)
			return s.cachedBlocklist, nil
		}
	}

	if resp.StatusCode == http.StatusOK {
		var body struct {
			UserID      string            `json:"userId"`
			Version     string            `json:"version"`
			GeneratedAt string            `json:"generatedAt"`
			Targets     []core.TargetItem `json:"targets"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			if s.cachedBlocklist != nil {
				return s.cachedBlocklist, nil
			}
			return nil, fmt.Errorf("failed to decode blocklist: %w", err)
		}

		if userID != "" && body.UserID != "" && body.UserID != userID {
			return nil, core.ErrUserMismatch
		}

		cleanETag := resp.Header.Get("ETag")
		cleanETag = strings.TrimPrefix(cleanETag, "W/")
		cleanETag = strings.Trim(cleanETag, `"`)
		version := body.Version
		if version == "" {
			version = cleanETag
		}

		targets := body.Targets
		if targets == nil {
			targets = []core.TargetItem{}
		}

		freshData := &core.BlocklistData{
			UserID:      body.UserID,
			Version:     version,
			GeneratedAt: body.GeneratedAt,
			LastSyncAt:  nowStr,
			Targets:     targets,
		}

		if err := s.cache.Save(freshData); err != nil {
			return freshData, fmt.Errorf("failed to persist fresh blocklist: %w", err)
		}

		s.cachedBlocklist = freshData
		s.lastKnownETag = version
		return freshData, nil
	}

	// 401 Unauthorized or 403 Forbidden: check error response body
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		bodyStr := string(bodyBytes)
		if strings.Contains(bodyStr, "DEVICE_REVOKED") || strings.Contains(bodyStr, "device_revoked") {
			// Invariant: Confirmed device revocation MUST NOT continue active enforcement
			s.cachedBlocklist = nil
			s.lastKnownETag = ""
			if s.cache != nil {
				_ = s.cache.Clear()
			}
			return nil, core.ErrDeviceRevoked
		}
		if strings.Contains(bodyStr, "CREDENTIAL_EXPIRED") || strings.Contains(bodyStr, "credential_expired") {
			if s.cachedBlocklist != nil {
				return s.cachedBlocklist, core.ErrCredentialExpired
			}
			return nil, core.ErrCredentialExpired
		}
		if s.cachedBlocklist != nil {
			// Maintain offline cache enforcement even if token expires until explicit disconnect
			return s.cachedBlocklist, core.ErrUnauthenticated
		}
		return nil, core.ErrUnauthenticated
	}

	if s.cachedBlocklist != nil {
		return s.cachedBlocklist, nil
	}

	return nil, fmt.Errorf("unexpected status %d from blocklist API", resp.StatusCode)
}

func (s *Syncer) GetCachedBlocklist() (*core.BlocklistData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cachedBlocklist != nil {
		return s.cachedBlocklist, nil
	}

	token, _ := s.credStore.GetToken()
	userID, _ := auth.ParseTokenUserID(token)
	data, err := s.cache.Get(userID)
	if err == nil && data != nil {
		s.cachedBlocklist = data
		return data, nil
	}

	return nil, errors.New("no cached blocklist available")
}

// ClearCache purges both in-memory and persistent on-disk policy caches.
func (s *Syncer) ClearCache() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cachedBlocklist = nil
	s.lastKnownETag = ""
	if s.cache != nil {
		return s.cache.Clear()
	}
	return nil
}
