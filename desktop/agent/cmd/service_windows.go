//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	win "github.com/devanshbaghel18/ONUSLY/desktop/core/windows"
)

func runServiceCommand(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("missing service subcommand. Usage: onusly-agent service [install|uninstall|start|stop|upgrade|status]")
	}

	action := args[0]
	serviceName := WindowsServiceName

	exePath, err := os.Executable()
	if err != nil {
		exePath = filepath.Join(os.Getenv("ProgramFiles"), "Onusly", "onusly-agent.exe")
	}

	mgr := win.NewWindowsServiceManager(serviceName, exePath)

	switch action {
	case "install":
		fmt.Printf("Installing %s using native Win32 SCM APIs...\n", serviceName)
		if err := mgr.InstallServiceNative(); err != nil {
			return fmt.Errorf("native SCM installation failed: %w", err)
		}
		fmt.Printf("[OK] Service %s installed successfully in SCM.\n", serviceName)
		return nil

	case "uninstall":
		fmt.Printf("Uninstalling %s using native Win32 SCM APIs...\n", serviceName)
		if err := mgr.UninstallServiceNative(); err != nil {
			return fmt.Errorf("native SCM uninstallation failed: %w", err)
		}
		fmt.Printf("[OK] Service %s uninstalled successfully from SCM.\n", serviceName)
		return nil

	case "start":
		fmt.Printf("Starting %s...\n", serviceName)
		if err := mgr.StartServiceNative(); err != nil {
			return fmt.Errorf("failed to start service: %w", err)
		}
		fmt.Printf("[OK] Service %s reached RUNNING state.\n", serviceName)
		return nil

	case "stop":
		fmt.Printf("Stopping %s...\n", serviceName)
		if err := mgr.StopServiceNative(); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		fmt.Printf("[OK] Service %s reached STOPPED state.\n", serviceName)
		return nil

	case "upgrade":
		fmt.Printf("Upgrading %s using native Win32 SCM APIs...\n", serviceName)
		wasRunning, err := mgr.UpgradeService(exePath, 10*time.Second)
		if err != nil {
			return fmt.Errorf("service upgrade failed: %w", err)
		}
		if wasRunning {
			fmt.Printf("[OK] Service %s upgraded and restarted successfully in SCM.\n", serviceName)
		} else {
			fmt.Printf("[OK] Service %s configuration upgraded successfully in SCM (service remains stopped).\n", serviceName)
		}
		return nil

	case "status":
		running, err := mgr.IsRunningNative()
		if err != nil {
			if errors.Is(err, win.ErrServiceNotFound) {
				fmt.Printf("Service %s is NOT INSTALLED.\n", serviceName)
				return nil
			}
			return fmt.Errorf("failed to query service status: %w", err)
		}
		if running {
			fmt.Printf("Service %s is RUNNING.\n", serviceName)
		} else {
			fmt.Printf("Service %s is STOPPED.\n", serviceName)
		}
		return nil

	default:
		return fmt.Errorf("unknown service action %q. Supported: install, uninstall, start, stop, upgrade, status", action)
	}
}

