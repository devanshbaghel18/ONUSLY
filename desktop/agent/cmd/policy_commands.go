package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/devanshbaghel18/ONUSLY/desktop/core"
)

func runPolicyCommand(args []string, configDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing policy subcommand. Usage: onusly-agent policy [status|apply|remove|verify]")
	}

	action := args[0]
	mgr, err := newPlatformPolicyManager(configDir)
	if err != nil {
		return fmt.Errorf("failed to initialize policy manager: %w", err)
	}

	switch action {
	case "status":
		return runPolicyStatus(mgr)
	case "apply":
		return runPolicyApply(mgr)
	case "remove":
		return runPolicyRemove(mgr)
	case "verify":
		return runPolicyVerify(mgr)
	default:
		return fmt.Errorf("unknown policy action %q. Supported: status, apply, remove, verify", action)
	}
}

func runPolicyStatus(mgr core.PolicyManager) error {
	fmt.Println("=== ONUSLY Browser Policy & Force-Lock Status ===")
	browsers, err := mgr.DetectBrowsers()
	if err != nil {
		return fmt.Errorf("failed to detect browsers: %w", err)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "BROWSER\tINSTALLED\tMANAGED POLICY\tPOLICY PATH")
	fmt.Fprintln(w, "-------\t---------\t--------------\t-----------")

	issues, _ := mgr.VerifyPolicies(browsers)
	issueMap := make(map[core.BrowserType]bool)
	for _, iss := range issues {
		issueMap[iss.Browser] = true
	}

	for _, b := range browsers {
		instStr := "No"
		if b.Installed {
			instStr = "Yes"
		}
		statusStr := "Missing / Not Locked"
		if b.Installed {
			if !issueMap[b.Type] {
				statusStr = "ACTIVE (Locked)"
			}
		} else {
			statusStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", b.Type, instStr, statusStr, b.PolicyDir)
	}
	_ = w.Flush()

	if len(issues) > 0 {
		fmt.Printf("\n[Notice] %d policy issues / unmanaged browsers detected.\n", len(issues))
		fmt.Println("To apply enterprise force-lock: onusly-agent policy apply")
	} else {
		fmt.Println("\n[OK] All installed browsers have active enterprise policies.")
	}
	return nil
}

func runPolicyApply(mgr core.PolicyManager) error {
	fmt.Println("Applying browser enterprise policies and registering Native Messaging Hosts...")
	browsers, err := mgr.DetectBrowsers()
	if err != nil {
		return fmt.Errorf("failed to detect browsers: %w", err)
	}

	if err := mgr.ApplyPolicies(browsers); err != nil {
		return fmt.Errorf("failed to apply policies: %w", err)
	}

	fmt.Println("[SUCCESS] Enterprise Force-Install Policy applied successfully.")
	fmt.Println("The ONUSLY extension is now locked in Chrome/Edge and cannot be disabled or removed.")
	return nil
}

func runPolicyRemove(mgr core.PolicyManager) error {
	fmt.Println("Reverting browser enterprise policies...")
	browsers, err := mgr.DetectBrowsers()
	if err != nil {
		return fmt.Errorf("failed to detect browsers: %w", err)
	}

	if err := mgr.RemovePolicies(browsers); err != nil {
		return fmt.Errorf("failed to remove policies: %w", err)
	}

	fmt.Println("[SUCCESS] Enterprise policies rolled back cleanly.")
	return nil
}

func runPolicyVerify(mgr core.PolicyManager) error {
	browsers, err := mgr.DetectBrowsers()
	if err != nil {
		return fmt.Errorf("failed to detect browsers: %w", err)
	}

	issues, err := mgr.VerifyPolicies(browsers)
	if err != nil {
		return fmt.Errorf("failed to verify policies: %w", err)
	}

	if len(issues) == 0 {
		fmt.Println("[PASS] Zero policy drift detected. All browser enterprise policies are valid.")
		return nil
	}

	fmt.Printf("[FAIL] Detected %d policy issues:\n", len(issues))
	for i, iss := range issues {
		fmt.Printf("  %d. [%s] %s: %s\n", i+1, iss.Browser, iss.Issue, iss.Details)
	}
	return nil
}
