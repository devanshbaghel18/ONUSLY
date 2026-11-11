# ONUSLY Windows Production Uninstaller
# D3.3 Milestone: Safe Service Removal, Reversible Policy Rollback & Data Retention Policy

[CmdletBinding()]
param(
    [string]$InstallDir = "${env:ProgramFiles}\Onusly",
    [string]$DataRoot = "${env:ProgramData}\Onusly",
    [string]$ServiceName = "OnuslyDesktopAgent",
    [switch]$RetainDeviceIdentity,
    [switch]$RetainLogs,
    [switch]$PurgeAll,
    [switch]$PurgeData, # Backward-compatible alias for PurgeAll
    [switch]$ForceResetDNS, # Explicit administrator override to reset orphaned loopback DNS adapters to DHCP when backup is missing
    [switch]$SkipElevationCheck,
    [switch]$SkipRegistry,
    [switch]$NonInteractive
)

$ErrorActionPreference = "Stop"

if ($PurgeData) {
    $PurgeAll = $true
}

function Write-Step {
    param([string]$Message)
    Write-Host "`n>>> [ONUSLY Uninstaller] $Message" -ForegroundColor Cyan
}

function Write-Success {
    param([string]$Message)
    Write-Host "  [OK] $Message" -ForegroundColor Green
}

function Write-Warn {
    param([string]$Message)
    Write-Host "  [WARN] $Message" -ForegroundColor Yellow
}

function Invoke-ScCommand {
    param(
        [Parameter(Mandatory=$true)][string]$Arguments
    )
    $pinfo = New-Object System.Diagnostics.ProcessStartInfo
    $pinfo.FileName = "$env:SystemRoot\System32\sc.exe"
    $pinfo.Arguments = $Arguments
    $pinfo.RedirectStandardOutput = $true
    $pinfo.RedirectStandardError = $true
    $pinfo.UseShellExecute = $false
    $pinfo.CreateNoWindow = $true
    $p = [System.Diagnostics.Process]::Start($pinfo)
    $stdout = $p.StandardOutput.ReadToEnd()
    $stderr = $p.StandardError.ReadToEnd()
    $p.WaitForExit()
    $combined = ($stdout + "`n" + $stderr).Trim()
    return [PSCustomObject]@{
        ExitCode = $p.ExitCode
        Stdout   = $stdout.Trim()
        Stderr   = $stderr.Trim()
        Output   = $combined
    }
}

function Wait-FileLockRelease {
    param(
        [Parameter(Mandatory=$true)][string]$Path,
        [int]$TimeoutSeconds = 5
    )
    if (-not (Test-Path $Path)) { return $true }
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        try {
            $stream = [System.IO.File]::Open($Path, [System.IO.FileMode]::Open, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
            if ($stream) {
                $stream.Close()
                $stream.Dispose()
                return $true
            }
        } catch {}
        Start-Sleep -Milliseconds 150
    }
    return $false
}

function Wait-ServiceState {
    param(
        [Parameter(Mandatory=$true)][string]$TargetServiceName,
        [Parameter(Mandatory=$true)][string]$TargetState, # "RUNNING" or "STOPPED"
        [int]$TimeoutSeconds = 10
    )
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSeconds)
    while ([DateTime]::UtcNow -lt $deadline) {
        $qex = Invoke-ScCommand "queryex $TargetServiceName"
        if ($TargetState -eq "STOPPED") {
            if ($qex.ExitCode -eq 1060 -or $qex.Output -match "STATE\s*:\s*1\s+STOPPED") {
                return $true
            }
        } elseif ($TargetState -eq "RUNNING") {
            if ($qex.Output -match "STATE\s*:\s*4\s+RUNNING") {
                return $true
            }
        }
        Start-Sleep -Milliseconds 200
    }
    return $true
}

