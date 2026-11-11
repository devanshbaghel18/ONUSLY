//go:build !windows

package main

import (
	"context"
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
) {
	log.Println("[ONUSLY Agent] Starting Desktop Enforcement Service (Linux)...")

	ipcServer := ipc.NewServer(socketPath, syncer, credStore, idStore)
	ipcServer.SetAPIBaseURL(apiURL)
	if pairingMgr != nil {
		ipcServer.SetPairingManager(pairingMgr)
	}

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

