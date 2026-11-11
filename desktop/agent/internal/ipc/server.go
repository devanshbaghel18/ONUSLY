package ipc

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/user"
	"runtime"
	"sync"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/dns"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

type Server struct {
	socketPath       string
	syncer           core.BlocklistSyncer
	credStore        core.CredentialStore
	identityStore    core.DeviceIdentityStore
	pairingMgr       *auth.PairingStateManager
	pairingClient    *auth.PairingClient
	apiBaseURL       string
	allowedUIDs      []uint32
	ownerSID         string
	sessionValidator func(clientSessionID uint32) error
	listener         net.Listener
	peerCredChecker  func(conn net.Conn, allowedUIDs []uint32) error
	groupLookup      func(name string) (*user.Group, error)
	isRootFn         func() bool
	chownFn          func(name string, uid, gid int) error
	chmodFn          func(name string, mode os.FileMode) error
	mu               sync.RWMutex
	running          bool
	stopChan         chan struct{}
	dnsServer        *dns.Server
	appsCtrl         *apps.AppEnforcementController
	adapterMgr       dns.AdapterManager
	backupFilePath   string

	// Windows fields
	hCancelEvent uintptr
}

func NewServer(
	socketPath string,
	syncer core.BlocklistSyncer,
	credStore core.CredentialStore,
	identityStore core.DeviceIdentityStore,
) *Server {
	return &Server{
		socketPath:      socketPath,
		syncer:          syncer,
		credStore:       credStore,
		identityStore:   identityStore,
		pairingClient:   auth.NewPairingClient(nil),
		peerCredChecker: checkPeerCredentials,
		groupLookup:     user.LookupGroup,
		isRootFn:        func() bool { return os.Geteuid() == 0 },
		chownFn:         os.Chown,
		chmodFn:         os.Chmod,
		stopChan:        make(chan struct{}),
	}
}

func (s *Server) defaultGroupLookup() func(name string) (*user.Group, error) {
	return user.LookupGroup
}

// SetGroupLookup overrides the group lookup function for testing (Linux).
func (s *Server) SetGroupLookup(fn func(name string) (*user.Group, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.groupLookup = fn
}

// SetIsRoot overrides the root check function for testing (Linux).
func (s *Server) SetIsRoot(fn func() bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isRootFn = fn
}

// SetFileOps overrides chown and chmod functions for testing (Linux).
func (s *Server) SetFileOps(chownFn func(string, int, int) error, chmodFn func(string, os.FileMode) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chownFn = chownFn
	s.chmodFn = chmodFn
}

// SetAllowedUIDs configures additional allowed Linux UIDs for system daemon mode.
func (s *Server) SetAllowedUIDs(uids []uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowedUIDs = uids
}

// SetPeerCredChecker overrides the peer credential checker (Linux testing).
func (s *Server) SetPeerCredChecker(fn func(conn net.Conn, allowedUIDs []uint32) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerCredChecker = fn
}

// SetDNSServer configures the DNS resolver instance for status and diagnostic reporting.
func (s *Server) SetDNSServer(ds *dns.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dnsServer = ds
}

// SetAppsController configures the application enforcement controller instance.
func (s *Server) SetAppsController(ctrl *apps.AppEnforcementController) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.appsCtrl = ctrl
}

// SetOwnerSID configures the authorized paired Windows user SID.
func (s *Server) SetOwnerSID(sid string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ownerSID = sid
}

// GetOwnerSID returns the currently configured paired Windows user SID.
func (s *Server) GetOwnerSID() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ownerSID
}

// SetSessionValidator overrides the session validation logic for testing or custom physical console tracking.
func (s *Server) SetSessionValidator(fn func(clientSessionID uint32) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionValidator = fn
}

// SetPairingManager configures the state manager for device pairing transitions.
func (s *Server) SetPairingManager(mgr *auth.PairingStateManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairingMgr = mgr
}

// GetPairingManager returns the configured pairing state manager.
func (s *Server) GetPairingManager() *auth.PairingStateManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pairingMgr
}

// SetPairingClient configures the pairing client used to communicate with the backend.
func (s *Server) SetPairingClient(client *auth.PairingClient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pairingClient = client
}

// SetAPIBaseURL configures the backend API URL for pairing and synchronization.
func (s *Server) SetAPIBaseURL(url string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.apiBaseURL = url
}

