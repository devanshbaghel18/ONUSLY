//go:build windows

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
	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

const (
	WindowsServiceName = "OnuslyDesktopAgent"
)

func runPlatformDaemon(
	ctx context.Context,
	pipePath string,
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
	sessionMonitor := win.NewConsoleSessionMonitor()

	// Reconcile and recover any interrupted two-phase pairing transaction
	if pairingMgr != nil {
		recovered, errRec := pairingMgr.Recover(ctx, credStore, syncer)
		if errRec != nil {
			log.Printf("[ONUSLY Windows] Pairing state recovery notice: %v", errRec)
		} else {
			log.Printf("[ONUSLY Windows] Pairing state: %s (generation %d, owner: %s)",
				recovered.Status, recovered.Generation, recovered.OwnerSID)
		}
	}

	workerFunc := func(
		workerCtx context.Context,
		cancelChan <-chan struct{},
		cancelEvent uintptr,
		monitor *win.ConsoleSessionMonitor,
	) error {
		log.Printf("[ONUSLY Windows] Starting Desktop Enforcement Service Worker on %s...", pipePath)

		ipcServer := ipc.NewServer(pipePath, syncer, credStore, idStore)
		ipcServer.SetAPIBaseURL(apiURL)

		if pairingMgr != nil {
			st, _ := pairingMgr.GetState()
			if st != nil && st.OwnerSID != "" {
				ipcServer.SetOwnerSID(st.OwnerSID)
			}
			ipcServer.SetPairingManager(pairingMgr)
		}

		if monitor != nil {
			ipcServer.SetSessionValidator(monitor.ValidateClientSession)
		}

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
				log.Printf("[ONUSLY Windows] DNS server init error: %v", errSrv)
			}
		}

		if dnsServer != nil {
			if errDnsStart := dnsServer.Start(workerCtx); errDnsStart != nil {
				log.Printf("[ONUSLY Windows] Notice: DNS resolver could not bind 127.0.0.1:53: %v", errDnsStart)
			} else {
				log.Printf("[ONUSLY Windows] Local DNS resolver active on %s (UDP/TCP)", dnsServer.ListenAddress())
			}
			ipcServer.SetDNSServer(dnsServer)
		}

		// Initialize native application enforcement subsystem
		appScanner := apps.NewWindowsInventoryScanner(30 * time.Minute)
		appMonitor := apps.NewWindowsProcessMonitor(1 * time.Second)
		appEnforcer := apps.NewWindowsAppEnforcer()
		appPolicyMgr := apps.NewAppPolicyManager()
		appController := apps.NewAppEnforcementController(appMonitor, appPolicyMgr, appEnforcer, appScanner)

		if errApps := appController.Start(workerCtx); errApps != nil {
			log.Printf("[ONUSLY Windows] Notice: App enforcement controller start error: %v", errApps)
		} else {
			log.Printf("[ONUSLY Windows] Native App Enforcement active (%s)", appEnforcer.CapabilityTier())
		}
		ipcServer.SetAppsController(appController)

		if err := ipcServer.Start(workerCtx); err != nil {
			log.Printf("[ONUSLY Windows] IPC Server Start error: %v", err)
			if dnsServer != nil {
				_ = dnsServer.Stop()
			}
			if appController != nil {
				_ = appController.Stop()
			}
			return err
		}

		backupFilePath := filepath.Join(configDir, "dns_backup.json")
		adapterMgr := newPlatformAdapterManager()

		handleLifecycleErr := func(err error) {
			if err == nil {
				return
			}
			if errors.Is(err, core.ErrDeviceRevoked) {
				log.Println("[ONUSLY Windows] CRITICAL: Device has been remotely REVOKED by account owner. Deactivating local enforcement immediately.")
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
						log.Printf("[ONUSLY Windows] Warning restoring DNS fail-open after revocation: %v", errRest)
					} else {
						log.Println("[ONUSLY Windows] Original DNS configuration restored from backup (fail-open) following revocation.")
					}
				}
			} else if errors.Is(err, core.ErrCredentialExpired) {
				log.Println("[ONUSLY Windows] WARNING: Device credentials have EXPIRED. Re-authentication required.")
				if pairingMgr != nil {
					_, _ = pairingMgr.Transition(auth.StatusAuthRequired, "")
				}
			}
		}

		// Initial blocklist sync
		initData, errSync := syncer.Sync(workerCtx)
		if errSync != nil {
			log.Printf("[ONUSLY Windows] Notice: initial sync: %v (offline cache active)", errSync)
			handleLifecycleErr(errSync)
		} else if initData != nil {
			log.Printf("[ONUSLY Windows] Initial sync complete: %d target(s) locked (version %s)",
				len(initData.Targets), initData.Version)
		}

		// Update DNS policy snapshot and App policy snapshot from synced data or offline cache
		if initData != nil {
			dnsPolicy.UpdateFromBlocklist(initData)
			appController.UpdateBlocklist(initData)
		} else {
			cachedBlock, errCache := syncer.GetCachedBlocklist()
			if errCache != nil {
				log.Printf("[ONUSLY Windows] Notice: offline cache lookup: %v", errCache)
			} else if cachedBlock != nil {
				log.Printf("[ONUSLY Windows] Loaded offline cache: %d target(s) (version %s)", len(cachedBlock.Targets), cachedBlock.Version)
				dnsPolicy.UpdateFromBlocklist(cachedBlock)
				appController.UpdateBlocklist(cachedBlock)
			}
		}

		// Initial heartbeat
		if errHb := reporter.SendHeartbeat(workerCtx, nil); errHb != nil {
			handleLifecycleErr(errHb)
		}

		syncTicker := time.NewTicker(60 * time.Second)
		defer syncTicker.Stop()

		heartbeatTicker := time.NewTicker(60 * time.Second)
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-cancelChan:
				log.Println("[ONUSLY Windows] Worker received cancellation. Stopping IPC, DNS, and App subsystems...")
				if dnsServer != nil {
					_ = dnsServer.Stop()
				}
				if appController != nil {
					_ = appController.Stop()
				}
				_ = ipcServer.Stop()
				return nil

			case <-workerCtx.Done():
				if dnsServer != nil {
					_ = dnsServer.Stop()
				}
				if appController != nil {
					_ = appController.Stop()
				}
				_ = ipcServer.Stop()
				return nil

			case <-syncTicker.C:
				// If console is detached, don't perform unnecessary sync
				currentSess := monitor.GetCurrent()
				if currentSess != nil && !currentSess.IsAttached {
					continue
				}
				res, err := syncer.Sync(workerCtx)
				if err != nil {
					log.Printf("[ONUSLY Windows] Sync notice: %v", err)
					handleLifecycleErr(err)
				} else if res != nil {
					dnsPolicy.UpdateFromBlocklist(res)
					appController.UpdateBlocklist(res)
					log.Printf("[ONUSLY Windows] Synchronized blocklist: %d target(s) (version %s)", len(res.Targets), res.Version)
				}

			case <-heartbeatTicker.C:
				currentSess := monitor.GetCurrent()
				var issues []string
				if currentSess == nil || !currentSess.IsAttached {
					issues = []string{} // Detached console reports Enforcing: false, Issues: []
				} else {
					if dnsServer != nil && !dnsServer.IsRunning() {
						issues = append(issues, "dns_resolver_offline")
					}
					if appController != nil {
						appStats := appController.Stats()
						if appStats.EnforcementFailed > 0 {
							issues = append(issues, "app_enforcement_failed")
						}
					}
				}
				if errHb := reporter.SendHeartbeat(workerCtx, issues); errHb != nil {
					handleLifecycleErr(errHb)
				}
			}
		}
	}

	// Attempt to start under SCM
	svc := win.NewWindowsService(WindowsServiceName, sessionMonitor, workerFunc)
	errRun := svc.Run()

	if errors.Is(errRun, win.ErrNotRunningAsService) {
		// Executed from interactive terminal: run in standalone console mode
		log.Println("[ONUSLY Windows] Running in Standalone Console Mode (Ctrl+C to exit)...")
		cancelChan := make(chan struct{})
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		go func() {
			<-sigChan
			log.Println("[ONUSLY Windows] Received interrupt. Shutting down...")
			close(cancelChan)
		}()

		_ = workerFunc(ctx, cancelChan, 0, sessionMonitor)
		log.Println("[ONUSLY Windows] Standalone agent stopped cleanly.")
	} else if errRun != nil {
		log.Fatalf("[ONUSLY Windows] Service fatal error: %v", errRun)
	}
}

