@echo off
:: =========================================================================
:: ONUSLY 1-Click Machine-Wide DNS Shield Switcher
:: =========================================================================

:: Auto-elevate to Administrator (triggers Windows "Allow" prompt)
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo [ONUSLY] Elevation required to configure network DNS.
    echo Requesting administrator permission...
    powershell -NoProfile -ExecutionPolicy Bypass -Command "Start-Process cmd -ArgumentList '/c \"\"%~f0\"\"' -Verb RunAs"
    exit /b
)

title ONUSLY Machine-Wide DNS Shield Controller
cls

echo =========================================================================
echo              ONUSLY Machine-Wide DNS Shield Controller
echo =========================================================================
echo.

powershell -NoProfile -ExecutionPolicy Bypass -Command "
$backupPath = 'C:\ProgramData\Onusly\config\dns_backup.json'

# Detect current redirection state across all active adapters
$activeAdapters = Get-NetAdapter | Where-Object { $_.Status -eq 'Up' }
$isCurrentlyActive = $false

foreach ($ad in $activeAdapters) {
    $dns = Get-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue
    if ($dns -and $dns.ServerAddresses -contains '127.0.0.1') {
        $isCurrentlyActive = $true
        break
    }
}

if ($isCurrentlyActive) {
    Write-Host '---------------------------------------------------------' -ForegroundColor Yellow
    Write-Host '  Current Status : [ACTIVE] (127.0.0.1 Loopback Enforcing)' -ForegroundColor Yellow
    Write-Host '  Next Action    : DEACTIVATING Shield...' -ForegroundColor Cyan
    Write-Host '---------------------------------------------------------' -ForegroundColor Yellow
    
    # 1. Restore DNS via backup or reset to DHCP
    if (Test-Path $backupPath) {
        & 'C:\onusly\ONUSLY\build\bin\onusly-agent.exe' dns restore
    }
    
    # Ensure all active adapters are restored
    foreach ($ad in $activeAdapters) {
        Set-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -ResetServerAddresses -ErrorAction SilentlyContinue
    }
    
    # Update backup JSON state to inactive
    if (Test-Path $backupPath) {
        try {
            $json = Get-Content $backupPath -Raw | ConvertFrom-Json
            $json.isActive = $false
            $json | ConvertTo-Json -Depth 5 | Set-Content $backupPath -Encoding UTF8
        } catch {}
    }
    
    ipconfig /flushdns | Out-Null
    Write-Host ''
    Write-Host ' [SUCCESS] DNS Shield DEACTIVATED!' -ForegroundColor Green
    Write-Host ' All network adapters returned to standard upstream DNS.' -ForegroundColor White
} else {
    Write-Host '---------------------------------------------------------' -ForegroundColor Gray
    Write-Host '  Current Status : [INACTIVE] (Using upstream DNS)' -ForegroundColor Gray
    Write-Host '  Next Action    : ACTIVATING Machine-Wide Shield (127.0.0.1)...' -ForegroundColor Cyan
    Write-Host '---------------------------------------------------------' -ForegroundColor Gray
    
    # Apply 127.0.0.1 to all active UP physical adapters
    $applied = 0
    foreach ($ad in $activeAdapters) {
        try {
            Set-DnsClientServerAddress -InterfaceIndex $ad.InterfaceIndex -ServerAddresses ('127.0.0.1') -ErrorAction Stop
            Write-Host ('   - Pointed ' + $ad.Name + ' to 127.0.0.1') -ForegroundColor Green
            $applied++
        } catch {
            Write-Host ('   - Warning on ' + $ad.Name + ': ' + $_.Exception.Message) -ForegroundColor Yellow
        }
    }
    
    # Update backup JSON state to active
    if (Test-Path $backupPath) {
        try {
            $json = Get-Content $backupPath -Raw | ConvertFrom-Json
            $json.isActive = $true
            $json | ConvertTo-Json -Depth 5 | Set-Content $backupPath -Encoding UTF8
        } catch {}
    }
    
    ipconfig /flushdns | Out-Null
    Write-Host ''
    if ($applied -gt 0) {
        Write-Host ' [SUCCESS] DNS Shield ACTIVATED!' -ForegroundColor Green
        Write-Host ' Distraction domains are now blocked across ALL browsers.' -ForegroundColor White
    } else {
        Write-Host ' [ERROR] Could not point active adapters to 127.0.0.1.' -ForegroundColor Red
    }
}
"

echo.
echo =========================================================================
echo  The Web Dashboard (http://localhost:5173) will reflect this state.
echo =========================================================================
echo.
timeout /t 3