// GetAPIBaseURL returns the backend API URL.
func (s *Server) GetAPIBaseURL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.apiBaseURL
}

// SetAdapterManager configures the platform network adapter manager for DNS redirection control.
func (s *Server) SetAdapterManager(mgr dns.AdapterManager, backupFilePath string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adapterMgr = mgr
	s.backupFilePath = backupFilePath
}

func (s *Server) dispatch(ctx context.Context, req core.IPCRequest, callerSID string) core.IPCResponse {
	switch req.Action {
	case "get-blocklist":
		blocklist, err := s.syncer.GetCachedBlocklist()
		if err != nil {
			return core.IPCResponse{
				Success: false,
				Error:   err.Error(),
			}
		}
		return core.IPCResponse{
			Success:   true,
			Blocklist: blocklist,
		}

	case "sync":
		blocklist, err := s.syncer.Sync(ctx)
		if err != nil {
			cached, _ := s.syncer.GetCachedBlocklist()
			return core.IPCResponse{
				Success:   true,
				Blocklist: cached,
			}
		}
		return core.IPCResponse{
			Success:   true,
			Blocklist: blocklist,
		}

	case "get-status":
		status := s.buildStatus()
		return core.IPCResponse{
			Success: true,
			Status:  status,
		}

	case "ping":
		return core.IPCResponse{
			Success: true,
		}

	case "pair-request":
		if s.pairingMgr == nil {
			return core.IPCResponse{
				Success: false,
				Error:   "pairing manager not configured on service",
			}
		}
		st, _ := s.pairingMgr.GetState()
		if st != nil && st.Status == auth.StatusPaired {
			tok, _ := s.credStore.GetToken()
			if tok != "" {
				return core.IPCResponse{
					Success: false,
					Error:   "device is already paired; unpair or reset first",
				}
			}
		}

		if _, err := s.pairingMgr.Transition(auth.StatusPairingPending, callerSID); err != nil {
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to transition to PAIRING_PENDING: %v", err),
			}
		}

		deviceID, err := s.identityStore.GetDeviceID()
		if err != nil {
			_, _ = s.pairingMgr.Transition(auth.StatusInstalledUnpaired, "")
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to get device ID: %v", err),
			}
		}

		apiURL := s.GetAPIBaseURL()
		if apiURL == "" {
			apiURL = "http://localhost:8081"
		}

		client := s.pairingClient
		if client == nil {
			client = auth.NewPairingClient(nil)
		}

		code, expiresAt, err := client.RequestCode(ctx, apiURL, "desktop_agent", "windows", deviceID)
		if err != nil {
			_, _ = s.pairingMgr.Transition(auth.StatusInstalledUnpaired, "")
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("pairing request failed: %v", err),
			}
		}

		return core.IPCResponse{
			Success:   true,
			PairCode:  code,
			ExpiresAt: expiresAt,
		}

	case "pair-poll":
		if req.Code == "" {
			return core.IPCResponse{
				Success: false,
				Error:   "missing pairing code",
			}
		}

		apiURL := s.GetAPIBaseURL()
		if apiURL == "" {
			apiURL = "http://localhost:8081"
		}

		client := s.pairingClient
		if client == nil {
			client = auth.NewPairingClient(nil)
		}

		token, err := client.PollForToken(ctx, apiURL, req.Code, 3*time.Minute)
		if err != nil {
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("pairing poll failed: %v", err),
			}
		}

		// Two-phase commit
		if s.pairingMgr != nil {
			if _, err := s.pairingMgr.Transition(auth.StatusPairingCommitPending, callerSID); err != nil {
				log.Printf("[IPC Server] Notice: transition to PAIRING_COMMIT_PENDING: %v", err)
			}
		}

		if err := s.credStore.SaveToken(token); err != nil {
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to save credential: %v", err),
			}
		}

		// Initial sync
		_, errSync := s.syncer.Sync(ctx)
		if errSync != nil {
			log.Printf("[IPC Server] Notice: initial sync after pairing: %v", errSync)
		}

		if s.pairingMgr != nil {
			if _, err := s.pairingMgr.Transition(auth.StatusPaired, callerSID); err != nil {
				log.Printf("[IPC Server] Notice: transition to PAIRED: %v", err)
			}
		}

		if callerSID != "" {
			s.SetOwnerSID(callerSID)
		}

		// Security Invariant: Return status only, ZERO tokens leaked to IPC
		return core.IPCResponse{
			Success: true,
			Status:  s.buildStatus(),
		}

	case "pair-cancel":
		if s.pairingMgr != nil {
			st, _ := s.pairingMgr.GetState()
			if st != nil && (st.Status == auth.StatusPairingPending || st.Status == auth.StatusPairingCommitPending) {
				_, _ = s.pairingMgr.Transition(auth.StatusInstalledUnpaired, "")
			}
		}
		return core.IPCResponse{
			Success: true,
		}

	case "diagnostics":
		return core.IPCResponse{
			Success:     true,
			Diagnostics: s.buildDiagnostics(),
			Status:      s.buildStatus(),
		}

	case "apps-inventory":
		var appList []any
		if s.appsCtrl != nil && s.appsCtrl.GetInventoryScanner() != nil {
			discovered, errScan := s.appsCtrl.GetInventoryScanner().ScanApplications(ctx)
			if errScan != nil {
				return core.IPCResponse{
					Success: false,
					Error:   fmt.Sprintf("failed to scan applications: %v", errScan),
				}
			}
			for _, app := range discovered {
				appList = append(appList, app)
			}
		}
		return core.IPCResponse{
			Success: true,
			Apps:    appList,
			Status:  s.buildStatus(),
		}

	case "apps-status":
		return core.IPCResponse{
			Success:     true,
			Status:      s.buildStatus(),
			Diagnostics: s.buildDiagnostics(),
		}

	case "dns-enable":
		s.mu.RLock()
		mgr := s.adapterMgr
		bPath := s.backupFilePath
		s.mu.RUnlock()
		if mgr == nil {
			return core.IPCResponse{
				Success: false,
				Error:   "adapter manager not configured on service",
			}
		}
		if err := mgr.ApplyLoopbackDNS("127.0.0.1", "::1", bPath); err != nil {
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to enable loopback DNS: %v", err),
			}
		}
		return core.IPCResponse{
			Success: true,
			Status:  s.buildStatus(),
		}

	case "dns-restore":
		s.mu.RLock()
		mgr := s.adapterMgr
		bPath := s.backupFilePath
		s.mu.RUnlock()
		if mgr == nil {
			return core.IPCResponse{
				Success: false,
				Error:   "adapter manager not configured on service",
			}
		}
		if err := mgr.RestoreDNS(bPath); err != nil {
			return core.IPCResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to restore original DNS: %v", err),
			}
		}
		return core.IPCResponse{
			Success: true,
			Status:  s.buildStatus(),
		}

	case "dns-status":
		s.mu.RLock()
		mgr := s.adapterMgr
		bPath := s.backupFilePath
		s.mu.RUnlock()
		active := false
		if mgr != nil && bPath != "" {
			active, _ = mgr.IsRedirectionActive(bPath)
		}
		return core.IPCResponse{
			Success: true,
			Status:  s.buildStatus(),
			Diagnostics: map[string]string{
				"dns_redirection_active": fmt.Sprintf("%t", active),
			},
		}

	default:
		return core.IPCResponse{
			Success: false,
			Error:   fmt.Sprintf("unsupported action %q", req.Action),
		}
	}
}

