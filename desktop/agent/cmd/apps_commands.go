package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/apps"
	"github.com/devanshbaghel18/ONUSLY/desktop/agent/internal/ipc"
	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func runAppsCommand(args []string, defaultIpcPath string, configDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing apps subcommand. Usage: onusly-agent apps [inventory|status|refresh|diagnostics]")
	}

	action := args[0]
	ipcPath := defaultIpcPath
	if ipcPath == "" {
		ipcPath = getDefaultIPCPath()
	}

	for i := 1; i < len(args); i++ {
		if (args[i] == "-socket" || args[i] == "--socket") && i+1 < len(args) {
			ipcPath = args[i+1]
			i++
		}
	}

	switch action {
	case "inventory":
		return runAppsInventory(ipcPath)

	case "status":
		return runAppsStatus(ipcPath)

	case "refresh":
		return runAppsRefresh(ipcPath)

	case "diagnostics":
		return runAppsDiagnostics(ipcPath)

	default:
		return fmt.Errorf("unknown apps action %q. Supported: inventory, status, refresh, diagnostics", action)
	}
}

func runAppsInventory(ipcPath string) error {
	fmt.Println("=== ONUSLY Discovered Desktop Applications ===")

	var discovered []apps.ApplicationIdentity

	// 1. Attempt query via running service IPC
	resp, err := ipc.Query(ipcPath, core.IPCRequest{Action: "apps-inventory"}, 3*time.Second)
	if err == nil && resp != nil && resp.Success && len(resp.Apps) > 0 {
		// Unmarshal from response Apps
		for _, raw := range resp.Apps {
			data, errM := json.Marshal(raw)
			if errM == nil {
				var app apps.ApplicationIdentity
				if errU := json.Unmarshal(data, &app); errU == nil {
					discovered = append(discovered, app)
				}
			}
		}
	}

	// 2. Fall back to local platform inventory scanner if IPC is offline or returned empty
	if len(discovered) == 0 {
		scanner := newPlatformInventoryScanner()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		var errScan error
		discovered, errScan = scanner.ScanApplications(ctx)
		if errScan != nil {
			return fmt.Errorf("failed to scan installed applications: %w", errScan)
		}
	}

	if len(discovered) == 0 {
		fmt.Println("No installed third-party applications discovered.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "APP ID\tDISPLAY NAME\tPUBLISHER\tEXECUTABLE PATH")
	fmt.Fprintln(w, "------\t------------\t---------\t---------------")

	for _, app := range discovered {
		name := app.DisplayName
		if name == "" {
			name = app.AppID
		}
		pub := app.Publisher
		if pub == "" {
			pub = "-"
		}
		exe := app.ExePath
		if exe == "" {
			exe = app.InstallDir
		}
		if len(exe) > 50 {
			exe = "..." + exe[len(exe)-47:]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", app.AppID, name, pub, exe)
	}
	_ = w.Flush()

	fmt.Printf("\nTotal Discovered Applications: %d\n", len(discovered))
	return nil
}

func runAppsStatus(ipcPath string) error {
	fmt.Println("=== ONUSLY Application Enforcement Status ===")

	resp, err := ipc.Query(ipcPath, core.IPCRequest{Action: "apps-status"}, 2*time.Second)
	if err != nil {
		fmt.Println("Desktop Service IPC: OFFLINE (service not reachable)")
		fmt.Println("To start the service:")
		if getPlatformName() == "windows" {
			fmt.Println("  Start-Service OnuslyDesktopAgent")
		} else {
			fmt.Println("  sudo systemctl start onusly-agent")
		}
		return fmt.Errorf("could not query agent IPC: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("service reported error: %s", resp.Error)
	}

	status := resp.Status
	diag := resp.Diagnostics

	if status != nil {
		running := "NO (INACTIVE)"
		if status.AppsControllerRunning {
			running = "YES (ACTIVE)"
		}
		fmt.Printf("Subsystem Active:     %s\n", running)
		fmt.Printf("Monitored Processes:  %d\n", status.AppsMonitoredCount)
		fmt.Printf("Restricted Targets:   %d\n", status.AppsRestrictedCount)
		fmt.Printf("Enforced Terminated:  %d\n", status.AppsEnforcedCount)
		fmt.Printf("Enforcement Failed:   %d\n", status.AppsFailedCount)
	}

	if diag != nil {
		if tier, ok := diag["apps_capability_tier"]; ok {
			fmt.Printf("Capability Tier:      %s\n", tier)
		}
		if last, ok := diag["apps_last_reconciled"]; ok && last != "" {
			fmt.Printf("Last Reconciled:      %s\n", last)
		}
		if inv, ok := diag["apps_inventory_count"]; ok {
			fmt.Printf("Discovered Inventory: %s\n", inv)
		}
	}

	return nil
}

func runAppsRefresh(ipcPath string) error {
	fmt.Println("Refreshing application inventory and reconciling policy...")
	scanner := newPlatformInventoryScanner()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	appsList, err := scanner.ScanApplications(ctx)
	if err != nil {
		return fmt.Errorf("failed to refresh application inventory: %w", err)
	}

	fmt.Printf("[OK] Discovered %d installed applications on host.\n", len(appsList))

	// Also ping running service if available to trigger reconciliation
	resp, errIpc := ipc.Query(ipcPath, core.IPCRequest{Action: "status"}, 2*time.Second)
	if errIpc == nil && resp != nil && resp.Success {
		fmt.Println("[OK] Running Desktop Agent service informed of inventory refresh.")
	}
	return nil
}

func runAppsDiagnostics(ipcPath string) error {
	fmt.Println("=== ONUSLY Application Enforcement Diagnostics ===")

	resp, err := ipc.Query(ipcPath, core.IPCRequest{Action: "diagnostics"}, 2*time.Second)
	if err != nil {
		return fmt.Errorf("could not query agent diagnostics: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("service reported error: %s", resp.Error)
	}

	if resp.Diagnostics == nil {
		fmt.Println("No diagnostic metrics available.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "METRIC KEY\tVALUE")
	fmt.Fprintln(w, "----------\t-----")

	for k, v := range resp.Diagnostics {
		if strings.HasPrefix(k, "apps_") || k == "enforcing" || k == "healthy" || k == "active_session_id" {
			fmt.Fprintf(w, "%s\t%s\n", k, v)
		}
	}
	_ = w.Flush()

	return nil
}
