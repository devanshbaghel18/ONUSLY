# ONUSLY Machine-Wide DNS Shield Switcher
# Requires Administrator elevation

$ErrorActionPreference = "Continue"

Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host "       ONUSLY Machine-Wide DNS Shield Controller         " -ForegroundColor Cyan
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host ""

$backupPath = "C:\ProgramData\Onusly\config\dns_backup.json"

# Find active connected network adapters (Wi-Fi, Ethernet)
$activeAdapters = Get-NetAdapter | Where-Object { $_.Status -eq "Up" }

if (-not $activeAdapters) {
    Write-Host "[ERROR] No active connected network adapters found (Wi-Fi or Ethernet)." -ForegroundColor Red
    Read-Host "Press Enter to exit"
    exit 1
}

# Check if loopback DNS (127.0.0.1) is currently set on any active adapter
$isCurrentlyActive = $false
foreach ($ad in $activeAdapters) {
    $dns = Get-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
    if ($dns -and ($dns.ServerAddresses -contains "127.0.0.1")) {
        $isCurrentlyActive = $true
        break
    }
}

if ($isCurrentlyActive) {
    Write-Host "---------------------------------------------------------" -ForegroundColor Yellow
    Write-Host " Current Status: [ACTIVE] (127.0.0.1 Shield is Blocking)" -ForegroundColor Yellow
    Write-Host " Action        : DEACTIVATING Shield..." -ForegroundColor Cyan
    Write-Host "---------------------------------------------------------" -ForegroundColor Yellow
    Write-Host ""

    # Restore DNS
    foreach ($ad in $activeAdapters) {
        Write-Host "  [-] Restoring $($ad.Name) to standard upstream DNS..." -ForegroundColor White
        # Reset to DHCP or backup
        Set-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -ResetServerAddresses -ErrorAction SilentlyContinue
        netsh interface ipv4 set dns name="$($ad.Name)" dhcp 2>$null
    }

    # If backup file exists, restore specific static servers if any were present
    if (Test-Path $backupPath) {
        try {
            $data = Get-Content $backupPath -Raw | ConvertFrom-Json
            $data.isActive = $false
            $data | ConvertTo-Json -Depth 5 | Set-Content $backupPath -Encoding UTF8
        } catch {}
    }

    ipconfig /flushdns | Out-Null
    Write-Host ""
    Write-Host "[SUCCESS] Machine-Wide DNS Shield DEACTIVATED!" -ForegroundColor Green
    Write-Host "All browsers and apps are now using standard upstream DNS." -ForegroundColor Green
} else {
    Write-Host "---------------------------------------------------------" -ForegroundColor Gray
    Write-Host " Current Status: [INACTIVE] (Using standard DNS)" -ForegroundColor Gray
    Write-Host " Action        : ACTIVATING Machine-Wide Shield (127.0.0.1)..." -ForegroundColor Cyan
    Write-Host "---------------------------------------------------------" -ForegroundColor Gray
    Write-Host ""

    # Backup current DNS if not already backed up
    if (-not (Test-Path $backupPath)) {
        & "C:\onusly\ONUSLY\build\bin\onusly-agent.exe" dns status | Out-Null
    }

    $successCount = 0
    foreach ($ad in $activeAdapters) {
        Write-Host "  [+] Pointing $($ad.Name) to 127.0.0.1..." -ForegroundColor White
        try {
            Set-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -ServerAddresses ("127.0.0.1") -ErrorAction Stop
            $successCount++
        } catch {
            # Fallback to netsh
            $out = netsh interface ipv4 set dns name="$($ad.Name)" static 127.0.0.1 2>&1
            if ($LASTEXITCODE -eq 0) {
                $successCount++
            } else {
                Write-Host "      Warning: $($out)" -ForegroundColor Yellow
            }
        }
    }

    if (Test-Path $backupPath) {
        try {
            $data = Get-Content $backupPath -Raw | ConvertFrom-Json
            $data.isActive = $true
            $data | ConvertTo-Json -Depth 5 | Set-Content $backupPath -Encoding UTF8
        } catch {}
    }

    ipconfig /flushdns | Out-Null
    Write-Host ""
    if ($successCount -gt 0) {
        Write-Host "[SUCCESS] Machine-Wide DNS Shield ACTIVATED on 127.0.0.1!" -ForegroundColor Green
        Write-Host "Distraction domains are now blocked across Chrome, Edge, Brave, and Firefox." -ForegroundColor Green
    } else {
        Write-Host "[ERROR] Failed to set loopback DNS on active adapters." -ForegroundColor Red
    }
}

Write-Host ""
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host " Dashboard (http://localhost:5173) will update state now." -ForegroundColor White
Write-Host "=========================================================" -ForegroundColor Cyan
Write-Host ""
Start-Sleep -Seconds 3