func (s *Server) buildStatus() *core.AgentStatus {
	token, _ := s.credStore.GetToken()
	userID, _ := auth.ParseTokenUserID(token)
	deviceID, _ := s.identityStore.GetDeviceID()

	pairingState := string(auth.StatusInstalledUnpaired)
	ownerSID := s.GetOwnerSID()

	if s.pairingMgr != nil {
		st, err := s.pairingMgr.GetState()
		if err == nil && st != nil {
			pairingState = string(st.Status)
			if st.OwnerSID != "" && ownerSID == "" {
				ownerSID = st.OwnerSID
			}
		}
	}

	blocklist, _ := s.syncer.GetCachedBlocklist()
	blocklistVersion := ""
	lastSync := ""
	targetsCount := 0

	if blocklist != nil {
		blocklistVersion = blocklist.Version
		lastSync = blocklist.LastSyncAt
		targetsCount = len(blocklist.Targets)
	}

	connected := token != "" && pairingState == string(auth.StatusPaired)
	enforcing := connected && targetsCount > 0

	var issues []string
	var enforcementStatus string
	if pairingState == string(auth.StatusRevoked) {
		enforcementStatus = "REVOKED"
		connected = false
		enforcing = false
		issues = append(issues, "device_revoked")
	} else if !connected {
		enforcementStatus = "UNPAIRED"
	} else if blocklist == nil || targetsCount == 0 {
		enforcing = false
		enforcementStatus = "ENFORCEMENT_DEGRADED"
		issues = append(issues, "policy_missing", "service_degraded")
	} else {
		enforcementStatus = "HEALTHY"
	}

	var dnsRunning bool
	var dnsBlocked, dnsForwarded uint64
	if s.dnsServer != nil {
		dnsRunning = s.dnsServer.IsRunning()
		_, blk, fwd, _, _ := s.dnsServer.GetStats()
		dnsBlocked = blk
		dnsForwarded = fwd
	}

	var appsRunning bool
	var appsMonitored, appsEnforced, appsFailed uint64
	var appsRestricted int
	if s.appsCtrl != nil {
		appsRunning = s.appsCtrl.IsRunning()
		st := s.appsCtrl.GetStats()
		appsMonitored = st.MonitoredProcesses
		appsEnforced = st.EnforcedTerminated
		appsFailed = st.EnforcementFailed
		snap := s.appsCtrl.GetPolicyManager().GetSnapshot()
		if snap != nil {
			for _, t := range snap.Targets {
				if t.Restricted {
					appsRestricted++
				}
			}
		}
	}

	dnsHealth := core.ComponentHealthUnavailable
	if dnsRunning {
		dnsHealth = core.ComponentHealthHealthy
	}

	appsHealth := core.ComponentHealthUnavailable
	if appsRunning {
		if appsFailed > 0 {
			appsHealth = core.ComponentHealthDegraded
		} else {
			appsHealth = core.ComponentHealthHealthy
		}
	}

	browserHealth := core.ComponentHealthHealthy
	syncHealth := core.ComponentHealthHealthy
	if blocklist == nil {
		syncHealth = core.ComponentHealthFailing
	}

	healthState := core.HealthStateHealthy
	lastErrCategory := "none"
	if pairingState == string(auth.StatusRevoked) {
		healthState = core.HealthStateDeviceRevoked
		lastErrCategory = "auth"
	} else if !connected {
		healthState = core.HealthStateAuthenticationRequired
		lastErrCategory = "auth"
	} else if blocklist == nil || targetsCount == 0 {
		healthState = core.HealthStatePolicyMissing
		lastErrCategory = "network"
	} else if !dnsRunning {
		healthState = core.HealthStateDNSUnavailable
		lastErrCategory = "driver"
	} else if appsFailed > 0 {
		healthState = core.HealthStateAppEnforcementDegraded
		lastErrCategory = "permission"
	} else if len(issues) > 0 {
		healthState = core.HealthStateDegraded
	}

	var dnsRedirectionActive bool
	s.mu.RLock()
	mgr := s.adapterMgr
	bPath := s.backupFilePath
	s.mu.RUnlock()
	if mgr != nil && bPath != "" {
		dnsRedirectionActive, _ = mgr.IsRedirectionActive(bPath)
	}

	return &core.AgentStatus{
		Connected:             connected,
		UserID:                userID,
		DeviceID:              deviceID,
		Platform:              runtime.GOOS,
		Version:               core.ReleaseVersion,
		LastSyncAt:            lastSync,
		BlocklistVersion:      blocklistVersion,
		TargetsCount:          targetsCount,
		Enforcing:             enforcing,
		EnforcementStatus:     enforcementStatus,
		HealthState:           healthState,
		DNSHealth:             dnsHealth,
		AppsHealth:            appsHealth,
		BrowserHealth:         browserHealth,
		SyncHealth:            syncHealth,
		LastSyncOutcome:       "success",
		LastErrorCategory:     lastErrCategory,
		LKGActive:             blocklist != nil && targetsCount > 0,
		CacheValid:            blocklist != nil,
		Issues:                issues,
		PairingState:          pairingState,
		OwnerSID:              ownerSID,
		DNSResolverRunning:    dnsRunning,
		DNSResolverListening:  dnsRunning,
		DNSRedirectionActive:  dnsRedirectionActive,
		DNSQueriesBlocked:     dnsBlocked,
		DNSQueriesForwarded:   dnsForwarded,
		AppsControllerRunning: appsRunning,
		AppsMonitoredCount:    appsMonitored,
		AppsRestrictedCount:   appsRestricted,
		AppsEnforcedCount:     appsEnforced,
		AppsFailedCount:       appsFailed,
	}
}

