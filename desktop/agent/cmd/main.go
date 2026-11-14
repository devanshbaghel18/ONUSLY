package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/heartbeat"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/identity"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func main() {
	var (
		apiURL        = flag.String("api-url", "http://localhost:8081", "ONUSLY backend API URL")
		configDir     = flag.String("config-dir", getDefaultConfigDir(), "Configuration directory path")
		dataDir       = flag.String("data-dir", getDefaultDataDir(), "Data storage directory path")
		ipcPath       = flag.String("socket", getDefaultIPCPath(), "IPC socket or named pipe path")
		pairedUIDFlag = flag.Uint("paired-uid", 0, "Authorized desktop user UID for system mode IPC (Linux)")
		pairedSIDFlag = flag.String("paired-sid", "", "Authorized desktop user SID for system mode IPC (Windows)")
		dnsListenFlag = flag.String("dns-listen", "127.0.0.1:53", "DNS resolver loopback listening address")
	)

	cmd := "run"
	var flagArgs []string
	var serviceSubArgs []string
	var dnsSubArgs []string
	var appsSubArgs []string
	var updateSubArgs []string
	var supportSubArgs []string
	var policySubArgs []string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if !strings.HasPrefix(arg, "-") && (arg == "run" || arg == "pair" || arg == "status" || arg == "diagnostics" || arg == "daemon" || arg == "service" || arg == "dns" || arg == "apps" || arg == "version" || arg == "update" || arg == "support" || arg == "policy") {
			cmd = arg
			if arg == "service" && i+1 < len(os.Args) {
				serviceSubArgs = os.Args[i+1:]
				break
			}
			if arg == "dns" && i+1 < len(os.Args) {
				dnsSubArgs = os.Args[i+1:]
				break
			}
			if arg == "apps" && i+1 < len(os.Args) {
				appsSubArgs = os.Args[i+1:]
				break
			}
			if arg == "update" && i+1 < len(os.Args) {
				updateSubArgs = os.Args[i+1:]
				break
			}
			if arg == "support" && i+1 < len(os.Args) {
				supportSubArgs = os.Args[i+1:]
				break
			}
			if arg == "policy" && i+1 < len(os.Args) {
				policySubArgs = os.Args[i+1:]
				break
			}
		} else {
			flagArgs = append(flagArgs, arg)
		}
	}

	_ = flag.CommandLine.Parse(flagArgs)

	if cmd == "service" {
		if err := runServiceCommand(serviceSubArgs); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "dns" {
		if err := runDNSCommand(dnsSubArgs, *configDir); err != nil {
			fmt.Fprintf(os.Stderr, "DNS Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "apps" {
		if err := runAppsCommand(appsSubArgs, *ipcPath, *configDir); err != nil {
			fmt.Fprintf(os.Stderr, "Apps Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "update" {
		if err := runUpdateCommand(updateSubArgs, *configDir, *dataDir); err != nil {
			fmt.Fprintf(os.Stderr, "Update Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if cmd == "policy" {
		if err := runPolicyCommand(policySubArgs, *configDir); err != nil {
			fmt.Fprintf(os.Stderr, "Policy Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	log.Printf("[ONUSLY Agent] Platform: %s, Config: %s, Data: %s, IPC: %s", getPlatformName(), *configDir, *dataDir, *ipcPath)

	_ = os.MkdirAll(*configDir, 0700)
	_ = os.MkdirAll(*dataDir, 0700)

	idStore := identity.NewFileDeviceIdentityStore(filepath.Join(*configDir, "device_id"))
	pairedStore := identity.NewFilePairedUserStore(filepath.Join(*configDir, "paired_uids"))
	credStore := newPlatformCredentialStore(*configDir)
	pairingMgr := auth.NewPairingStateManager(filepath.Join(*configDir, "pairing_state.json"))
	cacheStore := cache.NewFileBlocklistCache(filepath.Join(*dataDir, "blocklist_cache.json"))
	syncer := sync.NewSyncer(*apiURL, credStore, cacheStore, nil)
	reporter := heartbeat.NewReporter(*apiURL, credStore, idStore, syncer, getPlatformName(), core.ReleaseVersion, nil)

	if *pairedUIDFlag > 0 {
		_ = pairedStore.AddAllowedUID(uint32(*pairedUIDFlag))
	}

	switch cmd {
	case "version":
		fmt.Printf("ONUSLY Desktop Agent %s (Commit: %s, Built: %s, Platform: %s/%s)\n",
			core.ReleaseVersion, core.BuildCommit, core.BuildDate, runtime.GOOS, runtime.GOARCH)
		return
	case "pair":
		runPairing(
			context.Background(),
			*apiURL,
			*ipcPath,
			idStore,
			credStore,
			pairedStore,
			pairingMgr,
			syncer,
			uint32(*pairedUIDFlag),
			*pairedSIDFlag,
		)
	case "status":
		runStatus(*ipcPath, credStore, idStore, cacheStore, pairingMgr)
	case "diagnostics":
		runDiagnostics(*ipcPath, credStore, idStore, cacheStore, pairingMgr)
	case "support":
		if err := runSupportCommand(supportSubArgs, *ipcPath, credStore, idStore, cacheStore, pairingMgr); err != nil {
			fmt.Fprintf(os.Stderr, "Support Error: %v\n", err)
			os.Exit(1)
		}
		return
	case "run", "daemon":
		runPlatformDaemon(
			context.Background(),
			*ipcPath,
			syncer,
			credStore,
			idStore,
			pairedStore,
			reporter,
			pairingMgr,
			uint32(*pairedUIDFlag),
			*apiURL,
			*dnsListenFlag,
			*configDir,
		)
	default:
		fmt.Printf("Unknown command: %s. Valid commands: run, pair, status, diagnostics\n", cmd)
		os.Exit(1)
	}
}

func runPairing(
	ctx context.Context,
	apiURL string,
	ipcPath string,
	idStore *identity.FileDeviceIdentityStore,
	credStore core.CredentialStore,
	pairedStore *identity.FilePairedUserStore,
	pairingMgr *auth.PairingStateManager,
	syncer *sync.Syncer,
	explicitUID uint32,
	explicitSID string,
) {
	// 1. Probe if running background service IPC pipe is reachable
	pingResp, errPing := ipc.Query(ipcPath, core.IPCRequest{Action: "ping"}, 500*time.Millisecond)
	if errPing == nil && pingResp != nil && pingResp.Success {
		// Service IPC is active! Delegate pairing to the authoritative privileged service.
		fmt.Println("Connecting to running ONUSLY Desktop Service...")
		reqResp, errReq := ipc.Query(ipcPath, core.IPCRequest{Action: "pair-request"}, 10*time.Second)
		if errReq != nil {
			log.Fatalf("Failed to request pairing from service: %v", errReq)
		}
		if !reqResp.Success {
			log.Fatalf("Service rejected pairing request: %s", reqResp.Error)
		}

		deviceID, _ := idStore.GetDeviceID()
		ownerSID, ownerUID, _ := resolvePairingOwner()
		if explicitSID != "" {
			ownerSID = explicitSID
		}

		fmt.Println("=======================================================")
		fmt.Println("           ONUSLY DESKTOP AGENT PAIRING                ")
		fmt.Println("=======================================================")
		fmt.Printf("Platform:     %s\n", getPlatformName())
		fmt.Printf("Device ID:    %s\n", deviceID)
		if ownerSID != "" {
			fmt.Printf("Owner SID:    %s\n", ownerSID)
		}
		if ownerUID > 0 {
			fmt.Printf("Owner UID:    %d\n", ownerUID)
		}
		fmt.Printf("PAIRING CODE: %s\n", reqResp.PairCode)
		if reqResp.ExpiresAt > 0 {
			fmt.Printf("Expires in:   %s\n", time.Until(time.Unix(reqResp.ExpiresAt, 0)).Round(time.Second))
		}
		fmt.Println("-------------------------------------------------------")
		fmt.Println("Enter this 6-character code in your ONUSLY Dashboard:")
		fmt.Println("👉 http://localhost:5173 (under Active Device Coverage)")
		fmt.Println("Waiting for confirmation from service...")

		pollResp, errPoll := ipc.Query(ipcPath, core.IPCRequest{Action: "pair-poll", Code: reqResp.PairCode}, 4*time.Minute)
		if errPoll != nil {
			log.Fatalf("Pairing confirmation failed: %v", errPoll)
		}
		if !pollResp.Success {
			log.Fatalf("Pairing rejected by backend/service: %s", pollResp.Error)
		}

		fmt.Println("✅ Successfully authorized device with running service!")
		if pollResp.Status != nil {
			fmt.Printf("Pairing State:   %s\n", pollResp.Status.PairingState)
			fmt.Printf("Owner SID:       %s\n", pollResp.Status.OwnerSID)
			fmt.Printf("Account:         %s\n", pollResp.Status.UserID)
			fmt.Printf("Active Targets:  %d\n", pollResp.Status.TargetsCount)
			fmt.Printf("Enforcing:       %t\n", pollResp.Status.Enforcing)
		}
		return
	}

	// 2. Standalone fallback (if service is not installed/running)
	ownerSID, ownerUID, _ := resolvePairingOwner()
	if explicitSID != "" {
		ownerSID = explicitSID
	}
	if explicitUID > 0 {
		ownerUID = explicitUID
	}

	if ownerUID > 0 {
		_ = pairedStore.AddAllowedUID(ownerUID)
		fmt.Printf("Binding authorized desktop user UID: %d\n", ownerUID)
	}
	if ownerSID != "" {
		fmt.Printf("Binding authorized desktop user SID: %s\n", ownerSID)
	}

	// Transition to PAIRING_PENDING
	if _, err := pairingMgr.Transition(auth.StatusPairingPending, ownerSID); err != nil {
		log.Printf("Warning: pairing state transition to PAIRING_PENDING: %v", err)
	}

	deviceID, err := idStore.GetDeviceID()
	if err != nil {
		log.Fatalf("Failed to initialize device identity: %v", err)
	}

	client := auth.NewPairingClient(nil)
	code, expiresAt, err := client.RequestCode(ctx, apiURL, "desktop-agent", getPlatformName(), deviceID)
	if err != nil {
		log.Fatalf("Failed to request pairing code: %v", err)
	}

	fmt.Println("=======================================================")
	fmt.Println("           ONUSLY DESKTOP AGENT PAIRING (Standalone)  ")
	fmt.Println("=======================================================")
	fmt.Printf("Platform:     %s\n", getPlatformName())
	fmt.Printf("Device ID:    %s\n", deviceID)
	if ownerSID != "" {
		fmt.Printf("Owner SID:    %s\n", ownerSID)
	}
	if ownerUID > 0 {
		fmt.Printf("Owner UID:    %d\n", ownerUID)
	}
	fmt.Printf("PAIRING CODE: %s\n", code)
	fmt.Printf("Expires in:   %s\n", time.Until(time.Unix(expiresAt, 0)).Round(time.Second))
	fmt.Println("-------------------------------------------------------")
	fmt.Println("Enter this 6-character code in your ONUSLY Dashboard:")
	fmt.Println("👉 http://localhost:5173 (under Active Device Coverage)")
	fmt.Println("Waiting for confirmation...")

	token, err := client.PollForToken(ctx, apiURL, code, 5*time.Minute)
	if err != nil {
		log.Fatalf("Pairing failed: %v", err)
	}

	// Two-phase commit:
	if _, err := pairingMgr.Transition(auth.StatusPairingCommitPending, ownerSID); err != nil {
		log.Printf("Warning: pairing state transition to PAIRING_COMMIT_PENDING: %v", err)
	}

	if err := credStore.SaveToken(token); err != nil {
		log.Fatalf("Failed to save credentials: %v", err)
	}

	fmt.Println("✅ Successfully authorized device! Performing initial blocklist sync...")
	blocklist, err := syncer.Sync(ctx)
	if err != nil {
		fmt.Printf("Warning during initial sync: %v\n", err)
	} else {
		fmt.Printf("Authoritative blocklist synchronized: %d active target(s) locked (version %s)\n", len(blocklist.Targets), blocklist.Version)
	}

	if _, err := pairingMgr.Transition(auth.StatusPaired, ownerSID); err != nil {
		log.Printf("Warning: pairing state transition to PAIRED: %v", err)
	}

	fmt.Println("Ready. You can now start the ONUSLY service: onusly-agent run")
}

func runStatus(
	ipcPath string,
	credStore core.CredentialStore,
	idStore *identity.FileDeviceIdentityStore,
	cacheStore *cache.FileBlocklistCache,
	pairingMgr *auth.PairingStateManager,
) {
	// Check if running service is reachable over IPC
	statusResp, errIPC := ipc.Query(ipcPath, core.IPCRequest{Action: "get-status"}, 1*time.Second)
	if errIPC == nil && statusResp != nil && statusResp.Success && statusResp.Status != nil {
		st := statusResp.Status
		fmt.Println("=======================================================")
		fmt.Println("         ONUSLY DESKTOP AGENT (Live Service)           ")
		fmt.Println("=======================================================")
		fmt.Printf("Platform:        %s\n", st.Platform)
		fmt.Printf("Device ID:       %s\n", st.DeviceID)
		if st.HealthState != "" {
			fmt.Printf("Health State:    %s\n", st.HealthState)
		}
		fmt.Printf("Pairing State:   %s\n", st.PairingState)
		if st.OwnerSID != "" {
			fmt.Printf("Owner SID:       %s\n", st.OwnerSID)
		}
		if st.PairingState == "REVOKED" || st.EnforcementStatus == "REVOKED" {
			fmt.Println("Status:          REVOKED (Device revoked by account owner. Run 'onusly-agent pair' to re-pair.)")
		} else if st.Connected {
			if st.EnforcementStatus == "ENFORCEMENT_DEGRADED" || !st.Enforcing {
				fmt.Printf("Status:          CONNECTED (ENFORCEMENT_DEGRADED - Fail-Open Active, Account: %s)\n", st.UserID)
			} else {
				fmt.Printf("Status:          CONNECTED (HEALTHY_ENFORCING, Account: %s)\n", st.UserID)
			}
		} else {
			fmt.Println("Status:          NOT PAIRED (run 'onusly-agent pair')")
		}
		fmt.Printf("Enforcing:       %t\n", st.Enforcing)
		if st.BlocklistVersion != "" {
			fmt.Printf("Blocklist Vers:  %s\n", st.BlocklistVersion)
			fmt.Printf("Last Sync:       %s\n", st.LastSyncAt)
			fmt.Printf("Active Targets:  %d\n", st.TargetsCount)
		} else {
			fmt.Println("Cached Targets:  None")
		}
		fmt.Println("----------------- Component Health --------------------")
		if st.DNSHealth != "" {
			fmt.Printf("DNS Resolver:    %s (Running: %t, Blocked: %d, Fwd: %d)\n", st.DNSHealth, st.DNSResolverRunning, st.DNSQueriesBlocked, st.DNSQueriesForwarded)
		}
		if st.AppsHealth != "" {
			fmt.Printf("App Enforcer:    %s (Running: %t, Monitored: %d, Restr: %d, Enforced: %d)\n", st.AppsHealth, st.AppsControllerRunning, st.AppsMonitoredCount, st.AppsRestrictedCount, st.AppsEnforcedCount)
		}
		if st.BrowserHealth != "" {
			fmt.Printf("Browser Native:  %s\n", st.BrowserHealth)
		}
		if st.SyncHealth != "" {
			fmt.Printf("Policy Freshness:%s (LKG Active: %t)\n", st.SyncHealth, st.LKGActive)
		}
		if len(st.Issues) > 0 {
			fmt.Printf("Issues:          %v\n", st.Issues)
		}
		if st.LastErrorCategory != "" && st.LastErrorCategory != "none" {
			fmt.Printf("Last Error Cat:  %s\n", st.LastErrorCategory)
		}
		fmt.Println("=======================================================")
		return
	}

	// Standalone / offline inspection
	deviceID, _ := idStore.GetDeviceID()
	token, _ := credStore.GetToken()
	userID, _ := auth.ParseTokenUserID(token)
	blocklist, _ := cacheStore.Get(userID)
	pairRec, _ := pairingMgr.GetState()

	healthState := core.HealthStateOffline
	if pairRec != nil && pairRec.Status == auth.StatusRevoked {
		healthState = core.HealthStateDeviceRevoked
	} else if token == "" {
		healthState = core.HealthStateAuthenticationRequired
	} else if blocklist == nil || len(blocklist.Targets) == 0 {
		healthState = core.HealthStatePolicyMissing
	} else {
		healthState = core.HealthStateHealthy
	}

	fmt.Println("=======================================================")
	fmt.Println("         ONUSLY DESKTOP AGENT (Offline State)          ")
	fmt.Println("=======================================================")
	fmt.Printf("Platform:        %s\n", getPlatformName())
	fmt.Printf("Device ID:       %s\n", deviceID)
	fmt.Printf("Health State:    %s\n", healthState)
	if pairRec != nil {
		fmt.Printf("Pairing State:   %s (gen %d)\n", pairRec.Status, pairRec.Generation)
		if pairRec.OwnerSID != "" {
			fmt.Printf("Owner SID:       %s\n", pairRec.OwnerSID)
		}
	}
	if pairRec != nil && pairRec.Status == auth.StatusRevoked {
		fmt.Println("Status:          REVOKED (Device revoked by account owner. Run 'onusly-agent pair' to re-pair.)")
		fmt.Println("Enforcing:       false")
	} else if token != "" {
		if blocklist != nil && len(blocklist.Targets) > 0 {
			fmt.Printf("Status:          CONNECTED (HEALTHY_ENFORCING, Account: %s)\n", userID)
			fmt.Println("Enforcing:       true")
		} else {
			fmt.Printf("Status:          CONNECTED (ENFORCEMENT_DEGRADED - Fail-Open Active, Account: %s)\n", userID)
			fmt.Println("Enforcing:       false")
			fmt.Println("Issues:          [policy_missing service_degraded]")
		}
	} else {
		fmt.Println("Status:          NOT PAIRED (run 'onusly-agent pair')")
		fmt.Println("Enforcing:       false")
	}

	if blocklist != nil {
		fmt.Printf("Blocklist Vers:  %s\n", blocklist.Version)
		fmt.Printf("Last Sync:       %s\n", blocklist.LastSyncAt)
		fmt.Printf("Active Targets:  %d\n", len(blocklist.Targets))
		for _, t := range blocklist.Targets {
			fmt.Printf("  • %s (%s): %v\n", t.GoalTitle, t.Status, t.Domains)
		}
	} else {
		fmt.Println("Cached Targets:  None")
	}
	fmt.Println("=======================================================")
}

func runDiagnostics(
	ipcPath string,
	credStore core.CredentialStore,
	idStore *identity.FileDeviceIdentityStore,
	cacheStore *cache.FileBlocklistCache,
	pairingMgr *auth.PairingStateManager,
) {
	fmt.Println("=======================================================")
	fmt.Println("             ONUSLY AGENT DIAGNOSTICS                  ")
	fmt.Println("=======================================================")

	diagResp, errIPC := ipc.Query(ipcPath, core.IPCRequest{Action: "diagnostics"}, 5*time.Second)
	if errIPC == nil && diagResp != nil && diagResp.Success {
		fmt.Println("Service Status:    RUNNING (IPC reachable)")
		var keys []string
		for k := range diagResp.Diagnostics {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Printf("  %s: %s\n", k, diagResp.Diagnostics[k])
		}
	} else {
		fmt.Println("Service Status:    STOPPED / UNREACHABLE")
		deviceID, _ := idStore.GetDeviceID()
		st, _ := pairingMgr.GetState()
		fmt.Printf("  device_id:                %s\n", deviceID)
		if st != nil {
			fmt.Printf("  pairing_state:            %s (gen %d)\n", st.Status, st.Generation)
			fmt.Printf("  owner_sid:                %s\n", st.OwnerSID)
		}
		tok, _ := credStore.GetToken()
		fmt.Printf("  has_credentials:          %t\n", tok != "")
		fmt.Printf("  health_state:             %s\n", core.HealthStateOffline)
	}
	fmt.Println("=======================================================")
}

// SupportBundle represents privacy-safe diagnostic data for customer support.
type SupportBundle struct {
	BundleVersion    string            `json:"bundle_version"`
	GeneratedAt      string            `json:"generated_at"`
	PrivacyStatement string            `json:"privacy_statement"`
	System           SupportSystem     `json:"system"`
	Build            SupportBuild      `json:"build"`
	Device           SupportDevice     `json:"device"`
	Enforcement      map[string]string `json:"enforcement"`
	ServiceReachable bool              `json:"service_reachable"`
}

type SupportSystem struct {
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	NumCPU    int    `json:"num_cpu"`
	GoVersion string `json:"go_version"`
}

type SupportBuild struct {
	ReleaseVersion  string `json:"release_version"`
	SecurityVersion uint64 `json:"security_version"`
	BuildCommit     string `json:"build_commit"`
	BuildDate       string `json:"build_date"`
}

type SupportDevice struct {
	DeviceID       string `json:"device_id"`
	Platform       string `json:"platform"`
	PairingState   string `json:"pairing_state"`
	HasCredentials bool   `json:"has_credentials"`
	OwnerSID       string `json:"owner_sid,omitempty"`
}

func runSupportCommand(
	args []string,
	ipcPath string,
	credStore core.CredentialStore,
	idStore *identity.FileDeviceIdentityStore,
	cacheStore *cache.FileBlocklistCache,
	pairingMgr *auth.PairingStateManager,
) error {
	if len(args) == 0 || args[0] != "bundle" {
		return fmt.Errorf("usage: onusly-agent support bundle [--yes] [--output <path>]")
	}

	outputPath := "onusly-support-bundle.json"
	autoConsent := false

	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--yes" || arg == "-y" {
			autoConsent = true
		} else if (arg == "--output" || arg == "-o") && i+1 < len(args) {
			outputPath = args[i+1]
			i++
		}
	}

	if !autoConsent {
		fmt.Println("=======================================================")
		fmt.Println("   ONUSLY Privacy-Safe Support Bundle Generation       ")
		fmt.Println("=======================================================")
		fmt.Println("This utility collects anonymized, privacy-safe system")
		fmt.Println("and operational health telemetry to assist troubleshooting.")
		fmt.Println("")
		fmt.Println("Privacy Invariant:")
		fmt.Println("  - NO personal files, directory contents, or browsing history")
		fmt.Println("  - NO authentication tokens, JWTs, or passwords")
		fmt.Println("-------------------------------------------------------")
		fmt.Print("Do you consent to generate this support bundle? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		if input != "y" && input != "yes" {
			fmt.Println("Support bundle generation cancelled.")
			return nil
		}
	}

	deviceID, _ := idStore.GetDeviceID()
	pairRec, _ := pairingMgr.GetState()
	token, _ := credStore.GetToken()

	pairingState := "UNPAIRED"
	ownerSID := ""
	if pairRec != nil {
		pairingState = string(pairRec.Status)
		ownerSID = pairRec.OwnerSID
	}

	enforcementInfo := make(map[string]string)
	serviceReachable := false

	diagResp, errIPC := ipc.Query(ipcPath, core.IPCRequest{Action: "diagnostics"}, 3*time.Second)
	if errIPC == nil && diagResp != nil && diagResp.Success {
		serviceReachable = true
		for k, v := range diagResp.Diagnostics {
			// Redact any potential tokens or secrets defensively
			kLower := strings.ToLower(k)
			if strings.Contains(kLower, "token") || strings.Contains(kLower, "secret") || strings.Contains(kLower, "password") {
				continue
			}
			enforcementInfo[k] = v
		}
	} else {
		enforcementInfo["status"] = "offline"
		enforcementInfo["health_state"] = string(core.HealthStateOffline)
	}

	bundle := SupportBundle{
		BundleVersion:    "1.0.0",
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		PrivacyStatement: "This bundle contains strictly privacy-safe operational metadata. Zero personal data, user tokens, passwords, browsing URLs, or file contents are included.",
		System: SupportSystem{
			OS:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			NumCPU:    runtime.NumCPU(),
			GoVersion: runtime.Version(),
		},
		Build: SupportBuild{
			ReleaseVersion:  core.ReleaseVersion,
			SecurityVersion: core.SecurityVersion,
			BuildCommit:     core.BuildCommit,
			BuildDate:       core.BuildDate,
		},
		Device: SupportDevice{
			DeviceID:       deviceID,
			Platform:       getPlatformName(),
			PairingState:   pairingState,
			HasCredentials: token != "",
			OwnerSID:       ownerSID,
		},
		Enforcement:      enforcementInfo,
		ServiceReachable: serviceReachable,
	}

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize support bundle: %w", err)
	}

	if err := os.WriteFile(outputPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write support bundle: %w", err)
	}

	hash := sha256.Sum256(data)
	hashHex := hex.EncodeToString(hash[:])

	fmt.Println("✅ Privacy-safe support bundle generated successfully!")
	fmt.Printf("File:        %s (%d bytes)\n", outputPath, len(data))
	fmt.Printf("SHA-256:     %s\n", hashHex)
	fmt.Println("You can attach this file when filing a beta support ticket.")

	return nil
}
