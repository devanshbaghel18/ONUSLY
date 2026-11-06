package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/cache"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/heartbeat"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/identity"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
)

func getDefaultConfigDir() string {
	if os.Geteuid() == 0 {
		return "/etc/onusly"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "onusly")
}

func getDefaultDataDir() string {
	if os.Geteuid() == 0 {
		return "/var/lib/onusly"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "onusly")
}

func getDefaultSocketPath() string {
	if os.Geteuid() == 0 {
		return "/run/onusly/agent.sock"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "onusly", "agent.sock")
}

func main() {
	var (
		apiURL        = flag.String("api-url", "http://localhost:8080", "ONUSLY backend API URL")
		configDir     = flag.String("config-dir", getDefaultConfigDir(), "Configuration directory path")
		dataDir       = flag.String("data-dir", getDefaultDataDir(), "Data storage directory path")
		socketPath    = flag.String("socket", getDefaultSocketPath(), "IPC Unix domain socket path")
		pairedUIDFlag = flag.Uint("paired-uid", 0, "Authorized desktop user UID for system mode IPC")
	)

	cmd := "run"
	var flagArgs []string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if !strings.HasPrefix(arg, "-") && (arg == "run" || arg == "pair" || arg == "status" || arg == "daemon") {
			cmd = arg
		} else {
			flagArgs = append(flagArgs, arg)
		}
	}

	_ = flag.CommandLine.Parse(flagArgs)

	log.Printf("[ONUSLY Agent] Config Dir: %s, Data Dir: %s, Socket: %s", *configDir, *dataDir, *socketPath)

	_ = os.MkdirAll(*configDir, 0700)
	_ = os.MkdirAll(*dataDir, 0700)

	idStore := identity.NewFileDeviceIdentityStore(filepath.Join(*configDir, "device_id"))
	pairedStore := identity.NewFilePairedUserStore(filepath.Join(*configDir, "paired_uids"))
	credStore := auth.NewFileCredentialStore(filepath.Join(*configDir, "token"))
	tok, tokErr := credStore.GetToken()
	log.Printf("[ONUSLY Agent] Token path: %s, loaded len=%d, err=%v", filepath.Join(*configDir, "token"), len(tok), tokErr)
	cacheStore := cache.NewFileBlocklistCache(filepath.Join(*dataDir, "blocklist_cache.json"))
	syncer := sync.NewSyncer(*apiURL, credStore, cacheStore, nil)
	reporter := heartbeat.NewReporter(*apiURL, credStore, idStore, syncer, "linux", "1.0.0", nil)

	if *pairedUIDFlag > 0 {
		_ = pairedStore.AddAllowedUID(uint32(*pairedUIDFlag))
	}

	switch cmd {
	case "pair":
		runPairing(context.Background(), *apiURL, idStore, credStore, pairedStore, syncer, uint32(*pairedUIDFlag))
	case "status":
		runStatus(credStore, idStore, cacheStore)
	case "run", "daemon":
		runDaemon(context.Background(), *socketPath, syncer, credStore, idStore, pairedStore, reporter, uint32(*pairedUIDFlag))
	default:
		fmt.Printf("Unknown command: %s. Valid commands: run, pair, status\n", cmd)
		os.Exit(1)
	}
}

func runPairing(
	ctx context.Context,
	apiURL string,
	idStore *identity.FileDeviceIdentityStore,
	credStore *auth.FileCredentialStore,
	pairedStore *identity.FilePairedUserStore,
	syncer *sync.Syncer,
	explicitUID uint32,
) {
	var targetUID uint32
	if explicitUID > 0 {
		targetUID = explicitUID
	} else if sudoUID := os.Getenv("SUDO_UID"); sudoUID != "" {
		if parsed, err := strconv.ParseUint(sudoUID, 10, 32); err == nil {
			targetUID = uint32(parsed)
		}
	} else if os.Geteuid() != 0 {
		targetUID = uint32(os.Geteuid())
	}

	if targetUID > 0 {
		_ = pairedStore.AddAllowedUID(targetUID)
		fmt.Printf("Binding authorized desktop user UID: %d\n", targetUID)
	}

	deviceID, err := idStore.GetDeviceID()
	if err != nil {
		log.Fatalf("Failed to initialize device identity: %v", err)
	}

	client := auth.NewPairingClient(nil)
	code, expiresAt, err := client.RequestCode(ctx, apiURL, "desktop-agent", "linux", deviceID)
	if err != nil {
		log.Fatalf("Failed to request pairing code: %v", err)
	}

	fmt.Println("=======================================================")
	fmt.Println("           ONUSLY DESKTOP AGENT PAIRING                ")
	fmt.Println("=======================================================")
	fmt.Printf("Device ID:    %s\n", deviceID)
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
	fmt.Println("Ready. You can now start the ONUSLY service: onusly-agent run")
}

