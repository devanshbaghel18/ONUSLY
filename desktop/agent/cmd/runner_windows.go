//go:build windows

package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/auth"
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
) {
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

		if err := ipcServer.Start(workerCtx); err != nil {
			log.Printf("[ONUSLY Windows] IPC Server Start error: %v", err)
			return err
		}

		// Initial blocklist sync
		initData, errSync := syncer.Sync(workerCtx)
		if errSync != nil {
			log.Printf("[ONUSLY Windows] Notice: initial sync: %v (offline cache active)", errSync)
		} else if initData != nil {
			log.Printf("[ONUSLY Windows] Initial sync complete: %d target(s) locked (version %s)",
				len(initData.Targets), initData.Version)
		}

		// Initial heartbeat
		_ = reporter.SendHeartbeat(workerCtx, nil)

		syncTicker := time.NewTicker(60 * time.Second)
		defer syncTicker.Stop()

		heartbeatTicker := time.NewTicker(60 * time.Second)
		defer heartbeatTicker.Stop()

		for {
			select {
			case <-cancelChan:
				log.Println("[ONUSLY Windows] Worker received cancellation. Stopping IPC server...")
				_ = ipcServer.Stop()
				return nil

			case <-workerCtx.Done():
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
				} else if res != nil {
					log.Printf("[ONUSLY Windows] Synchronized blocklist: %d target(s) (version %s)", len(res.Targets), res.Version)
				}

			case <-heartbeatTicker.C:
				currentSess := monitor.GetCurrent()
				var issues []string
				if currentSess == nil || !currentSess.IsAttached {
					issues = []string{} // Detached console reports Enforcing: false, Issues: []
				}
				_ = reporter.SendHeartbeat(workerCtx, issues)
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