# 1. Elevation check
if (-not $SkipElevationCheck) {
    $isAdmin = ([Security.Principal.WindowsPrincipal][Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
    if (-not $isAdmin) {
        Write-Host "Administrator elevation is required to uninstall ONUSLY. Please run from an elevated PowerShell session." -ForegroundColor Red
        exit 1
    }
}

$logsDir = "$DataRoot\logs"
if (Test-Path $logsDir) {
    try { Start-Transcript -Path "$logsDir\uninstall.log" -Append -Force -ErrorAction SilentlyContinue | Out-Null } catch {}
}

# 2. Stop and Delete SCM Service
Write-Step "Stopping and Removing $ServiceName Service..."

$installedAgentBin = "$InstallDir\onusly-agent.exe"
if (-not $SkipRegistry -and (Test-Path $installedAgentBin)) {
    Write-Host "  Stopping and removing service via native SCM APIs..."
    try {
        $uninstSvcProc = Start-Process -FilePath $installedAgentBin -ArgumentList "service", "uninstall" -NoNewWindow -Wait -PassThru
        if ($uninstSvcProc.ExitCode -eq 0) {
            Write-Success "Service $ServiceName stopped and uninstalled successfully from SCM."
        } else {
            Write-Warn "Native service uninstall exited with code $($uninstSvcProc.ExitCode). Falling back to sc.exe cleanup."
            Invoke-ScCommand "stop $ServiceName" | Out-Null
            Wait-ServiceState -TargetServiceName $ServiceName -TargetState "STOPPED" -TimeoutSeconds 10 | Out-Null
            Invoke-ScCommand "delete $ServiceName" | Out-Null
        }
    } catch {
        Write-Warn "Notice invoking native service uninstaller: $_"
        Invoke-ScCommand "stop $ServiceName" | Out-Null
        Wait-ServiceState -TargetServiceName $ServiceName -TargetState "STOPPED" -TimeoutSeconds 10 | Out-Null
        Invoke-ScCommand "delete $ServiceName" | Out-Null
    }
} else {
    $svcCheck = Invoke-ScCommand "queryex $ServiceName"
    if ($svcCheck.ExitCode -eq 0) {
        Invoke-ScCommand "stop $ServiceName" | Out-Null
        Wait-ServiceState -TargetServiceName $ServiceName -TargetState "STOPPED" -TimeoutSeconds 10 | Out-Null
        $delRes = Invoke-ScCommand "delete $ServiceName"
        if ($delRes.ExitCode -eq 0 -or $delRes.ExitCode -eq 1060) {
            Write-Success "Service $ServiceName stopped and deleted from SCM."
        }
    } else {
        Write-Success "Service $ServiceName is not installed."
    }
}

# 3. Surgical Enterprise Policy & Native Host Rollback
Write-Step "Rolling back Enterprise Browser Policies and Native-Host Registrations..."

if (-not $SkipRegistry) {
    $backupStateFile = "$DataRoot\policy-backups\backup_state.json"

    if (Test-Path $backupStateFile) {
        try {
            $state = Get-Content $backupStateFile -Raw | ConvertFrom-Json
            $entries = @($state.entries)

            # Roll back entries in reverse order
            for ($i = $entries.Count - 1; $i -ge 0; $i--) {
                $entry = $entries[$i]
                $kPath = $entry.keyPath

                if (-not (Test-Path $kPath)) {
                    continue
                }

                if ($entry.createdByUs) {
                    # Newly created entry: delete only this specific value
                    if ($entry.valueName -eq "(Default)" -or $entry.valueName -eq "") {
                        Remove-Item -Path $kPath -Recurse -Force -ErrorAction SilentlyContinue
                    } else {
                        Remove-ItemProperty -Path $kPath -Name $entry.valueName -ErrorAction SilentlyContinue
                        # If the key was created by us and has 0 remaining values, remove key
                        if ($entry.keyCreated) {
                            $props = (Get-Item -Path $kPath -ErrorAction SilentlyContinue).Property
                            if (-not $props -or $props.Count -eq 0) {
                                Remove-Item -Path $kPath -Force -ErrorAction SilentlyContinue
                            }
                        }
                    }
                } elseif ($entry.priorType -eq "string") {
                    Set-ItemProperty -Path $kPath -Name $entry.valueName -Value $entry.priorValue -Type String -ErrorAction SilentlyContinue
                } elseif ($entry.priorType -eq "dword") {
                    Set-ItemProperty -Path $kPath -Name $entry.valueName -Value ([int]$entry.priorValue) -Type DWord -ErrorAction SilentlyContinue
                }
            }
            Write-Success "Surgically restored browser enterprise policies from backup state."
        } catch {
            Write-Warn "Error parsing backup state ($backupStateFile): $_. Falling back to targeted cleanup."
        }
    }

    # Targeted cleanup of native-messaging host keys
    $hostName = "com.onusly.desktop_agent"
    $nativeKeys = @(
        "HKLM:\SOFTWARE\Google\Chrome\NativeMessagingHosts\$hostName",
        "HKLM:\SOFTWARE\Microsoft\Edge\NativeMessagingHosts\$hostName",
        "HKLM:\SOFTWARE\BraveSoftware\Brave\NativeMessagingHosts\$hostName",
        "HKLM:\SOFTWARE\Chromium\NativeMessagingHosts\$hostName",
        "HKLM:\SOFTWARE\Mozilla\NativeMessagingHosts\$hostName"
    )

    foreach ($nk in $nativeKeys) {
        if (Test-Path $nk) {
            Remove-Item -Path $nk -Recurse -Force -ErrorAction SilentlyContinue
        }
    }

    # Targeted cleanup of Firefox ONUSLY extension settings
    $firefoxExtId = "extension@onusly.com"
    $ffExtKey = "HKLM:\SOFTWARE\Policies\Mozilla\Firefox\ExtensionSettings\$firefoxExtId"
    if (Test-Path $ffExtKey) {
        Remove-Item -Path $ffExtKey -Recurse -Force -ErrorAction SilentlyContinue
    }
    Write-Success "Cleaned up ONUSLY native-messaging host and extension policy registrations."
} else {
    Write-Host "  Skipping registry policy rollback (-SkipRegistry specified)." -ForegroundColor DarkGray
}

# 3.5. Fail-Open Network DNS Restoration
Write-Step "Checking Adapter DNS Redirection (Fail-Open Safety Restoration)..."
$dnsBackupFile = "$DataRoot\config\dns_backup.json"
if (-not (Test-Path $dnsBackupFile)) {
    if (Test-Path "$DataRoot\dns_backup.json") {
        $dnsBackupFile = "$DataRoot\dns_backup.json"
    }
}

if (Test-Path $dnsBackupFile) {
    try {
        $dnsState = Get-Content $dnsBackupFile -Raw | ConvertFrom-Json
        if ($dnsState.originalDns) {
            foreach ($adapter in $dnsState.originalDns) {
                if ($adapter.isDhcp) {
                    Write-Host "  Restoring $($adapter.interfaceAlias) to DHCP..." -ForegroundColor DarkGray
                    Set-DnsClientServerAddress -InterfaceIndex $adapter.interfaceIndex -ResetServerAddresses -ErrorAction SilentlyContinue
                } else {
                    Write-Host "  Restoring $($adapter.interfaceAlias) to Static DNS ($($adapter.serverAddresses -join ', '))..." -ForegroundColor DarkGray
                    Set-DnsClientServerAddress -InterfaceIndex $adapter.interfaceIndex -ServerAddresses $adapter.serverAddresses -ErrorAction SilentlyContinue
                }
            }
            Clear-DnsClientCache -ErrorAction SilentlyContinue
            Write-Success "Restored original DNS configuration across all network interfaces from backup."
        }
    } catch {
        Write-Warn "Error restoring adapter DNS settings from ${dnsBackupFile}: $_"
    }
} else {
    Write-Host "  No DNS backup file present; checking network adapters..." -ForegroundColor DarkGray
}

# D7.1 Safety Scan: Never blindly reset adapters pointing to 127.0.0.1 to DHCP if backup is missing/corrupt!
try {
    $loopbackAdapters = Get-DnsClientServerAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object { $_.ServerAddresses -contains "127.0.0.1" }
    if ($loopbackAdapters) {
        foreach ($la in $loopbackAdapters) {
            if ($ForceResetDNS) {
                Write-Host "  [FORCE] Administrator explicitly specified -ForceResetDNS. Resetting $($la.InterfaceAlias) (index $($la.InterfaceIndex)) to DHCP..." -ForegroundColor Yellow
                Set-DnsClientServerAddress -InterfaceIndex $la.InterfaceIndex -ResetServerAddresses -ErrorAction SilentlyContinue
            } else {
                Write-Host "  [WARNING] Adapter '$($la.InterfaceAlias)' (index $($la.InterfaceIndex)) is pointing to 127.0.0.1, but DNS backup is missing or corrupt." -ForegroundColor Yellow
                Write-Host "            To prevent discarding potentially pre-existing static DNS configurations (e.g. 1.1.1.1, 8.8.8.8), ONUSLY will not automatically force DHCP." -ForegroundColor Yellow
                Write-Host "            Manual recovery options:" -ForegroundColor Cyan
                Write-Host "              - If DHCP:   Set-DnsClientServerAddress -InterfaceIndex $($la.InterfaceIndex) -ResetServerAddresses" -ForegroundColor Cyan
                Write-Host "              - If Static: Set-DnsClientServerAddress -InterfaceIndex $($la.InterfaceIndex) -ServerAddresses <your-static-servers>" -ForegroundColor Cyan
                Write-Host "              - Or pass:   uninstall.ps1 -ForceResetDNS" -ForegroundColor Cyan
            }
        }
        Clear-DnsClientCache -ErrorAction SilentlyContinue
    }
} catch {
    # Non-fatal safety check
}

# 4. Binary and Program Files Cleanup
Write-Step "Cleaning up Installed Binaries & Manifests..."
if (Test-Path $InstallDir) {
    Wait-FileLockRelease -Path "$InstallDir\onusly-agent.exe" -TimeoutSeconds 5 | Out-Null
    Wait-FileLockRelease -Path "$InstallDir\onusly-native-host.exe" -TimeoutSeconds 5 | Out-Null
    Remove-Item -Path $InstallDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Success "Removed $InstallDir (binaries, manifests, staging, and backups)."
}

# 5. ProgramData & Retention Policy Cleanup
Write-Step "Applying Data Retention Policy..."

$secureDir = "$DataRoot\secure"
$configDir = "$DataRoot\config"
$storageDir = "$DataRoot\data"
$backupDir = "$DataRoot\policy-backups"

# Invariant: Pairing credentials MUST NEVER be silently retained after uninstall
if (Test-Path $secureDir) {
    try {
        if (-not $SkipElevationCheck) {
            takeown.exe /F $secureDir /R /D Y 2>&1 | Out-Null
            icacls.exe $secureDir /grant "*S-1-5-32-544:F" /T /C /Q 2>&1 | Out-Null
        }
        Remove-Item -Path $secureDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Success "Cryptographic pairing credentials purged (zero credential retention)."
    } catch {
        Write-Warn "Notice during secure credential purge: $_"
    }
}

# Temporary policy backups and runtime data cache
if (Test-Path $backupDir) {
    Remove-Item -Path $backupDir -Recurse -Force -ErrorAction SilentlyContinue
}
if (Test-Path $storageDir) {
    Remove-Item -Path $storageDir -Recurse -Force -ErrorAction SilentlyContinue
}

# Device identity handling
if ($RetainDeviceIdentity -and (Test-Path "$configDir\device_id")) {
    Write-Host "  Retaining device identity in $configDir (-RetainDeviceIdentity specified)." -ForegroundColor DarkGray
} else {
    if (Test-Path $configDir) {
        Remove-Item -Path $configDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Success "Purged device identity and configuration."
    }
}

# Diagnostic history handling
if ($RetainLogs -and (Test-Path $logsDir)) {
    Write-Host "  Retaining diagnostic history in $logsDir (-RetainLogs specified)." -ForegroundColor DarkGray
} else {
    try { Stop-Transcript -ErrorAction SilentlyContinue | Out-Null } catch {}
    if (Test-Path $logsDir) {
        Remove-Item -Path $logsDir -Recurse -Force -ErrorAction SilentlyContinue
        Write-Success "Purged diagnostic history logs."
    }
}

# PurgeAll handling
if ($PurgeAll -and (Test-Path $DataRoot)) {
    try { Stop-Transcript -ErrorAction SilentlyContinue | Out-Null } catch {}
    Remove-Item -Path $DataRoot -Recurse -Force -ErrorAction SilentlyContinue
    Write-Success "Purged entire data root $DataRoot (-PurgeAll specified)."
} else {
    # If DataRoot is empty, clean it up
    if (Test-Path $DataRoot) {
        $remaining = Get-ChildItem -Path $DataRoot -Force -ErrorAction SilentlyContinue
        if (-not $remaining -or $remaining.Count -eq 0) {
            Remove-Item -Path $DataRoot -Force -ErrorAction SilentlyContinue
        }
    }
}

Write-Host "`n=================================================================" -ForegroundColor Green
Write-Host "         ONUSLY WINDOWS DESKTOP UNINSTALLATION COMPLETE          " -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Green
Write-Host "Service:      Removed"
Write-Host "Binaries:     Removed ($InstallDir)"
Write-Host "Policies:     Reversibly Rolled Back"
Write-Host "Credentials:  Purged (zero persistent secrets)"
Write-Host "Device ID:    $(if ($RetainDeviceIdentity) { 'Retained' } else { 'Purged' })"
Write-Host "Logs:         $(if ($RetainLogs) { 'Retained' } else { 'Purged' })"
Write-Host "=================================================================`n"
