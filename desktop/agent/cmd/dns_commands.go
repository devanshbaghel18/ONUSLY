package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func runDNSCommand(args []string, configDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing dns subcommand. Usage: onusly-agent dns [enable|restore|status]")
	}

	action := args[0]
	backupPath := filepath.Join(configDir, "dns_backup.json")
	mgr := newPlatformAdapterManager()

	switch action {
	case "enable":
		fmt.Printf("Configuring network adapters for local DNS enforcement (backup: %s)...\n", backupPath)
		if err := mgr.ApplyLoopbackDNS("127.0.0.1", "::1", backupPath); err != nil {
			return fmt.Errorf("failed to enable loopback DNS redirection: %w", err)
		}
		fmt.Println("[OK] Loopback DNS redirection active (127.0.0.1 / ::1).")
		fmt.Printf("[OK] Original DNS settings backed up to: %s\n", backupPath)
		return nil

	case "restore":
		fmt.Printf("Restoring original adapter DNS configuration from %s...\n", backupPath)
		if err := mgr.RestoreDNS(backupPath); err != nil {
			return fmt.Errorf("failed to restore DNS configuration: %w", err)
		}
		fmt.Println("[OK] Fail-open restoration complete: adapters returned to original DNS settings.")
		return nil

	case "status":
		active, err := mgr.IsRedirectionActive(backupPath)
		if err != nil {
			fmt.Printf("Notice: could not read backup state: %v\n", err)
		}

		fmt.Println("=== ONUSLY DNS Redirection Status ===")
		if active {
			fmt.Println("Redirection Status: ACTIVE (pointing to local loopback 127.0.0.1)")
		} else {
			fmt.Println("Redirection Status: INACTIVE (using system/DHCP upstream DNS)")
		}
		fmt.Printf("Backup File:        %s\n", backupPath)

		adapters, errDisc := mgr.DiscoverAdapters()
		if errDisc != nil {
			fmt.Printf("Warning: error discovering adapters: %v\n", errDisc)
		} else {
			fmt.Println("\nDiscovered Network Adapters:")
			for _, a := range adapters {
				serverList := "(DHCP / automatic)"
				if len(a.ServerAddresses) > 0 {
					serverList = strings.Join(a.ServerAddresses, ", ")
				}
				physicalTag := ""
				if a.IsPhysical {
					physicalTag = " [Physical]"
				}
				fmt.Printf("  - [%d] %s%s: %s\n", a.InterfaceIndex, a.InterfaceAlias, physicalTag, serverList)
			}
		}

		// Probe running service IPC for DNS resolver stats
		ipcPath := getDefaultIPCPath()
		resp, errIpc := ipc.Query(ipcPath, core.IPCRequest{Action: "diagnostics"}, 1*time.Second)
		if errIpc == nil && resp != nil && resp.Success {
			fmt.Println("\nRunning Desktop Service DNS Subsystem:")
			if resp.Status != nil {
				fmt.Printf("  Resolver Running:   %t\n", resp.Status.DNSResolverRunning)
				fmt.Printf("  Queries Blocked:    %d\n", resp.Status.DNSQueriesBlocked)
				fmt.Printf("  Queries Forwarded:  %d\n", resp.Status.DNSQueriesForwarded)
			}
			if resp.Diagnostics != nil {
				if dTot, ok := resp.Diagnostics["dns_queries_total"]; ok {
					fmt.Printf("  Queries Total:      %s\n", dTot)
				}
				if dCached, ok := resp.Diagnostics["dns_queries_cached"]; ok {
					fmt.Printf("  Queries Cached:     %s\n", dCached)
				}
			}
		}

		return nil

	default:
		return fmt.Errorf("unknown dns action %q. Supported: enable, restore, status", action)
	}
}

func readBackupState(path string) map[string]any {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var res map[string]any
	_ = json.Unmarshal(data, &res)
	return res
}

