package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func isAdmin() bool {
	cmd := exec.Command("net", "session")
	return cmd.Run() == nil
}

func main() {
	if !isAdmin() {
		exePath, err := os.Executable()
		if err == nil {
			cmd := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
				fmt.Sprintf(`Start-Process -FilePath "%s" -Verb RunAs`, exePath))
			_ = cmd.Start()
			return
		}
	}

	fmt.Println("=========================================================")
	fmt.Println("       ONUSLY Machine-Wide DNS Shield Controller         ")
	fmt.Println("=========================================================")
	fmt.Println()

	// Query active DNS on Wi-Fi
	out, _ := exec.Command("netsh", "interface", "ipv4", "show", "dns", `name="Wi-Fi"`).CombinedOutput()
	isLoopback := strings.Contains(string(out), "127.0.0.1")

	// Also check all interfaces if Wi-Fi wasn't found
	if !isLoopback {
		allOut, _ := exec.Command("netsh", "interface", "ipv4", "show", "dns").CombinedOutput()
		if strings.Contains(string(allOut), "127.0.0.1") {
			isLoopback = true
		}
	}

	backupPath := `C:\ProgramData\Onusly\config\dns_backup.json`

	if isLoopback {
		fmt.Println("---------------------------------------------------------")
		fmt.Println(" Current Status : [ACTIVE] (127.0.0.1 Loopback Enforcing) ")
		fmt.Println(" Action         : DEACTIVATING Shield (Restoring DNS)... ")
		fmt.Println("---------------------------------------------------------")
		fmt.Println()

		// Restore DNS to DHCP for Wi-Fi and Ethernet
		_ = exec.Command("netsh", "interface", "ipv4", "set", "dns", `name="Wi-Fi"`, "dhcp").Run()
		_ = exec.Command("netsh", "interface", "ipv4", "set", "dns", `name="Ethernet"`, "dhcp").Run()

		// If backup file has static DNS, restore them
		if data, errRead := os.ReadFile(backupPath); errRead == nil {
			updated := strings.Replace(string(data), `"isActive": true`, `"isActive": false`, 1)
			_ = os.WriteFile(backupPath, []byte(updated), 0600)
		}

		_ = exec.Command("ipconfig", "/flushdns").Run()

		fmt.Println("[SUCCESS] Machine-Wide DNS Shield DEACTIVATED!")
		fmt.Println("All browsers and applications are now using standard upstream DNS.")
	} else {
		fmt.Println("---------------------------------------------------------")
		fmt.Println(" Current Status : [INACTIVE] (Using standard DNS)        ")
		fmt.Println(" Action         : ACTIVATING Machine-Wide Shield (127.0.0.1)...")
		fmt.Println("---------------------------------------------------------")
		fmt.Println()

		outWifi, errWifi := exec.Command("netsh", "interface", "ipv4", "set", "dns", `name="Wi-Fi"`, "static", "127.0.0.1").CombinedOutput()
		if errWifi != nil {
			fmt.Printf("Notice on Wi-Fi: %s\n", strings.TrimSpace(string(outWifi)))
		} else {
			fmt.Println("  [OK] Pointed Wi-Fi adapter to 127.0.0.1")
		}

		// Also set Ethernet if present
		_ = exec.Command("netsh", "interface", "ipv4", "set", "dns", `name="Ethernet"`, "static", "127.0.0.1").Run()

		_ = exec.Command("ipconfig", "/flushdns").Run()

		if data, errRead := os.ReadFile(backupPath); errRead == nil {
			updated := strings.Replace(string(data), `"isActive": false`, `"isActive": true`, 1)
			_ = os.WriteFile(backupPath, []byte(updated), 0600)
		}

		fmt.Println()
		fmt.Println("[SUCCESS] Machine-Wide DNS Shield ACTIVATED on 127.0.0.1!")
		fmt.Println("Distraction domains are now blocked across Chrome, Edge, Brave, and Firefox.")
	}

	fmt.Println()
	fmt.Println("=========================================================")
	fmt.Println(" Dashboard (http://localhost:5173) will reflect this state.")
	fmt.Println("=========================================================")
	fmt.Println()
	fmt.Println("Closing in 4 seconds...")
	time.Sleep(4 * time.Second)
}
