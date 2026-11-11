//go:build !windows

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/dns"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/heartbeat"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/identity"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/sync"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func runPlatformDaemon(
	ctx context.Context,
	socketPath string,
	syncer *sync.Syncer,
	credStore core.CredentialStore,
	idStore *identity.FileDeviceIdentityStore,
	pairedStore *identity.FilePairedUserStore,
	reporter *heartbeat.Reporter,
	pairingMgr *auth.PairingStateManager,
	explicitUID uint32,
	apiURL string,
	dnsListenAddr string,
	configDir string,
) {
	if dnsListenAddr == "" {
		dnsListenAddr = "127.0.0.1:53"
	}
	log.Println("[ONUSLY Agent] Starting Desktop Enforcement Service (Linux)...")

	// Initialize local DNS subsystem
	dnsCache := dns.NewMemoryDNSCache(5000)
	dnsPolicy := dns.NewPolicyManager(dnsCache)
	dnsUpstream, errUp := dns.NewUpstreamPool([]string{"1.1.1.1:53", "1.0.0.1:53", "8.8.8.8:53"}, 3*time.Second)
	var dnsServer *dns.Server
	if errUp == nil {
		var errSrv error
		dnsServer, errSrv = dns.NewServer(dns.ServerConfig{
			ListenAddress: dnsListenAddr,
			MaxCacheSize:  5000,
			QueryTimeout:  3 * time.Second,
		}, dnsPolicy, dnsCache, dnsUpstream)
		if errSrv != nil {
			log.Printf("[ONUSLY Agent] DNS server init error: %v", errSrv)
		}
	}

	if dnsServer != nil {
		if errDnsStart := dnsServer.Start(ctx); errDnsStart != nil {
			log.Printf("[ONUSLY Agent] Notice: DNS resolver could not bind 127.0.0.1:53: %v", errDnsStart)
		} else {
			log.Printf("[ONUSLY Agent] Local DNS resolver active on %s (UDP/TCP)", dnsServer.ListenAddress())
		}
	}

	// Initialize native application enforcement subsystem
	appScanner := apps.NewLinuxInventoryScanner(30 * time.Minute)
	appMonitor := apps.NewLinuxProcessMonitor(1 * time.Second)
	appEnforcer := apps.NewLinuxAppEnforcer()
	appPolicyMgr := apps.NewAppPolicyManager()
	appController := apps.NewAppEnforcementController(appMonitor, appPolicyMgr, appEnforcer, appScanner)

	if errApps := appController.Start(ctx); errApps != nil {
		log.Printf("[ONUSLY Agent] Notice: App enforcement controller start error: %v", errApps)
	} else {
		log.Printf("[ONUSLY Agent] Native App Enforcement active (%s)", appEnforcer.CapabilityTier())
	}

	ipcServer := ipc.NewServer(socketPath, syncer, credStore, idStore)
	ipcServer.SetAPIBaseURL(apiURL)
	if pairingMgr != nil {
		ipcServer.SetPairingManager(pairingMgr)
	}
	if dnsServer != nil {
		ipcServer.SetDNSServer(dnsServer)
	}
	ipcServer.SetAppsController(appController)

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

	backupFilePath := filepath.Join(configDir, "dns_backup.json")
	adapterMgr := newPlatformAdapterManager()
	ipcServer.SetAdapterManager(adapterMgr, backupFilePath)

	if err := ipcServer.Start(ctx); err != nil {
		if dnsServer != nil {
			_ = dnsServer.Stop()
		}
		if appController != nil {
			_ = appController.Stop()
		}
		log.Fatalf("[ONUSLY Agent] Failed to start IPC server: %v", err)
	}
	log.Printf("[ONUSLY Agent] Local IPC server listening on %s", socketPath)


	handleLifecycleErr := func(err error) {
		if err == nil {
			return
		}
		if errors.Is(err, core.ErrDeviceRevoked) {
			log.Println("[ONUSLY Linux] CRITICAL: Device has been remotely REVOKED by account owner. Deactivating local enforcement immediately.")
			if pairingMgr != nil {
				_, _ = pairingMgr.Transition(auth.StatusRevoked, "")
			}
			if credStore != nil {
				_ = credStore.ClearToken()
			}
			if syncer != nil {
				_ = syncer.ClearCache()
			}
			if dnsPolicy != nil {
				_, _ = dnsPolicy.UpdateFromBlocklist(&core.BlocklistData{})
			}
			if appController != nil {
				appController.UpdateBlocklist(&core.BlocklistData{})
			}
			if adapterMgr != nil && backupFilePath != "" {
				if errRest := adapterMgr.RestoreDNS(backupFilePath); errRest != nil {
					log.Printf("[ONUSLY Linux] Warning restoring DNS fail-open after revocation: %v", errRest)
				} else {
					log.Println("[ONUSLY Linux] Original DNS configuration restored from backup (fail-open) following revocation.")
				}
			}
		} else if errors.Is(err, core.ErrCredentialExpired) {
			log.Println("[ONUSLY Linux] WARNING: Device credentials have EXPIRED. Re-authentication required.")
			if pairingMgr != nil {
				_, _ = pairingMgr.Transition(auth.StatusAuthRequired, "")
			}
		}
	}

	// Perform initial blocklist synchronization
	initialData, err := syncer.Sync(ctx)
	if err != nil {
		log.Printf("[ONUSLY Agent] Notice: initial sync: %v (offline cache active)", err)
		handleLifecycleErr(err)
	} else {
		log.Printf("[ONUSLY Agent] Initial sync complete: %d target(s) locked (version %s)", len(initialData.Targets), initialData.Version)
	}

	// Update DNS policy and App policy snapshots from synced data or offline cache
	if initialData != nil {
		dnsPolicy.UpdateFromBlocklist(initialData)
		appController.UpdateBlocklist(initialData)
	} else {
		cachedBlock, errCache := syncer.GetCachedBlocklist()
		if errCache != nil {
			log.Printf("[ONUSLY Agent] Notice: offline cache lookup: %v", errCache)
		} else if cachedBlock != nil {
			log.Printf("[ONUSLY Agent] Loaded offline cache: %d target(s) (version %s)", len(cachedBlock.Targets), cachedBlock.Version)
			dnsPolicy.UpdateFromBlocklist(cachedBlock)
			appController.UpdateBlocklist(cachedBlock)
		}
	}

	// Send initial heartbeat
	if errHb := reporter.SendHeartbeat(ctx, nil); errHb != nil {
		handleLifecycleErr(errHb)
	}

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
			if dnsServer != nil {
				_ = dnsServer.Stop()
			}
			if appController != nil {
				_ = appController.Stop()
			}
			_ = ipcServer.Stop()
			log.Println("[ONUSLY Agent] Service stopped cleanly.")
			return

		case <-syncTicker.C:
			res, err := syncer.Sync(ctx)
			if err != nil {
				log.Printf("[ONUSLY Agent] Sync error: %v", err)
				handleLifecycleErr(err)
				_ = reporter.SendHeartbeat(ctx, []string{"service_degraded"})
			} else {
				log.Printf("[ONUSLY Agent] Synchronized blocklist: %d target(s) (version %s)", len(res.Targets), res.Version)
				dnsPolicy.UpdateFromBlocklist(res)
				appController.UpdateBlocklist(res)
				_ = reporter.SendHeartbeat(ctx, nil)
			}

		case <-heartbeatTicker.C:
			var issues []string
			if dnsServer != nil && !dnsServer.IsRunning() {
				issues = append(issues, "dns_resolver_offline")
			}
			if appController != nil {
				appStats := appController.Stats()
				if appStats.EnforcementFailed > 0 {
					issues = append(issues, "app_enforcement_failed")
				}
			}
			if errHb := reporter.SendHeartbeat(ctx, issues); errHb != nil {
				handleLifecycleErr(errHb)
			}
		}
	}
}