func runStatus(credStore *auth.FileCredentialStore, idStore *identity.FileDeviceIdentityStore, cacheStore *cache.FileBlocklistCache) {
	deviceID, _ := idStore.GetDeviceID()
	token, _ := credStore.GetToken()
	userID, _ := auth.ParseTokenUserID(token)
	blocklist, _ := cacheStore.Get(userID)

	fmt.Println("=======================================================")
	fmt.Println("               ONUSLY DESKTOP AGENT                    ")
	fmt.Println("=======================================================")
	fmt.Printf("Device ID:       %s\n", deviceID)
	if token != "" {
		fmt.Printf("Status:          CONNECTED (Account: %s)\n", userID)
	} else {
		fmt.Println("Status:          NOT PAIRED (run 'onusly-agent pair')")
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

func runDaemon(
	ctx context.Context,
	socketPath string,
	syncer *sync.Syncer,
	credStore *auth.FileCredentialStore,
	idStore *identity.FileDeviceIdentityStore,
	pairedStore *identity.FilePairedUserStore,
	reporter *heartbeat.Reporter,
	explicitUID uint32,
) {
	log.Println("[ONUSLY Agent] Starting Desktop Enforcement Service...")

	ipcServer := ipc.NewServer(socketPath, syncer, credStore, idStore)

	allowedUIDs, err := pairedStore.GetAllowedUIDs()
	if err != nil {
		log.Printf("[ONUSLY Agent] Warning: reading paired UIDs: %v", err)
	}
	if explicitUID > 0 {
		allowedUIDs = append(allowedUIDs, explicitUID)
	}
	if os.Geteuid() != 0 {
		allowedUIDs = append(allowedUIDs, uint32(os.Geteuid()))
	}
	ipcServer.SetAllowedUIDs(allowedUIDs)
	log.Printf("[ONUSLY Agent] Authorized IPC peer UIDs: %v (daemon UID: %d)", allowedUIDs, os.Geteuid())

	if err := ipcServer.Start(ctx); err != nil {
		log.Fatalf("[ONUSLY Agent] Failed to start IPC server: %v", err)
	}
	log.Printf("[ONUSLY Agent] Local IPC server listening on %s", socketPath)

	// Perform initial blocklist synchronization
	initialData, err := syncer.Sync(ctx)
	if err != nil {
		log.Printf("[ONUSLY Agent] Notice: initial sync: %v (offline cache active)", err)
	} else {
		log.Printf("[ONUSLY Agent] Initial sync complete: %d target(s) locked (version %s)", len(initialData.Targets), initialData.Version)
	}

	// Send initial heartbeat
	_ = reporter.SendHeartbeat(ctx, nil)

	// Periodic synchronization ticker (every 60s)
	syncTicker := time.NewTicker(60 * time.Second)
	defer syncTicker.Stop()

	// Periodic heartbeat ticker (every 60s)
	heartbeatTicker := time.NewTicker(60 * time.Second)
	defer heartbeatTicker.Stop()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigChan:
			log.Println("[ONUSLY Agent] Received termination signal. Shutting down gracefully...")
			_ = ipcServer.Stop()
			log.Println("[ONUSLY Agent] Service stopped cleanly.")
			return

		case <-syncTicker.C:
			res, err := syncer.Sync(ctx)
			if err != nil {
				log.Printf("[ONUSLY Agent] Sync error: %v", err)
				_ = reporter.SendHeartbeat(ctx, []string{"service_degraded"})
			} else {
				log.Printf("[ONUSLY Agent] Synchronized blocklist: %d target(s) (version %s)", len(res.Targets), res.Version)
				_ = reporter.SendHeartbeat(ctx, nil)
			}

		case <-heartbeatTicker.C:
			_ = reporter.SendHeartbeat(ctx, nil)
		}
	}
}