func (s *Server) buildDiagnostics() map[string]string {
	token, _ := s.credStore.GetToken()
	userID, _ := auth.ParseTokenUserID(token)
	deviceID, _ := s.identityStore.GetDeviceID()

	pairingState := string(auth.StatusInstalledUnpaired)
	ownerSID := s.GetOwnerSID()
	if s.pairingMgr != nil {
		st, _ := s.pairingMgr.GetState()
		if st != nil {
			pairingState = string(st.Status)
			if st.OwnerSID != "" {
				ownerSID = st.OwnerSID
			}
		}
	}

	blocklist, _ := s.syncer.GetCachedBlocklist()
	targetsCount := 0
	version := ""
	cacheValid := "false"
	enforcing := false
	enforcementStatus := "UNPAIRED"
	healthState := core.HealthStateAuthenticationRequired
	lastErrCategory := "auth"

	connected := token != "" && pairingState == string(auth.StatusPaired)
	if pairingState == string(auth.StatusRevoked) {
		healthState = core.HealthStateDeviceRevoked
		enforcementStatus = "REVOKED"
		lastErrCategory = "auth"
	} else if connected {
		if blocklist != nil && len(blocklist.Targets) > 0 {
			enforcing = true
			enforcementStatus = "HEALTHY"
			healthState = core.HealthStateHealthy
			lastErrCategory = "none"
		} else {
			enforcing = false
			enforcementStatus = "ENFORCEMENT_DEGRADED"
			healthState = core.HealthStatePolicyMissing
			lastErrCategory = "network"
		}
	}

	if blocklist != nil {
		targetsCount = len(blocklist.Targets)
		version = blocklist.Version
		cacheValid = "true"
	}

	diag := map[string]string{
		"platform":            runtime.GOOS,
		"arch":                runtime.GOARCH,
		"version":             core.ReleaseVersion,
		"build_commit":        core.BuildCommit,
		"build_date":          core.BuildDate,
		"device_id":           deviceID,
		"pairing_state":       pairingState,
		"owner_sid":           ownerSID,
		"has_credentials":     fmt.Sprintf("%t", token != ""),
		"user_id":             userID,
		"blocklist_version":   version,
		"targets_count":       fmt.Sprintf("%d", targetsCount),
		"cache_valid":         cacheValid,
		"lkg_active":          fmt.Sprintf("%t", blocklist != nil && targetsCount > 0),
		"enforcing":           fmt.Sprintf("%t", enforcing),
		"enforcement_status":  enforcementStatus,
		"health_state":        healthState,
		"browser_status":      "HEALTHY",
		"last_error_category": lastErrCategory,
		"service_running":     "true",
	}

	if s.dnsServer != nil {
		tot, blk, fwd, cch, fld := s.dnsServer.GetStats()
		diag["dns_resolver_running"] = fmt.Sprintf("%t", s.dnsServer.IsRunning())
		diag["dns_listen_address"] = s.dnsServer.ListenAddress()
		diag["dns_queries_total"] = fmt.Sprintf("%d", tot)
		diag["dns_queries_blocked"] = fmt.Sprintf("%d", blk)
		diag["dns_queries_forwarded"] = fmt.Sprintf("%d", fwd)
		diag["dns_queries_cached"] = fmt.Sprintf("%d", cch)
		diag["dns_queries_failed"] = fmt.Sprintf("%d", fld)
	}

	if s.appsCtrl != nil {
		st := s.appsCtrl.GetStats()
		diag["apps_controller_running"] = fmt.Sprintf("%t", s.appsCtrl.IsRunning())
		diag["apps_monitored_processes"] = fmt.Sprintf("%d", st.MonitoredProcesses)
		diag["apps_restricted_detected"] = fmt.Sprintf("%d", st.RestrictedDetected)
		diag["apps_enforced_terminated"] = fmt.Sprintf("%d", st.EnforcedTerminated)
		diag["apps_enforcement_failed"] = fmt.Sprintf("%d", st.EnforcementFailed)
		diag["apps_capability_tier"] = "Tier 1 - Detection & Response"
		if inv := s.appsCtrl.GetInventoryScanner(); inv != nil {
			if appsList, err := inv.ScanApplications(context.Background()); err == nil {
				diag["apps_inventory_count"] = fmt.Sprintf("%d", len(appsList))
			}
		}
	}

	return diag
}
